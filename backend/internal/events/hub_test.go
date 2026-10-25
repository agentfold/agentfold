package events

import (
	"testing"

	"github.com/google/uuid"
)

func TestHubDeliversToEverySubscriber(t *testing.T) {
	h := NewHub(4)
	a, cancelA := h.Subscribe()
	b, cancelB := h.Subscribe()
	defer cancelB()

	e := Event{Type: "card.created", ID: uuid.New()}
	h.Publish(e)
	if got := <-a; got != e {
		t.Fatalf("a got %+v", got)
	}
	if got := <-b; got != e {
		t.Fatalf("b got %+v", got)
	}

	cancelA()
	cancelA() // ending twice is harmless
	if _, ok := <-a; ok {
		t.Fatal("channel a should be closed")
	}
	h.Publish(e)
	if got := <-b; got != e {
		t.Fatalf("b got %+v after a left", got)
	}
}

func TestHubDropsSlowSubscribers(t *testing.T) {
	h := NewHub(1)
	ch, cancel := h.Subscribe()
	defer cancel()

	h.Publish(Event{Type: "first"})
	h.Publish(Event{Type: "second"}) // buffer full: subscriber is dropped
	if got := <-ch; got.Type != "first" {
		t.Fatalf("got %+v", got)
	}
	if _, ok := <-ch; ok {
		t.Fatal("slow subscriber should be closed")
	}
}
