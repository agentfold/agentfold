package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// retryJob returns the flow.retry job of the env card.
func (e *env) retryJob() json.RawMessage {
	payload, _ := json.Marshal(domain.RetryJob{CardID: e.card.ID})
	return payload
}

func TestRetryRunsTheLastFlowAgain(t *testing.T) {
	e := newEnv(t)
	e.addFlow("f", graph{
		nodes:   []string{"t:trigger", "go:move_card"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardEnteredLane, &e.todo), "go": map[string]any{"lane_id": e.doing}},
		edges:   []string{"t.next>go"},
	})
	flow := e.store.flows[0]
	e.store.cardRuns = map[uuid.UUID][]domain.FlowRun{e.card.ID: {{FlowID: flow.Version.FlowID, FlowName: "f", Status: domain.RunFailed}}}
	if err := e.engine.HandleRetry(context.Background(), e.retryJob()); err != nil {
		t.Fatal(err)
	}
	if run := e.onlyRun(); run.Trigger != RetryTrigger || run.Status != domain.RunDone {
		t.Fatalf("run %+v", run)
	}
	if e.store.cards[e.card.ID].LaneID != e.doing {
		t.Fatal("the flow did not run")
	}
}

func TestRetryOfADisabledFlowIsSkipped(t *testing.T) {
	e := newEnv(t)
	e.store.cardRuns = map[uuid.UUID][]domain.FlowRun{e.card.ID: {{FlowName: "gone", Status: domain.RunFailed}}}
	if err := e.engine.HandleRetry(context.Background(), e.retryJob()); err != nil {
		t.Fatal(err)
	}
	e.wantTimeline("flow.skipped")
	if len(e.store.runs) != 0 {
		t.Fatal("a run started")
	}
}

func TestRetryErrors(t *testing.T) {
	e := newEnv(t)
	if err := e.engine.HandleRetry(context.Background(), json.RawMessage(`[]`)); err == nil {
		t.Fatal("bad payload accepted")
	}
	// No runs: nothing to retry.
	if err := e.engine.HandleRetry(context.Background(), e.retryJob()); err != nil || len(e.store.runs) != 0 {
		t.Fatalf("got %v", err)
	}
	e.store.cardRuns = map[uuid.UUID][]domain.FlowRun{e.card.ID: {{Status: domain.RunFailed}}}
	for _, method := range []string{"ListCardFlowRuns", "RunnableFlows"} {
		e.store.errs = map[string]error{method: errBoom}
		if err := e.engine.HandleRetry(context.Background(), e.retryJob()); !errors.Is(err, errBoom) {
			t.Fatalf("%s: %v", method, err)
		}
	}
}

func TestWaitChildrenSkipsCanceledTasks(t *testing.T) {
	e := newChildEnv(t)
	if err := e.split(`[{"title":"API"},{"title":"Web"}]`); err != nil {
		t.Fatal(err)
	}
	web := e.store.cards[e.store.children[1]]
	now := time.Now()
	web.CanceledAt = &now
	e.store.cards[web.ID] = web
	if err := e.childMoved(e.store.children[0], e.done); err != nil {
		t.Fatal(err)
	}
	if wait := e.nodeRun("wait"); wait.OutputHandle != "all_done" || len(e.childStates(wait)) != 1 {
		t.Fatalf("wait %+v %s", wait, wait.Output)
	}
}

func (e *childEnv) childStates(nr domain.NodeRun) []childCard {
	var out struct {
		Children []childCard `json:"children"`
	}
	_ = json.Unmarshal(nr.Output, &out)
	return out.Children
}
