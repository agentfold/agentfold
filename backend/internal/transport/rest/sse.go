package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// boardEvent is the JSON sent in each SSE message (BoardEvent in the contract).
type boardEvent struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
	Data any       `json:"data,omitempty"`
}

// eventStream serves the board events as Server-Sent Events. The stream ends
// when the client leaves or falls behind; EventSource then reconnects and the
// client should reload what it shows.
type eventStream struct {
	hub       Subscriber
	heartbeat time.Duration
}

func (s eventStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	ch, cancel := s.hub.Subscribe()
	defer cancel()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if rc.Flush() != nil {
		return
	}

	ticker := time.NewTicker(s.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
		case e, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(boardEvent{Type: e.Type, ID: e.ID, Data: toEntity(e.Data)}) // API types always marshal
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
		}
		if rc.Flush() != nil {
			return
		}
	}
}
