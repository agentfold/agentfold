package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// fixture is a board with two lanes, a card type, a project and a card in the
// first lane that uses the project.
type fixture struct {
	lanes    []domain.Lane
	cardType domain.CardType
	project  domain.Project
	card     domain.Card
}

func newFixture(t *testing.T, s *Store) fixture {
	t.Helper()
	ctx := context.Background()
	f := fixture{lanes: createLanes(t, s, "Todo", "Done")}
	var err error
	if f.cardType, err = s.CreateCardType(ctx, domain.CardTypeInput{Name: "task"}); err != nil {
		t.Fatal(err)
	}
	if f.project, err = s.CreateProject(ctx, domain.ProjectInput{Name: "repo", LocalPath: "/src/repo"}); err != nil {
		t.Fatal(err)
	}
	f.card = f.newCard(t, s, f.lanes[0].ID, "first", &f.project.ID)
	return f
}

func (f fixture) newCard(t *testing.T, s *Store, laneID uuid.UUID, title string, projectID *uuid.UUID) domain.Card {
	t.Helper()
	card, err := s.CreateCard(context.Background(), domain.NewCard{
		CardInput: domain.CardInput{Title: title, CardTypeID: f.cardType.ID, ProjectID: projectID},
		LaneID:    laneID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return card
}

// laneTitles returns the card titles of a lane in order, checking positions.
func laneTitles(t *testing.T, s *Store, laneID uuid.UUID) []string {
	t.Helper()
	cards, err := s.ListCards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, c := range cards {
		if c.LaneID != laneID {
			continue
		}
		if c.Position != len(titles) {
			t.Fatalf("card %q at position %d, want %d", c.Title, c.Position, len(titles))
		}
		titles = append(titles, c.Title)
	}
	return titles
}

func eventTypes(t *testing.T, s *Store, cardID uuid.UUID) []string {
	t.Helper()
	events, err := s.ListCardEvents(context.Background(), cardID)
	if err != nil {
		t.Fatal(err)
	}
	types := make([]string, len(events))
	for i, e := range events {
		if e.CardID != cardID {
			t.Fatalf("event of another card: %+v", e)
		}
		types[i] = string(e.Type)
	}
	return types
}

func TestCardCRUD(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f := newFixture(t, s)

	child, err := s.CreateCard(ctx, domain.NewCard{
		CardInput:    domain.CardInput{Title: "child", Description: "d", CardTypeID: f.cardType.ID},
		LaneID:       f.lanes[0].ID,
		ParentCardID: &f.card.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.Position != 1 || child.ProjectID != nil || *child.ParentCardID != f.card.ID {
		t.Fatalf("unexpected child %+v", child)
	}

	children, err := s.ListChildCards(ctx, f.card.ID)
	if err != nil || len(children) != 1 || children[0].ID != child.ID {
		t.Fatalf("ListChildCards = %+v, %v", children, err)
	}

	updated, err := s.UpdateCard(ctx, child.ID, domain.CardInput{
		Title: "renamed", CardTypeID: f.cardType.ID, ProjectID: &f.project.ID,
	})
	if err != nil || updated.Title != "renamed" || *updated.ProjectID != f.project.ID || updated.Description != "" {
		t.Fatalf("UpdateCard = %+v, %v", updated, err)
	}
	got, err := s.GetCard(ctx, child.ID)
	if err != nil || got.Title != "renamed" {
		t.Fatalf("GetCard = %+v, %v", got, err)
	}
	assertNames(t, eventTypes(t, s, child.ID), "card.created", "card.updated")

	events, err := s.ListCardEvents(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	var created cardCreatedPayload
	if err := json.Unmarshal(events[0].Payload, &created); err != nil || created.LaneID != f.lanes[0].ID {
		t.Fatalf("created payload %s: %v", events[0].Payload, err)
	}

	if err := s.DeleteCard(ctx, f.card.ID); !errors.Is(err, domain.ErrInUse) {
		t.Fatalf("DeleteCard with children: %v", err)
	}
	if err := s.DeleteCard(ctx, child.ID); err != nil {
		t.Fatal(err)
	}
	f.newCard(t, s, f.lanes[0].ID, "second", nil)
	if err := s.DeleteCard(ctx, f.card.ID); err != nil {
		t.Fatal(err)
	}
	assertNames(t, laneTitles(t, s, f.lanes[0].ID), "second")
}

func TestCardNotFound(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f := newFixture(t, s)
	id := uuid.New()
	if _, err := s.GetCard(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetCard: %v", err)
	}
	if _, err := s.UpdateCard(ctx, id, domain.CardInput{Title: "x", CardTypeID: f.cardType.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UpdateCard: %v", err)
	}
	if _, err := s.MoveCard(ctx, id, f.lanes[0].ID, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("MoveCard: %v", err)
	}
	if err := s.DeleteCard(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("DeleteCard: %v", err)
	}
}

func TestCardInvalidReferences(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f := newFixture(t, s)
	missing := uuid.New()
	base := domain.CardInput{Title: "x", CardTypeID: f.cardType.ID}

	cases := map[string]domain.NewCard{
		"lane":    {CardInput: base, LaneID: missing},
		"type":    {CardInput: domain.CardInput{Title: "x", CardTypeID: missing}, LaneID: f.lanes[0].ID},
		"project": {CardInput: domain.CardInput{Title: "x", CardTypeID: f.cardType.ID, ProjectID: &missing}, LaneID: f.lanes[0].ID},
		"parent":  {CardInput: base, LaneID: f.lanes[0].ID, ParentCardID: &missing},
	}
	for name, in := range cases {
		if _, err := s.CreateCard(ctx, in); !errors.Is(err, domain.ErrInvalidReference) {
			t.Errorf("CreateCard with missing %s: %v", name, err)
		}
	}
	if _, err := s.UpdateCard(ctx, f.card.ID, domain.CardInput{Title: "x", CardTypeID: missing}); !errors.Is(err, domain.ErrInvalidReference) {
		t.Errorf("UpdateCard with missing type: %v", err)
	}
	if _, err := s.MoveCard(ctx, f.card.ID, missing, 0); !errors.Is(err, domain.ErrInvalidReference) {
		t.Errorf("MoveCard to missing lane: %v", err)
	}
}

func TestMoveCardWithinLane(t *testing.T) {
	tests := []struct {
		name     string
		from, to int
		want     []string
	}{
		{"up", 2, 0, []string{"c", "a", "b"}},
		{"down", 0, 1, []string{"b", "a", "c"}},
		{"past the end", 0, 50, []string{"b", "c", "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore(t)
			f := newFixture(t, s)
			lane := f.lanes[1].ID
			cards := []domain.Card{
				f.newCard(t, s, lane, "a", nil),
				f.newCard(t, s, lane, "b", nil),
				f.newCard(t, s, lane, "c", nil),
			}
			moved, err := s.MoveCard(context.Background(), cards[tt.from].ID, lane, tt.to)
			if err != nil {
				t.Fatal(err)
			}
			assertNames(t, laneTitles(t, s, lane), tt.want...)
			if tt.want[moved.Position] != moved.Title {
				t.Fatalf("moved card reports position %d", moved.Position)
			}
			assertNames(t, eventTypes(t, s, moved.ID), "card.created", "card.moved")
		})
	}
}

func TestMoveCardToSamePlaceRecordsNothing(t *testing.T) {
	s := newStore(t)
	f := newFixture(t, s)
	moved, err := s.MoveCard(context.Background(), f.card.ID, f.lanes[0].ID, 7)
	if err != nil || moved.Position != 0 {
		t.Fatalf("MoveCard = %+v, %v", moved, err)
	}
	assertNames(t, eventTypes(t, s, f.card.ID), "card.created")
}

func TestMoveCardAcrossLanes(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f := newFixture(t, s)
	todo, done := f.lanes[0].ID, f.lanes[1].ID
	f.newCard(t, s, todo, "second", nil)
	f.newCard(t, s, done, "x", nil)
	f.newCard(t, s, done, "y", nil)

	moved, err := s.MoveCard(ctx, f.card.ID, done, 1)
	if err != nil {
		t.Fatal(err)
	}
	if moved.LaneID != done || moved.Position != 1 {
		t.Fatalf("unexpected card %+v", moved)
	}
	assertNames(t, laneTitles(t, s, todo), "second")
	assertNames(t, laneTitles(t, s, done), "x", "first", "y")

	events, err := s.ListCardEvents(ctx, f.card.ID)
	if err != nil {
		t.Fatal(err)
	}
	var payload cardMovedPayload
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload != (cardMovedPayload{FromLaneID: todo, ToLaneID: done, Position: 1}) {
		t.Fatalf("moved payload %+v", payload)
	}
}

func TestCardErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("list", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.ListCards(cancelled()); err == nil {
			t.Fatal("ListCards: expected error")
		}
		if _, err := s.ListChildCards(cancelled(), uuid.New()); err == nil {
			t.Fatal("ListChildCards: expected error")
		}
		if _, err := s.ListCardEvents(cancelled(), uuid.New()); err == nil {
			t.Fatal("ListCardEvents: expected error")
		}
	})

	createFailures := []struct{ method string }{{"CountCardsInLane"}, {"InsertCardEvent"}}
	for _, tt := range createFailures {
		t.Run("create "+tt.method, func(t *testing.T) {
			s := newStore(t)
			f := newFixture(t, s)
			failOn(s, tt.method, 1)
			_, err := s.CreateCard(ctx, domain.NewCard{
				CardInput: domain.CardInput{Title: "x", CardTypeID: f.cardType.ID},
				LaneID:    f.lanes[0].ID,
			})
			if !errors.Is(err, errInjected) {
				t.Fatalf("got %v", err)
			}
		})
	}

	t.Run("update event", func(t *testing.T) {
		s := newStore(t)
		f := newFixture(t, s)
		failOn(s, "InsertCardEvent", 1)
		_, err := s.UpdateCard(ctx, f.card.ID, domain.CardInput{Title: "x", CardTypeID: f.cardType.ID})
		if !errors.Is(err, errInjected) {
			t.Fatalf("got %v", err)
		}
	})

	moveFailures := []struct {
		name   string
		method string
		n      int
		lane   int
	}{
		{"count", "CountCardsInLane", 1, 1},
		{"shift within lane", "ShiftCards", 1, 0},
		{"shift source lane", "ShiftCards", 1, 1},
		{"shift target lane", "ShiftCards", 2, 1},
		{"place", "PlaceCard", 1, 1},
		{"event", "InsertCardEvent", 1, 1},
	}
	for _, tt := range moveFailures {
		t.Run("move "+tt.name, func(t *testing.T) {
			s := newStore(t)
			f := newFixture(t, s)
			f.newCard(t, s, f.lanes[0].ID, "second", nil)
			failOn(s, tt.method, tt.n)
			if _, err := s.MoveCard(ctx, f.card.ID, f.lanes[tt.lane].ID, 1); !errors.Is(err, errInjected) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
