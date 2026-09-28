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
}
