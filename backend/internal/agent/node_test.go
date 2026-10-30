package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func TestNodeOutcome(t *testing.T) {
	cli := domain.NodeResult{Status: domain.NodeDone, OutputHandle: "done", Output: json.RawMessage(`{"result":"did it"}`)}
	schema := domain.AgentConfig{OutputSchema: json.RawMessage(`{"type":"object"}`)}
	agent, gateway := node{}, node{config: domain.AgentConfig{Gateway: true, MaxRounds: 2}}
	tests := []struct {
		name    string
		node    node
		cli     domain.NodeResult
		nr      domain.NodeRun
		pending int
		status  domain.NodeStatus
		handle  string
		output  string
		err     string
	}{
		{"failed", agent, domain.NodeResult{Status: domain.NodeFailed, OutputHandle: "failed"}, domain.NodeRun{}, 1, domain.NodeFailed, "failed", "", ""},
		{"questions", agent, cli, domain.NodeRun{}, 1, domain.NodeAwaitingInput, "", "", ""},
		{"plain", agent, cli, domain.NodeRun{}, 0, domain.NodeDone, "done", `{"result":"did it"}`, ""},
		{"summary and output", node{config: schema}, cli,
			domain.NodeRun{Summary: "s", StructuredOutput: json.RawMessage(`{"a":1}`)}, 0, domain.NodeDone, "done",
			`{"output":{"a":1},"result":"did it","summary":"s"}`, ""},
		{"output missing", node{config: schema}, cli, domain.NodeRun{}, 0, domain.NodeFailed, "failed", "", "without calling submit_output"},
		{"no decision", gateway, cli, domain.NodeRun{}, 0, domain.NodeFailed, "failed", "", "without calling approve or reject"},
		{"approved", gateway, cli, domain.NodeRun{Decision: domain.DecisionApproved, Iteration: 2}, 0, domain.NodeDone, "approved",
			`{"decision":"approved","result":"did it","round":2}`, ""},
		{"rejected", gateway, cli, domain.NodeRun{Decision: domain.DecisionRejected, Feedback: "add tests", Iteration: 1}, 0, domain.NodeDone, "rejected",
			`{"decision":"rejected","result":"add tests","round":1}`, ""},
		{"too many rounds", gateway, cli, domain.NodeRun{Decision: domain.DecisionRejected, Feedback: "x", Iteration: 2}, 0, domain.NodeDone, "max_rounds_exceeded", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.node.outcome(tt.cli, tt.nr, tt.pending)
			if got.Status != tt.status || got.OutputHandle != tt.handle || got.Error != "" && tt.err == "" {
				t.Fatalf("got %+v", got)
			}
			if tt.output != "" && string(got.Output) != tt.output {
				t.Fatalf("output %s", got.Output)
			}
			if tt.err != "" && !strings.Contains(got.Error, tt.err) {
				t.Fatalf("error %q", got.Error)
			}
		})
	}
}

func TestSystemPrompts(t *testing.T) {
	prompts := map[string]bool{}
	for _, c := range []domain.AgentConfig{
		{}, {OutputSchema: json.RawMessage(`{}`)}, {ReadOnly: true}, {Gateway: true}, {AllowedTools: []string{"mcp__orquestrator__create_cards"}, CardTypeIDs: []uuid.UUID{uuid.New()}},
	} {
		prompts[node{config: c}.systemPrompt()] = true
	}
	if len(prompts) != 5 {
		t.Fatal("each option adds its own instructions")
	}
}
