// Package events fans board changes out to live subscribers (the SSE stream).
package events

import (
	"sync"

	"github.com/google/uuid"
)

// Event is a change to the board. Type is "<entity>.<change>", e.g. "card.moved".
type Event struct {
	Type string
	ID   uuid.UUID
	// Data is the domain entity after the change; nil when it was deleted.
	Data any
}

// Hub delivers every published event to every subscriber. It lives in the
// server process; changes made by other processes will need Postgres
// LISTEN/NOTIFY behind the same interface.
type Hub struct {
	mu     sync.Mutex
	subs   map[chan Event]struct{}
	buffer int
}

// NewHub returns a Hub whose subscribers can fall up to buffer events behind.
func NewHub(buffer int) *Hub {
	return &Hub{subs: map[chan Event]struct{}{}, buffer: buffer}
}

// Subscribe returns a channel with the events published from now on and a
// function that ends the subscription. The channel is closed when the
// subscription ends, including when the subscriber falls too far behind:
// it should then reload the board and subscribe again.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, h.buffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() { h.drop(ch) }
}

// Publish delivers e to the subscribers without blocking.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
			h.dropLocked(ch)
		}
	}
}

func (h *Hub) drop(ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dropLocked(ch)
}

func (h *Hub) dropLocked(ch chan Event) {
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}
