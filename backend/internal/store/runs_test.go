package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// publishedRun creates a card, a published flow version and a run of it.
func publishedRun(t *testing.T, s *Store) (domain.FlowRun, domain.FlowVersion) {
	t.Helper()
	ctx := context.Background()
	f := newFixture(t, s)
	flow, err := s.CreateFlow(ctx, "dev", graphWith("a"))
	if err != nil {
		t.Fatal(err)
	}
	version, err := s.PublishFlow(ctx, flow.ID, graphWith("a"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateFlowRun(ctx, version.ID, f.card.ID, domain.EventCardCreated)
	if err != nil {
		t.Fatal(err)
	}
	return run, version
}

func TestRuns(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, version := publishedRun(t, s)
	if run.Status != domain.RunRunning || run.FinishedAt != nil {
		t.Fatalf("unexpected run %+v", run)
	}
	if got, err := s.GetFlowRun(ctx, run.ID); err != nil || got.ID != run.ID {
		t.Fatalf("GetFlowRun = %+v, %v", got, err)
	}
	if got, err := s.GetFlowVersion(ctx, version.ID); err != nil || got.Version != 1 || len(got.Graph.Nodes) != 1 {
		t.Fatalf("GetFlowVersion = %+v, %v", got, err)
	}

	n, err := s.CreateNodeRun(ctx, run.ID, "dev", 1, nil)
	if err != nil || n.Status != domain.NodeQueued || string(n.Input) != "{}" {
		t.Fatalf("CreateNodeRun = %+v, %v", n, err)
	}
	session := uuid.New()
	if n, err = s.StartNodeRun(ctx, n.ID, session); err != nil || n.Status != domain.NodeRunning || *n.SessionID != session || n.StartedAt == nil {
		t.Fatalf("StartNodeRun = %+v, %v", n, err)
	}
	result := domain.NodeResult{
		Status: domain.NodeDone, Output: json.RawMessage(`{"summary":"ok"}`), OutputHandle: "done",
		Usage: domain.Usage{TokensIn: 10, TokensOut: 20, CacheRead: 3, CacheWrite: 4, CostUSD: 0.25, Turns: 2, DurationMS: 900},
	}
	if n, err = s.FinishNodeRun(ctx, n.ID, result); err != nil {
		t.Fatal(err)
	}
	if n.Status != domain.NodeDone || n.OutputHandle != "done" || n.Error != "" || n.Usage != result.Usage || n.FinishedAt == nil {
		t.Fatalf("FinishNodeRun = %+v", n)
	}
	failed, err := s.FinishNodeRun(ctx, n.ID, domain.NodeResult{Status: domain.NodeFailed, Error: "boom"})
	if err != nil || failed.Error != "boom" || failed.OutputHandle != "" {
		t.Fatalf("FinishNodeRun failed = %+v, %v", failed, err)
	}
	if got, err := s.GetNodeRun(ctx, n.ID); err != nil || got.Error != "boom" {
		t.Fatalf("GetNodeRun = %+v, %v", got, err)
	}
}

func TestAgentSessions(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, _ := publishedRun(t, s)
	profile, err := s.CreateAgentProfile(ctx, domain.AgentClaude, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentSession(ctx, run.ID, "dev"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AgentSession before: %v", err)
	}
	session := uuid.New()
	if err := s.CreateAgentSession(ctx, run.ID, "dev", profile.ID, session); err != nil {
		t.Fatal(err)
	}
	if got, err := s.AgentSession(ctx, run.ID, "dev"); err != nil || got != session {
		t.Fatalf("AgentSession = %v, %v", got, err)
	}
	if err := s.CreateAgentSession(ctx, run.ID, "dev", profile.ID, session); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate session: %v", err)
	}
	if err := s.DeleteAgentProfile(ctx, profile.ID); !errors.Is(err, domain.ErrInUse) {
		t.Fatalf("delete profile in use: %v", err)
	}
}

func TestRunErrors(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.GetFlowVersion(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetFlowVersion: %v", err)
	}
}
