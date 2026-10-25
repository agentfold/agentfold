package service

import (
	"errors"
	"slices"
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

func TestCardRuns(t *testing.T) {
	s, repo, _ := newService()
	repo.flowRun = domain.FlowRun{ID: id}
	repo.nodeRun = domain.NodeRun{NodeKey: "dev"}
	runs, err := s.ListCardRuns(ctx, id)
	if err != nil || len(runs) != 1 || runs[0].Nodes[0].Type != domain.NodeAgent {
		t.Fatalf("ListCardRuns = %+v, %v", runs, err)
	}
	for _, method := range []string{"GetCard", "ListCardFlowRuns", "ListNodeRuns", "GetFlowVersion"} {
		s, repo, _ := newService()
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.ListCardRuns(ctx, id); !errors.Is(err, errRepo) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
	s, repo, _ = newService()
	repo.errs = map[string]error{"ListNodeRuns": errRepo}
	if _, err := s.GetRunDetail(ctx, id); !errors.Is(err, errRepo) {
		t.Fatalf("GetRunDetail: got %v", err)
	}
}

func TestNodeOutput(t *testing.T) {
	s, repo, _ := newService()
	if out, err := s.ListNodeOutput(ctx, id); err != nil || len(out) != 1 {
		t.Fatalf("ListNodeOutput = %+v, %v", out, err)
	}
	repo.errs = map[string]error{"GetNodeRun": errRepo}
	if _, err := s.ListNodeOutput(ctx, id); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}

func TestCancelFlowRun(t *testing.T) {
	s, repo, pub := newService()
	repo.flowRun = domain.FlowRun{ID: id, CardID: id, FlowName: "dev"}
	run, err := s.CancelFlowRun(ctx, id)
	if err != nil || run.Status != domain.RunCanceled || run.FlowName != "dev" {
		t.Fatalf("CancelFlowRun = %+v, %v", run, err)
	}
	if got := eventTypes(pub); !slices.Equal(got, []string{"flow_run.updated", "card_event.created"}) {
		t.Fatalf("events %q", got)
	}
	if p := repo.ins["AddCardEvent:flow.finished"].(flowFinishedPayload); p.Status != domain.RunCanceled || p.Error != "cancelado" {
		t.Fatalf("timeline %+v", p)
	}
	for _, method := range []string{"GetFlowRun", "CancelFlowRun"} {
		s, repo, pub := newService()
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.CancelFlowRun(ctx, id); !errors.Is(err, errRepo) || len(pub.events) != 0 {
			t.Fatalf("%s: got %v", method, err)
		}
	}
}

func TestEntityOfWorkerRuns(t *testing.T) {
	s, repo, _ := newService()
	if _, err := s.Entity(ctx, "node_run.updated", id); err != nil || repo.calls[0] != "GetNodeRun" {
		t.Fatalf("node_run: %v, %v", repo.calls, err)
	}
	if v, err := s.Entity(ctx, "node_output.created", id); err != nil || v != nil {
		t.Fatalf("node_output: %v, %v", v, err)
	}
}
