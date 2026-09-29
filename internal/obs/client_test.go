package obs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestAuthentication(t *testing.T) {
	got := authentication("password", "salt", "challenge")
	if got != "zTM5ki6L2vVvBQiTG9ckH1Lh64AbnCf6XZ226UmnkIA=" {
		t.Fatalf("autenticação inesperada: %s", got)
	}
}

func TestClientConnectsListsAndChangesScene(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(map[string]any{"op": 0, "d": map[string]any{"rpcVersion": 1}})
		var identify envelope
		_ = conn.ReadJSON(&identify)
		_ = conn.WriteJSON(map[string]any{"op": 2, "d": map[string]any{"negotiatedRpcVersion": 1}})
		for {
			var message envelope
			if err := conn.ReadJSON(&message); err != nil {
				return
			}
			var payload map[string]any
			_ = json.Unmarshal(message.D, &payload)
			responseData := map[string]any{}
			if payload["requestType"] == "GetSceneList" {
				responseData["scenes"] = []map[string]any{{"sceneName": "Abertura"}, {"sceneName": "Câmera"}}
			}
			if payload["requestType"] == "GetSceneItemList" {
				responseData["sceneItems"] = []map[string]any{{"sourceName": "Logo", "sceneItemId": 7, "sceneItemEnabled": true}}
			}
			if payload["requestType"] == "GetSourceScreenshot" {
				responseData["imageData"] = "data:image/jpeg;base64,/9j/2Q=="
			}
			_ = conn.WriteJSON(map[string]any{"op": 7, "d": map[string]any{
				"requestType": payload["requestType"], "requestId": payload["requestId"],
				"requestStatus": map[string]any{"result": true, "code": 100}, "responseData": responseData,
			}})
		}
	}))
	defer server.Close()

	parsed, _ := url.Parse(server.URL)
	parts := strings.Split(parsed.Host, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])
	client := NewClient()
	if err := client.Connect(context.Background(), Settings{Host: parts[0], Port: port}); err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect()
	scenes, err := client.Scenes(context.Background())
	if err != nil || len(scenes) != 2 || scenes[1].Name != "Câmera" {
		t.Fatalf("cenas inesperadas: %+v, erro: %v", scenes, err)
	}
	if err := client.SetScene(context.Background(), "Abertura"); err != nil {
		t.Fatal(err)
	}
	sources, err := client.Sources(context.Background(), "Abertura")
	if err != nil || len(sources) != 1 || sources[0].Name != "Logo" || !sources[0].Enabled {
		t.Fatalf("fontes inesperadas: %+v, erro: %v", sources, err)
	}
	if err := client.SetSourceVisible(context.Background(), "Abertura", "Logo", false); err != nil {
		t.Fatal(err)
	}
	preview, err := client.Screenshot(context.Background(), "Abertura", 640)
	if err != nil || len(preview) != 4 || preview[0] != 0xff {
		t.Fatalf("prévia inesperada: %v, erro: %v", preview, err)
	}
	if err := client.StartRecording(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.StopStreaming(context.Background()); err != nil {
		t.Fatal(err)
	}
}
