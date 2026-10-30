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

func (f *fakeStore) SetCardProperty(_ context.Context, id uuid.UUID, key, value string) (domain.Card, error) {
	if err := f.err("SetCardProperty"); err != nil {
		return domain.Card{}, err
	}
	c := f.cards[id]
	c.Properties = map[string]string{key: value}
	f.cards[id] = c
	return c, nil
}

func (f *fakeStore) ListCardsByID(_ context.Context, ids []uuid.UUID) ([]domain.Card, error) {
	var out []domain.Card
	for _, id := range ids {
		out = append(out, f.cards[id])
	}
	return out, f.err("ListCardsByID")
}

func (f *fakeStore) ListDependentCards(_ context.Context, id uuid.UUID) ([]domain.Card, error) {
	var out []domain.Card
	for _, c := range f.cards {
		if slices.Contains(c.DependsOn, id) {
			out = append(out, c)
		}
	}
	return out, f.err("ListDependentCards")
}

// propertyEnv has the env card depend on card dep: the env card runs a
// flow that checks dep for dev=ok, dep one that sets it.
type propertyEnv struct {
	*env
	dep domain.Card
}

func newPropertyEnv(t *testing.T, check map[string]any) *propertyEnv {
	t.Helper()
	e := &propertyEnv{env: newEnv(t)}
	e.dep = domain.Card{ID: uuid.New(), Title: "API", CardTypeID: uuid.New(), LaneID: e.todo}
	e.store.cards[e.dep.ID] = e.dep
	e.card.DependsOn = []uuid.UUID{e.dep.ID}
	e.store.cards[e.card.ID] = e.card
	e.addFlow("check", graph{
		nodes:   []string{"t:trigger", "check:check_property", "go:move_card", "back:end"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil, e.card.CardTypeID), "check": check, "go": map[string]any{"lane_id": e.doing}},
		edges:   []string{"t.next>check", "check.satisfied>go", "check.pending>back"},
	})
	e.addFlow("set", graph{
		nodes:   []string{"t:trigger", "set:set_property", "end:end"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil, e.dep.CardTypeID), "set": map[string]any{"key": " dev ", "value": " ok "}},
		edges:   []string{"t.next>set", "set.next>end"},
	})
	return e
}

// createdOf returns the trigger job of the creation of card.
func (e *env) createdOf(card domain.Card) json.RawMessage {
	ev := domain.CardEvent{ID: uuid.New(), CardID: card.ID, Type: domain.CardCreated, Payload: json.RawMessage(`{}`)}
	e.store.events[ev.ID] = ev
	job, _ := json.Marshal(domain.TriggerJob{CardEventID: ev.ID})
	return job
}

func (e *env) runOf(cardID uuid.UUID) domain.FlowRun {
	e.t.Helper()
	for _, r := range e.store.runs {
		if r.CardID == cardID {
			return r
		}
	}
	e.t.Fatal("no run")
	return domain.FlowRun{}
}

func (e *env) lastRunOf(key string) domain.NodeRun {
	e.t.Helper()
	for i := len(e.store.order) - 1; i >= 0; i-- {
		if nr := e.store.nodeRuns[e.store.order[i]]; nr.NodeKey == key {
			return nr
		}
	}
	e.t.Fatalf("no run of node %s", key)
	return domain.NodeRun{}
}

func TestCheckPropertyWithoutWait(t *testing.T) {
	e := newPropertyEnv(t, map[string]any{"key": "dev", "value": "ok"})
	e.trigger(e.created())
	check := e.lastRunOf("check")
	if check.OutputHandle != "pending" || !strings.Contains(string(check.Output), `Dependências sem dev=ok: \"API\" (`+e.dep.ID.String()+`).`) ||
		!strings.Contains(string(check.Output), `"satisfied":false`) {
		t.Fatalf("check %+v %s", check, check.Output)
	}
	if e.store.cards[e.card.ID].LaneID != e.todo {
		t.Fatal("the card moved on")
	}

	// dep's flow sets the property and hands on its input; the env card
	// does not wait, so nothing else happens.
	e.trigger(e.createdOf(e.dep))
	if got := e.store.cards[e.dep.ID].Properties; got["dev"] != "ok" {
		t.Fatalf("properties %v", got)
	}
	if set := e.lastRunOf("set"); set.OutputHandle != "next" || set.Status != domain.NodeDone {
		t.Fatalf("set %+v", set)
	}
	if !slices.Contains(e.pub.types(), "card.updated") {
		t.Fatalf("published %q", e.pub.types())
	}

	delete(e.store.runs, e.runOf(e.card.ID).ID)
	e.trigger(e.created())
	if check := e.lastRunOf("check"); check.OutputHandle != "satisfied" || !strings.Contains(string(check.Output), "Todas as dependências têm dev=ok.") {
		t.Fatalf("check %+v", check)
	}
	if e.store.cards[e.card.ID].LaneID != e.doing {
		t.Fatal("the card did not move on")
	}
}

func TestCheckPropertyWaits(t *testing.T) {
	e := newPropertyEnv(t, map[string]any{"key": "dev", "value": "ok", "wait": true})
	e.trigger(e.created())
	if run := e.runOf(e.card.ID); run.Status != domain.RunWaiting || e.lastRunOf("check").Status != domain.NodeWaiting {
		t.Fatalf("run %+v", run)
	}
	e.trigger(e.createdOf(e.dep))
	if check := e.lastRunOf("check"); check.OutputHandle != "satisfied" {
		t.Fatalf("check %+v", check)
	}
	if run := e.runOf(e.card.ID); run.Status != domain.RunDone || e.store.cards[e.card.ID].LaneID != e.doing {
		t.Fatalf("run %+v", run)
	}
}

func TestCheckPropertyWithoutDependencies(t *testing.T) {
	e := newPropertyEnv(t, map[string]any{"key": "dev", "value": "ok", "wait": true})
	e.card.DependsOn = nil
	e.store.cards[e.card.ID] = e.card
	e.trigger(e.created())
	if check := e.lastRunOf("check"); check.OutputHandle != "satisfied" {
		t.Fatalf("check %+v", check)
	}
}

func TestPropertyNodesWithBadConfigs(t *testing.T) {
	e := newPropertyEnv(t, map[string]any{})
	e.store.flows[0].Version.Graph.Nodes[1].Config = json.RawMessage(`[]`)
	e.store.flows[1].Version.Graph.Nodes[1].Config = json.RawMessage(`[]`)
	e.trigger(e.created())
	e.trigger(e.createdOf(e.dep))
	for _, card := range []uuid.UUID{e.card.ID, e.dep.ID} {
		if run := e.runOf(card); run.Status != domain.RunFailed || !strings.Contains(run.Error, "configuração inválida") {
			t.Fatalf("run %+v", run)
		}
	}
}

func TestPropertyStoreErrors(t *testing.T) {
	for _, tc := range []struct {
		method string
		// dep runs the flow of the dependency, which sets the property.
		dep bool
	}{
		{method: "GetCard"},
		{method: "ListCardsByID"},
		{method: "SetCardProperty", dep: true},
		{method: "ListDependentCards", dep: true},
	} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			e := newPropertyEnv(t, map[string]any{"key": "dev", "value": "ok"})
			e.store.errs[tc.method] = errBoom
			if tc.method == "GetCard" {
				e.store.failAfter[tc.method] = 1 // the trigger reads the card first
			}
			payload := e.created()
			if tc.dep {
				payload = e.createdOf(e.dep)
			}
			if err := e.engine.HandleTrigger(context.Background(), payload); !errors.Is(err, errBoom) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
