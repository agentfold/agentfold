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
	run, started, err := s.StartFlowRun(ctx, version, f.card.ID, domain.EventCardCreated)
	if err != nil || !started {
		t.Fatal(started, err)
	}
	return run, version
}

// publish creates an enabled flow with a published version.
func publish(t *testing.T, s *Store, name string) domain.FlowVersion {
	t.Helper()
	ctx := context.Background()
	flow, err := s.CreateFlow(ctx, name, graphWith("a"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateFlow(ctx, flow.ID, domain.FlowInput{Name: name, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	version, err := s.PublishFlow(ctx, flow.ID, graphWith("a"))
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func TestFlowRunLifecycle(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, version := publishedRun(t, s)
	if run.FlowID != version.FlowID || run.FlowVersionID != version.ID || run.Trigger != domain.EventCardCreated {
		t.Fatalf("unexpected run %+v", run)
	}
	if got, err := s.GetFlowRun(ctx, run.ID); err != nil || got.FlowName != "dev" {
		t.Fatalf("GetFlowRun = %+v, %v", got, err)
	}
	if _, started, err := s.StartFlowRun(ctx, version, run.CardID, domain.EventCardCreated); err != nil || started {
		t.Fatalf("second start while running = %v, %v", started, err)
	}
	if busy, err := s.CardHasActiveRun(ctx, run.CardID); err != nil || !busy {
		t.Fatalf("CardHasActiveRun = %v, %v", busy, err)
	}

	finished, err := s.FinishFlowRun(ctx, run.ID, domain.RunFailed, "boom")
	if err != nil || finished.Status != domain.RunFailed || finished.Error != "boom" || finished.FinishedAt == nil {
		t.Fatalf("FinishFlowRun = %+v, %v", finished, err)
	}
	if _, err := s.FinishFlowRun(ctx, run.ID, domain.RunDone, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("finishing twice: %v", err)
	}
	if busy, err := s.CardHasActiveRun(ctx, run.CardID); err != nil || busy {
		t.Fatalf("CardHasActiveRun after = %v, %v", busy, err)
	}
	again, started, err := s.StartFlowRun(ctx, version, run.CardID, domain.EventCardEnteredLane)
	if err != nil || !started || again.ID == run.ID {
		t.Fatalf("restart = %+v, %v, %v", again, started, err)
	}

	// An active run is the latest of its card, even if a later one ended.
	other := publish(t, s, "other")
	later, _, err := s.StartFlowRun(ctx, other, run.CardID, domain.EventCardCreated)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishFlowRun(ctx, later.ID, domain.RunDone, ""); err != nil {
		t.Fatal(err)
	}
	latest, err := s.ListLatestFlowRuns(ctx)
	if err != nil || len(latest) != 1 || latest[0].ID != again.ID || latest[0].FlowName != "dev" {
		t.Fatalf("ListLatestFlowRuns = %+v, %v", latest, err)
	}
	if _, err := s.FinishFlowRun(ctx, again.ID, domain.RunDone, ""); err != nil {
		t.Fatal(err)
	}
	if latest, err = s.ListLatestFlowRuns(ctx); err != nil || latest[0].ID != later.ID || latest[0].FlowName != "other" {
		t.Fatalf("ListLatestFlowRuns after = %+v, %v", latest, err)
	}

	if _, _, err := s.StartFlowRun(cancelled(), version, run.CardID, ""); err == nil {
		t.Error("StartFlowRun: expected error")
	}
	if _, err := s.CardHasActiveRun(cancelled(), run.CardID); err == nil {
		t.Error("CardHasActiveRun: expected error")
	}
	if _, err := s.ListLatestFlowRuns(cancelled()); err == nil {
		t.Error("ListLatestFlowRuns: expected error")
	}
}

func TestRunnableFlows(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.CreateFlow(ctx, "disabled", graphWith("a")); err != nil {
		t.Fatal(err)
	}
	first := publish(t, s, "on")
	second, err := s.PublishFlow(ctx, first.FlowID, graphWith("a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	flows, err := s.RunnableFlows(ctx)
	if err != nil || len(flows) != 1 || flows[0].Name != "on" || flows[0].Version.ID != second.ID || len(flows[0].Version.Graph.Nodes) != 2 {
		t.Fatalf("RunnableFlows = %+v, %v", flows, err)
	}

	if _, err := pool.Exec(ctx, "UPDATE flow_versions SET graph = '[]' WHERE id = $1", second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunnableFlows(ctx); err == nil {
		t.Error("RunnableFlows with a broken graph: expected error")
	}
	if _, err := s.RunnableFlows(cancelled()); err == nil {
		t.Error("RunnableFlows: expected error")
	}
}

func TestContinueNodeRun(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, _ := publishedRun(t, s)
	if n, err := s.CountNodeRuns(ctx, run.ID, "a"); err != nil || n != 0 {
		t.Fatalf("CountNodeRuns = %d, %v", n, err)
	}
	nr, err := s.CreateNodeRun(ctx, run.ID, "a", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.CountNodeRuns(ctx, run.ID, "a"); err != nil || n != 1 {
		t.Fatalf("CountNodeRuns = %d, %v", n, err)
	}
	if _, ok, err := s.ContinueNodeRun(ctx, nr.ID); err != nil || ok {
		t.Fatalf("continue a queued node = %v, %v", ok, err)
	}
	if _, err := s.FinishNodeRun(ctx, nr.ID, domain.NodeResult{Status: domain.NodeDone, OutputHandle: "done"}); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := s.ContinueNodeRun(ctx, nr.ID); err != nil || !ok || got.OutputHandle != "done" {
		t.Fatalf("continue = %+v, %v, %v", got, ok, err)
	}
	if _, ok, err := s.ContinueNodeRun(ctx, nr.ID); err != nil || ok {
		t.Fatalf("continue twice = %v, %v", ok, err)
	}
	if _, _, err := s.ContinueNodeRun(cancelled(), nr.ID); err == nil {
		t.Error("ContinueNodeRun: expected error")
	}
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
