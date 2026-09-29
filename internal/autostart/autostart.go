package autostart

import (
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"runtime"
)

const identifier = "com.obscontrol.server"

type Manager struct {
	goos       string
	home       string
	executable string
}

func New() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("localizar pasta do usuário: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("localizar executável: %w", err)
	}
	return &Manager{goos: runtime.GOOS, home: home, executable: executable}, nil
}

func NewForTest(goos, home, executable string) *Manager {
	return &Manager{goos: goos, home: home, executable: executable}
}

func (m *Manager) SetEnabled(enabled bool) error {
	path, content, err := m.definition()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("desativar inicialização automática: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("criar pasta de inicialização automática: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("ativar inicialização automática: %w", err)
	}
	return nil
}

func (m *Manager) Enabled() bool {
	path, _, err := m.definition()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func (m *Manager) definition() (string, string, error) {
	switch m.goos {
	case "darwin":
		path := filepath.Join(m.home, "Library", "LaunchAgents", identifier+".plist")
		content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string></array>
<key>RunAtLoad</key><true/>
</dict></plist>
`, identifier, html.EscapeString(m.executable))
		return path, content, nil
	case "windows":
		path := filepath.Join(m.home, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "OBS Remote Deck.cmd")
		return path, fmt.Sprintf("@start \"\" \"%s\"\r\n", m.executable), nil
	case "linux":
		path := filepath.Join(m.home, ".config", "autostart", "obs-control-server.desktop")
		return path, fmt.Sprintf("[Desktop Entry]\nType=Application\nName=OBS Remote Deck\nExec=\"%s\"\nX-GNOME-Autostart-enabled=true\n", m.executable), nil
	default:
		return "", "", fmt.Errorf("inicialização automática não suportada em %s", m.goos)
	}
}
