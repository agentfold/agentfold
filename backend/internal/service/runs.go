package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// GetFlowRun returns one flow run.
func (s *Service) GetFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error) {
	return s.repo.GetFlowRun(ctx, id)
}

// ListLatestFlowRuns returns one run per card that ever ran a flow: an
// active one if there is any, else the last one.
func (s *Service) ListLatestFlowRuns(ctx context.Context) ([]domain.FlowRun, error) {
	return s.repo.ListLatestFlowRuns(ctx)
}

// Entity returns the entity a board event of type typ ("<entity>.<change>")
// refers to, for the events other processes announce: cards, flow runs and
// card events.
func (s *Service) Entity(ctx context.Context, typ string, id uuid.UUID) (any, error) {
	entity, _, _ := strings.Cut(typ, ".")
	switch entity {
	case "card":
		return s.repo.GetCard(ctx, id)
	case "flow_run":
		return s.repo.GetFlowRun(ctx, id)
	case "card_event":
		return s.repo.GetCardEvent(ctx, id)
	}
	return nil, fmt.Errorf("no entity for event %q", typ)
}
