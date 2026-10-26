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
	repo.resumed = true
	list, err := s.ListCardArtifacts(ctx, id)
	if err != nil || len(list) != 1 || len(list[0].Versions) != 1 || len(list[0].Messages) != 1 || !list[0].Revising {
		t.Fatalf("ListCardArtifacts = %+v, %v", list, err)
	}
	if a, err := s.GetArtifact(ctx, id); err != nil || a.ID != id {
		t.Fatalf("GetArtifact = %+v, %v", a, err)
	}
	for _, method := range []string{"GetCard", "ListCardArtifacts", "ListArtifactVersions", "ListArtifactMessages", "ArtifactRevising"} {
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

func TestReviseArtifact(t *testing.T) {
	s, repo, pub := newService()
	repo.flowRun = domain.FlowRun{ID: uuid.New(), CardID: id, FlowName: "plan", Trigger: domain.TriggerRevision}
	msg, err := s.ReviseArtifact(ctx, id, 2, " more detail ")
	if err != nil || msg.Text != "more detail" || msg.Version != 2 {
		t.Fatalf("ReviseArtifact = %+v, %v", msg, err)
	}
	if got := eventTypes(pub); !slices.Equal(got, []string{"artifact.updated", "flow_run.created", "card_event.created"}) {
		t.Fatalf("events %q", got)
	}
	if payload := repo.ins["AddCardEvent:flow.started"].(revisionStarted); payload.ArtifactID != id || payload.Trigger != domain.TriggerRevision {
		t.Fatalf("timeline %+v", payload)
	}
	var verr *domain.ValidationError
	if _, err := s.ReviseArtifact(ctx, id, 1, " "); !errors.As(err, &verr) {
		t.Fatalf("empty text: %v", err)
	}
	repo.errs = map[string]error{"StartRevision": errRepo}
	if _, err := s.ReviseArtifact(ctx, id, 1, "x"); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}
