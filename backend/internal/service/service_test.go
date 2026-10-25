package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// fakeRepo records the calls it receives and fails them with err, or with
// errs[method] when set. Flow methods return flow.
type fakeRepo struct {
	err   error
	errs  map[string]error
	calls []string
	in    any            // input of the last call
	ins   map[string]any // input of the last call of each method
	flow  domain.Flow
}

func (f *fakeRepo) call(name string, in any) error {
	f.calls = append(f.calls, name)
	f.in = in
	if f.ins == nil {
		f.ins = map[string]any{}
	}
	f.ins[name] = in
	if err, ok := f.errs[name]; ok {
		return err
	}
	return f.err
}

func (f *fakeRepo) ListLanes(context.Context) ([]domain.Lane, error) {
	return []domain.Lane{{ID: laneID}}, f.call("ListLanes", nil)
}

func (f *fakeRepo) GetLane(_ context.Context, id uuid.UUID) (domain.Lane, error) {
	return domain.Lane{}, f.call("GetLane", id)
}

func (f *fakeRepo) CreateLane(_ context.Context, in domain.LaneInput) (domain.Lane, error) {
	return domain.Lane{}, f.call("CreateLane", in)
}

func (f *fakeRepo) UpdateLane(_ context.Context, _ uuid.UUID, in domain.LaneInput) (domain.Lane, error) {
	return domain.Lane{}, f.call("UpdateLane", in)
}

func (f *fakeRepo) MoveLane(_ context.Context, _ uuid.UUID, position int) (domain.Lane, error) {
	return domain.Lane{}, f.call("MoveLane", position)
}

func (f *fakeRepo) DeleteLane(_ context.Context, id uuid.UUID) error {
	return f.call("DeleteLane", id)
}

func (f *fakeRepo) ListProjects(context.Context) ([]domain.Project, error) {
	return nil, f.call("ListProjects", nil)
}

func (f *fakeRepo) GetProject(_ context.Context, id uuid.UUID) (domain.Project, error) {
	return domain.Project{}, f.call("GetProject", id)
}

func (f *fakeRepo) CreateProject(_ context.Context, in domain.ProjectInput) (domain.Project, error) {
	return domain.Project{}, f.call("CreateProject", in)
}

func (f *fakeRepo) UpdateProject(_ context.Context, _ uuid.UUID, in domain.ProjectInput) (domain.Project, error) {
	return domain.Project{}, f.call("UpdateProject", in)
}

func (f *fakeRepo) DeleteProject(_ context.Context, id uuid.UUID) error {
	return f.call("DeleteProject", id)
}

func (f *fakeRepo) ListCardTypes(context.Context) ([]domain.CardType, error) {
	return []domain.CardType{{ID: typeID}}, f.call("ListCardTypes", nil)
}

func (f *fakeRepo) GetCardType(_ context.Context, id uuid.UUID) (domain.CardType, error) {
	return domain.CardType{}, f.call("GetCardType", id)
}

func (f *fakeRepo) CreateCardType(_ context.Context, in domain.CardTypeInput) (domain.CardType, error) {
	return domain.CardType{}, f.call("CreateCardType", in)
}

func (f *fakeRepo) UpdateCardType(_ context.Context, _ uuid.UUID, in domain.CardTypeInput) (domain.CardType, error) {
	return domain.CardType{}, f.call("UpdateCardType", in)
}

func (f *fakeRepo) DeleteCardType(_ context.Context, id uuid.UUID) error {
	return f.call("DeleteCardType", id)
}

func (f *fakeRepo) ListCards(context.Context) ([]domain.Card, error) {
	return nil, f.call("ListCards", nil)
}

func (f *fakeRepo) ListChildCards(_ context.Context, parentID uuid.UUID) ([]domain.Card, error) {
	return nil, f.call("ListChildCards", parentID)
}

func (f *fakeRepo) GetCard(_ context.Context, id uuid.UUID) (domain.Card, error) {
	return domain.Card{}, f.call("GetCard", id)
}

func (f *fakeRepo) CreateCard(_ context.Context, in domain.NewCard) (domain.Card, error) {
	return domain.Card{}, f.call("CreateCard", in)
}

func (f *fakeRepo) UpdateCard(_ context.Context, _ uuid.UUID, in domain.CardInput) (domain.Card, error) {
	return domain.Card{}, f.call("UpdateCard", in)
}

func (f *fakeRepo) MoveCard(_ context.Context, _, _ uuid.UUID, position int) (domain.Card, error) {
	return domain.Card{}, f.call("MoveCard", position)
}

func (f *fakeRepo) DeleteCard(_ context.Context, id uuid.UUID) error {
	return f.call("DeleteCard", id)
}

func (f *fakeRepo) ListCardEvents(_ context.Context, cardID uuid.UUID) ([]domain.CardEvent, error) {
	return nil, f.call("ListCardEvents", cardID)
}

func (f *fakeRepo) ListFlows(context.Context) ([]domain.Flow, error) {
	return nil, f.call("ListFlows", nil)
}

func (f *fakeRepo) GetFlow(_ context.Context, id uuid.UUID) (domain.Flow, error) {
	return f.flow, f.call("GetFlow", id)
}

func (f *fakeRepo) CreateFlow(_ context.Context, name string, draft domain.Graph) (domain.Flow, error) {
	return f.flow, f.call("CreateFlow", fmt.Sprintf("%s %d", name, len(draft.Nodes)))
}

func (f *fakeRepo) UpdateFlow(_ context.Context, _ uuid.UUID, in domain.FlowInput) (domain.Flow, error) {
	return f.flow, f.call("UpdateFlow", in)
}

func (f *fakeRepo) SaveFlowDraft(_ context.Context, _ uuid.UUID, draft domain.Graph) (domain.Flow, error) {
	return f.flow, f.call("SaveFlowDraft", draft.Nodes != nil && draft.Edges != nil)
}

func (f *fakeRepo) DeleteFlow(_ context.Context, id uuid.UUID) error {
	return f.call("DeleteFlow", id)
}

func (f *fakeRepo) PublishFlow(_ context.Context, _ uuid.UUID, graph domain.Graph) (domain.FlowVersion, error) {
	return domain.FlowVersion{Version: 1}, f.call("PublishFlow", len(graph.Nodes))
}

func (f *fakeRepo) ListFlowVersions(_ context.Context, flowID uuid.UUID) ([]domain.FlowVersion, error) {
	return nil, f.call("ListFlowVersions", flowID)
}

// fakeGit answers Root with root, or fails with err.
type fakeGit struct {
	root string
	err  error
}

func (g fakeGit) Root(context.Context, string) (string, error) { return g.root, g.err }

var (
	ctx     = context.Background()
	id      = uuid.New()
	laneID  = uuid.New()
	typeID  = uuid.New()
	errRepo = errors.New("repo failed")
)

// fakePub records the published events.
type fakePub struct{ events []events.Event }

func (p *fakePub) Publish(e events.Event) { p.events = append(p.events, e) }

func newService() (*Service, *fakeRepo, *fakePub) {
	repo, pub := &fakeRepo{}, &fakePub{}
	return New(repo, fakeGit{root: "/repo"}, pub), repo, pub
}

func validCard() domain.CardInput {
	return domain.CardInput{Title: " t ", Description: " d ", CardTypeID: id}
}

// TestDelegation checks that every use case reaches the repository with the
// normalized input and returns its error.
func TestDelegation(t *testing.T) {
	tests := []struct {
		name   string
		call   func(*Service) error
		wantIn any
		event  string // published on success; empty for reads
	}{
		{"ListLanes", func(s *Service) error { _, err := s.ListLanes(ctx); return err }, nil, ""},
		{"GetLane", func(s *Service) error { _, err := s.GetLane(ctx, id); return err }, id, ""},
		{"CreateLane", func(s *Service) error {
			_, err := s.CreateLane(ctx, domain.LaneInput{Name: " A ", Color: "#ABCDEF"})
			return err
		}, domain.LaneInput{Name: "A", Color: "#abcdef"}, "lane.created"},
		{"UpdateLane", func(s *Service) error {
			_, err := s.UpdateLane(ctx, id, domain.LaneInput{Name: "B"})
			return err
		}, domain.LaneInput{Name: "B"}, "lane.updated"},
		{"MoveLane", func(s *Service) error { _, err := s.MoveLane(ctx, id, 2); return err }, 2, "lane.moved"},
		{"DeleteLane", func(s *Service) error { return s.DeleteLane(ctx, id) }, id, "lane.deleted"},
		{"ListProjects", func(s *Service) error { _, err := s.ListProjects(ctx); return err }, nil, ""},
		{"GetProject", func(s *Service) error { _, err := s.GetProject(ctx, id); return err }, id, ""},
		{"CreateProject", func(s *Service) error {
			_, err := s.CreateProject(ctx, domain.ProjectInput{Name: " p ", LocalPath: "/repo/sub"})
			return err
		}, domain.ProjectInput{Name: "p", LocalPath: "/repo"}, "project.created"},
		{"UpdateProject", func(s *Service) error {
			_, err := s.UpdateProject(ctx, id, domain.ProjectInput{Name: "p", LocalPath: "/repo"})
			return err
		}, domain.ProjectInput{Name: "p", LocalPath: "/repo"}, "project.updated"},
		{"DeleteProject", func(s *Service) error { return s.DeleteProject(ctx, id) }, id, "project.deleted"},
		{"ListCardTypes", func(s *Service) error { _, err := s.ListCardTypes(ctx); return err }, nil, ""},
		{"GetCardType", func(s *Service) error { _, err := s.GetCardType(ctx, id); return err }, id, ""},
		{"CreateCardType", func(s *Service) error {
			_, err := s.CreateCardType(ctx, domain.CardTypeInput{Name: " bug "})
			return err
		}, domain.CardTypeInput{Name: "bug"}, "card_type.created"},
		{"UpdateCardType", func(s *Service) error {
			_, err := s.UpdateCardType(ctx, id, domain.CardTypeInput{Name: "bug"})
			return err
		}, domain.CardTypeInput{Name: "bug"}, "card_type.updated"},
		{"DeleteCardType", func(s *Service) error { return s.DeleteCardType(ctx, id) }, id, "card_type.deleted"},
		{"ListCards", func(s *Service) error { _, err := s.ListCards(ctx); return err }, nil, ""},
		{"GetCard", func(s *Service) error { _, err := s.GetCard(ctx, id); return err }, id, ""},
		{"CreateCard", func(s *Service) error {
			_, err := s.CreateCard(ctx, domain.NewCard{CardInput: validCard(), LaneID: id})
			return err
		}, domain.NewCard{CardInput: domain.CardInput{Title: "t", Description: "d", CardTypeID: id}, LaneID: id}, "card.created"},
		{"UpdateCard", func(s *Service) error { _, err := s.UpdateCard(ctx, id, validCard()); return err },
			domain.CardInput{Title: "t", Description: "d", CardTypeID: id}, "card.updated"},
		{"MoveCard", func(s *Service) error { _, err := s.MoveCard(ctx, id, id, 0); return err }, 0, "card.moved"},
		{"DeleteCard", func(s *Service) error { return s.DeleteCard(ctx, id) }, id, "card.deleted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, pub := newService()
			if err := tt.call(s); err != nil {
				t.Fatal(err)
			}
			if tt.event == "" && len(pub.events) != 0 || tt.event != "" && (len(pub.events) != 1 || pub.events[0].Type != tt.event) {
				t.Fatalf("published %+v, want %q", pub.events, tt.event)
			}

			s, repo, pub := newService()
			repo.err = errRepo
			if err := tt.call(s); !errors.Is(err, errRepo) {
				t.Fatalf("got %v, want the repository error", err)
			}
			if len(repo.calls) != 1 || repo.calls[0] != tt.name {
				t.Fatalf("calls = %v", repo.calls)
			}
			if repo.in != tt.wantIn {
				t.Fatalf("repository got %+v, want %+v", repo.in, tt.wantIn)
			}
			if len(pub.events) != 0 {
				t.Fatalf("failed call published %+v", pub.events)
			}
		})
	}
}

// TestValidation checks that invalid input never reaches the repository.
func TestValidation(t *testing.T) {
	tests := []struct {
		name  string
		field string
		call  func(*Service) error
	}{
		{"CreateLane", "name", func(s *Service) error { _, err := s.CreateLane(ctx, domain.LaneInput{}); return err }},
		{"UpdateLane", "color", func(s *Service) error {
			_, err := s.UpdateLane(ctx, id, domain.LaneInput{Name: "a", Color: "red"})
			return err
		}},
		{"MoveLane", "position", func(s *Service) error { _, err := s.MoveLane(ctx, id, -1); return err }},
		{"CreateProject", "local_path", func(s *Service) error {
			_, err := s.CreateProject(ctx, domain.ProjectInput{Name: "p", LocalPath: "rel"})
			return err
		}},
		{"UpdateProject", "name", func(s *Service) error {
			_, err := s.UpdateProject(ctx, id, domain.ProjectInput{LocalPath: "/repo"})
			return err
		}},
		{"CreateCardType", "name", func(s *Service) error {
			_, err := s.CreateCardType(ctx, domain.CardTypeInput{})
			return err
		}},
		{"UpdateCardType", "color", func(s *Service) error {
			_, err := s.UpdateCardType(ctx, id, domain.CardTypeInput{Name: "x", Color: "#12"})
			return err
		}},
		{"CreateCard", "lane_id", func(s *Service) error {
			_, err := s.CreateCard(ctx, domain.NewCard{CardInput: validCard()})
			return err
		}},
		{"UpdateCard", "title", func(s *Service) error {
			_, err := s.UpdateCard(ctx, id, domain.CardInput{CardTypeID: id})
			return err
		}},
		{"MoveCard", "position", func(s *Service) error { _, err := s.MoveCard(ctx, id, id, -1); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, repo, _ := newService()
			var verr *domain.ValidationError
			if err := tt.call(s); !errors.As(err, &verr) || verr.Field != tt.field {
				t.Fatalf("got %v, want a validation error on %s", err, tt.field)
			}
			if len(repo.calls) != 0 {
				t.Fatalf("repository was called: %v", repo.calls)
			}
		})
	}
}

func TestProjectMustBeGitRepository(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, fakeGit{err: errors.New("not a repo")}, &fakePub{})
	var verr *domain.ValidationError
	_, err := s.CreateProject(ctx, domain.ProjectInput{Name: "p", LocalPath: "/tmp/x"})
	if !errors.As(err, &verr) || verr.Field != "local_path" {
		t.Fatalf("got %v", err)
	}
	if len(repo.calls) != 0 {
		t.Fatalf("repository was called: %v", repo.calls)
	}
}

// TestCardListsCheckTheCard checks that listing children or events of a
// missing card fails instead of returning an empty list.
func TestCardListsCheckTheCard(t *testing.T) {
	calls := map[string]func(*Service) error{
		"ListChildCards": func(s *Service) error { _, err := s.ListChildCards(ctx, id); return err },
		"ListCardEvents": func(s *Service) error { _, err := s.ListCardEvents(ctx, id); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			s, repo, _ := newService()
			repo.err = domain.ErrNotFound
			if err := call(s); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("got %v", err)
			}
			if len(repo.calls) != 1 || repo.calls[0] != "GetCard" {
				t.Fatalf("calls = %v", repo.calls)
			}

			s, repo, _ = newService()
			if err := call(s); err != nil {
				t.Fatal(err)
			}
			if len(repo.calls) != 2 || repo.calls[1] != name || repo.in != id {
				t.Fatalf("calls = %v, in = %v", repo.calls, repo.in)
			}
		})
	}
}
