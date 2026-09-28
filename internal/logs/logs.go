package logs

import (
	"sync"
	"time"
)

type Entry struct {
	ID       uint64    `json:"id"`
	Time     time.Time `json:"time"`
	Category string    `json:"category"`
	Level    string    `json:"level"`
	Message  string    `json:"message"`
}

type Buffer struct {
	mu      sync.RWMutex
	entries []Entry
	nextID  uint64
	limit   int
}

func New(limit int) *Buffer {
	if limit < 1 {
		limit = 500
	}
	return &Buffer{limit: limit}
}

func (b *Buffer) Add(category, level, message string) Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	entry := Entry{ID: b.nextID, Time: time.Now(), Category: category, Level: level, Message: message}
	b.entries = append(b.entries, entry)
	if len(b.entries) > b.limit {
		b.entries = append([]Entry(nil), b.entries[len(b.entries)-b.limit:]...)
	}
	return entry
}

func (b *Buffer) List(category string) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	entries := make([]Entry, 0, len(b.entries))
	for _, entry := range b.entries {
		if category == "" || category == "all" || entry.Category == category || (category == "errors" && entry.Level == "error") {
			entries = append(entries, entry)
		}
	}
	return entries
}
