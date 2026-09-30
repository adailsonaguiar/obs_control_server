package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"obs-control-server/internal/events"
	"obs-control-server/internal/logs"
	"obs-control-server/internal/obs"
	"obs-control-server/internal/ptz"
)

type OBSController interface {
	Status(context.Context) (obs.Status, error)
	Scenes(context.Context) ([]obs.Scene, error)
	SetScene(context.Context, string) error
	Screenshot(context.Context, string, int, int) ([]byte, error)
	Sources(context.Context, string) ([]obs.Source, error)
	SetSourceVisible(context.Context, string, string, bool) error
	StartRecording(context.Context) error
	StopRecording(context.Context) error
	StartStreaming(context.Context) error
	StopStreaming(context.Context) error
	Telemetry(context.Context) (obs.Telemetry, error)
	AudioInputs(context.Context) ([]obs.AudioInput, error)
	SetInputMute(context.Context, string, bool) error
	SetInputVolume(context.Context, string, float64) error
	StudioMode(context.Context) (obs.StudioMode, error)
	SetPreviewScene(context.Context, string) error
	SetTransitionDuration(context.Context, int) error
	TriggerTransition(context.Context) error
}

type Settings struct {
	Host  string
	Port  int
	Token string
}

type PTZController interface {
	Move(context.Context, string, int, string, int) error
	Zoom(context.Context, string, int, string, int) error
	Preset(context.Context, string, int, string, int) error
}

type Manager struct {
	mu       sync.RWMutex
	server   *http.Server
	listener net.Listener
	settings Settings
	obs      OBSController
	ptz      PTZController
	logs     *logs.Buffer
	events   *events.Hub
	commands map[string]cachedResponse
	audit    []AuditEvent
}

type cachedResponse struct {
	Status    int
	Header    http.Header
	Body      []byte
	CreatedAt time.Time
}

type AuditEvent struct {
	ID        uint64    `json:"id"`
	Time      time.Time `json:"time"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	CommandID string    `json:"commandId,omitempty"`
	Result    string    `json:"result"`
}

func New(controller OBSController, eventLogs *logs.Buffer) *Manager {
	return &Manager{obs: controller, ptz: ptz.NewClient(), logs: eventLogs, events: events.New(), commands: make(map[string]cachedResponse)}
}

func (m *Manager) Start(settings Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		return errors.New("servidor já está em execução")
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", settings.Host, settings.Port))
	if err != nil {
		return fmt.Errorf("iniciar servidor local: %w", err)
	}
	m.settings = settings
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", m.health)
	mux.Handle("GET /obs/status", m.auth(http.HandlerFunc(m.obsStatus)))
	mux.Handle("GET /obs/scenes", m.auth(http.HandlerFunc(m.obsScenes)))
	mux.Handle("POST /obs/scene", m.auth(http.HandlerFunc(m.setScene)))
	mux.Handle("GET /obs/preview", m.auth(http.HandlerFunc(m.obsPreview)))
	mux.Handle("GET /obs/sources", m.auth(http.HandlerFunc(m.obsSources)))
	mux.Handle("POST /obs/source/show", m.auth(http.HandlerFunc(m.showSource)))
	mux.Handle("POST /obs/source/hide", m.auth(http.HandlerFunc(m.hideSource)))
	mux.Handle("POST /obs/recording/start", m.auth(http.HandlerFunc(m.startRecording)))
	mux.Handle("POST /obs/recording/stop", m.auth(http.HandlerFunc(m.stopRecording)))
	mux.Handle("POST /obs/stream/start", m.auth(http.HandlerFunc(m.startStreaming)))
	mux.Handle("POST /obs/stream/stop", m.auth(http.HandlerFunc(m.stopStreaming)))
	mux.Handle("POST /ptz/move", m.auth(http.HandlerFunc(m.movePTZ)))
	mux.Handle("POST /ptz/zoom", m.auth(http.HandlerFunc(m.zoomPTZ)))
	mux.Handle("POST /ptz/preset", m.auth(http.HandlerFunc(m.presetPTZ)))
	mux.Handle("GET /api/v1/telemetry", m.auth(http.HandlerFunc(m.telemetry)))
	mux.Handle("GET /api/v1/audio/inputs", m.auth(http.HandlerFunc(m.audioInputs)))
	mux.Handle("PATCH /api/v1/audio/inputs/{name}", m.auth(http.HandlerFunc(m.updateAudioInput)))
	mux.Handle("GET /api/v1/studio-mode", m.auth(http.HandlerFunc(m.studioMode)))
	mux.Handle("POST /api/v1/studio-mode/preview", m.auth(http.HandlerFunc(m.setPreviewScene)))
	mux.Handle("POST /api/v1/transitions", m.auth(http.HandlerFunc(m.transition)))
	mux.Handle("GET /api/v1/diagnostics", m.auth(http.HandlerFunc(m.diagnostics)))
	mux.Handle("GET /api/v1/audit-events", m.auth(http.HandlerFunc(m.auditEvents)))
	mux.HandleFunc("GET /events", m.eventStream)
	mux.Handle("POST /server/restart", m.auth(http.HandlerFunc(m.restart)))
	m.server = &http.Server{Handler: m.cors(m.requestLogger(m.idempotent(mux))), ReadHeaderTimeout: 5 * time.Second}
	m.listener = listener
	server := m.server
	m.logs.Add("server", "info", fmt.Sprintf("Servidor iniciado em http://%s", listener.Addr()))
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			m.logs.Add("server", "error", fmt.Sprintf("Servidor interrompido: %v", err))
		}
	}()
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	server := m.server
	m.server = nil
	m.listener = nil
	m.mu.Unlock()
	if server == nil {
		return nil
	}
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("parar servidor local: %w", err)
	}
	m.logs.Add("server", "info", "Servidor local parado")
	return nil
}

func (m *Manager) Restart(ctx context.Context, settings Settings) error {
	if err := m.Stop(ctx); err != nil {
		return err
	}
	return m.Start(settings)
}

func (m *Manager) Running() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.server != nil
}

func (m *Manager) Address() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.listener == nil {
		return ""
	}
	return m.listener.Addr().String()
}

func (m *Manager) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"status": "ok", "service": "obs-control-server"})
}

func (m *Manager) obsStatus(writer http.ResponseWriter, request *http.Request) {
	status, err := m.obs.Status(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (m *Manager) obsScenes(writer http.ResponseWriter, request *http.Request) {
	scenes, err := m.obs.Scenes(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"scenes": scenes})
}

func (m *Manager) setScene(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		SceneName string `json:"sceneName"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || strings.TrimSpace(body.SceneName) == "" {
		writeError(writer, http.StatusBadRequest, errors.New("informe sceneName"))
		return
	}
	if err := m.obs.SetScene(request.Context(), body.SceneName); err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	m.logs.Add("obs", "info", fmt.Sprintf("Cena alterada para %q", body.SceneName))
	m.Publish("obs.scene.changed", map[string]any{"sceneName": body.SceneName})
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "sceneName": body.SceneName})
}

func (m *Manager) obsPreview(writer http.ResponseWriter, request *http.Request) {
	sceneName := strings.TrimSpace(request.URL.Query().Get("sceneName"))
	if sceneName == "" {
		writeError(writer, http.StatusBadRequest, errors.New("informe sceneName"))
		return
	}
	width := 640
	if value := request.URL.Query().Get("width"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			width = parsed
		}
	}
	quality := 50
	if value := request.URL.Query().Get("quality"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			quality = parsed
		}
	}
	image, err := m.obs.Screenshot(request.Context(), sceneName, width, quality)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	writer.Header().Set("Content-Type", "image/jpeg")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(image)
}

func (m *Manager) obsSources(writer http.ResponseWriter, request *http.Request) {
	sources, err := m.obs.Sources(request.Context(), request.URL.Query().Get("sceneName"))
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"sources": sources})
}

func (m *Manager) showSource(writer http.ResponseWriter, request *http.Request) {
	m.setSourceVisibility(writer, request, true)
}

func (m *Manager) hideSource(writer http.ResponseWriter, request *http.Request) {
	m.setSourceVisibility(writer, request, false)
}

func (m *Manager) setSourceVisibility(writer http.ResponseWriter, request *http.Request, visible bool) {
	var body struct {
		SceneName  string `json:"sceneName"`
		SourceName string `json:"sourceName"`
	}
	if err := decodeBody(writer, request, &body); err != nil || strings.TrimSpace(body.SourceName) == "" {
		writeError(writer, http.StatusBadRequest, errors.New("informe sourceName"))
		return
	}
	if err := m.obs.SetSourceVisible(request.Context(), body.SceneName, body.SourceName, visible); err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	action := "ocultada"
	if visible {
		action = "exibida"
	}
	m.logs.Add("obs", "info", fmt.Sprintf("Fonte %q %s", body.SourceName, action))
	m.Publish("obs.source.visibility", map[string]any{"sceneName": body.SceneName, "sourceName": body.SourceName, "visible": visible})
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "visible": visible})
}

func (m *Manager) startRecording(writer http.ResponseWriter, request *http.Request) {
	m.outputCommand(writer, request, "gravação", true, m.obs.StartRecording, "obs.recording")
}
func (m *Manager) stopRecording(writer http.ResponseWriter, request *http.Request) {
	m.outputCommand(writer, request, "gravação", false, m.obs.StopRecording, "obs.recording")
}
func (m *Manager) startStreaming(writer http.ResponseWriter, request *http.Request) {
	m.outputCommand(writer, request, "transmissão", true, m.obs.StartStreaming, "obs.streaming")
}
func (m *Manager) stopStreaming(writer http.ResponseWriter, request *http.Request) {
	m.outputCommand(writer, request, "transmissão", false, m.obs.StopStreaming, "obs.streaming")
}

type ptzCommand struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Direction string `json:"direction"`
	Speed     int    `json:"speed"`
}

func (m *Manager) movePTZ(writer http.ResponseWriter, request *http.Request) {
	var body ptzCommand
	if err := decodeBody(writer, request, &body); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	if err := m.ptz.Move(request.Context(), body.Host, body.Port, body.Direction, body.Speed); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	m.logs.Add("ptz", "info", fmt.Sprintf("Comando de movimento PTZ %q enviado para %s:%d", body.Direction, body.Host, body.Port))
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true})
}

func (m *Manager) zoomPTZ(writer http.ResponseWriter, request *http.Request) {
	var body ptzCommand
	if err := decodeBody(writer, request, &body); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	if err := m.ptz.Zoom(request.Context(), body.Host, body.Port, body.Direction, body.Speed); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	m.logs.Add("ptz", "info", fmt.Sprintf("Comando de zoom PTZ %q enviado para %s:%d", body.Direction, body.Host, body.Port))
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true})
}

type ptzPresetCommand struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Action string `json:"action"`
	Number int    `json:"number"`
}

func (m *Manager) presetPTZ(writer http.ResponseWriter, request *http.Request) {
	var body ptzPresetCommand
	if err := decodeBody(writer, request, &body); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	if err := m.ptz.Preset(request.Context(), body.Host, body.Port, body.Action, body.Number); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	m.logs.Add("ptz", "info", fmt.Sprintf("Preset PTZ %d (%s) enviado para %s:%d", body.Number, body.Action, body.Host, body.Port))
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true})
}

func (m *Manager) outputCommand(writer http.ResponseWriter, request *http.Request, label string, active bool, command func(context.Context) error, eventType string) {
	if err := command(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	state := "parada"
	if active {
		state = "iniciada"
	}
	m.logs.Add("obs", "info", fmt.Sprintf("%s %s", label, state))
	m.Publish(eventType, map[string]any{"active": active})
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "active": active, "commandId": request.Header.Get("Idempotency-Key"), "status": "confirmed", "confirmedAt": time.Now()})
}

func (m *Manager) telemetry(writer http.ResponseWriter, request *http.Request) {
	data, err := m.obs.Telemetry(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(writer, http.StatusOK, data)
}

func (m *Manager) audioInputs(writer http.ResponseWriter, request *http.Request) {
	inputs, err := m.obs.AudioInputs(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"inputs": inputs})
}

func (m *Manager) updateAudioInput(writer http.ResponseWriter, request *http.Request) {
	name := request.PathValue("name")
	var body struct {
		Muted    *bool    `json:"muted"`
		VolumeDB *float64 `json:"volumeDb"`
	}
	if name == "" || decodeBody(writer, request, &body) != nil || (body.Muted == nil && body.VolumeDB == nil) {
		writeError(writer, http.StatusBadRequest, errors.New("informe muted ou volumeDb"))
		return
	}
	if body.Muted != nil {
		if err := m.obs.SetInputMute(request.Context(), name, *body.Muted); err != nil {
			writeError(writer, http.StatusServiceUnavailable, err)
			return
		}
	}
	if body.VolumeDB != nil {
		if err := m.obs.SetInputVolume(request.Context(), name, *body.VolumeDB); err != nil {
			writeError(writer, http.StatusServiceUnavailable, err)
			return
		}
	}
	m.Publish("obs.audio.changed", map[string]any{"inputName": name})
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "commandId": request.Header.Get("Idempotency-Key"), "status": "confirmed"})
}

func (m *Manager) studioMode(writer http.ResponseWriter, request *http.Request) {
	data, err := m.obs.StudioMode(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(writer, http.StatusOK, data)
}

func (m *Manager) setPreviewScene(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		SceneName string `json:"sceneName"`
	}
	if decodeBody(writer, request, &body) != nil || strings.TrimSpace(body.SceneName) == "" {
		writeError(writer, http.StatusBadRequest, errors.New("informe sceneName"))
		return
	}
	if err := m.obs.SetPreviewScene(request.Context(), body.SceneName); err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	m.Publish("obs.studio.preview", body)
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "status": "confirmed", "commandId": request.Header.Get("Idempotency-Key")})
}

func (m *Manager) transition(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Duration int `json:"duration"`
	}
	if decodeBody(writer, request, &body) != nil {
		writeError(writer, http.StatusBadRequest, errors.New("payload inválido"))
		return
	}
	if body.Duration > 0 {
		if err := m.obs.SetTransitionDuration(request.Context(), body.Duration); err != nil {
			writeError(writer, http.StatusServiceUnavailable, err)
			return
		}
	}
	if err := m.obs.TriggerTransition(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, err)
		return
	}
	m.Publish("obs.studio.transition", map[string]any{"duration": body.Duration})
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "status": "confirmed", "commandId": request.Header.Get("Idempotency-Key")})
}

func (m *Manager) diagnostics(writer http.ResponseWriter, request *http.Request) {
	now := time.Now()
	status, err := m.obs.Status(request.Context())
	obsState, obsMessage := "healthy", "OBS conectado e respondendo"
	if err != nil || !status.Connected {
		obsState, obsMessage = "critical", "OBS desconectado; verifique o aplicativo e o obs-websocket"
	}
	streamState, streamMessage := "no-data", "Transmissão não iniciada"
	if status.Streaming {
		streamState, streamMessage = "healthy", "Saída de streaming ativa"
	}
	links := []map[string]any{
		{"id": "panel", "label": "Painel", "state": "healthy", "lastSeen": now, "message": "Requisição autenticada recebida"},
		{"id": "service", "label": "Serviço local", "state": "healthy", "lastSeen": now, "message": "API local em execução"},
		{"id": "agent", "label": "Agente no computador", "state": "healthy", "lastSeen": now, "message": "Agente local disponível"},
		{"id": "obs", "label": "OBS Studio", "state": obsState, "lastSeen": now, "message": obsMessage},
		{"id": "stream", "label": "Plataforma de streaming", "state": streamState, "lastSeen": now, "message": streamMessage},
	}
	writeJSON(writer, http.StatusOK, map[string]any{"generatedAt": now, "links": links})
}

func (m *Manager) auditEvents(writer http.ResponseWriter, _ *http.Request) {
	m.mu.RLock()
	entries := append([]AuditEvent(nil), m.audit...)
	m.mu.RUnlock()
	writeJSON(writer, http.StatusOK, map[string]any{"events": entries})
}

func (m *Manager) eventStream(writer http.ResponseWriter, request *http.Request) {
	m.mu.RLock()
	token := m.settings.Token
	m.mu.RUnlock()
	provided := request.URL.Query().Get("token")
	if authorization := request.Header.Get("Authorization"); strings.HasPrefix(authorization, "Bearer ") {
		provided = strings.TrimPrefix(authorization, "Bearer ")
	}
	if token == "" || provided != token {
		writeError(writer, http.StatusUnauthorized, errors.New("token inválido ou ausente"))
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	connection, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	id, channel := m.events.Subscribe()
	defer m.events.Unsubscribe(id)
	m.logs.Add("requests", "info", "Cliente conectado ao WebSocket de eventos")
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case event, ok := <-channel:
			if !ok || connection.WriteJSON(event) != nil {
				return
			}
		case <-request.Context().Done():
			return
		case <-ping.C:
			if connection.WriteControl(websocket.PingMessage, nil, time.Now().Add(3*time.Second)) != nil {
				return
			}
		}
	}
}

func (m *Manager) Publish(eventType string, data any) { m.events.Publish(eventType, data) }

func (m *Manager) restart(writer http.ResponseWriter, _ *http.Request) {
	m.mu.RLock()
	settings := m.settings
	m.mu.RUnlock()
	writeJSON(writer, http.StatusAccepted, map[string]any{"ok": true, "message": "reinicialização agendada"})
	go func() {
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := m.Restart(ctx, settings); err != nil {
			m.logs.Add("server", "error", fmt.Sprintf("Falha ao reiniciar servidor: %v", err))
		}
	}()
}

func (m *Manager) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		m.mu.RLock()
		token := m.settings.Token
		m.mu.RUnlock()
		if token == "" || request.Header.Get("Authorization") != "Bearer "+token {
			writeError(writer, http.StatusUnauthorized, errors.New("token inválido ou ausente"))
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (m *Manager) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		m.logs.Add("requests", "info", fmt.Sprintf("%s %s", request.Method, request.URL.Path))
		next.ServeHTTP(writer, request)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	body   []byte
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
func (r *responseRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.body = append(r.body, body...)
	return r.ResponseWriter.Write(body)
}

func (m *Manager) idempotent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost && request.Method != http.MethodPatch {
			next.ServeHTTP(writer, request)
			return
		}
		key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
		if key != "" {
			m.mu.RLock()
			cached, found := m.commands[key]
			m.mu.RUnlock()
			if found {
				for name, values := range cached.Header {
					for _, value := range values {
						writer.Header().Add(name, value)
					}
				}
				writer.Header().Set("Idempotency-Replayed", "true")
				writer.WriteHeader(cached.Status)
				_, _ = writer.Write(cached.Body)
				return
			}
		}
		recorder := &responseRecorder{ResponseWriter: writer}
		next.ServeHTTP(recorder, request)
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}
		result := "confirmed"
		if recorder.status >= 400 {
			result = "failed"
		}
		actor := request.RemoteAddr
		if host, _, err := net.SplitHostPort(actor); err == nil {
			actor = host
		}
		m.mu.Lock()
		if key != "" && recorder.status < 500 {
			m.commands[key] = cachedResponse{Status: recorder.status, Header: recorder.Header().Clone(), Body: append([]byte(nil), recorder.body...), CreatedAt: time.Now()}
			if len(m.commands) > 500 {
				for id, response := range m.commands {
					if time.Since(response.CreatedAt) > 10*time.Minute {
						delete(m.commands, id)
					}
				}
			}
		}
		m.audit = append(m.audit, AuditEvent{ID: uint64(len(m.audit) + 1), Time: time.Now(), Actor: actor, Action: request.Method + " " + request.URL.Path, CommandID: key, Result: result})
		if len(m.audit) > 500 {
			m.audit = append([]AuditEvent(nil), m.audit[len(m.audit)-500:]...)
		}
		m.mu.Unlock()
	})
}

func (m *Manager) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func decodeBody(writer http.ResponseWriter, request *http.Request, output any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(output)
}

func writeError(writer http.ResponseWriter, status int, err error) {
	writeJSON(writer, status, map[string]any{"error": err.Error()})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
