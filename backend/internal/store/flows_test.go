package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func graphWith(ids ...string) domain.Graph {
	g := domain.Graph{Nodes: []domain.Node{}, Edges: []domain.Edge{}}
	for _, id := range ids {
		g.Nodes = append(g.Nodes, domain.Node{ID: id, Type: domain.NodeEnd, Config: json.RawMessage(`{}`)})
	}
	return g
}

func nodeIDs(g domain.Graph) []string {
	ids := make([]string, len(g.Nodes))
	for i, n := range g.Nodes {
		ids[i] = n.ID
	}
	return ids
}

func TestFlowCRUD(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f, err := s.CreateFlow(ctx, "dev", graphWith("a"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "dev" || f.Enabled || f.LatestVersion != 0 || len(f.Draft.Nodes) != 1 {
		t.Fatalf("unexpected flow %+v", f)
	}
	if _, err := s.CreateFlow(ctx, "alpha", graphWith()); err != nil {
		t.Fatal(err)
	}

	f, err = s.UpdateFlow(ctx, f.ID, domain.FlowInput{Name: "develop", Enabled: true})
	if err != nil || f.Name != "develop" || !f.Enabled {
		t.Fatalf("UpdateFlow = %+v, %v", f, err)
	}
	f, err = s.SaveFlowDraft(ctx, f.ID, graphWith("a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	assertNames(t, nodeIDs(f.Draft), "a", "b")

	list, err := s.ListFlows(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "alpha" {
		t.Fatalf("ListFlows = %+v, %v", list, err)
	}
	if err := s.DeleteFlow(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFlow(ctx, f.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetFlow after delete: %v", err)
	}
}

func TestPublishFlow(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	f, err := s.CreateFlow(ctx, "dev", graphWith())
	if err != nil {
		t.Fatal(err)
	}
	for i, ids := range [][]string{{"v1"}, {"v2a", "v2b"}} {
		v, err := s.PublishFlow(ctx, f.ID, graphWith(ids...))
		if err != nil {
			t.Fatal(err)
		}
		if v.Version != i+1 || v.FlowID != f.ID {
			t.Fatalf("unexpected version %+v", v)
		}
	}
	f, err = s.GetFlow(ctx, f.ID)
	if err != nil || f.LatestVersion != 2 {
		t.Fatalf("GetFlow = %+v, %v", f, err)
	}
	versions, err := s.ListFlowVersions(ctx, f.ID)
	if err != nil || len(versions) != 2 || versions[0].Version != 2 {
		t.Fatalf("ListFlowVersions = %+v, %v", versions, err)
	}
	assertNames(t, nodeIDs(versions[0].Graph), "v2a", "v2b")
}

func TestFlowErrors(t *testing.T) {
	ctx := context.Background()
	missing := uuid.New()

	t.Run("not found", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.UpdateFlow(ctx, missing, domain.FlowInput{Name: "x"}); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("UpdateFlow: %v", err)
		}
		if _, err := s.SaveFlowDraft(ctx, missing, graphWith()); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("SaveFlowDraft: %v", err)
		}
		if _, err := s.PublishFlow(ctx, missing, graphWith()); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("PublishFlow: %v", err)
		}
		if err := s.DeleteFlow(ctx, missing); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("DeleteFlow: %v", err)
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.CreateFlow(ctx, "dev", graphWith()); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateFlow(ctx, "dev", graphWith()); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("CreateFlow: %v", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.ListFlows(cancelled()); err == nil {
			t.Error("ListFlows: expected error")
		}
		if _, err := s.ListFlowVersions(cancelled(), missing); err == nil {
			t.Error("ListFlowVersions: expected error")
		}
	})

	t.Run("corrupted draft", func(t *testing.T) {
		s := newStore(t)
		var id uuid.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO flows (name, draft) VALUES ('bad', '"x"') RETURNING id`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := s.GetFlow(ctx, id); err == nil {
			t.Error("GetFlow: expected error")
		}
		if _, err := s.ListFlows(ctx); err == nil {
			t.Error("ListFlows: expected error")
		}
	})

	for _, method := range []string{"NextFlowVersion", "InsertFlowVersion"} {
		t.Run("publish "+method, func(t *testing.T) {
			s := newStore(t)
			f, err := s.CreateFlow(ctx, "dev", graphWith())
			if err != nil {
				t.Fatal(err)
			}
			failOn(s, method, 1)
			if _, err := s.PublishFlow(ctx, f.ID, graphWith()); !errors.Is(err, errInjected) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
