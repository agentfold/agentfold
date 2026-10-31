package engine

import (
	"errors"
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
