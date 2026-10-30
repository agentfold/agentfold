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

// taskRepo backs the task use cases: card types, cards (children of the
// run card id), their runs and the node runs of those runs.
type taskRepo struct {
	*fakeRepo
	types    []domain.CardType
	projects []domain.Project
	cards    map[uuid.UUID]domain.Card
	runs     map[uuid.UUID][]domain.FlowRun
	nodes    []domain.NodeRun
}

func (r *taskRepo) ListCardTypes(context.Context) ([]domain.CardType, error) {
	return r.types, r.call("ListCardTypes", nil)
}

func (r *taskRepo) ListProjects(context.Context) ([]domain.Project, error) {
	return r.projects, r.call("ListProjects", nil)
}

func (r *taskRepo) GetCard(_ context.Context, cardID uuid.UUID) (domain.Card, error) {
	if err := r.call("GetCard", cardID); err != nil {
		return domain.Card{}, err
	}
	c, ok := r.cards[cardID]
	if !ok {
		return c, domain.ErrNotFound
	}
	return c, nil
}

func (r *taskRepo) ListChildCards(_ context.Context, parentID uuid.UUID) ([]domain.Card, error) {
	var out []domain.Card
	for _, c := range r.cards {
		if c.ParentCardID != nil && *c.ParentCardID == parentID {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b domain.Card) int { return strings.Compare(a.Title, b.Title) })
	return out, r.call("ListChildCards", parentID)
}

func (r *taskRepo) ListCardFlowRuns(_ context.Context, cardID uuid.UUID) ([]domain.FlowRun, error) {
	return r.runs[cardID], r.call("ListCardFlowRuns", cardID)
}

func (r *taskRepo) ListNodeRuns(_ context.Context, flowRunID uuid.UUID) ([]domain.NodeRun, error) {
	return r.nodes, r.call("ListNodeRuns", flowRunID)
}

func (r *taskRepo) CreateChildCards(_ context.Context, cards []domain.ChildCard) ([]domain.Card, error) {
	out := make([]domain.Card, len(cards))
	for i, c := range cards {
		out[i] = domain.Card{ID: uuid.New(), Title: c.Title, LaneID: c.LaneID, CardTypeID: c.CardTypeID}
	}
	return out, r.call("CreateChildCards", cards)
}

func (r *taskRepo) UpdateCard(_ context.Context, cardID uuid.UUID, in domain.CardInput) (domain.Card, error) {
	return domain.Card{ID: cardID, Description: in.Description}, r.call("UpdateCard", in)
}

// newTaskService returns a service whose agent run is on card id, with a
// task child of it and another card that is not.
func newTaskService() (*Service, *taskRepo, *fakePub, AgentRun, domain.Card) {
	pub := &fakePub{}
	todo := uuid.New()
	parent := id
	child := domain.Card{ID: uuid.New(), Title: "API", ParentCardID: &parent, LaneID: laneID, CardTypeID: typeID}
	other := domain.Card{ID: uuid.New(), Title: "Other"}
	repo := &taskRepo{
		fakeRepo: &fakeRepo{},
		types:    []domain.CardType{{ID: typeID, Name: "task", InitialLaneID: &todo}, {ID: uuid.New(), Name: "epic"}},
		projects: []domain.Project{{ID: uuid.New(), Name: "web"}},
		cards:    map[uuid.UUID]domain.Card{child.ID: child, other.ID: other},
		runs:     map[uuid.UUID][]domain.FlowRun{},
	}
	run := agentRun(domain.NodeAgent, `{}`)
	return New(repo, fakeGit{}, pub, fakeSealer{}), repo, pub, run, child
}

func TestCreateTasks(t *testing.T) {
	s, repo, pub, run, _ := newTaskService()
	specs := []TaskSpec{
		{Type: " task ", ChildCardSpec: domain.ChildCardSpec{Ref: "ui", Title: "UI", Project: "web", DependsOn: []string{"api"}}},
		{Type: "task", ChildCardSpec: domain.ChildCardSpec{Ref: "api", Title: "API"}},
	}
	cards, err := s.CreateTasks(ctx, run, specs)
	if err != nil || len(cards) != 2 || cards[0].Title != "UI" || cards[0].LaneID != *repo.types[0].InitialLaneID || cards[0].CardTypeID != typeID {
		t.Fatalf("CreateTasks = %+v, %v", cards, err)
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

func TestCreateTasksFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		specs []TaskSpec
		errs  map[string]error
		want  string
	}{
		{name: "no tasks", want: "informe ao menos uma task"},
		{name: "unknown type", specs: []TaskSpec{{Type: "bug", ChildCardSpec: domain.ChildCardSpec{Title: "A"}}}, want: `a task "A" usa o tipo "bug", que não existe (tipos: task, epic)`},
		{name: "no initial lane", specs: []TaskSpec{{Type: "epic", ChildCardSpec: domain.ChildCardSpec{Title: "A"}}}, want: `o tipo "epic" não tem baia inicial`},
		{name: "unknown dependency", specs: []TaskSpec{{Type: "task", ChildCardSpec: domain.ChildCardSpec{Title: "A"}}},
			errs: map[string]error{"CreateChildCards": domain.ErrUnknownDependency}, want: "uma task depende de um id que não é de nenhum card"},
		{name: "store", specs: []TaskSpec{{Type: "task", ChildCardSpec: domain.ChildCardSpec{Title: "A"}}},
			errs: map[string]error{"CreateChildCards": errRepo}, want: errRepo.Error()},
		{name: "timeline", specs: []TaskSpec{{Type: "task", ChildCardSpec: domain.ChildCardSpec{Title: "A"}}},
			errs: map[string]error{"AddCardEvent:children.created": errRepo}, want: errRepo.Error()},
		{name: "types", specs: []TaskSpec{{}}, errs: map[string]error{"ListCardTypes": errRepo}, want: errRepo.Error()},
		{name: "projects", specs: []TaskSpec{{}}, errs: map[string]error{"ListProjects": errRepo}, want: errRepo.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo, _, run, _ := newTaskService()
			repo.errs = tc.errs
			if _, err := s.CreateTasks(ctx, run, tc.specs); err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestListTasks(t *testing.T) {
	s, repo, _, run, child := newTaskService()
	second := domain.Card{ID: uuid.New(), Title: "Web", ParentCardID: child.ParentCardID}
	repo.cards[second.ID] = second
	repo.runs[child.ID] = []domain.FlowRun{{ID: uuid.New(), FlowName: "dev", Status: domain.RunFailed, Error: "boom"}}
	repo.nodes = []domain.NodeRun{
		{Output: json.RawMessage(`{"result":"first"}`)},
		{Output: json.RawMessage(`{"result":"rejected: fix the tests"}`)},
		{},
	}
	tasks, err := s.ListTasks(ctx, run)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("ListTasks = %+v, %v", tasks, err)
	}
	api := tasks[0]
	if api.TypeName != "task" || api.LaneName != "" || *api.LastRun != (TaskRun{FlowName: "dev", Status: domain.RunFailed, Error: "boom", Result: "rejected: fix the tests"}) {
		t.Fatalf("api %+v %+v", api, api.LastRun)
	}
	if tasks[1].LastRun != nil {
		t.Fatalf("web %+v", tasks[1])
	}
	repo.nodes = []domain.NodeRun{{}}
	if tasks, _ = s.ListTasks(ctx, run); tasks[0].LastRun.Result != "" {
		t.Fatalf("result %+v", tasks[0].LastRun)
	}
	for _, method := range []string{"ListChildCards", "ListCardTypes", "ListLanes", "ListCardFlowRuns", "ListNodeRuns"} {
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.ListTasks(ctx, run); !errors.Is(err, errRepo) {
			t.Fatalf("%s: %v", method, err)
		}
	}
}

func TestTaskMustBeAChild(t *testing.T) {
	s, repo, _, run, _ := newTaskService()
	var other uuid.UUID
	for cardID, c := range repo.cards {
		if c.ParentCardID == nil {
			other = cardID
		}
	}
	foreign := domain.Card{ID: uuid.New(), ParentCardID: &other}
	repo.cards[foreign.ID] = foreign
	for _, cardID := range []uuid.UUID{other, foreign.ID, uuid.New()} {
		if _, err := s.CancelTask(ctx, run, cardID); err == nil || !strings.Contains(err.Error(), "não é uma task deste card") {
			t.Fatalf("%s: %v", cardID, err)
		}
		if _, err := s.RetryTask(ctx, run, cardID, ""); err == nil || !strings.Contains(err.Error(), "não é uma task deste card") {
			t.Fatalf("%s: %v", cardID, err)
		}
	}
	repo.errs = map[string]error{"GetCard": errRepo}
	if _, err := s.CancelTask(ctx, run, other); !errors.Is(err, errRepo) {
		t.Fatalf("got %v", err)
	}
}

func TestRetryTask(t *testing.T) {
	s, repo, pub, run, child := newTaskService()
	if _, err := s.RetryTask(ctx, run, child.ID, ""); err == nil || !strings.Contains(err.Error(), "ainda não rodou nenhum fluxo") {
		t.Fatalf("no runs: %v", err)
	}
	repo.runs[child.ID] = []domain.FlowRun{{Status: domain.RunRunning}}
	if _, err := s.RetryTask(ctx, run, child.ID, ""); err == nil || !strings.Contains(err.Error(), "ainda está rodando") {
		t.Fatalf("running: %v", err)
	}
	repo.runs[child.ID] = []domain.FlowRun{{Status: domain.RunFailed}}
	if _, err := s.RetryTask(ctx, run, child.ID, ""); err != nil || slices.Contains(repo.calls, "UpdateCard") {
		t.Fatalf("plain retry: %v %q", err, repo.calls)
	}
	if _, err := s.RetryTask(ctx, run, child.ID, " use the v2 API "); err != nil {
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
		if _, err := s.RetryTask(ctx, run, child.ID, "new"); !errors.Is(err, errRepo) {
			t.Fatalf("%s: %v", method, err)
		}
	}
}

func TestCancelTask(t *testing.T) {
	s, repo, pub, run, child := newTaskService()
	if card, err := s.CancelTask(ctx, run, child.ID); err != nil || card.ID != child.ID {
		t.Fatalf("no runs: %+v %v", card, err)
	}
	repo.runs[child.ID] = []domain.FlowRun{{ID: uuid.New(), Status: domain.RunDone}}
	if _, err := s.CancelTask(ctx, run, child.ID); err != nil || slices.Contains(repo.calls, "CancelFlowRun") {
		t.Fatalf("ended run: %v", err)
	}
	active := uuid.New()
	repo.runs[child.ID] = []domain.FlowRun{{ID: active, Status: domain.RunWaiting}}
	if _, err := s.CancelTask(ctx, run, child.ID); err != nil || repo.ins["CancelFlowRun"] != active {
		t.Fatalf("active run: %v", err)
	}
	if got := eventTypes(pub); !slices.Contains(got, "flow_run.updated") || got[0] != "card.updated" {
		t.Fatalf("events %q", got)
	}
	for _, method := range []string{"CancelCard", "ListCardFlowRuns", "CancelFlowRun"} {
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.CancelTask(ctx, run, child.ID); !errors.Is(err, errRepo) {
			t.Fatalf("%s: %v", method, err)
		}
	}
}
