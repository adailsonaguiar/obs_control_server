package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"obs-control-server/internal/logs"
	"obs-control-server/internal/obs"
)

type fakeOBS struct{ scene string }

func (f *fakeOBS) Status(context.Context) (obs.Status, error) {
	return obs.Status{Connected: true, CurrentScene: f.scene}, nil
}
func (f *fakeOBS) Scenes(context.Context) ([]obs.Scene, error) {
	return []obs.Scene{{Name: "Abertura"}, {Name: "Câmera"}}, nil
}
func (f *fakeOBS) SetScene(_ context.Context, name string) error { f.scene = name; return nil }

func TestProtectedSceneWorkflow(t *testing.T) {
	controller := &fakeOBS{scene: "Abertura"}
	manager := New(controller, logs.New(20))
	if err := manager.Start(Settings{Host: "127.0.0.1", Port: 0, Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop(context.Background())
	client := &http.Client{Timeout: time.Second}
	baseURL := "http://" + manager.Address()

	response, err := client.Get(baseURL + "/obs/scenes")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("esperava 401, recebeu %d", response.StatusCode)
	}
	response.Body.Close()

	body, _ := json.Marshal(map[string]string{"sceneName": "Câmera"})
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/obs/scene", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || controller.scene != "Câmera" {
		t.Fatalf("troca de cena falhou: status=%d cena=%s", response.StatusCode, controller.scene)
	}
}

func TestHealthDoesNotRequireToken(t *testing.T) {
	manager := New(&fakeOBS{}, logs.New(20))
	if err := manager.Start(Settings{Host: "127.0.0.1", Port: 0, Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop(context.Background())
	response, err := http.Get("http://" + manager.Address() + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d", response.StatusCode)
	}
}
