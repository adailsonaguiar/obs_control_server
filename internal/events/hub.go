package events

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	ID   uint64    `json:"id"`
	Time time.Time `json:"time"`
	Type string    `json:"type"`
	Data any       `json:"data,omitempty"`
}

type Hub struct {
	mu       sync.RWMutex
	clients  map[uint64]chan Event
	nextID   atomic.Uint64
	nextConn atomic.Uint64
}

func New() *Hub { return &Hub{clients: make(map[uint64]chan Event)} }

func (h *Hub) Subscribe() (uint64, <-chan Event) {
	id := h.nextConn.Add(1)
	channel := make(chan Event, 32)
	h.mu.Lock()
	h.clients[id] = channel
	h.mu.Unlock()
	return id, channel
}

func (h *Hub) Unsubscribe(id uint64) {
	h.mu.Lock()
	if channel, ok := h.clients[id]; ok {
		delete(h.clients, id)
		close(channel)
	}
	h.mu.Unlock()
}

func (h *Hub) Publish(eventType string, data any) Event {
	event := Event{ID: h.nextID.Add(1), Time: time.Now(), Type: eventType, Data: data}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, channel := range h.clients {
		select {
		case channel <- event:
		default:
		}
	}
	return event
}

func (e Event) JSON() []byte {
	data, _ := json.Marshal(e)
	return data
}
