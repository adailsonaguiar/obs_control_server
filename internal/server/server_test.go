package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"obs-control-server/internal/events"
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
func (f *fakeOBS) Screenshot(context.Context, string, int, int) ([]byte, error) {
	return []byte{0xff, 0xd8, 0xff, 0xd9}, nil
}
func (f *fakeOBS) Sources(context.Context, string) ([]obs.Source, error) {
	return []obs.Source{{SceneName: "Abertura", Name: "Logo", ID: 7, Enabled: true}}, nil
}
func (f *fakeOBS) SetSourceVisible(context.Context, string, string, bool) error { return nil }
func (f *fakeOBS) StartRecording(context.Context) error                         { return nil }
func (f *fakeOBS) StopRecording(context.Context) error                          { return nil }
func (f *fakeOBS) StartStreaming(context.Context) error                         { return nil }
func (f *fakeOBS) StopStreaming(context.Context) error                          { return nil }

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

func TestPreviewRequiresTokenAndReturnsJPEG(t *testing.T) {
	manager := New(&fakeOBS{}, logs.New(20))
	if err := manager.Start(Settings{Host: "127.0.0.1", Port: 0, Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop(context.Background())
	request, _ := http.NewRequest(http.MethodGet, "http://"+manager.Address()+"/obs/preview?sceneName=Abertura&width=640", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("prévia inesperada: status=%d content-type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
}

func TestEventsWebSocketRequiresTokenAndReceivesCommands(t *testing.T) {
	manager := New(&fakeOBS{}, logs.New(20))
	if err := manager.Start(Settings{Host: "127.0.0.1", Port: 0, Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop(context.Background())
	websocketURL := "ws://" + manager.Address() + "/events?token=secret"
	connection, _, err := websocket.DefaultDialer.Dial(websocketURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	request, _ := http.NewRequest(http.MethodPost, "http://"+manager.Address()+"/obs/recording/start", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	var event events.Event
	if err := connection.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "obs.recording" {
		t.Fatalf("evento inesperado: %+v", event)
	}
}
