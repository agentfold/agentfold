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

// readOnlyTools are blocked for read-only nodes, such as reviewers.
var readOnlyTools = []string{"Edit", "MultiEdit", "Write", "NotebookEdit"}

// node is an agent node, as the executor runs it.
type node struct {
	config domain.AgentConfig
}

// findNode returns the agent node key of a graph. Read-only nodes run with
// the editing tools blocked.
func findNode(g domain.Graph, key string) (node, error) {
	i := slices.IndexFunc(g.Nodes, func(n domain.Node) bool { return n.ID == key })
	if i < 0 {
		return node{}, fmt.Errorf("node %q not found in the flow version", key)
	}
	n := g.Nodes[i]
	if n.Type != domain.NodeAgent {
		return node{}, fmt.Errorf("node %q is a %s node, not an agent", key, n.Type)
	}
	var c domain.AgentConfig
	err := json.Unmarshal(n.Config, &c)
	if c.ReadOnly {
		c.DisallowedTools = append(append([]string{}, c.DisallowedTools...), readOnlyTools...)
	}
	return node{config: c}, err
}

// Instructions appended to Claude's system prompt.
const (
	basePrompt = "You are working on a card of the Orquestrator board. " +
		"Never commit, push or open pull requests: leave your changes in the working tree; the board takes it from there. " +
		"The orquestrator MCP server connects you to the board: get_card shows the card, with its artifacts and its parent card, list_projects the projects of the board and list_card_types its card types. " +
		"When you need a decision or information only a human has, call ask_human with your questions and then end your turn right away, " +
		"without going on: you will be resumed with the answers. "
	agentPrompt  = "When you finish, call report_done with a short summary of what you did."
	outputPrompt = " Before finishing, call submit_output with your result; it must match the schema the tool shows."
	tasksPrompt  = " You can split this card into tasks, child cards that the flows of their type carry out: first plan every task, " +
		"each one standing on its own, then create them all with one create_tasks call, linking with depends_on the tasks that must be done first. " +
		"Creating tasks does not wait for them: finish with a summary and end your turn; the flow resumes you with how they went."
	readOnlyPrompt = " Do not edit files: this step only reads the working tree (see git status and git diff)."
	gatewayPrompt  = " This step is a gate: you decide whether the work can move forward. Before you finish, call exactly one board tool: " +
		"approve when the work meets the requirements, or reject with feedback the next agent can act on — what is wrong, " +
		"where (file and line when it applies) and what to change. Ending without calling one of them fails this step."
)

// systemPrompt is what the node is told about the board.
func (n node) systemPrompt() string {
	prompt := basePrompt + agentPrompt
	if n.config.HasOutputSchema() {
		prompt += outputPrompt
	}
	if n.config.AllowsBoardTool("create_tasks") {
		prompt += tasksPrompt
	}
	if n.config.ReadOnly {
		prompt += readOnlyPrompt
	}
	if n.config.Gateway {
		prompt += gatewayPrompt
	}
	return prompt
}

// outcome completes the result of the CLI with what the agent reported
// through the board tools: questions left to answer put the node on hold,
// and gateway decisions pick the node output.
func (n node) outcome(cli domain.NodeResult, nr domain.NodeRun, pending int) domain.NodeResult {
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
	if n.config.HasOutputSchema() && nr.StructuredOutput == nil {
		return failure(cli, "the agent ended without calling submit_output")
	}
	out := map[string]any{"result": result.Result}
	if n.config.Gateway {
		switch {
		case nr.Decision == "":
			return failure(cli, "the agent ended without calling approve or reject")
		case nr.Decision == domain.DecisionApproved:
			cli.OutputHandle = "approved"
		case nr.Iteration >= n.config.MaxRounds:
			cli.OutputHandle = "max_rounds_exceeded"
		default:
			cli.OutputHandle = "rejected"
		}
		out["decision"] = nr.Decision
		out["round"] = nr.Iteration
		if nr.Feedback != "" {
			// The next agent acts on the feedback, not on the final message.
			out["result"] = nr.Feedback
		}
	}
	if nr.Summary != "" {
		out["summary"] = nr.Summary
	}
	if nr.StructuredOutput != nil {
		out["output"] = nr.StructuredOutput
	}
	cli.Output, _ = json.Marshal(out) // plain values and raw JSON
	return cli
}

func failure(r domain.NodeResult, msg string) domain.NodeResult {
	r.Status, r.OutputHandle, r.Output, r.Error = domain.NodeFailed, "failed", nil, msg
	return r
}
