package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"obs-control-server/internal/events"
	"obs-control-server/internal/logs"
	"obs-control-server/internal/obs"
)

type OBSController interface {
	Status(context.Context) (obs.Status, error)
	Scenes(context.Context) ([]obs.Scene, error)
	SetScene(context.Context, string) error
	Sources(context.Context, string) ([]obs.Source, error)
	SetSourceVisible(context.Context, string, string, bool) error
	StartRecording(context.Context) error
	StopRecording(context.Context) error
	StartStreaming(context.Context) error
	StopStreaming(context.Context) error
}

type Settings struct {
	Host  string
	Port  int
	Token string
}

type Manager struct {
	mu       sync.RWMutex
	server   *http.Server
	listener net.Listener
	settings Settings
	obs      OBSController
	logs     *logs.Buffer
	events   *events.Hub
}

func New(controller OBSController, eventLogs *logs.Buffer) *Manager {
	return &Manager{obs: controller, logs: eventLogs, events: events.New()}
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
	mux.Handle("GET /obs/sources", m.auth(http.HandlerFunc(m.obsSources)))
	mux.Handle("POST /obs/source/show", m.auth(http.HandlerFunc(m.showSource)))
	mux.Handle("POST /obs/source/hide", m.auth(http.HandlerFunc(m.hideSource)))
	mux.Handle("POST /obs/recording/start", m.auth(http.HandlerFunc(m.startRecording)))
	mux.Handle("POST /obs/recording/stop", m.auth(http.HandlerFunc(m.stopRecording)))
	mux.Handle("POST /obs/stream/start", m.auth(http.HandlerFunc(m.startStreaming)))
	mux.Handle("POST /obs/stream/stop", m.auth(http.HandlerFunc(m.stopStreaming)))
	mux.HandleFunc("GET /events", m.eventStream)
	mux.Handle("POST /server/restart", m.auth(http.HandlerFunc(m.restart)))
	m.server = &http.Server{Handler: m.cors(m.requestLogger(mux)), ReadHeaderTimeout: 5 * time.Second}
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
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "active": active})
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

func (m *Manager) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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
