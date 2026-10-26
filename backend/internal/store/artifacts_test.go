package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// collected is a finished run whose node run "dev" collected an artifact.
type collected struct {
	run      domain.FlowRun
	nr       domain.NodeRun
	artifact domain.Artifact
	session  uuid.UUID
}

func collect(t *testing.T, s *Store) collected {
	t.Helper()
	ctx := context.Background()
	run, nr := runningNode(t, s)
	session := uuid.New()
	if err := s.CreateAgentSession(ctx, run.ID, "dev", profileFor(t, s), session); err != nil {
		t.Fatal(err)
	}
	artifact, version, created, err := s.SaveArtifact(ctx, run.CardID, nr.ID, "Plano", "plan.md", "# v1")
	if err != nil || version != 1 || !created || artifact.Name != "Plano" || artifact.FrozenAt != nil {
		t.Fatalf("SaveArtifact = %+v %d %v, %v", artifact, version, created, err)
	}
	if _, err := s.FinishNodeRun(ctx, nr.ID, domain.NodeResult{Status: domain.NodeDone}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishFlowRun(ctx, run.ID, domain.RunDone, ""); err != nil {
		t.Fatal(err)
	}
	return collected{run: run, nr: nr, artifact: artifact, session: session}
}

func profileFor(t *testing.T, s *Store) uuid.UUID {
	t.Helper()
	p, err := s.CreateAgentProfile(context.Background(), domain.AgentClaude, "dev-"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func TestSaveArtifact(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := collect(t, s)
	if a, version, created, err := s.SaveArtifact(ctx, c.run.CardID, c.nr.ID, "Plano", "plan.md", "# v1"); err != nil || version != 1 || created || a.ID != c.artifact.ID {
		t.Fatalf("the same content adds no version: %+v %d %v, %v", a, version, created, err)
	}
	if _, version, created, err := s.SaveArtifact(ctx, c.run.CardID, c.nr.ID, "Plano", "plan.md", "# v2"); err != nil || version != 2 || !created {
		t.Fatalf("new content: %d %v, %v", version, created, err)
	}
	versions, err := s.ListArtifactVersions(ctx, c.artifact.ID)
	if err != nil || len(versions) != 2 || versions[0].Content != "# v1" || versions[1].Version != 2 || versions[1].Path != "plan.md" || versions[1].NodeRunID != c.nr.ID {
		t.Fatalf("versions %+v, %v", versions, err)
	}
	list, err := s.ListCardArtifacts(ctx, c.run.CardID)
	if err != nil || len(list) != 1 || list[0].ID != c.artifact.ID {
		t.Fatalf("ListCardArtifacts = %+v, %v", list, err)
	}
	if got, err := s.GetArtifact(ctx, c.artifact.ID); err != nil || got.CardID != c.run.CardID {
		t.Fatalf("GetArtifact = %+v, %v", got, err)
	}
	if _, err := s.GetArtifact(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing artifact: %v", err)
	}
	for _, method := range []string{"UpsertArtifact", "LatestArtifactVersion", "InsertArtifactVersion"} {
		failOn(s, method, 1)
		if _, _, _, err := s.SaveArtifact(ctx, c.run.CardID, c.nr.ID, "Plano", "plan.md", "# v3"); !errors.Is(err, errInjected) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
}

func TestRevision(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := collect(t, s)
	msg, run, err := s.StartRevision(ctx, c.artifact.ID, 1, "more detail")
	if err != nil || msg.Role != domain.RoleHuman || msg.Version != 1 || msg.Text != "more detail" || *msg.FlowRunID != run.ID {
		t.Fatalf("StartRevision = %+v, %v", msg, err)
	}
	if run.Trigger != domain.TriggerRevision || run.FlowVersionID != c.run.FlowVersionID || run.FlowName != "dev" || run.Status != domain.RunRunning {
		t.Fatalf("revision run %+v", run)
	}
	nodes, err := s.ListNodeRuns(ctx, run.ID)
	if err != nil || len(nodes) != 1 || nodes[0].NodeKey != "dev" || nodes[0].Status != domain.NodeQueued {
		t.Fatalf("revision node runs %+v, %v", nodes, err)
	}
	if session, err := s.AgentSession(ctx, run.ID, "dev"); err != nil || session != c.session {
		t.Fatalf("the revision resumes the session: %v, %v", session, err)
	}
	var prompt string
	if err := pool.QueryRow(ctx, `SELECT payload->>'prompt' FROM jobs WHERE kind = 'agent.run'`).Scan(&prompt); err != nil ||
		prompt != domain.RevisionPrompt("Plano", "plan.md", 1, "more detail") {
		t.Fatalf("revision prompt %q, %v", prompt, err)
	}
	if revising, err := s.ArtifactRevising(ctx, c.artifact.ID); err != nil || !revising {
		t.Fatalf("revising %v, %v", revising, err)
	}
	if _, _, err := s.StartRevision(ctx, c.artifact.ID, 1, "again"); !errors.Is(err, domain.ErrCardBusy) {
		t.Fatalf("revising a busy card: %v", err)
	}

	if _, _, created, err := s.SaveArtifact(ctx, c.run.CardID, nodes[0].ID, "Plano", "plan.md", "# v2"); err != nil || !created {
		t.Fatal(err)
	}
	reply, err := s.AddRevisionReply(ctx, run.ID, "done")
	if err != nil || reply.Role != domain.RoleAgent || reply.Version != 2 || reply.ArtifactID != c.artifact.ID {
		t.Fatalf("AddRevisionReply = %+v, %v", reply, err)
	}
	messages, err := s.ListArtifactMessages(ctx, c.artifact.ID)
	if err != nil || len(messages) != 2 || messages[0].ID != msg.ID || messages[1].ID != reply.ID {
		t.Fatalf("messages %+v, %v", messages, err)
	}
	if _, err := s.AddRevisionReply(ctx, c.run.ID, "no revision"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("reply to a run that revised nothing: %v", err)
	}
	if _, err := s.FinishFlowRun(ctx, run.ID, domain.RunDone, ""); err != nil {
		t.Fatal(err)
	}
	if revising, err := s.ArtifactRevising(ctx, c.artifact.ID); err != nil || revising {
		t.Fatalf("revising after the run %v, %v", revising, err)
	}

	if _, _, err := s.StartRevision(ctx, c.artifact.ID, 3, "x"); err == nil {
		t.Fatal("revising a version that does not exist")
	}
	if _, _, err := s.StartRevision(ctx, uuid.New(), 1, "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revising a missing artifact: %v", err)
	}
	for _, method := range []string{
		"LatestArtifactVersion", "GetNodeRun", "GetFlowRun", "CardHasActiveRun", "InsertFlowRun", "InsertNodeRun",
		"GetAgentSession", "InsertAgentSession", "InsertArtifactMessage", "InsertJob",
	} {
		failOn(s, method, 1)
		if _, _, err := s.StartRevision(ctx, c.artifact.ID, 1, "x"); !errors.Is(err, errInjected) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
	for _, method := range []string{"GetRevisionMessage", "LatestArtifactVersion", "InsertArtifactMessage"} {
		failOn(s, method, 1)
		if _, err := s.AddRevisionReply(ctx, run.ID, "x"); !errors.Is(err, errInjected) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
	for _, read := range []func(context.Context) error{
		func(ctx context.Context) error { _, err := s.ListCardArtifacts(ctx, c.run.CardID); return err },
		func(ctx context.Context) error { _, err := s.ListArtifactVersions(ctx, c.artifact.ID); return err },
		func(ctx context.Context) error { _, err := s.ListArtifactMessages(ctx, c.artifact.ID); return err },
	} {
		if err := read(cancelled()); err == nil {
			t.Error("expected an error")
		}
	}
}

func TestRevisionWithoutSession(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	run, nr := runningNode(t, s)
	artifact, _, _, err := s.SaveArtifact(ctx, run.CardID, nr.ID, "Plano", "plan.md", "# v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.StartRevision(ctx, artifact.ID, 1, "x"); !errors.Is(err, domain.ErrCardBusy) {
		t.Fatalf("revising while the run that collected it runs: %v", err)
	}
	if _, err := s.FinishFlowRun(ctx, run.ID, domain.RunDone, ""); err != nil {
		t.Fatal(err)
	}
	revision, revRun, err := s.StartRevision(ctx, artifact.ID, 1, "x")
	if err != nil || revision.FlowRunID == nil {
		t.Fatalf("StartRevision = %+v, %v", revision, err)
	}
	if _, err := s.AgentSession(ctx, revRun.ID, "dev"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no session to resume: %v", err)
	}
}

// TestFreezeArtifacts: moving the card to another lane freezes its
// artifacts, but not those the run moving it collected.
func TestFreezeArtifacts(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := collect(t, s)
	card, err := s.GetCard(ctx, c.run.CardID)
	if err != nil {
		t.Fatal(err)
	}
	lanes, err := s.ListLanes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	other := lanes[1].ID
	if other == card.LaneID {
		other = lanes[0].ID
	}
	if _, err := s.MoveCard(ctx, card.ID, card.LaneID, 0); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.GetArtifact(ctx, c.artifact.ID); a.FrozenAt != nil {
		t.Fatal("moving within the lane froze the artifact")
	}
	failOn(s, "FreezeCardArtifacts", 1)
	if _, err := s.MoveCard(ctx, card.ID, other, 0); !errors.Is(err, errInjected) {
		t.Fatalf("FreezeCardArtifacts: %v", err)
	}
	s = New(pool)

	second, err := s.CreateCard(ctx, domain.NewCard{CardInput: domain.CardInput{Title: "second", CardTypeID: card.CardTypeID}, LaneID: card.LaneID})
	if err != nil {
		t.Fatal(err)
	}
	version, err := s.GetFlowVersion(ctx, c.run.FlowVersionID)
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := s.StartFlowRun(ctx, version, second.ID, domain.EventCardCreated)
	if err != nil {
		t.Fatal(err)
	}
	nr, err := s.CreateNodeRun(ctx, run.ID, "dev", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	active, _, _, err := s.SaveArtifact(ctx, second.ID, nr.ID, "Outro", "other.md", "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveCard(ctx, card.ID, other, 0); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.GetArtifact(ctx, c.artifact.ID); a.FrozenAt == nil {
		t.Fatal("leaving the lane did not freeze the artifact")
	}
	if _, _, err := s.StartRevision(ctx, c.artifact.ID, 1, "x"); !errors.Is(err, domain.ErrArtifactFrozen) {
		t.Fatalf("revising a frozen artifact: %v", err)
	}
	if _, err := s.MoveCard(ctx, run.CardID, other, 0); err != nil {
		t.Fatal(err)
	}
	if a, _ := s.GetArtifact(ctx, active.ID); a.FrozenAt != nil {
		t.Fatal("the active run's artifact froze")
	}
	if a, _, _, err := s.SaveArtifact(ctx, c.run.CardID, c.nr.ID, "Plano", "plan.md", "# v1"); err != nil || a.FrozenAt != nil {
		t.Fatalf("collecting again opens it: %+v, %v", a, err)
	}
}
