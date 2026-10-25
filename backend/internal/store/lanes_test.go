package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func createLanes(t *testing.T, s *Store, names ...string) []domain.Lane {
	t.Helper()
	lanes := make([]domain.Lane, len(names))
	for i, name := range names {
		lane, err := s.CreateLane(context.Background(), domain.LaneInput{Name: name, Color: "#112233"})
		if err != nil {
			t.Fatal(err)
		}
		lanes[i] = lane
	}
	return lanes
}

func laneNames(t *testing.T, s *Store) []string {
	t.Helper()
	lanes, err := s.ListLanes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(lanes))
	for i, lane := range lanes {
		if lane.Position != i {
			t.Fatalf("lane %q at position %d, want %d", lane.Name, lane.Position, i)
		}
		names[i] = lane.Name
	}
	return names
}

func assertNames(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestLaneCRUD(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	lanes := createLanes(t, s, "Todo", "Doing")
	if lanes[1].Position != 1 || lanes[1].Color != "#112233" || lanes[1].ID == uuid.Nil {
		t.Fatalf("unexpected lane %+v", lanes[1])
	}

	got, err := s.GetLane(ctx, lanes[0].ID)
	if err != nil || got.Name != "Todo" {
		t.Fatalf("GetLane = %+v, %v", got, err)
	}
	updated, err := s.UpdateLane(ctx, lanes[0].ID, domain.LaneInput{Name: "Backlog", Color: ""})
	if err != nil || updated.Name != "Backlog" || updated.Color != "" {
		t.Fatalf("UpdateLane = %+v, %v", updated, err)
	}
	assertNames(t, laneNames(t, s), "Backlog", "Doing")

	if err := s.DeleteLane(ctx, lanes[0].ID); err != nil {
		t.Fatal(err)
	}
	assertNames(t, laneNames(t, s), "Doing")
}

func TestLaneNotFound(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	id := uuid.New()
	if _, err := s.GetLane(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetLane: %v", err)
	}
	if _, err := s.UpdateLane(ctx, id, domain.LaneInput{Name: "x"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UpdateLane: %v", err)
	}
	if _, err := s.MoveLane(ctx, id, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("MoveLane: %v", err)
	}
	if err := s.DeleteLane(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("DeleteLane: %v", err)
	}
}

func TestMoveLane(t *testing.T) {
	tests := []struct {
		name     string
		from, to int
		want     []string
	}{
		{"left", 2, 0, []string{"C", "A", "B", "D"}},
		{"right", 0, 2, []string{"B", "C", "A", "D"}},
		{"same place", 1, 1, []string{"A", "B", "C", "D"}},
		{"past the end", 1, 99, []string{"A", "C", "D", "B"}},
		{"negative", 3, -5, []string{"D", "A", "B", "C"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStore(t)
			lanes := createLanes(t, s, "A", "B", "C", "D")
			moved, err := s.MoveLane(context.Background(), lanes[tt.from].ID, tt.to)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want[moved.Position] != moved.Name {
				t.Fatalf("moved lane reports position %d", moved.Position)
			}
			assertNames(t, laneNames(t, s), tt.want...)
		})
	}
}

func TestDeleteLaneInUse(t *testing.T) {
	s := newStore(t)
	f := newFixture(t, s)
	if err := s.DeleteLane(context.Background(), f.lanes[0].ID); !errors.Is(err, domain.ErrInUse) {
		t.Fatalf("DeleteLane: %v", err)
	}
}

func TestLaneErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("list", func(t *testing.T) {
		if _, err := newStore(t).ListLanes(cancelled()); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("create count", func(t *testing.T) {
		s := newStore(t)
		failOn(s, "CountLanes", 1)
		if _, err := s.CreateLane(ctx, domain.LaneInput{Name: "A"}); !errors.Is(err, errInjected) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("create invalid", func(t *testing.T) {
		if _, err := newStore(t).CreateLane(ctx, domain.LaneInput{Name: ""}); err == nil {
			t.Fatal("expected check violation")
		}
	})
	for _, method := range []string{"CountLanes", "ShiftLanes", "SetLanePosition"} {
		t.Run("move "+method, func(t *testing.T) {
			s := newStore(t)
			lanes := createLanes(t, s, "A", "B")
			failOn(s, method, 1)
			if _, err := s.MoveLane(ctx, lanes[0].ID, 1); !errors.Is(err, errInjected) {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("delete", func(t *testing.T) {
		s := newStore(t)
		lanes := createLanes(t, s, "A")
		failOn(s, "DeleteLane", 1)
		if err := s.DeleteLane(ctx, lanes[0].ID); !errors.Is(err, errInjected) {
			t.Fatalf("got %v", err)
		}
	})
}
