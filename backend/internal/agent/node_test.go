package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"orquestrator/internal/domain"
)

func TestNodeOutcome(t *testing.T) {
	cli := domain.NodeResult{Status: domain.NodeDone, OutputHandle: "done", Output: json.RawMessage(`{"result":"did it"}`)}
	yes, no := true, false
	schema := domain.AgentConfig{OutputSchema: json.RawMessage(`{"type":"object"}`)}
	agent, review := node{typ: domain.NodeAgent}, node{typ: domain.NodeReview, maxRounds: 2}
	comments := []domain.ReviewComment{{Body: "add tests"}}
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
		{"summary and output", node{typ: domain.NodeAgent, config: schema}, cli,
			domain.NodeRun{Summary: "s", StructuredOutput: json.RawMessage(`{"a":1}`)}, 0, domain.NodeDone, "done",
			`{"output":{"a":1},"result":"did it","summary":"s"}`, ""},
		{"output missing", node{typ: domain.NodeAgent, config: schema}, cli, domain.NodeRun{}, 0, domain.NodeFailed, "failed", "", "without calling submit_output"},
		{"no review", review, cli, domain.NodeRun{}, 0, domain.NodeFailed, "failed", "", "without calling submit_review"},
		{"approved", review, cli, domain.NodeRun{ReviewApproved: &yes, Iteration: 2}, 0, domain.NodeDone, "approved",
			`{"approved":true,"comments":[],"result":"The review approved the changes.","round":2}`, ""},
		{"changes", review, cli, domain.NodeRun{ReviewApproved: &no, Iteration: 1}, 0, domain.NodeDone, "changes_requested", "", ""},
		{"too many rounds", review, cli, domain.NodeRun{ReviewApproved: &no, Iteration: 2}, 0, domain.NodeDone, "max_rounds_exceeded", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cs []domain.ReviewComment
			if tt.nr.ReviewApproved != nil && !*tt.nr.ReviewApproved {
				cs = comments
			}
			got := tt.node.outcome(tt.cli, tt.nr, tt.pending, cs)
			if got.Status != tt.status || got.OutputHandle != tt.handle || got.Error != "" && tt.err == "" {
				t.Fatalf("got %+v", got)
			}
			if tt.output != "" && string(got.Output) != tt.output {
				t.Fatalf("output %s", got.Output)
			}
			if tt.err != "" && !containsText(got.Error, tt.err) {
				t.Fatalf("error %q", got.Error)
			}
		})
	}
}

func containsText(s, sub string) bool { return strings.Contains(s, sub) }

func TestSystemPrompts(t *testing.T) {
	agent := node{typ: domain.NodeAgent}.systemPrompt()
	output := node{typ: domain.NodeAgent, config: domain.AgentConfig{OutputSchema: json.RawMessage(`{}`)}}.systemPrompt()
	review := node{typ: domain.NodeReview}.systemPrompt()
	if agent == output || agent == review || len(output) <= len(agent) {
		t.Fatal("each node kind gets its own instructions")
	}
}
