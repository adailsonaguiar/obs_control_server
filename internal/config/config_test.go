package config

import (
	"path/filepath"
	"testing"
)

func TestStoreCreatesAndPersistsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := store.Get()
	if cfg.Server.Host != "0.0.0.0" || !cfg.Server.AllowLAN || cfg.Server.Port != 3456 || len(cfg.Server.APIToken) != 48 {
		t.Fatalf("defaults inesperados: %+v", cfg.Server)
	}
	cfg.Server.Port = 4567
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Get().Server.Port != 4567 {
		t.Fatal("configuração não foi persistida")
	}
}

func TestConfigRejectsExternalBindWithoutLAN(t *testing.T) {
	cfg := Default()
	cfg.Server.AllowLAN = false
	cfg.Server.Host = "0.0.0.0"
	if err := cfg.Validate(); err == nil {
		t.Fatal("host externo deveria ser rejeitado sem acesso LAN")
	}
}

func TestProfilesPersistAndSwitch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := store.Get()
	production := Profile{Server: cfg.Server, OBS: cfg.OBS}
	production.Server.Port = 4567
	production.OBS.Host = "obs-studio.local"
	cfg.Profiles["Produção"] = production
	cfg, err = cfg.SwitchProfile("Produção")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Get().ActiveProfile != "Produção" || reloaded.Get().Server.Port != 4567 {
		t.Fatalf("perfil não persistido: %+v", reloaded.Get())
	}
}

func TestLANRequiresExternalBind(t *testing.T) {
	cfg := Default()
	cfg.Server.Host = "127.0.0.1"
	if err := cfg.Validate(); err == nil {
		t.Fatal("LAN deveria exigir host 0.0.0.0")
	}
	cfg.Server.Host = "0.0.0.0"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
