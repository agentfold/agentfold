package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func TestCardTypeCRUD(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	bug, err := s.CreateCardType(ctx, domain.CardTypeInput{Name: "bug", Color: "#ff0000"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCardType(ctx, domain.CardTypeInput{Name: "feature"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListCardTypes(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "bug" {
		t.Fatalf("ListCardTypes = %+v, %v", list, err)
	}
	got, err := s.GetCardType(ctx, bug.ID)
	if err != nil || got.Color != "#ff0000" {
		t.Fatalf("GetCardType = %+v, %v", got, err)
	}
	updated, err := s.UpdateCardType(ctx, bug.ID, domain.CardTypeInput{Name: "defect", Color: "#00ff00"})
	if err != nil || updated.Name != "defect" || updated.Color != "#00ff00" {
		t.Fatalf("UpdateCardType = %+v, %v", updated, err)
	}
	if err := s.DeleteCardType(ctx, bug.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCardType(ctx, bug.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetCardType after delete: %v", err)
	}
}

func TestCardTypeErrors(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.ListCardTypes(cancelled()); err == nil {
		t.Fatal("ListCardTypes: expected error")
	}
	f := newFixture(t, s)
	if _, err := s.CreateCardType(ctx, domain.CardTypeInput{Name: f.cardType.Name}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}
	if err := s.DeleteCardType(ctx, f.cardType.ID); !errors.Is(err, domain.ErrInUse) {
		t.Fatalf("DeleteCardType in use: %v", err)
	}
}

func TestCardTypeInitialLane(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	lanes := createLanes(t, s, "Todo", "Doing")
	task, err := s.CreateCardType(ctx, domain.CardTypeInput{Name: "task", InitialLaneID: &lanes[0].ID})
	if err != nil || *task.InitialLaneID != lanes[0].ID {
		t.Fatalf("CreateCardType = %+v, %v", task, err)
	}
	task, err = s.UpdateCardType(ctx, task.ID, domain.CardTypeInput{Name: "task", InitialLaneID: &lanes[1].ID})
	if err != nil || *task.InitialLaneID != lanes[1].ID {
		t.Fatalf("UpdateCardType = %+v, %v", task, err)
	}
	// Deleting the lane leaves the type without an initial lane.
	if err := s.DeleteLane(ctx, lanes[1].ID); err != nil {
		t.Fatal(err)
	}
	if task, _ = s.GetCardType(ctx, task.ID); task.InitialLaneID != nil {
		t.Fatalf("type %+v", task)
	}
	gone := uuid.New()
	if _, err := s.CreateCardType(ctx, domain.CardTypeInput{Name: "bug", InitialLaneID: &gone}); !errors.Is(err, domain.ErrInvalidReference) {
		t.Fatalf("unknown lane: %v", err)
	}
}
