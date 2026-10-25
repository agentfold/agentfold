package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"orquestrator/internal/domain"
)

// publishable is trigger -> end.
func publishable() domain.Graph {
	return domain.Graph{
		Nodes: []domain.Node{
			{ID: "t", Type: domain.NodeTrigger, Config: json.RawMessage(`{"event":"card_created"}`)},
			{ID: "e", Type: domain.NodeEnd},
		},
		Edges: []domain.Edge{{ID: "x", Source: "t", SourceHandle: "next", Target: "e"}},
	}
}

func eventTypes(p *fakePub) []string {
	out := make([]string, len(p.events))
	for i, e := range p.events {
		out[i] = e.Type
	}
	return out
}

func TestFlowReads(t *testing.T) {
	s, repo, _ := newService()
	repo.flow = domain.Flow{ID: id, Draft: domain.NewFlowGraph()}

	detail, err := s.GetFlow(ctx, id)
	if err != nil || detail.ID != id || len(detail.Problems) != 1 {
		t.Fatalf("GetFlow = %+v, %v", detail, err)
	}
	if _, err := s.ListFlows(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListFlowVersions(ctx, id); err != nil || repo.calls[len(repo.calls)-1] != "ListFlowVersions" {
		t.Fatalf("ListFlowVersions: %v, calls %v", err, repo.calls)
	}

	repo.errs = map[string]error{"GetFlow": domain.ErrNotFound}
	if _, err := s.ListFlowVersions(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ListFlowVersions of a missing flow: %v", err)
	}
}

func TestFlowDetailFailures(t *testing.T) {
	for _, method := range []string{"GetFlow", "ListLanes", "ListCardTypes"} {
		t.Run(method, func(t *testing.T) {
			s, repo, _ := newService()
			repo.errs = map[string]error{method: errRepo}
			if _, err := s.GetFlow(ctx, id); !errors.Is(err, errRepo) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestCreateFlow(t *testing.T) {
	s, repo, pub := newService()
	if _, err := s.CreateFlow(ctx, " "); err == nil || len(repo.calls) != 0 {
		t.Fatalf("blank name: %v, calls %v", err, repo.calls)
	}
	if _, err := s.CreateFlow(ctx, " dev "); err != nil {
		t.Fatal(err)
	}
	if repo.calls[0] != "CreateFlow" || repo.ins["CreateFlow"] != "dev 1" {
		t.Fatalf("calls %v, ins %v", repo.calls, repo.ins)
	}
	if !reflect.DeepEqual(eventTypes(pub), []string{"flow.created"}) {
		t.Fatalf("events %v", eventTypes(pub))
	}
}

func TestUpdateFlow(t *testing.T) {
	t.Run("invalid name", func(t *testing.T) {
		s, repo, _ := newService()
		if _, err := s.UpdateFlow(ctx, id, domain.FlowInput{}); err == nil || len(repo.calls) != 0 {
			t.Fatalf("got %v, calls %v", err, repo.calls)
		}
	})
	t.Run("disable", func(t *testing.T) {
		s, repo, pub := newService()
		if _, err := s.UpdateFlow(ctx, id, domain.FlowInput{Name: "dev"}); err != nil {
			t.Fatal(err)
		}
		if repo.calls[0] != "UpdateFlow" || !reflect.DeepEqual(eventTypes(pub), []string{"flow.updated"}) {
			t.Fatalf("calls %v, events %v", repo.calls, eventTypes(pub))
		}
	})
	t.Run("enable unpublished", func(t *testing.T) {
		s, _, _ := newService()
		var verr *domain.ValidationError
		if _, err := s.UpdateFlow(ctx, id, domain.FlowInput{Name: "dev", Enabled: true}); !errors.As(err, &verr) || verr.Field != "enabled" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("enable missing", func(t *testing.T) {
		s, repo, _ := newService()
		repo.errs = map[string]error{"GetFlow": domain.ErrNotFound}
		if _, err := s.UpdateFlow(ctx, id, domain.FlowInput{Name: "dev", Enabled: true}); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("enable published", func(t *testing.T) {
		s, repo, _ := newService()
		repo.flow.LatestVersion = 1
		detail, err := s.UpdateFlow(ctx, id, domain.FlowInput{Name: "dev", Enabled: true})
		if err != nil || detail.LatestVersion != 1 {
			t.Fatalf("got %+v, %v", detail, err)
		}
	})
}

func TestSaveFlowDraft(t *testing.T) {
	s, repo, pub := newService()
	if _, err := s.SaveFlowDraft(ctx, id, domain.Graph{}); err != nil {
		t.Fatal(err)
	}
	if repo.ins["SaveFlowDraft"] != true {
		t.Fatal("the draft was not normalized")
	}
	if !reflect.DeepEqual(eventTypes(pub), []string{"flow.updated"}) {
		t.Fatalf("events %v", eventTypes(pub))
	}
}

func TestPublishFlow(t *testing.T) {
	t.Run("missing flow", func(t *testing.T) {
		s, repo, _ := newService()
		repo.errs = map[string]error{"GetFlow": domain.ErrNotFound}
		if _, err := s.PublishFlow(ctx, id); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("draft with problems", func(t *testing.T) {
		s, repo, pub := newService()
		repo.flow.Draft = domain.NewFlowGraph()
		var gerr *domain.GraphError
		if _, err := s.PublishFlow(ctx, id); !errors.As(err, &gerr) || len(gerr.Problems) != 1 {
			t.Fatalf("got %v", err)
		}
		if len(pub.events) != 0 {
			t.Fatalf("events %v", eventTypes(pub))
		}
	})
	t.Run("store failure", func(t *testing.T) {
		s, repo, pub := newService()
		repo.flow.Draft = publishable()
		repo.errs = map[string]error{"PublishFlow": errRepo}
		if _, err := s.PublishFlow(ctx, id); !errors.Is(err, errRepo) || len(pub.events) != 0 {
			t.Fatalf("got %v, events %v", err, eventTypes(pub))
		}
	})
	t.Run("published", func(t *testing.T) {
		s, repo, pub := newService()
		repo.flow.Draft = publishable()
		version, err := s.PublishFlow(ctx, id)
		if err != nil || version.Version != 1 || repo.ins["PublishFlow"] != 2 {
			t.Fatalf("got %+v, %v, ins %v", version, err, repo.ins)
		}
		if !reflect.DeepEqual(eventTypes(pub), []string{"flow.published"}) || pub.events[0].ID != id {
			t.Fatalf("events %+v", pub.events)
		}
	})
}

func TestDeleteFlow(t *testing.T) {
	s, _, pub := newService()
	if err := s.DeleteFlow(ctx, id); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(eventTypes(pub), []string{"flow.deleted"}) {
		t.Fatalf("events %v", eventTypes(pub))
	}
}
