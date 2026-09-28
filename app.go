package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"obs-control-server/internal/config"
	"obs-control-server/internal/logs"
	"obs-control-server/internal/obs"
	"obs-control-server/internal/server"
)

type ConfigView struct {
	Server        config.Server `json:"server"`
	OBSHost       string        `json:"obsHost"`
	OBSPort       int           `json:"obsPort"`
	AutoConnect   bool          `json:"autoConnect"`
	AutoReconnect bool          `json:"autoReconnect"`
	HasPassword   bool          `json:"hasPassword"`
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
	connectMu sync.Mutex
	cancel    context.CancelFunc
}

func NewApp() (*App, error) {
	store, err := config.NewStore("")
	if err != nil {
		return nil, err
	}
	eventLogs := logs.New(500)
	obsClient := obs.NewClient()
	return &App{store: store, logs: eventLogs, obs: obsClient, server: server.New(obsClient, eventLogs)}, nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
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

func (a *App) GetConfig() ConfigView {
	cfg := a.store.Get()
	return ConfigView{Server: cfg.Server, OBSHost: cfg.OBS.Host, OBSPort: cfg.OBS.Port,
		AutoConnect: cfg.OBS.AutoConnect, AutoReconnect: cfg.OBS.AutoReconnect, HasPassword: cfg.OBS.Password != ""}
}

func (a *App) SaveConfig(view ConfigView, password string) error {
	previous := a.store.Get()
	updated := config.Config{Server: view.Server, OBS: config.OBS{Host: view.OBSHost, Port: view.OBSPort,
		AutoConnect: view.AutoConnect, AutoReconnect: view.AutoReconnect, Password: password}}
	if password == "" && view.HasPassword {
		updated.OBS.Password = previous.OBS.Password
	}
	if err := a.store.Save(updated); err != nil {
		return err
	}
	a.logs.Add("server", "info", "Configurações salvas")
	return nil
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

func (a *App) StartServer() error { return a.startServer(a.store.Get()) }

func (a *App) StopServer() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return a.server.Stop(ctx)
}

func (a *App) RestartServer() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return a.server.Restart(ctx, serverSettings(a.store.Get()))
}

func (a *App) ConnectOBS() error { return a.connectOBS(a.store.Get()) }

func (a *App) DisconnectOBS() {
	a.obs.Disconnect()
	a.logs.Add("obs", "info", "OBS desconectado")
}

func (a *App) TestOBS() error {
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

func obsSettings(cfg config.Config) obs.Settings {
	return obs.Settings{Host: cfg.OBS.Host, Port: cfg.OBS.Port, Password: cfg.OBS.Password}
}

func serverSettings(cfg config.Config) server.Settings {
	return server.Settings{Host: cfg.Server.Host, Port: cfg.Server.Port, Token: cfg.Server.APIToken}
}
