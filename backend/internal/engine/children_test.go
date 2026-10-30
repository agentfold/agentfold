package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func (f *fakeStore) ListProjects(context.Context) ([]domain.Project, error) {
	return f.projects, f.err("ListProjects")
}

func (f *fakeStore) CreateChildCards(_ context.Context, cards []domain.ChildCard) ([]domain.Card, error) {
	if err := f.err("CreateChildCards"); err != nil {
		return nil, err
	}
	out := make([]domain.Card, len(cards))
	for i, in := range cards {
		if !f.lanes[in.LaneID] {
			return nil, fmt.Errorf("%w: lane", domain.ErrInvalidReference)
		}
		out[i] = domain.Card{
			ID: uuid.New(), Title: in.Title, Description: in.Description, CardTypeID: in.CardTypeID,
			ProjectID: in.ProjectID, ParentCardID: in.ParentCardID, LaneID: in.LaneID, DependsOn: in.DependsOn,
		}
	}
	for i, in := range cards {
		for _, j := range in.After {
			out[i].DependsOn = append(out[i].DependsOn, out[j].ID)
		}
	}
	for _, c := range out {
		f.cards[c.ID] = c
		f.children = append(f.children, c.ID)
	}
	return out, nil
}

func (f *fakeStore) ListChildCards(_ context.Context, parentID uuid.UUID) ([]domain.Card, error) {
	var out []domain.Card
	for _, id := range f.children {
		if c := f.cards[id]; c.ParentCardID != nil && *c.ParentCardID == parentID {
			out = append(out, c)
		}
	}
	return out, f.err("ListChildCards")
}

func (f *fakeStore) ListCardFlowRuns(_ context.Context, cardID uuid.UUID) ([]domain.FlowRun, error) {
	return f.cardRuns[cardID], f.err("ListCardFlowRuns")
}

func (f *fakeStore) WaitNodeRun(_ context.Context, id uuid.UUID) (domain.NodeRun, error) {
	if err := f.err("WaitNodeRun"); err != nil {
		return domain.NodeRun{}, err
	}
	nr := f.nodeRuns[id]
	nr.Status = domain.NodeWaiting
	f.nodeRuns[id] = nr
	return nr, nil
}

func (f *fakeStore) WaitingNodeRun(_ context.Context, cardID uuid.UUID) (domain.NodeRun, error) {
	if err := f.err("WaitingNodeRun"); err != nil {
		return domain.NodeRun{}, err
	}
	for _, id := range f.order {
		nr := f.nodeRuns[id]
		if r := f.runs[nr.FlowRunID]; r.CardID == cardID && r.Status == domain.RunWaiting && nr.Status == domain.NodeWaiting {
			return nr, nil
		}
	}
	return domain.NodeRun{}, domain.ErrNotFound
}

func (f *fakeStore) SettleWait(_ context.Context, id uuid.UUID, r domain.NodeResult) (domain.NodeRun, bool, error) {
	if err := f.err("SettleWait"); err != nil {
		return domain.NodeRun{}, false, err
	}
	nr := f.nodeRuns[id]
	if nr.Status != domain.NodeWaiting || f.unsettled {
		return nr, false, nil
	}
	run := f.runs[nr.FlowRunID]
	run.Status = domain.RunRunning
	f.runs[run.ID] = run
	nr, err := f.FinishNodeRun(context.Background(), id, r)
	return nr, true, err
}

// childEnv runs, for the env card, a flow that splits it into child cards
// (from the agent's structured output) and waits for them in lane done.
type childEnv struct {
	*env
	done uuid.UUID
}

func newChildEnv(t *testing.T) *childEnv {
	t.Helper()
	e := &childEnv{env: newEnv(t), done: uuid.New()}
	e.store.lanes[e.done] = true
	e.store.projects = []domain.Project{{ID: uuid.New(), Name: "chat-backend"}, {ID: uuid.New(), Name: "chat-web"}}
	e.addFlow("split", graph{
		nodes: []string{"t:trigger", "split:agent", "create:create_cards", "wait:wait_children", "move:move_card", "end:end"},
		configs: map[string]any{
			"t":      triggerCfg(domain.EventCardCreated, nil),
			"create": map[string]any{"lane_id": e.todo, "card_type_id": uuid.New()},
			"wait":   map[string]any{"done_lane_id": e.done},
			"move":   map[string]any{"lane_id": e.done},
		},
		edges: []string{"t.next>split", "split.done>create", "create.next>wait", "wait.all_done>move", "move.next>end"},
	})
	return e
}

// split runs the flow up to the agent, which hands on cards.
func (e *childEnv) split(cards string) error {
	e.t.Helper()
	e.trigger(e.created())
	e.agents.result = func(domain.NodeRun) domain.NodeResult {
		return domain.NodeResult{Status: domain.NodeDone, OutputHandle: "done", Output: json.RawMessage(`{"result":"r","output":{"cards":` + cards + `}}`)}
	}
	return e.runAgent()
}

// childMoved moves a child card to lane and handles its trigger job.
func (e *childEnv) childMoved(child uuid.UUID, lane uuid.UUID) error {
	c := e.store.cards[child]
	from := c.LaneID
	c.LaneID = lane
	e.store.cards[child] = c
	data, _ := json.Marshal(map[string]uuid.UUID{"from_lane_id": from, "to_lane_id": lane})
	ev := domain.CardEvent{ID: uuid.New(), CardID: child, Type: domain.CardMoved, Payload: data}
	e.store.events[ev.ID] = ev
	job, _ := json.Marshal(domain.TriggerJob{CardEventID: ev.ID})
	return e.engine.HandleTrigger(context.Background(), job)
}

func (e *childEnv) nodeRun(key string) domain.NodeRun {
	e.t.Helper()
	for i := len(e.store.order) - 1; i >= 0; i-- {
		if nr := e.store.nodeRuns[e.store.order[i]]; nr.NodeKey == key {
			return nr
		}
	}
	e.t.Fatalf("no run of node %s", key)
	return domain.NodeRun{}
}

func TestCreateCardsAndWaitChildren(t *testing.T) {
	e := newChildEnv(t)
	if err := e.split(`[{"title":"API","description":"Go","project":"chat-backend"},{"title":"Web"}]`); err != nil {
		t.Fatal(err)
	}
	if len(e.store.children) != 2 {
		t.Fatalf("children %v", e.store.children)
	}
	api, web := e.store.cards[e.store.children[0]], e.store.cards[e.store.children[1]]
	if *api.ParentCardID != e.card.ID || *api.ProjectID != e.store.projects[0].ID || api.Description != "Go" || api.LaneID != e.todo || web.ProjectID != nil {
		t.Fatalf("api %+v, web %+v", api, web)
	}
	if out := string(e.nodeRun("create").Output); !strings.Contains(out, `"title":"API","project":"chat-backend"`) {
		t.Fatalf("create output %s", out)
	}
	if e.onlyRunOf(e.card.ID).Status != domain.RunWaiting || e.nodeRun("wait").Status != domain.NodeWaiting {
		t.Fatal("the run does not wait for the children")
	}
	e.wantTimeline("flow.started", "node.started split", "node.finished split done", "node.started create",
		"children.created create", "node.finished create done", "node.started wait")
	if got := e.pub.types(); !slices.Contains(got, "card.created") || got[len(got)-1] != "flow_run.updated" {
		t.Fatalf("published %q", got)
	}

	// One child done is not enough; a child moving elsewhere changes nothing.
	if err := e.childMoved(api.ID, e.done); err != nil {
		t.Fatal(err)
	}
	if err := e.childMoved(web.ID, e.doing); err != nil {
		t.Fatal(err)
	}
	if e.onlyRunOf(e.card.ID).Status != domain.RunWaiting {
		t.Fatal("the run went on before every child was done")
	}
	if err := e.childMoved(web.ID, e.done); err != nil {
		t.Fatal(err)
	}
	if run := e.onlyRunOf(e.card.ID); run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
	wait := e.nodeRun("wait")
	if wait.OutputHandle != "all_done" || !strings.Contains(string(wait.Output), `"state":"done"`) {
		t.Fatalf("wait %+v", wait)
	}
	if e.store.cards[e.card.ID].LaneID != e.done {
		t.Fatal("the parent did not move on")
	}
}

// onlyRunOf returns the single run of a card.
func (e *env) onlyRunOf(cardID uuid.UUID) domain.FlowRun {
	e.t.Helper()
	var out []domain.FlowRun
	for _, r := range e.store.runs {
		if r.CardID == cardID {
			out = append(out, r)
		}
	}
	if len(out) != 1 {
		e.t.Fatalf("got %d runs, want 1", len(out))
	}
	return out[0]
}

func TestFailedChildSettlesTheWait(t *testing.T) {
	e := newChildEnv(t)
	if err := e.split(`[{"title":"API"},{"title":"Web"}]`); err != nil {
		t.Fatal(err)
	}
	child := e.store.children[1]
	e.store.cardRuns = map[uuid.UUID][]domain.FlowRun{child: {{Status: domain.RunCanceled}, {Status: domain.RunDone}}}
	payload, _ := json.Marshal(domain.ChildChangedJob{CardID: child})
	if err := e.engine.HandleChildChanged(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	wait := e.nodeRun("wait")
	if wait.OutputHandle != "some_failed" || !strings.Contains(string(wait.Output), `"state":"failed"`) {
		t.Fatalf("wait %+v", wait)
	}
	// some_failed leads nowhere here: the run ends.
	if run := e.onlyRunOf(e.card.ID); run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
}

func TestChildFlowFinishingWakesTheParent(t *testing.T) {
	e := newChildEnv(t)
	if err := e.split(`[{"title":"API"}]`); err != nil {
		t.Fatal(err)
	}
	child := e.store.cards[e.store.children[0]]
	// The child's own flow moves it to done and ends: both wake the parent,
	// which settles once.
	childRun := domain.FlowRun{ID: uuid.New(), CardID: child.ID, Status: domain.RunRunning}
	e.store.runs[childRun.ID] = childRun
	child.LaneID = e.done
	e.store.cards[child.ID] = child
	if err := e.engine.finish(context.Background(), &run{FlowRun: childRun}, domain.RunDone, ""); err != nil {
		t.Fatal(err)
	}
	if e.onlyRunOf(e.card.ID).Status != domain.RunDone {
		t.Fatal("the parent did not wake up")
	}
	if err := e.childMoved(child.ID, e.done); err != nil {
		t.Fatal(err)
	}
}

func TestWaitWithoutChildrenGoesOn(t *testing.T) {
	e := newEnv(t)
	done := e.doing
	e.addFlow("f", graph{
		nodes:   []string{"t:trigger", "wait:wait_children"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil), "wait": map[string]any{"done_lane_id": done}},
		edges:   []string{"t.next>wait"},
	})
	e.trigger(e.created())
	if run := e.onlyRun(); run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
	if nr := e.store.lastNodeRun(); nr.OutputHandle != "all_done" {
		t.Fatalf("wait %+v", nr)
	}
}

func TestCreateCardsFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cards string
		setup func(e *childEnv)
		msg   string
	}{
		{"no cards", `[]`, nil, "a entrada não tem cards para criar"},
		{"unknown project", `[{"title":"A","project":"chat-app"}]`, nil, `o card "A" usa o projeto "chat-app", que não existe (projetos: chat-backend, chat-web)`},
		{"lane gone", `[{"title":"A"}]`, func(e *childEnv) { delete(e.store.lanes, e.todo) }, "a baia ou o tipo dos cards criados não existe mais"},
		{"unknown dependency", `[{"title":"A"}]`, func(e *childEnv) {
			e.store.errs["CreateChildCards"] = fmt.Errorf("%w: %w", domain.ErrInvalidReference, domain.ErrUnknownDependency)
		}, "um card da lista depende de um card que não existe"},
		{"bad config", `[{"title":"A"}]`, func(e *childEnv) {
			e.store.flows[0].Version.Graph.Nodes[2].Config = json.RawMessage(`[]`)
		}, "configuração inválida"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newChildEnv(t)
			if tc.setup != nil {
				tc.setup(e)
			}
			if err := e.split(tc.cards); err != nil {
				t.Fatal(err)
			}
			if run := e.onlyRun(); run.Status != domain.RunFailed || !strings.Contains(run.Error, tc.msg) {
				t.Fatalf("run %+v", run)
			}
		})
	}
}

func TestBadWaitConfigFails(t *testing.T) {
	e := newChildEnv(t)
	e.store.flows[0].Version.Graph.Nodes[3].Config = json.RawMessage(`[]`)
	if err := e.split(`[{"title":"A"}]`); err != nil {
		t.Fatal(err)
	}
	if nr := e.nodeRun("wait"); nr.OutputHandle != "some_failed" || nr.Status != domain.NodeFailed {
		t.Fatalf("wait %+v", nr)
	}
	if run := e.onlyRun(); run.Status != domain.RunFailed {
		t.Fatalf("run %+v", run)
	}
}

func TestChildStoreErrors(t *testing.T) {
	for _, tc := range []struct {
		method string
		after  int
		// late fails when a child settles the wait, not on the way there.
		late bool
	}{
		{method: "ListProjects"},
		{method: "CreateChildCards"},
		{method: "AddCardEvent:children.created"},
		{method: "ListChildCards"},
		{method: "ListCardFlowRuns"},
		{method: "WaitNodeRun"},
		{method: "WaitFlowRun"},
		{method: "WaitingNodeRun"},
		{method: "GetFlowRun", after: 1},
		{method: "GetFlowVersion", after: 1},
		{method: "ListChildCards", late: true},
		{method: "SettleWait", late: true},
		{method: "GetCard", late: true},
	} {
		t.Run(fmt.Sprintf("%s after %d late %v", tc.method, tc.after, tc.late), func(t *testing.T) {
			e := newChildEnv(t)
			if tc.late {
				if err := e.split(`[{"title":"A"}]`); err != nil {
					t.Fatal(err)
				}
			}
			e.store.errs[tc.method] = errBoom
			e.store.failAfter[tc.method] = tc.after
			var err error
			if tc.late {
				err = e.childMoved(e.store.children[0], e.done)
			} else {
				err = e.split(`[{"title":"A"}]`)
			}
			if !errors.Is(err, errBoom) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestSettlingRaces(t *testing.T) {
	// The run ended while the wait was being held.
	e := newChildEnv(t)
	e.store.errs["WaitFlowRun"] = domain.ErrNotFound
	if err := e.split(`[{"title":"A"}]`); err != nil {
		t.Fatal(err)
	}
	// Another job settled the wait first.
	e = newChildEnv(t)
	if err := e.split(`[{"title":"A"}]`); err != nil {
		t.Fatal(err)
	}
	e.store.unsettled = true
	if err := e.childMoved(e.store.children[0], e.done); err != nil {
		t.Fatal(err)
	}
	if e.onlyRunOf(e.card.ID).Status != domain.RunWaiting {
		t.Fatal("a wait settled elsewhere was followed")
	}
}

func TestChildChangedJobs(t *testing.T) {
	e := newChildEnv(t)
	if err := e.engine.HandleChildChanged(context.Background(), json.RawMessage(`[]`)); err == nil {
		t.Fatal("bad payload accepted")
	}
	payload, _ := json.Marshal(domain.ChildChangedJob{CardID: e.card.ID})
	if err := e.engine.HandleChildChanged(context.Background(), payload); err != nil {
		t.Fatalf("card without parent: %v", err)
	}
	e.store.errs["GetCard"] = domain.ErrNotFound
	if err := e.engine.HandleChildChanged(context.Background(), payload); err != nil {
		t.Fatalf("deleted card: %v", err)
	}
	e.store.errs["GetCard"] = errBoom
	if err := e.engine.HandleChildChanged(context.Background(), payload); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
}

func TestFinishingTheRunOfADeletedCard(t *testing.T) {
	e := newEnv(t)
	e.addFlow("f", graph{
		nodes:   []string{"t:trigger", "end:end"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil)},
		edges:   []string{"t.next>end"},
	})
	e.store.errs["GetCard"] = domain.ErrNotFound
	e.store.failAfter["GetCard"] = 1
	if err := e.engine.HandleTrigger(context.Background(), e.created()); err != nil {
		t.Fatal(err)
	}
	if run := e.onlyRun(); run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
}
