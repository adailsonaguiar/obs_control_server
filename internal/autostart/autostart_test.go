package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnableAndDisableForSupportedPlatforms(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		t.Run(platform, func(t *testing.T) {
			home := t.TempDir()
			manager := NewForTest(platform, home, filepath.Join(home, "OBS Remote Deck"))
			if err := manager.SetEnabled(true); err != nil {
				t.Fatal(err)
			}
			if !manager.Enabled() {
				t.Fatal("inicialização deveria estar ativa")
			}
			path, _, _ := manager.definition()
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(data), "OBS Remote Deck") {
				t.Fatalf("definição inválida: %s, %v", data, err)
			}
			if err := manager.SetEnabled(false); err != nil || manager.Enabled() {
				t.Fatalf("falha ao desativar: %v", err)
			}
		})
	}
}
