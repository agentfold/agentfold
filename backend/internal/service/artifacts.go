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
	// Revising tells whether the agent is revising it now.
	Revising bool
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
	d.Revising, err = s.repo.ArtifactRevising(ctx, a.ID)
	return d, err
}

// ReviseArtifact sends what a human asks in the review chat of an artifact,
// reading version, to the agent that collected it: the agent resumes its
// session, updates the file and the board collects it again. The card stays
// where it is.
func (s *Service) ReviseArtifact(ctx context.Context, id uuid.UUID, version int, text string) (domain.ArtifactMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.ArtifactMessage{}, &domain.ValidationError{Field: "text", Message: "is required"}
	}
	msg, run, err := s.repo.StartRevision(ctx, id, version, text)
	if err != nil {
		return domain.ArtifactMessage{}, err
	}
	s.pub.Publish(events.Event{Type: "artifact.updated", ID: id})
	s.pub.Publish(events.Event{Type: "flow_run.created", ID: run.ID, Data: run})
	return msg, s.timeline(ctx, run.CardID, domain.FlowStarted, revisionStarted{
		FlowRunID: run.ID, FlowID: run.FlowID, FlowName: run.FlowName, Trigger: run.Trigger, ArtifactID: id,
	})
}

// revisionStarted is the flow.started entry of a revision run.
type revisionStarted struct {
	FlowRunID  uuid.UUID `json:"flow_run_id"`
	FlowID     uuid.UUID `json:"flow_id"`
	FlowName   string    `json:"flow_name"`
	Trigger    string    `json:"trigger"`
	ArtifactID uuid.UUID `json:"artifact_id"`
}
