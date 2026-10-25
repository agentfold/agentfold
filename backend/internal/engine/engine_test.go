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

	"orquestrator/internal/agent"
	"orquestrator/internal/domain"
	"orquestrator/internal/events"
	"orquestrator/internal/store"
)

var errBoom = errors.New("boom")

// fakeStore keeps the board in memory; errs fails methods by name, and
// failAfter lets a method succeed that many times before failing.
type fakeStore struct {
	events    map[uuid.UUID]domain.CardEvent
	timeline  []domain.CardEvent
	cards     map[uuid.UUID]domain.Card
	lanes     map[uuid.UUID]bool
	flows     []domain.RunnableFlow
	runs      map[uuid.UUID]domain.FlowRun
	nodeRuns  map[uuid.UUID]domain.NodeRun
	order     []uuid.UUID // node runs in creation order
	continued map[uuid.UUID]bool
	jobs      []store.NewJob
	errs      map[string]error
	failAfter map[string]int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		events:    map[uuid.UUID]domain.CardEvent{},
		cards:     map[uuid.UUID]domain.Card{},
		lanes:     map[uuid.UUID]bool{},
		runs:      map[uuid.UUID]domain.FlowRun{},
		nodeRuns:  map[uuid.UUID]domain.NodeRun{},
		continued: map[uuid.UUID]bool{},
		errs:      map[string]error{},
		failAfter: map[string]int{},
	}
}

func (f *fakeStore) err(name string) error {
	if n, ok := f.failAfter[name]; ok {
		if n > 0 {
			f.failAfter[name] = n - 1
			return nil
		}
		return f.errs[name]
	}
	return f.errs[name]
}

func (f *fakeStore) GetCardEvent(_ context.Context, id uuid.UUID) (domain.CardEvent, error) {
	if err := f.err("GetCardEvent"); err != nil {
		return domain.CardEvent{}, err
	}
	e, ok := f.events[id]
	if !ok {
		return e, domain.ErrNotFound
	}
	return e, nil
}

func (f *fakeStore) AddCardEvent(_ context.Context, cardID uuid.UUID, typ domain.CardEventType, payload any) (domain.CardEvent, error) {
	if err := f.err("AddCardEvent:" + string(typ)); err != nil {
		return domain.CardEvent{}, err
	}
	data, _ := json.Marshal(payload)
	e := domain.CardEvent{ID: uuid.New(), CardID: cardID, Type: typ, Payload: data}
	f.events[e.ID] = e
	f.timeline = append(f.timeline, e)
	return e, nil
}

func (f *fakeStore) GetCard(_ context.Context, id uuid.UUID) (domain.Card, error) {
	if err := f.err("GetCard"); err != nil {
		return domain.Card{}, err
	}
	return f.cards[id], nil
}

func (f *fakeStore) MoveCard(_ context.Context, id, laneID uuid.UUID, _ int) (domain.Card, error) {
	if err := f.err("MoveCard"); err != nil {
		return domain.Card{}, err
	}
	if !f.lanes[laneID] {
		return domain.Card{}, fmt.Errorf("%w: lane", domain.ErrInvalidReference)
	}
	card := f.cards[id]
	card.LaneID = laneID
	f.cards[id] = card
	return card, nil
}

func (f *fakeStore) RunnableFlows(context.Context) ([]domain.RunnableFlow, error) {
	return f.flows, f.err("RunnableFlows")
}

func (f *fakeStore) GetFlowVersion(_ context.Context, id uuid.UUID) (domain.FlowVersion, error) {
	if err := f.err("GetFlowVersion"); err != nil {
		return domain.FlowVersion{}, err
	}
	for _, fl := range f.flows {
		if fl.Version.ID == id {
			return fl.Version, nil
		}
	}
	return domain.FlowVersion{}, domain.ErrNotFound
}

func (f *fakeStore) StartFlowRun(_ context.Context, version domain.FlowVersion, cardID uuid.UUID, trigger string) (domain.FlowRun, bool, error) {
	if err := f.err("StartFlowRun"); err != nil {
		return domain.FlowRun{}, false, err
	}
	for _, r := range f.runs {
		if r.CardID == cardID && r.FlowID == version.FlowID && r.Status.Active() {
			return domain.FlowRun{}, false, nil
		}
	}
	r := domain.FlowRun{
		ID: uuid.New(), FlowID: version.FlowID, FlowVersionID: version.ID,
		CardID: cardID, Trigger: trigger, Status: domain.RunRunning,
	}
	f.runs[r.ID] = r
	return r, true, nil
}

func (f *fakeStore) GetFlowRun(_ context.Context, id uuid.UUID) (domain.FlowRun, error) {
	if err := f.err("GetFlowRun"); err != nil {
		return domain.FlowRun{}, err
	}
	r := f.runs[id]
	for _, fl := range f.flows {
		if fl.Version.FlowID == r.FlowID {
			r.FlowName = fl.Name
		}
	}
	return r, nil
}

func (f *fakeStore) FinishFlowRun(_ context.Context, id uuid.UUID, status domain.RunStatus, msg string) (domain.FlowRun, error) {
	if err := f.err("FinishFlowRun"); err != nil {
		return domain.FlowRun{}, err
	}
	r := f.runs[id]
	if !r.Status.Active() {
		return r, domain.ErrNotFound
	}
	r.Status, r.Error = status, msg
	f.runs[id] = r
	return r, nil
}

func (f *fakeStore) CountNodeRuns(_ context.Context, flowRunID uuid.UUID, nodeKey string) (int, error) {
	n := 0
	for _, nr := range f.nodeRuns {
		if nr.FlowRunID == flowRunID && nr.NodeKey == nodeKey {
			n++
		}
	}
	return n, f.err("CountNodeRuns")
}

func (f *fakeStore) CreateNodeRun(_ context.Context, flowRunID uuid.UUID, nodeKey string, iteration int, input json.RawMessage) (domain.NodeRun, error) {
	if err := f.err("CreateNodeRun"); err != nil {
		return domain.NodeRun{}, err
	}
	nr := domain.NodeRun{ID: uuid.New(), FlowRunID: flowRunID, NodeKey: nodeKey, Iteration: iteration, Status: domain.NodeQueued, Input: input}
	f.nodeRuns[nr.ID] = nr
	f.order = append(f.order, nr.ID)
	return nr, nil
}

func (f *fakeStore) FinishNodeRun(_ context.Context, id uuid.UUID, r domain.NodeResult) (domain.NodeRun, error) {
	if err := f.err("FinishNodeRun"); err != nil {
		return domain.NodeRun{}, err
	}
	nr := f.nodeRuns[id]
	nr.Status, nr.Output, nr.OutputHandle, nr.Error = r.Status, r.Output, r.OutputHandle, r.Error
	f.nodeRuns[id] = nr
	return nr, nil
}

func (f *fakeStore) ContinueNodeRun(_ context.Context, id uuid.UUID) (domain.NodeRun, bool, error) {
	if err := f.err("ContinueNodeRun"); err != nil {
		return domain.NodeRun{}, false, err
	}
	nr := f.nodeRuns[id]
	if f.continued[id] || (nr.Status != domain.NodeDone && nr.Status != domain.NodeFailed) {
		return domain.NodeRun{}, false, nil
	}
	f.continued[id] = true
	return nr, true, nil
}

func (f *fakeStore) EnqueueJob(_ context.Context, j store.NewJob) (uuid.UUID, error) {
	if err := f.err("EnqueueJob"); err != nil {
		return uuid.Nil, err
	}
	f.jobs = append(f.jobs, j)
	return uuid.New(), nil
}

// lastNodeRun returns the node run created last.
func (f *fakeStore) lastNodeRun() domain.NodeRun {
	return f.nodeRuns[f.order[len(f.order)-1]]
}

// timelineTypes lists the timeline entries, with the node of node entries.
func (f *fakeStore) timelineTypes() []string {
	out := make([]string, len(f.timeline))
	for i, e := range f.timeline {
		var p struct {
			NodeKey string `json:"node_key"`
			Status  string `json:"status"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		out[i] = strings.TrimSuffix(strings.Join([]string{string(e.Type), p.NodeKey, p.Status}, " "), " ")
		out[i] = strings.Join(strings.Fields(out[i]), " ")
	}
	return out
}

type recorder struct{ events []events.Event }

func (r *recorder) Publish(e events.Event) { r.events = append(r.events, e) }

func (r *recorder) types() []string {
	out := make([]string, 0, len(r.events))
	for _, e := range r.events {
		if e.Type != "card_event.created" {
			out = append(out, e.Type)
		}
	}
	return out
}

// fakeAgents finishes the node run of each agent.run job with result.
type fakeAgents struct {
	store  *fakeStore
	result func(nr domain.NodeRun) domain.NodeResult
	err    error
	calls  []agent.Payload
}

func (a *fakeAgents) Handle(_ context.Context, payload json.RawMessage) error {
	var p agent.Payload
	_ = json.Unmarshal(payload, &p)
	a.calls = append(a.calls, p)
	if a.result != nil {
		nr := a.store.nodeRuns[p.NodeRunID]
		_, _ = a.store.FinishNodeRun(context.Background(), p.NodeRunID, a.result(nr))
	}
	return a.err
}

func done(text string) domain.NodeResult {
	out, _ := json.Marshal(map[string]string{"result": text})
	return domain.NodeResult{Status: domain.NodeDone, OutputHandle: "done", Output: out}
}

// graph builds a flow graph: nodes as "id:type" with configs by id, edges
// as "source.handle>target".
type graph struct {
	nodes   []string
	configs map[string]any
	edges   []string
}

func (g graph) build() domain.Graph {
	var out domain.Graph
	for _, n := range g.nodes {
		id, typ, _ := strings.Cut(n, ":")
		cfg, ok := g.configs[id]
		if !ok {
			cfg = map[string]any{}
		}
		raw, _ := json.Marshal(cfg)
		if s, isRaw := cfg.(string); isRaw {
			raw = json.RawMessage(s)
		}
		out.Nodes = append(out.Nodes, domain.Node{ID: id, Type: domain.NodeType(typ), Config: raw})
	}
	for _, e := range g.edges {
		src, target, _ := strings.Cut(e, ">")
		node, handle, _ := strings.Cut(src, ".")
		out.Edges = append(out.Edges, domain.Edge{ID: e, Source: node, SourceHandle: handle, Target: target})
	}
	return out
}

// env is an engine over a fake board with one card in lane todo.
type env struct {
	t      *testing.T
	store  *fakeStore
	agents *fakeAgents
	pub    *recorder
	engine *Engine
	card   domain.Card
	todo   uuid.UUID
	doing  uuid.UUID
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st := newFakeStore()
	e := &env{t: t, store: st, pub: &recorder{}, todo: uuid.New(), doing: uuid.New()}
	st.lanes[e.todo], st.lanes[e.doing] = true, true
	e.agents = &fakeAgents{store: st}
	e.engine = &Engine{Store: st, Agents: e.agents, Pub: e.pub}
	e.card = domain.Card{ID: uuid.New(), CardTypeID: uuid.New(), LaneID: e.todo}
	st.cards[e.card.ID] = e.card
	return e
}

// addFlow adds an enabled flow whose last version has g.
func (e *env) addFlow(name string, g graph) {
	flow := domain.RunnableFlow{Name: name, Version: domain.FlowVersion{ID: uuid.New(), FlowID: uuid.New(), Version: 1, Graph: g.build()}}
	e.store.flows = append(e.store.flows, flow)
}

// cardEvent records a card event and returns its trigger job payload.
func (e *env) cardEvent(typ domain.CardEventType, payload any) json.RawMessage {
	data, _ := json.Marshal(payload)
	ev := domain.CardEvent{ID: uuid.New(), CardID: e.card.ID, Type: typ, Payload: data}
	e.store.events[ev.ID] = ev
	job, _ := json.Marshal(domain.TriggerJob{CardEventID: ev.ID})
	return job
}

func (e *env) created() json.RawMessage {
	return e.cardEvent(domain.CardCreated, map[string]uuid.UUID{"lane_id": e.todo})
}

func (e *env) moved(from, to uuid.UUID) json.RawMessage {
	return e.cardEvent(domain.CardMoved, map[string]uuid.UUID{"from_lane_id": from, "to_lane_id": to})
}

func (e *env) trigger(payload json.RawMessage) {
	e.t.Helper()
	if err := e.engine.HandleTrigger(context.Background(), payload); err != nil {
		e.t.Fatal(err)
	}
}

// runAgent runs the last queued agent job.
func (e *env) runAgent() error {
	e.t.Helper()
	job := e.store.jobs[len(e.store.jobs)-1]
	payload, _ := json.Marshal(job.Payload)
	return e.engine.HandleAgent(context.Background(), payload)
}

func (e *env) onlyRun() domain.FlowRun {
	e.t.Helper()
	if len(e.store.runs) != 1 {
		e.t.Fatalf("got %d runs, want 1", len(e.store.runs))
	}
	for _, r := range e.store.runs {
		return r
	}
	return domain.FlowRun{}
}

func (e *env) wantTimeline(want ...string) {
	e.t.Helper()
	if got := e.store.timelineTypes(); !slices.Equal(got, want) {
		e.t.Fatalf("timeline:\n got %q\nwant %q", got, want)
	}
}

func triggerCfg(event string, lane *uuid.UUID, types ...uuid.UUID) map[string]any {
	return map[string]any{"event": event, "lane_id": lane, "card_type_ids": types}
}

func TestAgentThenMoveCard(t *testing.T) {
	e := newEnv(t)
	e.addFlow("dev", graph{
		nodes:   []string{"t:trigger", "dev:agent", "move:move_card", "end:end"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardEnteredLane, &e.todo), "move": map[string]any{"lane_id": e.doing}},
		edges:   []string{"t.next>dev", "dev.done>move", "move.next>end"},
	})
	e.trigger(e.created())

	run := e.onlyRun()
	if run.Trigger != domain.EventCardEnteredLane || run.Status != domain.RunRunning {
		t.Fatalf("run %+v", run)
	}
	if len(e.store.jobs) != 1 || e.store.jobs[0].Kind != agent.JobKind || e.store.jobs[0].MaxAttempts != 2 {
		t.Fatalf("jobs %+v", e.store.jobs)
	}
	if p := e.store.jobs[0].Payload.(agent.Payload); p.Prompt != "" || p.NodeRunID != e.store.lastNodeRun().ID {
		t.Fatalf("payload %+v", p)
	}
	e.wantTimeline("flow.started", "node.started dev")

	e.agents.result = func(domain.NodeRun) domain.NodeResult { return done("ok") }
	if err := e.runAgent(); err != nil {
		t.Fatal(err)
	}
	if got := e.store.cards[e.card.ID].LaneID; got != e.doing {
		t.Fatal("the card did not move")
	}
	if run = e.onlyRun(); run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
	e.wantTimeline("flow.started", "node.started dev", "node.finished dev done",
		"node.started move", "node.finished move done", "node.started end", "node.finished end done", "flow.finished done")
	if got := e.pub.types(); !slices.Equal(got, []string{"flow_run.created", "card.moved", "flow_run.updated"}) {
		t.Fatalf("published %q", got)
	}
	i := slices.IndexFunc(e.pub.events, func(ev events.Event) bool { return ev.Type == "flow_run.updated" })
	if run := e.pub.events[i].Data.(domain.FlowRun); run.FlowName != "dev" {
		t.Fatalf("finished run without its flow name: %+v", run)
	}

	// A retried job does not follow the output again.
	if err := e.runAgent(); err != nil {
		t.Fatal(err)
	}
	if len(e.store.timeline) != 8 {
		t.Fatalf("the retried job changed the timeline: %q", e.store.timelineTypes())
	}
}

func TestReentryResumesWithThePreviousOutput(t *testing.T) {
	e := newEnv(t)
	e.addFlow("loop", graph{
		nodes:   []string{"t:trigger", "dev:agent", "rev:agent"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil)},
		edges:   []string{"t.next>dev", "dev.done>rev", "rev.done>dev"},
	})
	e.trigger(e.created())
	e.agents.result = func(nr domain.NodeRun) domain.NodeResult {
		if nr.NodeKey == "rev" {
			return done("fix the tests")
		}
		return done("ok")
	}
	for range 2 {
		if err := e.runAgent(); err != nil {
			t.Fatal(err)
		}
	}
	nr := e.store.lastNodeRun()
	p := e.store.jobs[len(e.store.jobs)-1].Payload.(agent.Payload)
	if nr.NodeKey != "dev" || nr.Iteration != 2 || p.Prompt != "fix the tests" {
		t.Fatalf("re-entered %+v with %+v", nr, p)
	}
}

func TestOutputsWithoutEdgesEndTheRun(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result domain.NodeResult
		status domain.RunStatus
		msg    string
	}{
		{"done", done("ok"), domain.RunDone, ""},
		{"failed", domain.NodeResult{Status: domain.NodeFailed, OutputHandle: "failed", Error: "no result"}, domain.RunFailed, "no result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.addFlow("dev", graph{
				nodes:   []string{"t:trigger", "dev:agent"},
				configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil)},
				edges:   []string{"t.next>dev"},
			})
			e.trigger(e.created())
			e.agents.result = func(domain.NodeRun) domain.NodeResult { return tc.result }
			if err := e.runAgent(); err != nil {
				t.Fatal(err)
			}
			if run := e.onlyRun(); run.Status != tc.status || run.Error != tc.msg {
				t.Fatalf("run %+v", run)
			}
		})
	}
}

func TestBoardNodesThatFailEndTheRun(t *testing.T) {
	missing := uuid.New()
	for _, tc := range []struct {
		name  string
		nodes []string
		cfg   any
		edges []string
		msg   string
	}{
		{"review", []string{"x:review"}, nil, nil, "o nó x (review) ainda não é suportado"},
		{"lane gone", []string{"x:move_card"}, map[string]any{"lane_id": missing}, nil, "a baia de destino não existe mais"},
		{"bad config", []string{"x:move_card"}, `{"lane_id": 1}`, nil, "configuração inválida"},
		{"unknown node", []string{"x:move_card"}, map[string]any{"lane_id": missing}, []string{"t.next>nowhere"}, "o nó nowhere não existe"},
		{"cycle", []string{"x:end", "a:move_card", "b:move_card"}, nil, []string{"t.next>a", "a.next>b", "b.next>a"}, "passou por 100 nós"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			configs := map[string]any{"t": triggerCfg(domain.EventCardCreated, nil), "a": map[string]any{"lane_id": e.doing}, "b": map[string]any{"lane_id": e.todo}}
			if tc.cfg != nil {
				configs["x"] = tc.cfg
			}
			edges := tc.edges
			if edges == nil {
				edges = []string{"t.next>x"}
			}
			e.addFlow("f", graph{nodes: append([]string{"t:trigger"}, tc.nodes...), configs: configs, edges: edges})
			e.trigger(e.created())
			if run := e.onlyRun(); run.Status != domain.RunFailed || !strings.Contains(run.Error, tc.msg) {
				t.Fatalf("run %+v", run)
			}
		})
	}
}

func TestTriggerMatching(t *testing.T) {
	other := uuid.New()
	for _, tc := range []struct {
		name    string
		trigger any
		event   func(e *env) json.RawMessage
		starts  bool
	}{
		{"created", triggerCfg(domain.EventCardCreated, nil), (*env).created, true},
		{"created in the lane", triggerCfg(domain.EventCardEnteredLane, nil), (*env).created, false},
		{"moved is not created", triggerCfg(domain.EventCardCreated, nil), func(e *env) json.RawMessage { return e.moved(e.todo, e.doing) }, false},
		{"entered the lane", nil, func(e *env) json.RawMessage { return e.moved(e.todo, e.doing) }, true},
		{"entered another lane", nil, func(e *env) json.RawMessage { return e.moved(e.doing, e.todo) }, false},
		{"moved within the lane", nil, func(e *env) json.RawMessage { return e.moved(e.doing, e.doing) }, false},
		{"other card type", triggerCfg(domain.EventCardCreated, nil, other), (*env).created, false},
		{"unknown event", triggerCfg("card_deleted", nil), (*env).created, false},
		{"bad trigger config", `{"event": 1}`, (*env).created, false},
		{"bad event payload", triggerCfg(domain.EventCardCreated, nil), func(e *env) json.RawMessage { return e.cardEvent(domain.CardCreated, "x") }, false},
		{"updated", triggerCfg(domain.EventCardCreated, nil), func(e *env) json.RawMessage { return e.cardEvent(domain.CardUpdated, map[string]any{}) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			cfg := tc.trigger
			if cfg == nil {
				cfg = triggerCfg(domain.EventCardEnteredLane, &e.doing, e.card.CardTypeID)
			}
			e.addFlow("f", graph{nodes: []string{"t:trigger", "end:end"}, configs: map[string]any{"t": cfg}, edges: []string{"t.next>end"}})
			e.addFlow("no trigger", graph{nodes: []string{"end:end"}})
			e.trigger(tc.event(e))
			if started := len(e.store.runs) == 1; started != tc.starts {
				t.Fatalf("started = %v", started)
			}
		})
	}
}

func TestTriggerWithoutNextEndsTheRun(t *testing.T) {
	e := newEnv(t)
	e.addFlow("f", graph{nodes: []string{"t:trigger"}, configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil)}})
	e.trigger(e.created())
	if run := e.onlyRun(); run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
}

func TestRunningFlowIsSkipped(t *testing.T) {
	e := newEnv(t)
	e.addFlow("dev", graph{
		nodes:   []string{"t:trigger", "dev:agent"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil)},
		edges:   []string{"t.next>dev"},
	})
	e.trigger(e.created())
	e.trigger(e.created())
	e.onlyRun()
	e.wantTimeline("flow.started", "node.started dev", "flow.skipped")
}

func TestDeletedCardIsIgnored(t *testing.T) {
	e := newEnv(t)
	payload, _ := json.Marshal(domain.TriggerJob{CardEventID: uuid.New()})
	e.trigger(payload)
}

func TestRunEndedWhileTheAgentRan(t *testing.T) {
	e := newEnv(t)
	e.addFlow("dev", graph{
		nodes:   []string{"t:trigger", "dev:agent", "end:end"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil)},
		edges:   []string{"t.next>dev", "dev.done>end"},
	})
	e.trigger(e.created())
	run := e.onlyRun()
	run.Status = domain.RunCanceled
	e.store.runs[run.ID] = run
	e.agents.result = func(domain.NodeRun) domain.NodeResult { return done("ok") }
	if err := e.runAgent(); err != nil {
		t.Fatal(err)
	}
	e.wantTimeline("flow.started", "node.started dev", "node.finished dev done")
}

func TestFinishingAnEndedRun(t *testing.T) {
	e := newEnv(t)
	e.addFlow("f", graph{nodes: []string{"t:trigger"}, configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil)}})
	e.trigger(e.created())
	r := &run{FlowRun: e.onlyRun()}
	if err := e.engine.finish(context.Background(), r, domain.RunFailed, ""); err != nil {
		t.Fatalf("finishing an ended run: %v", err)
	}
}

func TestPromptFrom(t *testing.T) {
	for in, want := range map[string]string{
		`{"result": "fix it"}`: "fix it",
		`{"other": 1}`:         `{"other": 1}`,
		`[1]`:                  `[1]`,
		``:                     ``,
	} {
		if got := promptFrom(json.RawMessage(in)); got != want {
			t.Errorf("promptFrom(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestBadPayloads(t *testing.T) {
	e := newEnv(t)
	if err := e.engine.HandleTrigger(context.Background(), json.RawMessage(`[`)); err == nil {
		t.Error("HandleTrigger: expected error")
	}
	if err := e.engine.HandleAgent(context.Background(), json.RawMessage(`[`)); err == nil {
		t.Error("HandleAgent: expected error")
	}
}

func TestAgentErrorsAreReturned(t *testing.T) {
	e := newEnv(t)
	e.addFlow("dev", graph{
		nodes:   []string{"t:trigger", "dev:agent"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil)},
		edges:   []string{"t.next>dev"},
	})
	e.trigger(e.created())
	e.agents.err = errBoom
	if err := e.runAgent(); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
}

// TestStoreErrors fails each store call the engine makes and checks the
// error reaches the job.
func TestStoreErrors(t *testing.T) {
	agentFlow := graph{
		nodes: []string{"t:trigger", "dev:agent", "move:move_card", "end:end"},
		edges: []string{"t.next>dev", "dev.done>move", "move.next>end"},
	}
	for _, tc := range []struct {
		method string
		after  int
		// agent fails after the agent ran, when following its output.
		agent bool
	}{
		{method: "GetCardEvent"},
		{method: "GetCard"},
		{method: "RunnableFlows"},
		{method: "StartFlowRun"},
		{method: "AddCardEvent:flow.started"},
		{method: "AddCardEvent:node.started"},
		{method: "CountNodeRuns"},
		{method: "CreateNodeRun"},
		{method: "EnqueueJob"},
		{method: "ContinueNodeRun", agent: true},
		{method: "GetFlowRun", agent: true},
		{method: "GetFlowVersion", agent: true},
		{method: "AddCardEvent:node.finished", agent: true},
		{method: "MoveCard", agent: true},
		{method: "FinishNodeRun", after: 1, agent: true},
		{method: "AddCardEvent:node.finished", after: 1, agent: true},
		{method: "FinishFlowRun", agent: true},
		{method: "AddCardEvent:flow.finished", agent: true},
	} {
		t.Run(fmt.Sprintf("%s after %d", tc.method, tc.after), func(t *testing.T) {
			e := newEnv(t)
			agentFlow.configs = map[string]any{"t": triggerCfg(domain.EventCardCreated, nil), "move": map[string]any{"lane_id": e.doing}}
			e.addFlow("dev", agentFlow)
			payload := e.created()
			e.agents.result = func(domain.NodeRun) domain.NodeResult { return done("ok") }
			if tc.agent {
				e.trigger(payload)
			}
			e.store.errs[tc.method] = errBoom
			e.store.failAfter[tc.method] = tc.after
			var err error
			if tc.agent {
				err = e.runAgent()
			} else {
				err = e.engine.HandleTrigger(context.Background(), payload)
			}
			if !errors.Is(err, errBoom) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestSkipErrors(t *testing.T) {
	e := newEnv(t)
	e.addFlow("dev", graph{
		nodes:   []string{"t:trigger", "dev:agent"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil)},
		edges:   []string{"t.next>dev"},
	})
	e.trigger(e.created())
	e.store.errs["AddCardEvent:flow.skipped"] = errBoom
	if err := e.engine.HandleTrigger(context.Background(), e.created()); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
}

func TestEndingAfterABoardNodeFails(t *testing.T) {
	e := newEnv(t)
	e.addFlow("f", graph{
		nodes:   []string{"t:trigger", "move:move_card"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil), "move": map[string]any{"lane_id": e.doing}},
		edges:   []string{"t.next>move"},
	})
	e.store.errs["FinishFlowRun"] = errBoom
	if err := e.engine.HandleTrigger(context.Background(), e.created()); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
}
