// Package service holds the board use cases. REST and MCP are adapters over it.
package service

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// Repository persists the board. *store.Store implements it.
type Repository interface {
	ListLanes(ctx context.Context) ([]domain.Lane, error)
	GetLane(ctx context.Context, id uuid.UUID) (domain.Lane, error)
	CreateLane(ctx context.Context, in domain.LaneInput) (domain.Lane, error)
	UpdateLane(ctx context.Context, id uuid.UUID, in domain.LaneInput) (domain.Lane, error)
	MoveLane(ctx context.Context, id uuid.UUID, position int) (domain.Lane, error)
	DeleteLane(ctx context.Context, id uuid.UUID) error

	ListProjects(ctx context.Context) ([]domain.Project, error)
	GetProject(ctx context.Context, id uuid.UUID) (domain.Project, error)
	CreateProject(ctx context.Context, in domain.ProjectInput) (domain.Project, error)
	UpdateProject(ctx context.Context, id uuid.UUID, in domain.ProjectInput) (domain.Project, error)
	DeleteProject(ctx context.Context, id uuid.UUID) error

	ListCardTypes(ctx context.Context) ([]domain.CardType, error)
	GetCardType(ctx context.Context, id uuid.UUID) (domain.CardType, error)
	CreateCardType(ctx context.Context, in domain.CardTypeInput) (domain.CardType, error)
	UpdateCardType(ctx context.Context, id uuid.UUID, in domain.CardTypeInput) (domain.CardType, error)
	DeleteCardType(ctx context.Context, id uuid.UUID) error

	ListCards(ctx context.Context) ([]domain.Card, error)
	ListChildCards(ctx context.Context, parentID uuid.UUID) ([]domain.Card, error)
	GetCard(ctx context.Context, id uuid.UUID) (domain.Card, error)
	CreateCard(ctx context.Context, in domain.NewCard) (domain.Card, error)
	UpdateCard(ctx context.Context, id uuid.UUID, in domain.CardInput) (domain.Card, error)
	MoveCard(ctx context.Context, id, laneID uuid.UUID, position int) (domain.Card, error)
	DeleteCard(ctx context.Context, id uuid.UUID) error
	ListCardEvents(ctx context.Context, cardID uuid.UUID) ([]domain.CardEvent, error)
	GetCardEvent(ctx context.Context, id uuid.UUID) (domain.CardEvent, error)

	AddCardEvent(ctx context.Context, cardID uuid.UUID, typ domain.CardEventType, payload any) (domain.CardEvent, error)

	CardHasActiveRun(ctx context.Context, cardID uuid.UUID) (bool, error)
	GetFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error)
	GetFlowVersion(ctx context.Context, id uuid.UUID) (domain.FlowVersion, error)
	ListLatestFlowRuns(ctx context.Context) ([]domain.FlowRun, error)
	ListNodeRuns(ctx context.Context, flowRunID uuid.UUID) ([]domain.NodeRun, error)
	GetNodeRun(ctx context.Context, id uuid.UUID) (domain.NodeRun, error)
	ListNodeOutput(ctx context.Context, nodeRunID uuid.UUID) ([]domain.NodeOutput, error)
	ListCardFlowRuns(ctx context.Context, cardID uuid.UUID) ([]domain.FlowRun, error)
	CancelFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error)
	RunningNodeRunByToken(ctx context.Context, tokenHash []byte) (domain.NodeRun, error)
	SetNodeRunSummary(ctx context.Context, id uuid.UUID, summary string) (domain.NodeRun, error)
	SetNodeRunOutput(ctx context.Context, id uuid.UUID, output json.RawMessage) (domain.NodeRun, error)
	SubmitReview(ctx context.Context, nr domain.NodeRun, cardID uuid.UUID, approved bool, comments []domain.ReviewInput) ([]domain.ReviewComment, error)

	AddQuestions(ctx context.Context, cardID, nodeRunID uuid.UUID, texts []string) ([]domain.Question, error)
	ListCardQuestions(ctx context.Context, cardID uuid.UUID) ([]domain.Question, error)
	AnswerQuestion(ctx context.Context, id uuid.UUID, answer string) (domain.Question, error)
	ResumeIfAnswered(ctx context.Context, nodeRunID uuid.UUID) (domain.FlowRun, bool, error)

	ListFlows(ctx context.Context) ([]domain.Flow, error)
	GetFlow(ctx context.Context, id uuid.UUID) (domain.Flow, error)
	CreateFlow(ctx context.Context, name string, draft domain.Graph) (domain.Flow, error)
	UpdateFlow(ctx context.Context, id uuid.UUID, in domain.FlowInput) (domain.Flow, error)
	SaveFlowDraft(ctx context.Context, id uuid.UUID, draft domain.Graph) (domain.Flow, error)
	DeleteFlow(ctx context.Context, id uuid.UUID) error
	PublishFlow(ctx context.Context, id uuid.UUID, graph domain.Graph) (domain.FlowVersion, error)
	ListFlowVersions(ctx context.Context, flowID uuid.UUID) ([]domain.FlowVersion, error)

	ListAgentProfiles(ctx context.Context) ([]domain.AgentProfile, error)
	GetAgentProfile(ctx context.Context, id uuid.UUID) (domain.AgentProfile, error)
	GetAgentProfileByName(ctx context.Context, name string) (domain.AgentProfile, error)
	CreateAgentProfile(ctx context.Context, kind domain.AgentKind, name string) (domain.AgentProfile, error)
	RenameAgentProfile(ctx context.Context, id uuid.UUID, name string) (domain.AgentProfile, error)
	SetAgentCredential(ctx context.Context, id uuid.UUID, kind domain.CredentialKind, sealed []byte) (domain.AgentProfile, error)
	DeleteAgentProfile(ctx context.Context, id uuid.UUID) error

	ListMcpServers(ctx context.Context) ([]domain.McpServer, error)
	GetMcpServer(ctx context.Context, id uuid.UUID) (domain.McpServer, error)
	McpSecrets(ctx context.Context, id uuid.UUID) ([]byte, error)
	CreateMcpServer(ctx context.Context, r domain.McpServerRecord) (domain.McpServer, error)
	UpdateMcpServer(ctx context.Context, id uuid.UUID, r domain.McpServerRecord) (domain.McpServer, error)
	DeleteMcpServer(ctx context.Context, id uuid.UUID) error
}

// GitInspector finds the repository that contains a path. *gitrepo.Inspector
// implements it.
type GitInspector interface {
	Root(ctx context.Context, path string) (string, error)
}

// Publisher receives every change made through the service. *events.Hub
// implements it.
type Publisher interface {
	Publish(e events.Event)
}

// Service implements the board use cases.
type Service struct {
	repo    Repository
	git     GitInspector
	pub     Publisher
	secrets Sealer
}

// New returns a Service.
func New(repo Repository, git GitInspector, pub Publisher, secrets Sealer) *Service {
	return &Service{repo: repo, git: git, pub: pub, secrets: secrets}
}

// entity is a domain value that board events can refer to.
type entity interface {
	EntityID() uuid.UUID
}

// changed publishes the change of v unless the operation failed.
func changed[T entity](s *Service, typ string, v T, err error) (T, error) {
	if err == nil {
		s.pub.Publish(events.Event{Type: typ, ID: v.EntityID(), Data: v})
	}
	return v, err
}

// deleted publishes the deletion of id unless the operation failed.
func (s *Service) deleted(typ string, id uuid.UUID, err error) error {
	if err == nil {
		s.pub.Publish(events.Event{Type: typ, ID: id})
	}
	return err
}

// ListLanes returns the lanes in board order.
func (s *Service) ListLanes(ctx context.Context) ([]domain.Lane, error) {
	return s.repo.ListLanes(ctx)
}

// GetLane returns one lane.
func (s *Service) GetLane(ctx context.Context, id uuid.UUID) (domain.Lane, error) {
	return s.repo.GetLane(ctx, id)
}

// CreateLane appends a lane to the board.
func (s *Service) CreateLane(ctx context.Context, in domain.LaneInput) (domain.Lane, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.Lane{}, err
	}
	lane, err := s.repo.CreateLane(ctx, in)
	return changed(s, "lane.created", lane, err)
}

// UpdateLane changes the name and color of a lane.
func (s *Service) UpdateLane(ctx context.Context, id uuid.UUID, in domain.LaneInput) (domain.Lane, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.Lane{}, err
	}
	lane, err := s.repo.UpdateLane(ctx, id, in)
	return changed(s, "lane.updated", lane, err)
}

// MoveLane moves a lane to another position of the board.
func (s *Service) MoveLane(ctx context.Context, id uuid.UUID, position int) (domain.Lane, error) {
	if err := domain.ValidatePosition(position); err != nil {
		return domain.Lane{}, err
	}
	lane, err := s.repo.MoveLane(ctx, id, position)
	return changed(s, "lane.moved", lane, err)
}

// DeleteLane removes an empty lane.
func (s *Service) DeleteLane(ctx context.Context, id uuid.UUID) error {
	return s.deleted("lane.deleted", id, s.repo.DeleteLane(ctx, id))
}

// ListProjects returns the projects ordered by name.
func (s *Service) ListProjects(ctx context.Context) ([]domain.Project, error) {
	return s.repo.ListProjects(ctx)
}

// GetProject returns one project.
func (s *Service) GetProject(ctx context.Context, id uuid.UUID) (domain.Project, error) {
	return s.repo.GetProject(ctx, id)
}

// CreateProject registers a local git repository as a project.
func (s *Service) CreateProject(ctx context.Context, in domain.ProjectInput) (domain.Project, error) {
	in, err := s.normalizeProject(ctx, in)
	if err != nil {
		return domain.Project{}, err
	}
	project, err := s.repo.CreateProject(ctx, in)
	return changed(s, "project.created", project, err)
}

// UpdateProject changes the name and path of a project.
func (s *Service) UpdateProject(ctx context.Context, id uuid.UUID, in domain.ProjectInput) (domain.Project, error) {
	in, err := s.normalizeProject(ctx, in)
	if err != nil {
		return domain.Project{}, err
	}
	project, err := s.repo.UpdateProject(ctx, id, in)
	return changed(s, "project.updated", project, err)
}

// normalizeProject validates the input and replaces the path by the root of
// the git repository that contains it, since worktrees are created from it.
func (s *Service) normalizeProject(ctx context.Context, in domain.ProjectInput) (domain.ProjectInput, error) {
	in, err := in.Normalize()
	if err != nil {
		return in, err
	}
	root, err := s.git.Root(ctx, in.LocalPath)
	if err != nil {
		return in, &domain.ValidationError{Field: "local_path", Message: "must be a git repository"}
	}
	in.LocalPath = root
	return in, nil
}

// DeleteProject removes a project no card refers to.
func (s *Service) DeleteProject(ctx context.Context, id uuid.UUID) error {
	return s.deleted("project.deleted", id, s.repo.DeleteProject(ctx, id))
}

// ListCardTypes returns the card types ordered by name.
func (s *Service) ListCardTypes(ctx context.Context) ([]domain.CardType, error) {
	return s.repo.ListCardTypes(ctx)
}

// GetCardType returns one card type.
func (s *Service) GetCardType(ctx context.Context, id uuid.UUID) (domain.CardType, error) {
	return s.repo.GetCardType(ctx, id)
}

// CreateCardType registers a card type.
func (s *Service) CreateCardType(ctx context.Context, in domain.CardTypeInput) (domain.CardType, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.CardType{}, err
	}
	cardType, err := s.repo.CreateCardType(ctx, in)
	return changed(s, "card_type.created", cardType, err)
}

// UpdateCardType changes the name and color of a card type.
func (s *Service) UpdateCardType(ctx context.Context, id uuid.UUID, in domain.CardTypeInput) (domain.CardType, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.CardType{}, err
	}
	cardType, err := s.repo.UpdateCardType(ctx, id, in)
	return changed(s, "card_type.updated", cardType, err)
}

// DeleteCardType removes a card type no card uses.
func (s *Service) DeleteCardType(ctx context.Context, id uuid.UUID) error {
	return s.deleted("card_type.deleted", id, s.repo.DeleteCardType(ctx, id))
}

// ListCards returns every card, grouped by lane in board order.
func (s *Service) ListCards(ctx context.Context) ([]domain.Card, error) {
	return s.repo.ListCards(ctx)
}

// ListChildCards returns the children of a card.
func (s *Service) ListChildCards(ctx context.Context, parentID uuid.UUID) ([]domain.Card, error) {
	if _, err := s.repo.GetCard(ctx, parentID); err != nil {
		return nil, err
	}
	return s.repo.ListChildCards(ctx, parentID)
}

// GetCard returns one card.
func (s *Service) GetCard(ctx context.Context, id uuid.UUID) (domain.Card, error) {
	return s.repo.GetCard(ctx, id)
}

// CreateCard adds a card to the end of a lane.
func (s *Service) CreateCard(ctx context.Context, in domain.NewCard) (domain.Card, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.Card{}, err
	}
	card, err := s.repo.CreateCard(ctx, in)
	return changed(s, "card.created", card, err)
}

// UpdateCard changes the editable fields of a card.
func (s *Service) UpdateCard(ctx context.Context, id uuid.UUID, in domain.CardInput) (domain.Card, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.Card{}, err
	}
	card, err := s.repo.UpdateCard(ctx, id, in)
	return changed(s, "card.updated", card, err)
}

// MoveCard places a card at a position of a lane. A card is held in place
// while a flow runs for it: only the flow moves it then.
func (s *Service) MoveCard(ctx context.Context, id, laneID uuid.UUID, position int) (domain.Card, error) {
	if err := domain.ValidatePosition(position); err != nil {
		return domain.Card{}, err
	}
	busy, err := s.repo.CardHasActiveRun(ctx, id)
	if err != nil {
		return domain.Card{}, err
	}
	if busy {
		return domain.Card{}, domain.ErrCardBusy
	}
	card, err := s.repo.MoveCard(ctx, id, laneID, position)
	return changed(s, "card.moved", card, err)
}

// DeleteCard removes a card without children.
func (s *Service) DeleteCard(ctx context.Context, id uuid.UUID) error {
	return s.deleted("card.deleted", id, s.repo.DeleteCard(ctx, id))
}

// ListCardEvents returns the timeline of a card, oldest first.
func (s *Service) ListCardEvents(ctx context.Context, cardID uuid.UUID) ([]domain.CardEvent, error) {
	if _, err := s.repo.GetCard(ctx, cardID); err != nil {
		return nil, err
	}
	return s.repo.ListCardEvents(ctx, cardID)
}
