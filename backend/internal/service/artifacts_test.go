package service

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func TestArtifactReads(t *testing.T) {
	s, repo, _ := newService()
	list, err := s.ListCardArtifacts(ctx, id)
	if err != nil || len(list) != 1 || len(list[0].Versions) != 1 || len(list[0].Messages) != 1 || list[0].Review != domain.ReviewOpen {
		t.Fatalf("ListCardArtifacts = %+v, %v", list, err)
	}
	if a, err := s.GetArtifact(ctx, id); err != nil || a.ID != id {
		t.Fatalf("GetArtifact = %+v, %v", a, err)
	}
	for _, method := range []string{"GetCard", "ListCardArtifacts", "ListArtifactVersions", "ListArtifactMessages", "ArtifactReview"} {
		s, repo, _ := newService()
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.ListCardArtifacts(ctx, id); !errors.Is(err, errRepo) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
	repo.errs = map[string]error{"GetArtifact": errRepo}
	if _, err := s.GetArtifact(ctx, id); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}

func TestArtifactReviewDecisions(t *testing.T) {
	s, repo, pub := newService()
	repo.nodeRun = domain.NodeRun{ID: uuid.New(), FlowRunID: uuid.New()}
	msg, err := s.RequestArtifactChanges(ctx, id, " more detail ")
	if err != nil || msg.Text != "more detail" {
		t.Fatalf("RequestArtifactChanges = %+v, %v", msg, err)
	}
	if err := s.ApproveArtifact(ctx, id); err != nil {
		t.Fatal(err)
	}
	want := []string{"artifact.updated", "node_run.updated", "flow_run.updated"}
	if got := eventTypes(pub); !slices.Equal(got, append(want, want...)) {
		t.Fatalf("events %q", got)
	}
	var verr *domain.ValidationError
	if _, err := s.RequestArtifactChanges(ctx, id, " "); !errors.As(err, &verr) {
		t.Fatalf("empty text: %v", err)
	}
	for _, method := range []string{"RequestChanges", "GetFlowRun"} {
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.RequestArtifactChanges(ctx, id, "x"); !errors.Is(err, errRepo) {
			t.Fatalf("%s: got %v", method, err)
		}
	}
	repo.errs = map[string]error{"ApproveReview": errRepo}
	if err := s.ApproveArtifact(ctx, id); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}
