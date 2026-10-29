package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// ArtifactDetail is an artifact with its versions and review chat.
type ArtifactDetail struct {
	domain.Artifact
	Versions []domain.ArtifactVersion
	Messages []domain.ArtifactMessage
	Review   domain.ReviewState
}

// ListCardArtifacts returns the artifacts of a card in detail, oldest first.
func (s *Service) ListCardArtifacts(ctx context.Context, cardID uuid.UUID) ([]ArtifactDetail, error) {
	if _, err := s.repo.GetCard(ctx, cardID); err != nil {
		return nil, err
	}
	artifacts, err := s.repo.ListCardArtifacts(ctx, cardID)
	if err != nil {
		return nil, err
	}
	out := make([]ArtifactDetail, len(artifacts))
	for i, a := range artifacts {
		if out[i], err = s.artifactDetail(ctx, a); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// GetArtifact returns an artifact in detail.
func (s *Service) GetArtifact(ctx context.Context, id uuid.UUID) (ArtifactDetail, error) {
	a, err := s.repo.GetArtifact(ctx, id)
	if err != nil {
		return ArtifactDetail{}, err
	}
	return s.artifactDetail(ctx, a)
}

func (s *Service) artifactDetail(ctx context.Context, a domain.Artifact) (ArtifactDetail, error) {
	d := ArtifactDetail{Artifact: a}
	var err error
	if d.Versions, err = s.repo.ListArtifactVersions(ctx, a.ID); err != nil {
		return d, err
	}
	if d.Messages, err = s.repo.ListArtifactMessages(ctx, a.ID); err != nil {
		return d, err
	}
	d.Review, err = s.repo.ArtifactReview(ctx, a.ID)
	return d, err
}

// RequestArtifactChanges sends what a human asks in the review of an
// artifact to the agent its HumanReview node sends changes to: the agent
// updates the file in its session and the run comes back to the review.
func (s *Service) RequestArtifactChanges(ctx context.Context, id uuid.UUID, text string) (domain.ArtifactMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.ArtifactMessage{}, &domain.ValidationError{Field: "text", Message: "is required"}
	}
	msg, nr, err := s.repo.RequestChanges(ctx, id, text)
	if err != nil {
		return domain.ArtifactMessage{}, err
	}
	return msg, s.decided(ctx, id, nr)
}

// ApproveArtifact approves the review an artifact is under: the flow goes
// on from its HumanReview node.
func (s *Service) ApproveArtifact(ctx context.Context, id uuid.UUID) error {
	nr, err := s.repo.ApproveReview(ctx, id)
	if err != nil {
		return err
	}
	return s.decided(ctx, id, nr)
}

// decided announces a review the human decided; the engine follows it.
func (s *Service) decided(ctx context.Context, artifactID uuid.UUID, nr domain.NodeRun) error {
	s.pub.Publish(events.Event{Type: "artifact.updated", ID: artifactID})
	s.pub.Publish(events.Event{Type: "node_run.updated", ID: nr.ID, Data: nr})
	run, err := s.repo.GetFlowRun(ctx, nr.FlowRunID)
	if err == nil {
		s.pub.Publish(events.Event{Type: "flow_run.updated", ID: run.ID, Data: run})
	}
	return err
}
