package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// childRepo backs the child card use cases: card types, cards (children of the
// run card id), their runs and the node runs of those runs.
type childRepo struct {
	*fakeRepo
	types    []domain.CardType
	projects []domain.Project
	cards    map[uuid.UUID]domain.Card
	runs     map[uuid.UUID][]domain.FlowRun
	nodes    []domain.NodeRun
}

func (r *childRepo) ListCardTypes(context.Context) ([]domain.CardType, error) {
	return r.types, r.call("ListCardTypes", nil)
}

func (r *childRepo) ListProjects(context.Context) ([]domain.Project, error) {
	return r.projects, r.call("ListProjects", nil)
}

func (r *childRepo) GetCard(_ context.Context, cardID uuid.UUID) (domain.Card, error) {
	if err := r.call("GetCard", cardID); err != nil {
		return domain.Card{}, err
	}
	c, ok := r.cards[cardID]
	if !ok {
		return c, domain.ErrNotFound
	}
	return c, nil
}

func (r *childRepo) ListChildCards(_ context.Context, parentID uuid.UUID) ([]domain.Card, error) {
	var out []domain.Card
	for _, c := range r.cards {
		if c.ParentCardID != nil && *c.ParentCardID == parentID {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b domain.Card) int { return strings.Compare(a.Title, b.Title) })
	return out, r.call("ListChildCards", parentID)
}

func (r *childRepo) ListCardFlowRuns(_ context.Context, cardID uuid.UUID) ([]domain.FlowRun, error) {
	return r.runs[cardID], r.call("ListCardFlowRuns", cardID)
}

func (r *childRepo) ListNodeRuns(_ context.Context, flowRunID uuid.UUID) ([]domain.NodeRun, error) {
	return r.nodes, r.call("ListNodeRuns", flowRunID)
}

func (r *childRepo) CreateChildCards(_ context.Context, cards []domain.ChildCard) ([]domain.Card, error) {
	out := make([]domain.Card, len(cards))
	for i, c := range cards {
		out[i] = domain.Card{ID: uuid.New(), Title: c.Title, LaneID: c.LaneID, CardTypeID: c.CardTypeID}
	}
	return out, r.call("CreateChildCards", cards)
}

func (r *childRepo) UpdateCard(_ context.Context, cardID uuid.UUID, in domain.CardInput) (domain.Card, error) {
	return domain.Card{ID: cardID, Description: in.Description}, r.call("UpdateCard", in)
}

// newChildService returns a service whose agent run is on card id, which
// creates cards of the types task and epic, with a child of it and another
// card that is not.
func newChildService() (*Service, *childRepo, *fakePub, AgentRun, domain.Card) {
	pub := &fakePub{}
	todo := uuid.New()
	parent := id
	child := domain.Card{ID: uuid.New(), Title: "API", ParentCardID: &parent, LaneID: laneID, CardTypeID: typeID}
	other := domain.Card{ID: uuid.New(), Title: "Other"}
	repo := &childRepo{
		fakeRepo: &fakeRepo{},
		types:    []domain.CardType{{ID: typeID, Name: "task", InitialLaneID: &todo}, {ID: epicID, Name: "epic"}, {ID: uuid.New(), Name: "bug", InitialLaneID: &todo}},
		projects: []domain.Project{{ID: uuid.New(), Name: "web"}},
		cards:    map[uuid.UUID]domain.Card{child.ID: child, other.ID: other},
		runs:     map[uuid.UUID][]domain.FlowRun{},
	}
	run := agentRun(domain.NodeAgent, `{"card_type_ids":["`+typeID.String()+`","`+epicID.String()+`"]}`)
	return New(repo, fakeGit{}, pub, fakeSealer{}), repo, pub, run, child
}

var epicID = uuid.New()

func TestCreateCards(t *testing.T) {
	s, repo, pub, run, _ := newChildService()
	specs := []CardSpec{
		{Type: " task ", ChildCardSpec: domain.ChildCardSpec{Ref: "ui", Title: "UI", Project: "web", DependsOn: []string{"api"}}},
		{Type: "task", ChildCardSpec: domain.ChildCardSpec{Ref: "api", Title: "API"}},
	}
	cards, err := s.CreateCards(ctx, run, specs)
	if err != nil || len(cards) != 2 || cards[0].Title != "UI" || cards[0].LaneID != *repo.types[0].InitialLaneID || cards[0].CardTypeID != typeID {
		t.Fatalf("CreateCards = %+v, %v", cards, err)
	}
	created := repo.ins["CreateChildCards"].([]domain.ChildCard)
	if *created[0].ParentCardID != id || !slices.Equal(created[0].After, []int{1}) || *created[0].ProjectID != repo.projects[0].ID {
		t.Fatalf("created %+v", created)
	}
	if got := eventTypes(pub); !slices.Equal(got, []string{"card.created", "card.created", "card_event.created"}) {
		t.Fatalf("events %q", got)
	}
	p := repo.ins["AddCardEvent:children.created"].(childrenCreatedPayload)
	if p.FlowRunID != run.FlowRun.ID || p.NodeKey != "dev" || len(p.Cards) != 2 || p.Cards[0].Project != "web" {
		t.Fatalf("timeline %+v", p)
	}
}

func TestCreateCardsOfOneType(t *testing.T) {
	s, _, _, run, _ := newChildService()
	run.Node.Config = json.RawMessage(`{"card_type_ids":["` + typeID.String() + `"]}`)
	cards, err := s.CreateCards(ctx, run, []CardSpec{{ChildCardSpec: domain.ChildCardSpec{Title: "A"}}})
	if err != nil || cards[0].CardTypeID != typeID {
		t.Fatalf("CreateCards = %+v, %v", cards, err)
	}
	run.Node.Config = json.RawMessage(`{"card_type_ids":["` + uuid.NewString() + `"]}`)
	if _, err := s.CreateCards(ctx, run, []CardSpec{{}}); err == nil || !strings.HasPrefix(err.Error(), "nenhum dos tipos de card deste passo existe mais") {
		t.Fatalf("no types: %v", err)
	}
}

func TestCreateCardsFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		specs []CardSpec
		errs  map[string]error
		want  string
	}{
		{name: "no cards", want: "informe ao menos um card"},
		{name: "unknown type", specs: []CardSpec{{Type: "bug", ChildCardSpec: domain.ChildCardSpec{Title: "A"}}}, want: `o card "A" usa o tipo "bug", mas este passo cria cards dos tipos: task, epic`},
		{name: "no type", specs: []CardSpec{{ChildCardSpec: domain.ChildCardSpec{Title: "A"}}}, want: `o card "A" usa o tipo "", mas este passo`},
		{name: "no initial lane", specs: []CardSpec{{Type: "epic", ChildCardSpec: domain.ChildCardSpec{Title: "A"}}}, want: `o tipo "epic" não tem baia inicial`},
		{name: "unknown dependency", specs: []CardSpec{{Type: "task", ChildCardSpec: domain.ChildCardSpec{Title: "A"}}},
			errs: map[string]error{"CreateChildCards": domain.ErrUnknownDependency}, want: "um card depende de um id que não é de nenhum card"},
		{name: "store", specs: []CardSpec{{Type: "task", ChildCardSpec: domain.ChildCardSpec{Title: "A"}}},
			errs: map[string]error{"CreateChildCards": errRepo}, want: errRepo.Error()},
		{name: "timeline", specs: []CardSpec{{Type: "task", ChildCardSpec: domain.ChildCardSpec{Title: "A"}}},
			errs: map[string]error{"AddCardEvent:children.created": errRepo}, want: errRepo.Error()},
		{name: "types", specs: []CardSpec{{}}, errs: map[string]error{"ListCardTypes": errRepo}, want: errRepo.Error()},
		{name: "projects", specs: []CardSpec{{}}, errs: map[string]error{"ListProjects": errRepo}, want: errRepo.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo, _, run, _ := newChildService()
			repo.errs = tc.errs
			if _, err := s.CreateCards(ctx, run, tc.specs); err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestListChildren(t *testing.T) {
	s, repo, _, run, child := newChildService()
	second := domain.Card{ID: uuid.New(), Title: "Web", ParentCardID: child.ParentCardID}
	repo.cards[second.ID] = second
	repo.runs[child.ID] = []domain.FlowRun{{ID: uuid.New(), FlowName: "dev", Status: domain.RunFailed, Error: "boom"}}
	repo.nodes = []domain.NodeRun{
		{Output: json.RawMessage(`{"result":"first"}`)},
		{Output: json.RawMessage(`{"result":"rejected: fix the tests"}`)},
		{},
	}
	children, err := s.ListChildren(ctx, run)
	if err != nil || len(children) != 2 {
		t.Fatalf("ListChildren = %+v, %v", children, err)
	}
	api := children[0]
	if api.TypeName != "task" || api.LaneName != "" || *api.LastRun != (ChildRun{FlowName: "dev", Status: domain.RunFailed, Error: "boom", Result: "rejected: fix the tests"}) {
		t.Fatalf("api %+v %+v", api, api.LastRun)
	}
	if children[1].LastRun != nil {
		t.Fatalf("web %+v", children[1])
	}
	repo.nodes = []domain.NodeRun{{}}
	if children, _ = s.ListChildren(ctx, run); children[0].LastRun.Result != "" {
		t.Fatalf("result %+v", children[0].LastRun)
	}
	for _, method := range []string{"ListChildCards", "ListCardTypes", "ListLanes", "ListCardFlowRuns", "ListNodeRuns"} {
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.ListChildren(ctx, run); !errors.Is(err, errRepo) {
			t.Fatalf("%s: %v", method, err)
		}
	}
}

func TestChildMustBeAChild(t *testing.T) {
	s, repo, _, run, _ := newChildService()
	var other uuid.UUID
	for cardID, c := range repo.cards {
		if c.ParentCardID == nil {
			other = cardID
		}
	}
	foreign := domain.Card{ID: uuid.New(), ParentCardID: &other}
	repo.cards[foreign.ID] = foreign
	for _, cardID := range []uuid.UUID{other, foreign.ID, uuid.New()} {
		if _, err := s.CancelChild(ctx, run, cardID); err == nil || !strings.Contains(err.Error(), "não é filho deste card") {
			t.Fatalf("%s: %v", cardID, err)
		}
		if _, err := s.RetryChild(ctx, run, cardID, ""); err == nil || !strings.Contains(err.Error(), "não é filho deste card") {
			t.Fatalf("%s: %v", cardID, err)
		}
	}
	repo.errs = map[string]error{"GetCard": errRepo}
	if _, err := s.CancelChild(ctx, run, other); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}

func TestRetryChild(t *testing.T) {
	s, repo, pub, run, child := newChildService()
	if _, err := s.RetryChild(ctx, run, child.ID, ""); err == nil || !strings.Contains(err.Error(), "ainda não rodou nenhum fluxo") {
		t.Fatalf("no runs: %v", err)
	}
	repo.runs[child.ID] = []domain.FlowRun{{Status: domain.RunRunning}}
	if _, err := s.RetryChild(ctx, run, child.ID, ""); err == nil || !strings.Contains(err.Error(), "ainda está rodando") {
		t.Fatalf("running: %v", err)
	}
	repo.runs[child.ID] = []domain.FlowRun{{Status: domain.RunFailed}}
	if _, err := s.RetryChild(ctx, run, child.ID, ""); err != nil || slices.Contains(repo.calls, "UpdateCard") {
		t.Fatalf("plain retry: %v %q", err, repo.calls)
	}
	if _, err := s.RetryChild(ctx, run, child.ID, " use the v2 API "); err != nil {
		t.Fatal(err)
	}
	if in := repo.ins["UpdateCard"].(domain.CardInput); in.Description != "use the v2 API" || in.Title != "API" || in.CardTypeID != typeID {
		t.Fatalf("update %+v", in)
	}
	if got := eventTypes(pub); !slices.Equal(got, []string{"card.updated", "card.updated", "card.updated"}) {
		t.Fatalf("events %q", got)
	}
	for _, method := range []string{"ListCardFlowRuns", "UpdateCard", "RetryCard"} {
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.RetryChild(ctx, run, child.ID, "new"); !errors.Is(err, errRepo) {
			t.Fatalf("%s: %v", method, err)
		}
	}
}

func TestCancelChild(t *testing.T) {
	s, repo, pub, run, child := newChildService()
	if card, err := s.CancelChild(ctx, run, child.ID); err != nil || card.ID != child.ID {
		t.Fatalf("no runs: %+v %v", card, err)
	}
	repo.runs[child.ID] = []domain.FlowRun{{ID: uuid.New(), Status: domain.RunDone}}
	if _, err := s.CancelChild(ctx, run, child.ID); err != nil || slices.Contains(repo.calls, "CancelFlowRun") {
		t.Fatalf("ended run: %v", err)
	}
	active := uuid.New()
	repo.runs[child.ID] = []domain.FlowRun{{ID: active, Status: domain.RunWaiting}}
	if _, err := s.CancelChild(ctx, run, child.ID); err != nil || repo.ins["CancelFlowRun"] != active {
		t.Fatalf("active run: %v", err)
	}
	if got := eventTypes(pub); !slices.Contains(got, "flow_run.updated") || got[0] != "card.updated" {
		t.Fatalf("events %q", got)
	}
	for _, method := range []string{"CancelCard", "ListCardFlowRuns", "CancelFlowRun"} {
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.CancelChild(ctx, run, child.ID); !errors.Is(err, errRepo) {
			t.Fatalf("%s: %v", method, err)
		}
	}
}
