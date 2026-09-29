package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"obs-control-server/internal/autostart"
	"obs-control-server/internal/config"
	"obs-control-server/internal/logs"
	"obs-control-server/internal/obs"
	"obs-control-server/internal/server"
)

// AppVersion is the user-facing application version. It may be replaced at
// build time with: -ldflags "-X main.AppVersion=2.1.0".
var AppVersion = "0.0.1"

type ConfigView struct {
	Server        config.Server      `json:"server"`
	Application   config.Application `json:"application"`
	OBSHost       string             `json:"obsHost"`
	OBSPort       int                `json:"obsPort"`
	AutoConnect   bool               `json:"autoConnect"`
	AutoReconnect bool               `json:"autoReconnect"`
	HasPassword   bool               `json:"hasPassword"`
	ActiveProfile string             `json:"activeProfile"`
	ProfileNames  []string           `json:"profileNames"`
}

type Snapshot struct {
	ServerRunning bool       `json:"serverRunning"`
	ServerAddress string     `json:"serverAddress"`
	OBS           obs.Status `json:"obs"`
}

type App struct {
	ctx       context.Context
	store     *config.Store
	logs      *logs.Buffer
	obs       *obs.Client
	server    *server.Manager
	autostart *autostart.Manager
	connectMu sync.Mutex
	cancel    context.CancelFunc
}

func NewApp() *App {
	eventLogs := logs.New(500)
	obsClient := obs.NewClient()
	autostartManager, _ := autostart.New()
	return &App{logs: eventLogs, obs: obsClient, server: server.New(obsClient, eventLogs), autostart: autostartManager}
}

func (a *App) GetVersion() string {
	return AppVersion
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	store, err := config.NewStore("")
	if err != nil {
		a.logs.Add("server", "error", fmt.Sprintf("Falha ao carregar configuração: %v", err))
		return
	}
	a.store = store
	reconnectContext, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	cfg := a.store.Get()
	if cfg.Server.AutoStart {
		if err := a.startServer(cfg); err != nil {
			a.logs.Add("server", "error", err.Error())
		}
	}
	if cfg.OBS.AutoConnect {
		go func() {
			if err := a.connectOBS(cfg); err != nil {
				a.logs.Add("obs", "error", err.Error())
			}
		}()
	}
	go a.reconnectLoop(reconnectContext)
	go a.eventForwardLoop(reconnectContext)
}

func (a *App) shutdown(context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
	a.obs.Disconnect()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = a.server.Stop(ctx)
}

func (a *App) beforeClose(ctx context.Context) bool {
	if a.store != nil && a.store.Get().Application.MinimizeToTray {
		runtime.WindowHide(ctx)
		return true
	}
	return false
}

func (a *App) GetConfig() ConfigView {
	if a.store == nil {
		return configView(config.Default())
	}
	cfg := a.store.Get()
	return configView(cfg)
}

func (a *App) SaveConfig(view ConfigView, password string) error {
	if a.store == nil {
		return errors.New("armazenamento de configuração indisponível")
	}
	previous := a.store.Get()
	serverConfig := view.Server
	if serverConfig.AllowLAN {
		serverConfig.Host = "0.0.0.0"
	} else {
		serverConfig.Host = "127.0.0.1"
	}
	obsConfig := config.OBS{Host: view.OBSHost, Port: view.OBSPort, AutoConnect: view.AutoConnect,
		AutoReconnect: view.AutoReconnect, Password: password}
	if password == "" && view.HasPassword {
		obsConfig.Password = previous.OBS.Password
	}
	updated := previous.WithActiveSettings(serverConfig, obsConfig)
	updated.Application = view.Application
	if a.autostart != nil {
		if err := a.autostart.SetEnabled(updated.Application.LaunchAtLogin); err != nil {
			return err
		}
	}
	if err := a.store.Save(updated); err != nil {
		return err
	}
	a.logs.Add("server", "info", "Configurações salvas")
	return nil
}

func (a *App) CreateProfile(name string) error {
	if a.store == nil {
		return errors.New("configuração indisponível")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("informe o nome do perfil")
	}
	cfg := a.store.Get()
	if _, exists := cfg.Profiles[name]; exists {
		return fmt.Errorf("o perfil %q já existe", name)
	}
	cfg.Profiles[name] = config.Profile{Server: cfg.Server, OBS: cfg.OBS}
	cfg, _ = cfg.SwitchProfile(name)
	if err := a.store.Save(cfg); err != nil {
		return err
	}
	a.logs.Add("server", "info", fmt.Sprintf("Perfil %q criado", name))
	return nil
}

func (a *App) SwitchProfile(name string) error {
	if a.store == nil {
		return errors.New("configuração indisponível")
	}
	cfg, err := a.store.Get().SwitchProfile(name)
	if err != nil {
		return err
	}
	wasRunning := a.server.Running()
	if err := a.store.Save(cfg); err != nil {
		return err
	}
	a.obs.Disconnect()
	if wasRunning {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := a.server.Restart(ctx, serverSettings(cfg)); err != nil {
			return err
		}
	}
	if cfg.OBS.AutoConnect {
		go func() { _ = a.connectOBS(cfg) }()
	}
	a.logs.Add("server", "info", fmt.Sprintf("Perfil ativo alterado para %q", name))
	return nil
}

func (a *App) DeleteProfile(name string) error {
	if a.store == nil {
		return errors.New("configuração indisponível")
	}
	cfg := a.store.Get()
	if len(cfg.Profiles) <= 1 {
		return errors.New("é necessário manter ao menos um perfil")
	}
	if _, exists := cfg.Profiles[name]; !exists {
		return fmt.Errorf("perfil %q não encontrado", name)
	}
	delete(cfg.Profiles, name)
	if cfg.ActiveProfile == name {
		names := profileNames(cfg)
		cfg, _ = cfg.SwitchProfile(names[0])
	}
	return a.store.Save(cfg)
}

func (a *App) GetSnapshot() Snapshot {
	status := obs.Status{Connected: a.obs.Connected()}
	if status.Connected {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if current, err := a.obs.Status(ctx); err == nil {
			status = current
		}
	}
	return Snapshot{ServerRunning: a.server.Running(), ServerAddress: a.server.Address(), OBS: status}
}

func (a *App) StartServer() error {
	if a.store == nil {
		return errors.New("configuração indisponível")
	}
	return a.startServer(a.store.Get())
}

func (a *App) StopServer() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return a.server.Stop(ctx)
}

func (a *App) RestartServer() error {
	if a.store == nil {
		return errors.New("configuração indisponível")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return a.server.Restart(ctx, serverSettings(a.store.Get()))
}

func (a *App) ConnectOBS() error {
	if a.store == nil {
		return errors.New("configuração indisponível")
	}
	return a.connectOBS(a.store.Get())
}

func (a *App) DisconnectOBS() {
	a.obs.Disconnect()
	a.logs.Add("obs", "info", "OBS desconectado")
}

func (a *App) TestOBS() error {
	if a.store == nil {
		return errors.New("configuração indisponível")
	}
	cfg := a.store.Get()
	temporary := obs.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := temporary.Connect(ctx, obsSettings(cfg)); err != nil {
		a.logs.Add("obs", "error", fmt.Sprintf("Teste de conexão falhou: %v", err))
		return err
	}
	temporary.Disconnect()
	a.logs.Add("obs", "info", "Teste de conexão concluído com sucesso")
	return nil
}

func (a *App) GetScenes() ([]obs.Scene, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return a.obs.Scenes(ctx)
}

func (a *App) SetScene(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := a.obs.SetScene(ctx, name); err != nil {
		return err
	}
	a.logs.Add("obs", "info", fmt.Sprintf("Cena alterada para %q", name))
	return nil
}

func (a *App) GetSources(sceneName string) ([]obs.Source, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return a.obs.Sources(ctx, sceneName)
}

func (a *App) SetSourceVisible(sceneName, sourceName string, visible bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := a.obs.SetSourceVisible(ctx, sceneName, sourceName, visible); err != nil {
		return err
	}
	a.logs.Add("obs", "info", fmt.Sprintf("Visibilidade da fonte %q alterada para %t", sourceName, visible))
	a.server.Publish("obs.source.visibility", map[string]any{"sceneName": sceneName, "sourceName": sourceName, "visible": visible})
	return nil
}

func (a *App) SetRecording(active bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	if active {
		err = a.obs.StartRecording(ctx)
	} else {
		err = a.obs.StopRecording(ctx)
	}
	if err != nil {
		return err
	}
	a.logs.Add("obs", "info", fmt.Sprintf("Gravação ativa: %t", active))
	a.server.Publish("obs.recording", map[string]any{"active": active})
	return nil
}

func (a *App) SetStreaming(active bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	if active {
		err = a.obs.StartStreaming(ctx)
	} else {
		err = a.obs.StopStreaming(ctx)
	}
	if err != nil {
		return err
	}
	a.logs.Add("obs", "info", fmt.Sprintf("Transmissão ativa: %t", active))
	a.server.Publish("obs.streaming", map[string]any{"active": active})
	return nil
}

func (a *App) MinimizeToTray() { runtime.WindowHide(a.ctx) }

func (a *App) GetLogs(category string) []logs.Entry { return a.logs.List(category) }

func (a *App) startServer(cfg config.Config) error { return a.server.Start(serverSettings(cfg)) }

func (a *App) connectOBS(cfg config.Config) error {
	a.connectMu.Lock()
	defer a.connectMu.Unlock()
	if a.obs.Connected() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if err := a.obs.Connect(ctx, obsSettings(cfg)); err != nil {
		return err
	}
	a.logs.Add("obs", "info", fmt.Sprintf("Conectado ao OBS em %s:%d", cfg.OBS.Host, cfg.OBS.Port))
	return nil
}

func (a *App) reconnectLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.store == nil {
				continue
			}
			cfg := a.store.Get()
			if !cfg.OBS.AutoReconnect || a.obs.Connected() {
				continue
			}
			if err := a.connectOBS(cfg); err != nil && !errors.Is(err, context.Canceled) {
				a.logs.Add("obs", "error", fmt.Sprintf("Reconexão ao OBS falhou: %v", err))
			}
		}
	}
}

func (a *App) eventForwardLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-a.obs.Events():
			a.server.Publish("obs.event."+event.Type, event.Data)
		}
	}
}

func obsSettings(cfg config.Config) obs.Settings {
	return obs.Settings{Host: cfg.OBS.Host, Port: cfg.OBS.Port, Password: cfg.OBS.Password}
}

func serverSettings(cfg config.Config) server.Settings {
	return server.Settings{Host: cfg.Server.Host, Port: cfg.Server.Port, Token: cfg.Server.APIToken}
}

func configView(cfg config.Config) ConfigView {
	return ConfigView{Server: cfg.Server, Application: cfg.Application, OBSHost: cfg.OBS.Host, OBSPort: cfg.OBS.Port,
		AutoConnect: cfg.OBS.AutoConnect, AutoReconnect: cfg.OBS.AutoReconnect, HasPassword: cfg.OBS.Password != "",
		ActiveProfile: cfg.ActiveProfile, ProfileNames: profileNames(cfg)}
}

func profileNames(cfg config.Config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
