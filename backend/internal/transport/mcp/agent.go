package mcp

import (
	"context"
	"encoding/json"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"orquestrator/internal/domain"
	"orquestrator/internal/service"
)

type (
	noInput  struct{}
	askInput struct {
		Questions []string `json:"questions" jsonschema:"the questions for the human, one per item"`
	}
	doneInput struct {
		Summary string `json:"summary" jsonschema:"a short summary of what you did"`
	}
	approveInput struct {
		Reason string `json:"reason,omitempty" jsonschema:"why the work can move forward"`
	}
	rejectInput struct {
		Feedback string `json:"feedback" jsonschema:"what is wrong, where (file and line when it applies) and what to change"`
	}
)

// agentServer exposes the tools of the agent running a node.
func agentServer(board Board, run service.AgentRun) *sdk.Server {
	s := sdk.NewServer(implementation, nil)
	addTool(s, "get_card", "The card you are working on.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, cardView, error) {
			card, err := board.GetCard(ctx, run.FlowRun.CardID)
			return nil, toCard(card), err
		})
	addTool(s, "ask_human",
		"Asks a human questions you cannot answer yourself. End your turn right after calling it: you are resumed with the answers.",
		func(ctx context.Context, _ *sdk.CallToolRequest, in askInput) (*sdk.CallToolResult, any, error) {
			if _, err := board.AskHuman(ctx, run, in.Questions); err != nil {
				return nil, nil, err
			}
			return text("The questions were sent. End your turn now, without going on; you will be resumed with the answers."), nil, nil
		})
	addTool(s, "report_done", "Reports what you did, when you finish.",
		func(ctx context.Context, _ *sdk.CallToolRequest, in doneInput) (*sdk.CallToolResult, any, error) {
			if err := board.ReportDone(ctx, run, in.Summary); err != nil {
				return nil, nil, err
			}
			return text("Summary recorded."), nil, nil
		})
	var cfg domain.AgentConfig
	_ = json.Unmarshal(run.Node.Config, &cfg) // published configs are valid
	if cfg.HasOutputSchema() {
		addSubmitOutput(s, board, run, cfg.OutputSchema)
	}
	if cfg.Gateway {
		addGateway(s, board, run)
	}
	return s
}

// addGateway adds the tools a gateway node decides with.
func addGateway(s *sdk.Server, board Board, run service.AgentRun) {
	addTool(s, "approve", "Approves the work: the flow moves forward. Call it once, at the end, instead of reject.",
		func(ctx context.Context, _ *sdk.CallToolRequest, in approveInput) (*sdk.CallToolResult, any, error) {
			if err := board.Decide(ctx, run, domain.DecisionApproved, in.Reason); err != nil {
				return nil, nil, err
			}
			return text("Approved. Finish now."), nil, nil
		})
	addTool(s, "reject",
		"Rejects the work: the flow takes it back with your feedback. Call it once, at the end, instead of approve.",
		func(ctx context.Context, _ *sdk.CallToolRequest, in rejectInput) (*sdk.CallToolResult, any, error) {
			if err := board.Decide(ctx, run, domain.DecisionRejected, in.Feedback); err != nil {
				return nil, nil, err
			}
			return text("Rejected. Finish now."), nil, nil
		})
}

// addSubmitOutput adds the tool that takes the structured output of the
// node; its input schema shows the output schema to the agent.
func addSubmitOutput(s *sdk.Server, board Board, run service.AgentRun, schema json.RawMessage) {
	input := map[string]any{
		"type":       "object",
		"properties": map[string]any{"output": schema},
		"required":   []string{"output"},
	}
	s.AddTool(&sdk.Tool{
		Name:        "submit_output",
		Description: "Submits the result of your work; it must match the schema of the output property. Call it before finishing.",
		InputSchema: input,
	}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		var in struct {
			Output json.RawMessage `json:"output"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &in); err != nil || in.Output == nil {
			return errorResult("the arguments need an output property"), nil
		}
		if err := board.SubmitOutput(ctx, run, in.Output); err != nil {
			return errorResult(err.Error()), nil
		}
		return text("Output recorded."), nil
	})
}

func errorResult(msg string) *sdk.CallToolResult {
	r := text(msg)
	r.IsError = true
	return r
}
