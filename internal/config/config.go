package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const appDirectory = "obs-control-server"

type Server struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	AutoStart bool   `json:"autoStart"`
	APIToken  string `json:"apiToken"`
}

type OBS struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Password      string `json:"password"`
	AutoConnect   bool   `json:"autoConnect"`
	AutoReconnect bool   `json:"autoReconnect"`
}

type Config struct {
	Server Server `json:"server"`
	OBS    OBS    `json:"obs"`
}

func Default() Config {
	return Config{
		Server: Server{Host: "127.0.0.1", Port: 3456, AutoStart: true, APIToken: newToken()},
		OBS:    OBS{Host: "localhost", Port: 4455, AutoConnect: true, AutoReconnect: true},
	}
}

func (c Config) Validate() error {
	if c.Server.Host != "127.0.0.1" && c.Server.Host != "localhost" {
		return errors.New("a versão 1 aceita apenas conexões locais")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return errors.New("a porta do servidor deve estar entre 1 e 65535")
	}
	if c.Server.APIToken == "" {
		return errors.New("o token da API é obrigatório")
	}
	if c.OBS.Host == "" {
		return errors.New("o host do OBS é obrigatório")
	}
	if c.OBS.Port < 1 || c.OBS.Port > 65535 {
		return errors.New("a porta do OBS deve estar entre 1 e 65535")
	}
	return nil
}

type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func NewStore(path string) (*Store, error) {
	if path == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("localizar diretório de configuração: %w", err)
		}
		path = filepath.Join(base, appDirectory, "config.json")
	}
	store := &Store{path: path, cfg: Default()}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *Store) Save(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("codificar configuração: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("criar diretório de configuração: %w", err)
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("gravar configuração: %w", err)
	}
	if err := os.Rename(temporary, s.path); err != nil {
		return fmt.Errorf("salvar configuração: %w", err)
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	return nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s.Save(s.cfg)
	}
	if err != nil {
		return fmt.Errorf("ler configuração: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("decodificar configuração: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuração inválida: %w", err)
	}
	s.cfg = cfg
	return nil
}

func newToken() string {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("gerar token seguro: %v", err))
	}
	return hex.EncodeToString(buffer)
}
