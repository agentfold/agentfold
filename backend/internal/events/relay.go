package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Changes receives the changes other processes announce.
// *store.ChangeListener implements it.
type Changes interface {
	Next(ctx context.Context) (typ string, id uuid.UUID, err error)
	Close()
}

// Relay publishes in the hub the changes other processes make to the board
// (workers running flows), loading the changed entities.
type Relay struct {
	Listen func(ctx context.Context) (Changes, error)
	Load   func(ctx context.Context, typ string, id uuid.UUID) (any, error)
	Hub    *Hub
	// Retry is the pause before listening again when listening fails.
	Retry time.Duration
	Log   *slog.Logger
}

// Run relays changes until ctx ends. Changes announced while it is not
// listening are lost; clients reload the board when they reconnect.
func (r *Relay) Run(ctx context.Context) {
	for {
		err := r.relay(ctx)
		if ctx.Err() != nil {
			return
		}
		r.Log.Error("cannot relay board changes, retrying", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.Retry):
		}
	}
}

func (r *Relay) relay(ctx context.Context) error {
	changes, err := r.Listen(ctx)
	if err != nil {
		return err
	}
	defer changes.Close()
	for {
		typ, id, err := changes.Next(ctx)
		if err != nil {
			return err
		}
		data, err := r.Load(ctx, typ, id)
		if err != nil {
			r.Log.Warn("cannot load a changed entity", "type", typ, "id", id, "error", err)
			continue
		}
		r.Hub.Publish(Event{Type: typ, ID: id, Data: data})
	}
}
