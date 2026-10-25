// Package service holds the board use cases. REST and MCP are adapters over it.
package service

import (
	"context"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
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
}

// GitInspector finds the repository that contains a path. *gitrepo.Inspector
// implements it.
type GitInspector interface {
	Root(ctx context.Context, path string) (string, error)
}

// Service implements the board use cases.
type Service struct {
	repo Repository
	git  GitInspector
}

// New returns a Service.
func New(repo Repository, git GitInspector) *Service {
	return &Service{repo: repo, git: git}
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
	return s.repo.CreateLane(ctx, in)
}

// UpdateLane changes the name and color of a lane.
func (s *Service) UpdateLane(ctx context.Context, id uuid.UUID, in domain.LaneInput) (domain.Lane, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.Lane{}, err
	}
	return s.repo.UpdateLane(ctx, id, in)
}

// MoveLane moves a lane to another position of the board.
func (s *Service) MoveLane(ctx context.Context, id uuid.UUID, position int) (domain.Lane, error) {
	if err := domain.ValidatePosition(position); err != nil {
		return domain.Lane{}, err
	}
	return s.repo.MoveLane(ctx, id, position)
}

// DeleteLane removes an empty lane.
func (s *Service) DeleteLane(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteLane(ctx, id)
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
	return s.repo.CreateProject(ctx, in)
}

// UpdateProject changes the name and path of a project.
func (s *Service) UpdateProject(ctx context.Context, id uuid.UUID, in domain.ProjectInput) (domain.Project, error) {
	in, err := s.normalizeProject(ctx, in)
	if err != nil {
		return domain.Project{}, err
	}
	return s.repo.UpdateProject(ctx, id, in)
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
	return s.repo.DeleteProject(ctx, id)
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
	return s.repo.CreateCardType(ctx, in)
}

// UpdateCardType changes the name and color of a card type.
func (s *Service) UpdateCardType(ctx context.Context, id uuid.UUID, in domain.CardTypeInput) (domain.CardType, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.CardType{}, err
	}
	return s.repo.UpdateCardType(ctx, id, in)
}

// DeleteCardType removes a card type no card uses.
func (s *Service) DeleteCardType(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteCardType(ctx, id)
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
	return s.repo.CreateCard(ctx, in)
}

// UpdateCard changes the editable fields of a card.
func (s *Service) UpdateCard(ctx context.Context, id uuid.UUID, in domain.CardInput) (domain.Card, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.Card{}, err
	}
	return s.repo.UpdateCard(ctx, id, in)
}

// MoveCard places a card at a position of a lane.
func (s *Service) MoveCard(ctx context.Context, id, laneID uuid.UUID, position int) (domain.Card, error) {
	if err := domain.ValidatePosition(position); err != nil {
		return domain.Card{}, err
	}
	return s.repo.MoveCard(ctx, id, laneID, position)
}

// DeleteCard removes a card without children.
func (s *Service) DeleteCard(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteCard(ctx, id)
}

// ListCardEvents returns the timeline of a card, oldest first.
func (s *Service) ListCardEvents(ctx context.Context, cardID uuid.UUID) ([]domain.CardEvent, error) {
	if _, err := s.repo.GetCard(ctx, cardID); err != nil {
		return nil, err
	}
	return s.repo.ListCardEvents(ctx, cardID)
}
