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

	"obs-control-server/internal/logs"
	"obs-control-server/internal/obs"
)

type OBSController interface {
	Status(context.Context) (obs.Status, error)
	Scenes(context.Context) ([]obs.Scene, error)
	SetScene(context.Context, string) error
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
}

func New(controller OBSController, eventLogs *logs.Buffer) *Manager {
	return &Manager{obs: controller, logs: eventLogs}
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
	mux.Handle("POST /server/restart", m.auth(http.HandlerFunc(m.restart)))
	m.server = &http.Server{Handler: m.requestLogger(mux), ReadHeaderTimeout: 5 * time.Second}
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
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "sceneName": body.SceneName})
}

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

func writeError(writer http.ResponseWriter, status int, err error) {
	writeJSON(writer, status, map[string]any{"error": err.Error()})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
