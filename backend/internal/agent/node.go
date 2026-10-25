package agent

import (
	"encoding/json"
	"fmt"
	"slices"

	"orquestrator/internal/domain"
)

// boardServer names the board MCP server in the --mcp-config of a run; its
// tools are mcp__orquestrator__*.
const boardServer = "orquestrator"

// readOnlyTools are blocked for reviewers: only the developer agent edits.
var readOnlyTools = []string{"Edit", "MultiEdit", "Write", "NotebookEdit"}

// node is an agent or review node, as the executor runs it.
type node struct {
	typ    domain.NodeType
	config domain.AgentConfig
	// maxRounds is the review rounds limit; 0 for agent nodes.
	maxRounds int
}

// findNode returns the agent or review node key of a graph. A review runs
// like an agent with read-only tools.
func findNode(g domain.Graph, key string) (node, error) {
	i := slices.IndexFunc(g.Nodes, func(n domain.Node) bool { return n.ID == key })
	if i < 0 {
		return node{}, fmt.Errorf("node %q not found in the flow version", key)
	}
	n := g.Nodes[i]
	switch n.Type {
	case domain.NodeAgent:
		var c domain.AgentConfig
		err := json.Unmarshal(n.Config, &c)
		return node{typ: n.Type, config: c}, err
	case domain.NodeReview:
		var c domain.ReviewConfig
		err := json.Unmarshal(n.Config, &c)
		return node{typ: n.Type, maxRounds: c.MaxRounds, config: domain.AgentConfig{
			ProfileID:       c.ProfileID,
			Prompt:          c.Prompt,
			AllowedTools:    c.AllowedTools,
			DisallowedTools: append(append([]string{}, c.DisallowedTools...), readOnlyTools...),
		}}, err
	}
	return node{}, fmt.Errorf("node %q is a %s node, not an agent", key, n.Type)
}

// Instructions appended to Claude's system prompt.
const (
	basePrompt = "You are working on a card of the Orquestrator board. " +
		"Never commit, push or open pull requests: leave your changes in the working tree; the board takes it from there. " +
		"The orquestrator MCP server connects you to the board: get_card shows the card. " +
		"When you need a decision or information only a human has, call ask_human with your questions and then end your turn right away, " +
		"without going on: you will be resumed with the answers. "
	agentPrompt  = "When you finish, call report_done with a short summary of what you did."
	outputPrompt = " Before finishing, call submit_output with your result; it must match the schema the tool shows."
	reviewPrompt = "You are reviewing the changes in the working tree (see git status and git diff); do not edit files. " +
		"Finish by calling submit_review: approved true if the work is done, or false with the comments the developer must address."
)

// systemPrompt is what the node is told about the board.
func (n node) systemPrompt() string {
	if n.typ == domain.NodeReview {
		return basePrompt + reviewPrompt
	}
	if n.config.HasOutputSchema() {
		return basePrompt + agentPrompt + outputPrompt
	}
	return basePrompt + agentPrompt
}

// outcome completes the result of the CLI with what the agent reported
// through the board tools: questions left to answer put the node on hold,
// and reviews and structured outputs pick the node output.
func (n node) outcome(cli domain.NodeResult, nr domain.NodeRun, pending int, comments []domain.ReviewComment) domain.NodeResult {
	if cli.Status == domain.NodeFailed {
		return cli
	}
	if pending > 0 {
		cli.Status, cli.OutputHandle, cli.Output = domain.NodeAwaitingInput, "", nil
		return cli
	}
	var result struct {
		Result string `json:"result"`
	}
	_ = json.Unmarshal(cli.Output, &result) // the CLI outcome always holds it
	if n.typ == domain.NodeReview {
		return n.reviewOutcome(cli, nr, comments)
	}
	if n.config.HasOutputSchema() && nr.StructuredOutput == nil {
		return failure(cli, "the agent ended without calling submit_output")
	}
	out := map[string]any{"result": result.Result}
	if nr.Summary != "" {
		out["summary"] = nr.Summary
	}
	if nr.StructuredOutput != nil {
		out["output"] = nr.StructuredOutput
	}
	cli.Output, _ = json.Marshal(out) // plain values and raw JSON
	return cli
}

func (n node) reviewOutcome(cli domain.NodeResult, nr domain.NodeRun, comments []domain.ReviewComment) domain.NodeResult {
	if nr.ReviewApproved == nil {
		return failure(cli, "the reviewer ended without calling submit_review")
	}
	approved := *nr.ReviewApproved
	switch {
	case approved:
		cli.OutputHandle = "approved"
	case nr.Iteration >= n.maxRounds:
		cli.OutputHandle = "max_rounds_exceeded"
	default:
		cli.OutputHandle = "changes_requested"
	}
	type comment struct {
		File string `json:"file,omitempty"`
		Line *int   `json:"line,omitempty"`
		Body string `json:"body"`
	}
	list := make([]comment, len(comments))
	for i, c := range comments {
		list[i] = comment{File: c.File, Line: c.Line, Body: c.Body}
	}
	cli.Output, _ = json.Marshal(map[string]any{ // plain values
		"result":   domain.ReviewPrompt(approved, comments),
		"approved": approved,
		"round":    nr.Iteration,
		"comments": list,
	})
	return cli
}

func failure(r domain.NodeResult, msg string) domain.NodeResult {
	r.Status, r.OutputHandle, r.Output, r.Error = domain.NodeFailed, "failed", nil, msg
	return r
}
