package service

import (
	"context"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// FlowDetail is a flow with what prevents its draft from being published.
type FlowDetail struct {
	domain.Flow
	Problems []domain.GraphProblem
}

// ListFlows returns the flows ordered by name.
func (s *Service) ListFlows(ctx context.Context) ([]domain.Flow, error) {
	return s.repo.ListFlows(ctx)
}

// GetFlow returns a flow and the problems of its draft.
func (s *Service) GetFlow(ctx context.Context, id uuid.UUID) (FlowDetail, error) {
	return s.detail(ctx)(s.repo.GetFlow(ctx, id))
}

// CreateFlow creates a disabled flow whose draft holds a single trigger.
func (s *Service) CreateFlow(ctx context.Context, name string) (FlowDetail, error) {
	in, err := domain.FlowInput{Name: name}.Normalize()
	if err != nil {
		return FlowDetail{}, err
	}
	flow, err := s.repo.CreateFlow(ctx, in.Name, domain.NewFlowGraph())
	return s.detail(ctx)(changed(s, "flow.created", flow, err))
}

// UpdateFlow renames a flow and turns it on or off. Only flows with a
// published version can be turned on.
func (s *Service) UpdateFlow(ctx context.Context, id uuid.UUID, in domain.FlowInput) (FlowDetail, error) {
	in, err := in.Normalize()
	if err != nil {
		return FlowDetail{}, err
	}
	if in.Enabled {
		current, err := s.repo.GetFlow(ctx, id)
		if err != nil {
			return FlowDetail{}, err
		}
		if current.LatestVersion == 0 {
			return FlowDetail{}, &domain.ValidationError{Field: "enabled", Message: "publish a version before enabling the flow"}
		}
	}
	flow, err := s.repo.UpdateFlow(ctx, id, in)
	return s.detail(ctx)(changed(s, "flow.updated", flow, err))
}

// SaveFlowDraft replaces the draft of a flow, sealing the secret headers of
// its http_request nodes. Drafts may have problems; they are returned so
// the editor can show them.
func (s *Service) SaveFlowDraft(ctx context.Context, id uuid.UUID, draft domain.Graph) (FlowDetail, error) {
	sealed, err := draft.Normalized().SealSecrets(s.secrets.Seal)
	if err != nil {
		return FlowDetail{}, err
	}
	flow, err := s.repo.SaveFlowDraft(ctx, id, sealed)
	return s.detail(ctx)(changed(s, "flow.updated", flow, err))
}

// PublishFlow freezes the draft into a new version, if it has no problems.
func (s *Service) PublishFlow(ctx context.Context, id uuid.UUID) (domain.FlowVersion, error) {
	detail, err := s.GetFlow(ctx, id)
	if err != nil {
		return domain.FlowVersion{}, err
	}
	if len(detail.Problems) > 0 {
		return domain.FlowVersion{}, &domain.GraphError{Problems: detail.Problems}
	}
	version, err := s.repo.PublishFlow(ctx, id, detail.Draft)
	if err == nil {
		s.pub.Publish(events.Event{Type: "flow.published", ID: id})
	}
	return version, err
}

// DeleteFlow removes a flow and its versions.
func (s *Service) DeleteFlow(ctx context.Context, id uuid.UUID) error {
	return s.deleted("flow.deleted", id, s.repo.DeleteFlow(ctx, id))
}

// ListFlowVersions returns the versions of a flow, newest first.
func (s *Service) ListFlowVersions(ctx context.Context, id uuid.UUID) ([]domain.FlowVersion, error) {
	if _, err := s.repo.GetFlow(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.ListFlowVersions(ctx, id)
}

// detail returns a function that adds the draft problems to the result of a
// flow operation.
func (s *Service) detail(ctx context.Context) func(domain.Flow, error) (FlowDetail, error) {
	return func(flow domain.Flow, err error) (FlowDetail, error) {
		if err != nil {
			return FlowDetail{}, err
		}
		refs, err := s.references(ctx)
		if err != nil {
			return FlowDetail{}, err
		}
		return FlowDetail{Flow: flow, Problems: flow.Draft.Problems(refs)}, nil
	}
}

// references collects the lanes and card types that flow configs may use.
func (s *Service) references(ctx context.Context) (domain.References, error) {
	refs := domain.References{
		Lanes:      map[uuid.UUID]bool{},
		CardTypes:  map[uuid.UUID]bool{},
		Profiles:   map[uuid.UUID]bool{},
		McpServers: map[uuid.UUID]bool{},
	}
	lanes, err := s.repo.ListLanes(ctx)
	if err != nil {
		return refs, err
	}
	for _, l := range lanes {
		refs.Lanes[l.ID] = true
	}
	types, err := s.repo.ListCardTypes(ctx)
	if err != nil {
		return refs, err
	}
	for _, t := range types {
		refs.CardTypes[t.ID] = true
	}
	profiles, err := s.repo.ListAgentProfiles(ctx)
	if err != nil {
		return refs, err
	}
	for _, p := range profiles {
		refs.Profiles[p.ID] = true
	}
	servers, err := s.repo.ListMcpServers(ctx)
	if err != nil {
		return refs, err
	}
	for _, m := range servers {
		refs.McpServers[m.ID] = true
	}
	return refs, nil
}
