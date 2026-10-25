package service

import (
	"errors"
	"testing"

	"orquestrator/internal/domain"
)

func TestBusyCardsCannotBeMoved(t *testing.T) {
	s, repo, pub := newService()
	repo.busy = true
	if _, err := s.MoveCard(ctx, id, id, 0); !errors.Is(err, domain.ErrCardBusy) {
		t.Fatalf("got %v", err)
	}
	if len(repo.calls) != 0 || len(pub.events) != 0 {
		t.Fatalf("calls %v, events %v", repo.calls, pub.events)
	}

	s, repo, _ = newService()
	repo.errs = map[string]error{"CardHasActiveRun": errRepo}
	if _, err := s.MoveCard(ctx, id, id, 0); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}

func TestEntity(t *testing.T) {
	s, repo, _ := newService()
	for typ, want := range map[string]string{
		"card.moved":         "GetCard",
		"flow_run.updated":   "GetFlowRun",
		"card_event.created": "GetCardEvent",
	} {
		repo.calls = nil
		if _, err := s.Entity(ctx, typ, id); err != nil || len(repo.calls) != 1 || repo.calls[0] != want {
			t.Fatalf("%s: calls %v, %v", typ, repo.calls, err)
		}
	}
	if _, err := s.Entity(ctx, "lane.created", id); err == nil {
		t.Fatal("expected an error for an entity other processes do not change")
	}
}
