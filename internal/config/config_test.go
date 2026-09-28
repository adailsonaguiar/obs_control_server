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
	if cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != 3456 || len(cfg.Server.APIToken) != 48 {
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

func TestConfigRejectsExternalBind(t *testing.T) {
	cfg := Default()
	cfg.Server.Host = "0.0.0.0"
	if err := cfg.Validate(); err == nil {
		t.Fatal("host externo deveria ser rejeitado na versão 1")
	}
}
