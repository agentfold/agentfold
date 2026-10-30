package store

import (
	"context"
	"encoding/json"
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
	if err != nil || version != 1 || !created || artifact.Name != "Plano" || artifact.ReviewNodeRunID != nil {
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

// reviewing is a run whose node "dev" collected the artifact "Plano" and
// whose HumanReview node "review" awaits the human.
type reviewing struct {
	run      domain.FlowRun
	dev      domain.NodeRun
	review   domain.NodeRun
	artifact domain.Artifact
}

func review(t *testing.T, s *Store) reviewing {
	t.Helper()
	ctx := context.Background()
	run, dev := runningNode(t, s)
	if _, _, _, err := s.SaveArtifact(ctx, run.CardID, dev.ID, "Plano", "plan.md", "# v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishNodeRun(ctx, dev.ID, domain.NodeResult{Status: domain.NodeDone}); err != nil {
		t.Fatal(err)
	}
	nr, err := s.CreateNodeRun(ctx, run.ID, "review", 1, json.RawMessage(`{"result":"planned"}`))
	if err != nil {
		t.Fatal(err)
	}
	held, artifacts, err := s.OpenReview(ctx, nr.ID, run.CardID, nil, "ignored")
	if err != nil || held.Status != domain.NodeAwaitingReview || len(artifacts) != 1 || *artifacts[0].ReviewNodeRunID != nr.ID {
		t.Fatalf("OpenReview = %+v %+v, %v", held, artifacts, err)
	}
	return reviewing{run: run, dev: dev, review: held, artifact: artifacts[0]}
}

func reviewState(t *testing.T, s *Store, artifactID uuid.UUID) domain.ReviewState {
	t.Helper()
	state, err := s.ArtifactReview(context.Background(), artifactID)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestReviewRequestChanges(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := review(t, s)
	if got, _ := s.GetFlowRun(ctx, r.run.ID); got.Status != domain.RunWaiting || !got.AwaitingReview {
		t.Fatalf("the run waits for the review: %+v", got)
	}
	if latest, _ := s.ListLatestFlowRuns(ctx); len(latest) != 1 || !latest[0].AwaitingReview {
		t.Fatalf("latest runs %+v", latest)
	}
	if runs, _ := s.ListCardFlowRuns(ctx, r.run.CardID); len(runs) != 1 || !runs[0].AwaitingReview {
		t.Fatalf("card runs %+v", runs)
	}
	if state := reviewState(t, s, r.artifact.ID); state != domain.ReviewOpen {
		t.Fatalf("state %s", state)
	}
	if messages, _ := s.ListArtifactMessages(ctx, r.artifact.ID); len(messages) != 0 {
		t.Fatalf("the first review has no agent answer: %+v", messages)
	}

	msg, nr, err := s.RequestChanges(ctx, r.artifact.ID, "more detail")
	if err != nil || msg.Role != domain.RoleHuman || msg.Version != 1 || msg.Text != "more detail" || *msg.NodeRunID != r.review.ID {
		t.Fatalf("RequestChanges = %+v, %v", msg, err)
	}
	if nr.Status != domain.NodeDone || nr.OutputHandle != "changes_requested" || domain.MessageFrom(nr.Output) != domain.ReviewPrompt("Plano", "plan.md", "more detail") {
		t.Fatalf("decided node %+v %s", nr, nr.Output)
	}
	if got, _ := s.GetFlowRun(ctx, r.run.ID); got.Status != domain.RunRunning || got.AwaitingReview {
		t.Fatalf("the run goes on: %+v", got)
	}
	var job string
	if err := pool.QueryRow(ctx, `SELECT payload->>'node_run_id' FROM jobs WHERE kind = 'flow.continue'`).Scan(&job); err != nil || job != r.review.ID.String() {
		t.Fatalf("continue job %q, %v", job, err)
	}
	if state := reviewState(t, s, r.artifact.ID); state != domain.ReviewRevising {
		t.Fatalf("state %s", state)
	}
	if _, _, err := s.RequestChanges(ctx, r.artifact.ID, "again"); !errors.Is(err, domain.ErrReviewClosed) {
		t.Fatalf("asking while the agent revises: %v", err)
	}
	if _, err := s.ApproveReview(ctx, r.artifact.ID); !errors.Is(err, domain.ErrReviewClosed) {
		t.Fatalf("approving while the agent revises: %v", err)
	}

	// The agent revises and the run comes back to the review.
	dev, err := s.CreateNodeRun(ctx, r.run.ID, "dev", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.SaveArtifact(ctx, r.run.CardID, dev.ID, "Plano", "plan.md", "# v2"); err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateNodeRun(ctx, r.run.ID, "review", 2, json.RawMessage(`{"result":"revised"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, artifacts, err := s.OpenReview(ctx, again.ID, r.run.CardID, []string{"Plano"}, "revised"); err != nil || len(artifacts) != 1 {
		t.Fatalf("OpenReview again = %+v, %v", artifacts, err)
	}
	messages, err := s.ListArtifactMessages(ctx, r.artifact.ID)
	if err != nil || len(messages) != 2 || messages[1].Role != domain.RoleAgent || messages[1].Text != "revised" ||
		messages[1].Version != 2 || *messages[1].NodeRunID != again.ID {
		t.Fatalf("messages %+v, %v", messages, err)
	}

	approved, err := s.ApproveReview(ctx, r.artifact.ID)
	if err != nil || approved.OutputHandle != "approved" || string(approved.Output) != `{"result": "revised"}` {
		t.Fatalf("ApproveReview = %+v %s, %v", approved, approved.Output, err)
	}
	if state := reviewState(t, s, r.artifact.ID); state != domain.ReviewApproved {
		t.Fatalf("state %s", state)
	}

	// Coming back to the review after an approval adds no answer.
	third, err := s.CreateNodeRun(ctx, r.run.ID, "review", 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OpenReview(ctx, third.ID, r.run.CardID, nil, "x"); err != nil {
		t.Fatal(err)
	}
	if messages, _ := s.ListArtifactMessages(ctx, r.artifact.ID); len(messages) != 2 {
		t.Fatalf("messages %+v", messages)
	}
}

func TestReviewStates(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := review(t, s)
	if _, err := s.CancelFlowRun(ctx, r.run.ID); err != nil {
		t.Fatal(err)
	}
	if state := reviewState(t, s, r.artifact.ID); state != domain.ReviewClosed {
		t.Fatalf("canceled review: %s", state)
	}
	if _, _, err := s.RequestChanges(ctx, r.artifact.ID, "x"); !errors.Is(err, domain.ErrReviewClosed) {
		t.Fatalf("asking in a canceled review: %v", err)
	}

	s = newStore(t)
	c := collect(t, s)
	if state := reviewState(t, s, c.artifact.ID); state != domain.ReviewNone {
		t.Fatalf("never reviewed: %s", state)
	}
	if _, err := s.ApproveReview(ctx, c.artifact.ID); !errors.Is(err, domain.ErrReviewClosed) {
		t.Fatalf("approving with no review: %v", err)
	}
	if _, err := s.ApproveReview(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("approving a missing artifact: %v", err)
	}
	if _, err := s.ArtifactReview(cancelled(), c.artifact.ID); err == nil {
		t.Fatal("expected an error")
	}
}

func TestOpenReviewCases(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := collect(t, s)
	version, err := s.GetFlowVersion(ctx, c.run.FlowVersionID)
	if err != nil {
		t.Fatal(err)
	}
	run, _, err := s.StartFlowRun(ctx, version, c.run.CardID, domain.EventCardCreated)
	if err != nil {
		t.Fatal(err)
	}
	nr, err := s.CreateNodeRun(ctx, run.ID, "review", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	// With no names, only what this run collected: nothing here.
	if got, artifacts, err := s.OpenReview(ctx, nr.ID, run.CardID, nil, ""); err != nil || len(artifacts) != 0 || got.Status != domain.NodeQueued {
		t.Fatalf("nothing to review = %+v %+v, %v", got, artifacts, err)
	}
	// By name, what an earlier run collected too.
	if _, artifacts, err := s.OpenReview(ctx, nr.ID, run.CardID, []string{"Plano", "Outro"}, ""); err != nil || len(artifacts) != 1 {
		t.Fatalf("by name = %+v, %v", artifacts, err)
	}
	if _, _, err := s.OpenReview(ctx, uuid.New(), run.CardID, nil, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing node run: %v", err)
	}

	ended, err := s.CreateNodeRun(ctx, run.ID, "review", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishFlowRun(ctx, run.ID, domain.RunCanceled, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.OpenReview(ctx, ended.ID, run.CardID, []string{"Plano"}, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("the run ended: %v", err)
	}
}

func TestReviewFailures(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := review(t, s)
	for _, method := range []string{"GetArtifactForUpdate", "GetNodeRunForUpdate", "LatestArtifactVersion", "InsertArtifactMessage", "SetFlowRunStatus", "FinishNodeRun", "InsertJob"} {
		failOn(s, method, 1)
		if _, _, err := s.RequestChanges(ctx, r.artifact.ID, "x"); !errors.Is(err, errInjected) {
			t.Fatalf("RequestChanges %s: got %v", method, err)
		}
	}
	failOn(s, "GetArtifactForUpdate", 1)
	if _, err := s.ApproveReview(ctx, r.artifact.ID); !errors.Is(err, errInjected) {
		t.Fatalf("ApproveReview: got %v", err)
	}
	s = New(pool)
	if _, _, err := s.RequestChanges(ctx, r.artifact.ID, "x"); err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateNodeRun(ctx, r.run.ID, "review", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{
		"GetNodeRunForUpdate", "OpenArtifactReview", "AwaitReviewNodeRun", "SetFlowRunStatus",
		"GetNodeRunByIteration", "ReviewRequest", "LatestArtifactVersion", "InsertArtifactMessage",
	} {
		failOn(s, method, 1)
		if _, _, err := s.OpenReview(ctx, again.ID, r.run.CardID, nil, "x"); !errors.Is(err, errInjected) {
			t.Fatalf("OpenReview %s: got %v", method, err)
		}
	}
	for _, read := range []func(context.Context) error{
		func(ctx context.Context) error { _, err := s.ListCardArtifacts(ctx, r.run.CardID); return err },
		func(ctx context.Context) error { _, err := s.ListArtifactVersions(ctx, r.artifact.ID); return err },
		func(ctx context.Context) error { _, err := s.ListArtifactMessages(ctx, r.artifact.ID); return err },
	} {
		if err := read(cancelled()); err == nil {
			t.Error("expected an error")
		}
	}
}
