package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orquestrator/internal/agent"
	"orquestrator/internal/domain"
)

func TestPromptsReadOtherNodes(t *testing.T) {
	e := newEnv(t)
	e.card.Title = "Login"
	e.store.cards[e.card.ID] = e.card
	e.addFlow("plan", graph{
		nodes: []string{"t:trigger", "plan:agent", "rev:agent", "fix:agent"},
		configs: map[string]any{
			"t":    triggerCfg(domain.EventCardCreated, nil),
			"plan": map[string]any{"prompt": "plan [{{nodes.rev}}]"},
			"rev":  map[string]any{"prompt": "review {{nodes.plan}} of {{card.title}} {{ .Kept }}"},
			"fix":  map[string]any{"prompt": "fix it"},
		},
		edges: []string{"t.next>plan", "plan.done>rev", "rev.done>fix"},
	})
	e.trigger(e.created())
	if p := e.store.jobs[0].Payload.(agent.Payload); p.NodePrompt != "plan []" {
		t.Fatalf("plan payload %+v", p)
	}
	e.agents.result = func(domain.NodeRun) domain.NodeResult { return done("the plan") }
	if err := e.runAgent(); err != nil {
		t.Fatal(err)
	}
	if p := e.store.jobs[1].Payload.(agent.Payload); p.NodePrompt != "review the plan of Login {{ .Kept }}" || p.Prompt != "the plan" {
		t.Fatalf("rev payload %+v", p)
	}
	if err := e.runAgent(); err != nil {
		t.Fatal(err)
	}
	if p := e.store.jobs[2].Payload.(agent.Payload); p.NodePrompt != "" {
		t.Fatalf("a prompt without placeholders was rendered: %+v", p)
	}
}

func TestPromptPlaceholderErrors(t *testing.T) {
	for _, method := range []string{"ListNodeRuns", "GetCard"} {
		t.Run(method, func(t *testing.T) {
			e := newEnv(t)
			e.addFlow("plan", graph{
				nodes: []string{"t:trigger", "plan:agent", "rev:agent"},
				configs: map[string]any{
					"t":   triggerCfg(domain.EventCardCreated, nil),
					"rev": map[string]any{"prompt": "{{nodes.plan}}"},
				},
				edges: []string{"t.next>plan", "plan.done>rev"},
			})
			e.trigger(e.created())
			e.agents.result = func(domain.NodeRun) domain.NodeResult { return done("ok") }
			e.store.errs[method] = errBoom
			if err := e.runAgent(); !errors.Is(err, errBoom) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestTransformNode(t *testing.T) {
	e := newEnv(t)
	e.addFlow("calc", graph{
		nodes: []string{"t:trigger", "plan:agent", "calc:transform", "bad:transform", "dev:agent"},
		configs: map[string]any{
			"t":    triggerCfg(domain.EventCardCreated, nil),
			"calc": map[string]any{"output": map[string]any{"n": "=len(nodes.plan) * 2", "from": "{{input}}"}},
			"bad":  map[string]any{"output": "=nodes.calc.n.x + 1"},
		},
		edges: []string{"t.next>plan", "plan.done>calc", "calc.next>bad", "bad.failed>dev"},
	})
	e.trigger(e.created())
	e.agents.result = func(domain.NodeRun) domain.NodeResult { return done("abc") }
	if err := e.runAgent(); err != nil {
		t.Fatal(err)
	}
	var calc, bad domain.NodeRun
	for _, id := range e.store.order {
		switch nr := e.store.nodeRuns[id]; nr.NodeKey {
		case "calc":
			calc = nr
		case "bad":
			bad = nr
		}
	}
	if calc.Status != domain.NodeDone || calc.OutputHandle != "next" || string(calc.Output) != `{"output":{"from":"abc","n":6}}` {
		t.Fatalf("calc %+v (%s)", calc, calc.Output)
	}
	if bad.Status != domain.NodeFailed || bad.OutputHandle != "failed" || bad.Error == "" {
		t.Fatalf("bad %+v", bad)
	}
	if p := e.store.jobs[len(e.store.jobs)-1].Payload.(agent.Payload); !strings.Contains(p.Prompt, `"n": 6`) {
		t.Fatalf("the agent after got %+v", p)
	}
	e.wantTimeline("flow.started", "node.started plan", "node.finished plan done", "node.started calc", "node.finished calc done",
		"node.started bad", "node.finished bad failed", "node.started dev")

	e2 := newEnv(t)
	e2.addFlow("calc", graph{
		nodes:   []string{"t:trigger", "calc:transform"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil), "calc": map[string]any{"output": 1}},
		edges:   []string{"t.next>calc"},
	})
	e2.store.errs["ListNodeRuns"] = errBoom
	if err := e2.engine.HandleTrigger(context.Background(), e2.created()); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
}

func TestSwitchNode(t *testing.T) {
	run := func(t *testing.T, size string) *env {
		e := newEnv(t)
		e.card.Properties = map[string]string{"size": size}
		e.store.cards[e.card.ID] = e.card
		e.addFlow("pick", graph{
			nodes: []string{"t:trigger", "pick:switch", "big:move_card", "small:end", "broken:end"},
			configs: map[string]any{
				"t": triggerCfg(domain.EventCardCreated, nil),
				"pick": map[string]any{"cases": []map[string]any{
					{"name": "big", "expression": `int(card.properties.size) > 10`},
				}},
				"big": map[string]any{"lane_id": e.doing},
			},
			edges: []string{"t.next>pick", "pick.big>big", "pick.else>small", "pick.failed>broken"},
		})
		e.trigger(e.created())
		return e
	}
	run(t, "20").wantTimeline("flow.started", "node.started pick", "node.finished pick done", "node.started big", "node.finished big done", "flow.finished done")
	run(t, "2").wantTimeline("flow.started", "node.started pick", "node.finished pick done", "node.started small", "node.finished small done", "flow.finished done")
	e := run(t, "x")
	e.wantTimeline("flow.started", "node.started pick", "node.finished pick failed", "node.started broken", "node.finished broken done", "flow.finished done")
	if nr := e.store.nodeRuns[e.store.order[0]]; nr.OutputHandle != "failed" || nr.Error == "" {
		t.Fatalf("pick %+v", nr)
	}

	e2 := newEnv(t)
	e2.addFlow("pick", graph{
		nodes:   []string{"t:trigger", "pick:switch"},
		configs: map[string]any{"t": triggerCfg(domain.EventCardCreated, nil), "pick": map[string]any{"cases": []map[string]any{{"name": "a", "expression": "true"}}}},
		edges:   []string{"t.next>pick"},
	})
	e2.store.errs["ListNodeRuns"] = errBoom
	if err := e2.engine.HandleTrigger(context.Background(), e2.created()); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
}
