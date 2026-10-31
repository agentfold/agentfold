package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func (f *fakeStore) StartSplit(_ context.Context, split domain.NodeRun, parent *domain.Branch, names []string) ([]domain.Branch, error) {
	if err := f.err("StartSplit"); err != nil {
		return nil, err
	}
	var out []domain.Branch
	for _, name := range names {
		b := domain.Branch{
			ID: uuid.New(), FlowRunID: split.FlowRunID, SplitNodeRunID: split.ID, SplitKey: split.NodeKey,
			Name: name, Label: split.NodeKey + "." + name, Status: domain.BranchActive,
		}
		if parent != nil {
			b.ParentID, b.Label = &parent.ID, parent.Label+"/"+b.Label
		}
		f.branches[b.ID] = b
		f.branchOrder = append(f.branchOrder, b.ID)
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeStore) GetBranch(_ context.Context, id uuid.UUID) (domain.Branch, error) {
	if err := f.err("GetBranch"); err != nil {
		return domain.Branch{}, err
	}
	b, ok := f.branches[id]
	if !ok {
		return b, domain.ErrNotFound
	}
	return b, nil
}

func (f *fakeStore) SettleBranch(_ context.Context, id uuid.UUID, end domain.BranchEnd, decide func([]domain.Branch) bool) (domain.Settlement, error) {
	if err := f.err("SettleBranch"); err != nil {
		return domain.Settlement{}, err
	}
	b := f.branches[id]
	split := b.SplitNodeRunID
	if f.settled[split] || b.Status != domain.BranchActive {
		return domain.Settlement{}, nil
	}
	b.Status, b.Output, b.Error = end.Status, end.Output, end.Error
	f.branches[id] = b
	if !decide(f.splitBranches(split)) {
		return domain.Settlement{Branches: f.splitBranches(split)}, nil
	}
	f.settled[split] = true
	for changed := true; changed; {
		changed = false
		for _, bid := range f.branchOrder {
			x := f.branches[bid]
			nested := x.ParentID != nil && f.branches[*x.ParentID].Status == domain.BranchCanceled
			if x.Status == domain.BranchActive && (x.SplitNodeRunID == split || nested) {
				x.Status, changed = domain.BranchCanceled, true
				f.branches[bid] = x
				for nid, nr := range f.nodeRuns {
					if nr.BranchID != nil && *nr.BranchID == bid && nr.Status != domain.NodeDone && nr.Status != domain.NodeFailed {
						nr.Status, nr.Error = domain.NodeFailed, "ramo cancelado"
						f.nodeRuns[nid] = nr
					}
				}
			}
		}
	}
	st := domain.Settlement{Settled: true, Branches: f.splitBranches(split)}
	if b.ParentID != nil {
		parent := f.branches[*b.ParentID]
		st.Parent = &parent
	}
	return st, nil
}

func (f *fakeStore) splitBranches(split uuid.UUID) []domain.Branch {
	var out []domain.Branch
	for _, id := range f.branchOrder {
		if b := f.branches[id]; b.SplitNodeRunID == split {
			out = append(out, b)
		}
	}
	return out
}

// branchJobs takes the queued flow.branch jobs, in order.
func (e *env) branchJobs() []json.RawMessage {
	var out []json.RawMessage
	rest := e.store.jobs[:0]
	for _, j := range e.store.jobs {
		if j.Kind == domain.JobFlowBranch {
			payload, _ := json.Marshal(j.Payload)
			out = append(out, payload)
		} else {
			rest = append(rest, j)
		}
	}
	e.store.jobs = rest
	return out
}

// runBranches runs the queued flow.branch jobs, and those they queue,
// until none is left.
func (e *env) runBranches() {
	e.t.Helper()
	for jobs := e.branchJobs(); len(jobs) > 0; jobs = e.branchJobs() {
		for _, payload := range jobs {
			if err := e.engine.HandleBranch(context.Background(), payload); err != nil {
				e.t.Fatal(err)
			}
		}
	}
}

func (e *env) nodeRunOf(key string) domain.NodeRun {
	e.t.Helper()
	for i := len(e.store.order) - 1; i >= 0; i-- {
		if nr := e.store.nodeRuns[e.store.order[i]]; nr.NodeKey == key {
			return nr
		}
	}
	e.t.Fatalf("no node run of %s", key)
	return domain.NodeRun{}
}

func value(v any) map[string]any { return map[string]any{"output": v} }

// splitFlow is t -> s (branches a and b) -> a/b -> j -> done (end) or
// failures (some_failed, end), with the node configs given.
func splitFlow(e *env, configs map[string]any) {
	if configs["t"] == nil {
		configs["t"] = triggerCfg(domain.EventCardCreated, nil)
	}
	if configs["s"] == nil {
		configs["s"] = map[string]any{"branches": []string{"a", "b"}}
	}
	e.addFlow("par", graph{
		nodes:   []string{"t:trigger", "s:split", "a:transform", "b:transform", "j:join", "done:end", "failures:end"},
		configs: configs,
		edges:   []string{"t.next>s", "s.a>a", "s.b>b", "a.next>j", "b.next>j", "j.all_done>done", "j.some_failed>failures"},
	})
}

func TestSplitJoinWaitsForAll(t *testing.T) {
	e := newEnv(t)
	splitFlow(e, map[string]any{"a": value("=1"), "b": value("=0 + 1")})
	e.trigger(e.created())
	jobs := e.branchJobs()
	if len(jobs) != 2 {
		t.Fatalf("jobs %q", jobs)
	}
	if err := e.engine.HandleBranch(context.Background(), jobs[0]); err != nil {
		t.Fatal(err)
	}
	if run := e.onlyRun(); run.Status != domain.RunRunning {
		t.Fatalf("the run did not wait for b: %+v", run)
	}
	if err := e.engine.HandleBranch(context.Background(), jobs[1]); err != nil {
		t.Fatal(err)
	}
	j := e.nodeRunOf("j")
	if j.OutputHandle != "all_done" || j.BranchID != nil || string(j.Output) != `{"output":{"branches":{"a":1,"b":1}}}` {
		t.Fatalf("join %+v (%s)", j, j.Output)
	}
	if a := e.nodeRunOf("a"); a.Branch != "s.a" {
		t.Fatalf("a ran in %q", a.Branch)
	}
	if run := e.onlyRun(); run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
	e.wantTimeline("flow.started", "node.started s", "node.finished s done",
		"node.started a", "node.finished a done", "node.started b", "node.finished b done",
		"node.started j", "node.finished j done", "node.started done", "node.finished done done", "flow.finished done")
	var started struct {
		Branch string `json:"branch"`
	}
	_ = json.Unmarshal(e.store.timeline[3].Payload, &started)
	if started.Branch != "s.a" {
		t.Fatalf("timeline branch %q", started.Branch)
	}
}

func TestSplitJoinFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		join   map[string]any
		handle string
		// canceled: b was canceled before it ran.
		canceled bool
	}{
		{"all", map[string]any{}, "some_failed", false},
		{"fail fast", map[string]any{"mode": "all", "fail_fast": true}, "some_failed", true},
		// b arrives: the first arrival goes on, failures aside.
		{"first", map[string]any{"mode": "first"}, "all_done", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			splitFlow(e, map[string]any{"a": value("=nodes.gone.x + 1"), "b": value("=2"), "j": tc.join})
			e.trigger(e.created())
			e.runBranches()
			j := e.nodeRunOf("j")
			if j.OutputHandle != tc.handle || !strings.Contains(string(j.Output), `"failed":{"a":`) {
				t.Fatalf("join %+v (%s)", j, j.Output)
			}
			ran := false
			for _, nr := range e.store.nodeRuns {
				ran = ran || nr.NodeKey == "b"
			}
			if ran == tc.canceled {
				t.Fatalf("b ran: %v", ran)
			}
		})
	}
}

func TestSplitJoinFirstArrival(t *testing.T) {
	e := newEnv(t)
	splitFlow(e, map[string]any{"a": value("=1"), "b": value("=2"), "j": map[string]any{"mode": "first"}})
	e.trigger(e.created())
	e.runBranches()
	j := e.nodeRunOf("j")
	if j.OutputHandle != "all_done" || string(j.Output) != `{"output":{"branches":{"a":1}}}` {
		t.Fatalf("join %+v (%s)", j, j.Output)
	}
	for _, b := range e.store.branches {
		if b.Name == "b" && b.Status != domain.BranchCanceled {
			t.Fatalf("b %+v", b)
		}
	}
}

func TestBranchesWithoutJoin(t *testing.T) {
	for _, tc := range []struct {
		name, b string
		status  domain.RunStatus
	}{
		{"all done", "=2", domain.RunDone},
		{"one failed", "=nodes.gone.x + 1", domain.RunFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.addFlow("par", graph{
				nodes: []string{"t:trigger", "s:split", "a:transform", "b:transform", "end:end"},
				configs: map[string]any{
					"t": triggerCfg(domain.EventCardCreated, nil), "s": map[string]any{"branches": []string{"a", "b", "unlinked"}},
					"a": value("=1"), "b": value(tc.b),
				},
				edges: []string{"t.next>s", "s.a>a", "s.b>b", "a.next>end"},
			})
			e.trigger(e.created())
			e.runBranches()
			if run := e.onlyRun(); run.Status != tc.status {
				t.Fatalf("run %+v", run)
			}
			if tc.status == domain.RunFailed && !strings.HasPrefix(e.onlyRun().Error, "o ramo s.b falhou: ") {
				t.Fatalf("error %q", e.onlyRun().Error)
			}
		})
	}
}

func TestNestedSplits(t *testing.T) {
	e := newEnv(t)
	e.addFlow("par", graph{
		nodes: []string{"t:trigger", "s:split", "a:transform", "in:split", "x:transform", "y:transform", "ij:join",
			"b:transform", "j:join", "done:end"},
		configs: map[string]any{
			"t":  triggerCfg(domain.EventCardCreated, nil),
			"s":  map[string]any{"branches": []string{"a", "b"}},
			"in": map[string]any{"branches": []string{"x", "y"}},
			"a":  value("=1"), "x": value("=10"), "y": value("=nodes.gone.v + 1"), "b": value("=2"),
		},
		edges: []string{"t.next>s", "s.a>a", "a.next>in", "in.x>x", "in.y>y", "x.next>ij", "y.next>ij", "ij.some_failed>j",
			"s.b>b", "b.next>j", "j.all_done>done"},
	})
	e.trigger(e.created())
	e.runBranches()
	if x := e.nodeRunOf("x"); x.Branch != "s.a/in.x" {
		t.Fatalf("x ran in %q", x.Branch)
	}
	if ij := e.nodeRunOf("ij"); ij.Branch != "s.a" || ij.OutputHandle != "some_failed" {
		t.Fatalf("inner join %+v", ij)
	}
	if j := e.nodeRunOf("j"); j.Branch != "" || j.OutputHandle != "all_done" {
		t.Fatalf("join %+v (%s)", j, j.Output)
	}
	if run := e.onlyRun(); run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
}

func TestBranchCanceledWhileItsNodeRan(t *testing.T) {
	ep := newEndpoint(t, `{"ok":true}`, 200)
	e := newEnv(t)
	e.engine.HTTP, e.engine.Secrets = ep.Client(), fakeOpener{}
	e.addFlow("par", graph{
		nodes: []string{"t:trigger", "s:split", "h:http_request", "b:transform", "j:join"},
		configs: map[string]any{
			"t": triggerCfg(domain.EventCardCreated, nil), "s": map[string]any{"branches": []string{"a", "b"}},
			"h": map[string]any{"method": "GET", "url": ep.URL}, "b": value("=2"), "j": map[string]any{"mode": "first"},
		},
		edges: []string{"t.next>s", "s.a>h", "s.b>b", "h.success>j", "b.next>j"},
	})
	e.trigger(e.created())
	e.runBranches()
	if run := e.onlyRun(); run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
	if err := e.runHTTP(); err != nil {
		t.Fatal(err)
	}
	if len(ep.requests) != 0 {
		t.Fatal("the request of a canceled branch went out")
	}

	// An HTTP node that finishes in an active branch goes on in it.
	e = newEnv(t)
	e.engine.HTTP, e.engine.Secrets = ep.Client(), fakeOpener{}
	e.addFlow("par", graph{
		nodes: []string{"t:trigger", "s:split", "h:http_request", "b:transform", "j:join"},
		configs: map[string]any{
			"t": triggerCfg(domain.EventCardCreated, nil), "s": map[string]any{"branches": []string{"a", "b"}},
			"h": map[string]any{"method": "GET", "url": ep.URL}, "b": value("=2"),
		},
		edges: []string{"t.next>s", "s.a>h", "s.b>b", "h.success>j", "b.next>j"},
	})
	e.trigger(e.created())
	e.runBranches()
	if err := e.runHTTP(); err != nil {
		t.Fatal(err)
	}
	if j := e.nodeRunOf("j"); !strings.Contains(string(j.Output), `"a":{"body":{"ok":true},"status":200}`) {
		t.Fatalf("join %s", j.Output)
	}
}

func TestJoinOnTheMainPath(t *testing.T) {
	e := newEnv(t)
	e.addFlow("bad", graph{
		nodes:   []string{"t:trigger", "j:join"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil)},
		edges:   []string{"t.next>j"},
	})
	e.trigger(e.created())
	if run := e.onlyRun(); run.Status != domain.RunFailed || run.Error != "um join foi alcançado fora de um ramo" {
		t.Fatalf("run %+v", run)
	}
}

func TestBranchJobsThatDoNothing(t *testing.T) {
	e := newEnv(t)
	if err := e.engine.HandleBranch(context.Background(), json.RawMessage(`[`)); err == nil {
		t.Error("bad payload accepted")
	}
	gone, _ := json.Marshal(domain.BranchJob{BranchID: uuid.New()})
	if err := e.engine.HandleBranch(context.Background(), gone); err != nil {
		t.Errorf("missing branch: %v", err)
	}
	splitFlow(e, map[string]any{"a": value("=1"), "b": value("=2")})
	e.trigger(e.created())
	jobs := e.branchJobs()
	run := e.onlyRun()
	run.Status = domain.RunCanceled
	e.store.runs[run.ID] = run
	if err := e.engine.HandleBranch(context.Background(), jobs[0]); err != nil || len(e.store.order) != 1 {
		t.Fatalf("a branch of a canceled run ran: %v", err)
	}
}

func TestBranchStoreErrors(t *testing.T) {
	for _, tc := range []struct {
		method string
		after  int
		// branch fails while a branch job runs, not when splitting.
		branch bool
	}{
		{method: "FinishNodeRun"},
		{method: "AddCardEvent:node.finished"},
		{method: "StartSplit"},
		{method: "EnqueueJob"},
		{method: "GetBranch", branch: true},
		{method: "GetFlowRun", branch: true},
		{method: "GetFlowVersion", branch: true},
		{method: "SettleBranch", branch: true},
		{method: "CreateNodeRun", after: 1, branch: true},
		{method: "FinishNodeRun", after: 1, branch: true},
		{method: "AddCardEvent:node.finished", after: 1, branch: true},
	} {
		t.Run(tc.method, func(t *testing.T) {
			e := newEnv(t)
			splitFlow(e, map[string]any{"a": value("=1"), "b": value("=2"), "j": map[string]any{"mode": "first"}})
			payload := e.created()
			if tc.branch {
				e.trigger(payload)
			}
			e.store.errs[tc.method] = errBoom
			e.store.failAfter[tc.method] = tc.after
			var err error
			if tc.branch {
				err = e.engine.HandleBranch(context.Background(), e.branchJobs()[0])
			} else {
				err = e.engine.HandleTrigger(context.Background(), payload)
			}
			if !errors.Is(err, errBoom) {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("GetBranch when continuing", func(t *testing.T) {
		ep := newEndpoint(t, `{}`, 200)
		e := newEnv(t)
		e.engine.HTTP, e.engine.Secrets = ep.Client(), fakeOpener{}
		e.addFlow("par", graph{
			nodes: []string{"t:trigger", "s:split", "h:http_request", "b:end", "j:join"},
			configs: map[string]any{
				"t": triggerCfg(domain.EventCardCreated, nil), "s": map[string]any{"branches": []string{"a", "b"}},
				"h": map[string]any{"method": "GET", "url": ep.URL},
			},
			edges: []string{"t.next>s", "s.a>h", "s.b>b", "h.success>j"},
		})
		e.trigger(e.created())
		e.runBranches()
		e.store.errs["GetBranch"] = errBoom
		if err := e.runHTTP(); !errors.Is(err, errBoom) {
			t.Fatalf("got %v", err)
		}
	})
}
