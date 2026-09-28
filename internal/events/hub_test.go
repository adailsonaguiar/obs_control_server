package events

import (
	"testing"
	"time"
)

func TestHubPublishesAndUnsubscribes(t *testing.T) {
	hub := New()
	id, channel := hub.Subscribe()
	event := hub.Publish("obs.scene.changed", map[string]string{"sceneName": "Câmera"})
	select {
	case received := <-channel:
		if received.ID != event.ID || received.Type != event.Type {
			t.Fatalf("evento inesperado: %+v", received)
		}
	case <-time.After(time.Second):
		t.Fatal("evento não recebido")
	}
	hub.Unsubscribe(id)
	if _, ok := <-channel; ok {
		t.Fatal("canal deveria estar fechado")
	}
}
