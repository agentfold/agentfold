package events

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type change struct {
	typ string
	id  uuid.UUID
	err error
}

// fakeChanges hands out the changes sent on next; closed counts Close calls.
type fakeChanges struct {
	next   chan change
	mu     sync.Mutex
	closed int
}

func (f *fakeChanges) Next(ctx context.Context) (string, uuid.UUID, error) {
	select {
	case <-ctx.Done():
		return "", uuid.Nil, ctx.Err()
	case c := <-f.next:
		return c.typ, c.id, c.err
	}
}

func (f *fakeChanges) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
}

func TestRelay(t *testing.T) {
	hub := NewHub(8)
	events, stop := hub.Subscribe()
	defer stop()
	changes := &fakeChanges{next: make(chan change)}
	var listens int
	listened := make(chan struct{}, 4)
	relay := &Relay{
		Listen: func(context.Context) (Changes, error) {
			listens++
			listened <- struct{}{}
			if listens == 1 {
				return nil, errors.New("database down")
			}
			return changes, nil
		},
		Load: func(_ context.Context, typ string, id uuid.UUID) (any, error) {
			if typ == "card.deleted" {
				return nil, errors.New("gone")
			}
			return "loaded " + id.String(), nil
		},
		Hub:   hub,
		Retry: time.Millisecond,
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		relay.Run(ctx)
	}()

	<-listened // fails, then retries
	<-listened
	id := uuid.New()
	changes.next <- change{typ: "card.deleted", id: uuid.New()} // cannot load: skipped
	changes.next <- change{typ: "card.moved", id: id}
	if e := <-events; e.Type != "card.moved" || e.ID != id || e.Data != "loaded "+id.String() {
		t.Fatalf("got %+v", e)
	}
	changes.next <- change{err: errors.New("connection lost")} // listens again
	<-listened
	cancel()
	<-done
	changes.mu.Lock()
	defer changes.mu.Unlock()
	if changes.closed != 2 {
		t.Fatalf("closed %d listeners, want 2", changes.closed)
	}
}

func TestRelayStopsWhileWaitingToRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	relay := &Relay{
		Listen: func(context.Context) (Changes, error) {
			cancel()
			return nil, errors.New("database down")
		},
		Retry: time.Hour,
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	relay.Run(ctx) // returns although the retry is an hour away
}

func TestRelayStopsWhileWaitingToRetryAfterAnError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	relay := &Relay{
		Listen: func(context.Context) (Changes, error) { return nil, errors.New("database down") },
		Retry:  time.Hour,
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	time.AfterFunc(10*time.Millisecond, cancel)
	relay.Run(ctx)
}
