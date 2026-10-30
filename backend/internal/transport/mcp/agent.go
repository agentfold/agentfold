package mcp

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
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
	// agentProjectsOutput leaves the local paths out: agents see their
	// workspace, not the user's checkouts.
	agentProjectsOutput struct {
		Projects []namedView `json:"projects"`
	}
	rejectInput struct {
		Feedback string `json:"feedback" jsonschema:"what is wrong, where (file and line when it applies) and what to change"`
	}
)

// artifactView is the latest version of an artifact, as get_card shows it.
type artifactView struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Path    string `json:"path"`
	// Review is where its review stands: approved once a human approved it.
	Review  string `json:"review"`
	Content string `json:"content"`
}

type cardArtifactsView struct {
	cardView
	Artifacts []artifactView `json:"artifacts"`
}

type agentCardView struct {
	cardArtifactsView
	Parent *cardArtifactsView `json:"parent,omitempty"`
}

// cardWithArtifacts is a card with the latest version of its artifacts.
func cardWithArtifacts(ctx context.Context, board Board, id uuid.UUID) (cardArtifactsView, error) {
	card, err := board.GetCard(ctx, id)
	if err != nil {
		return cardArtifactsView{}, err
	}
	artifacts, err := board.ListCardArtifacts(ctx, id)
	view := cardArtifactsView{cardView: toCard(card), Artifacts: make([]artifactView, 0, len(artifacts))}
	for _, a := range artifacts {
		latest := a.Versions[len(a.Versions)-1] // artifacts have a version at least
		view.Artifacts = append(view.Artifacts, artifactView{
			Name: a.Name, Version: latest.Version, Path: latest.Path, Review: string(a.Review), Content: latest.Content,
		})
	}
	return view, err
}

// agentServer exposes the tools of the agent running a node.
func agentServer(ctx context.Context, board Board, run service.AgentRun) *sdk.Server {
	s := sdk.NewServer(implementation, nil)
	addTool(s, "get_card", "The card you are working on, with its artifacts (latest versions), and its parent card with the parent's artifacts, if it has one.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, agentCardView, error) {
			view, err := cardWithArtifacts(ctx, board, run.FlowRun.CardID)
			if err != nil || view.ParentCardID == nil {
				return nil, agentCardView{cardArtifactsView: view}, err
			}
			parent, err := cardWithArtifacts(ctx, board, *view.ParentCardID)
			return nil, agentCardView{cardArtifactsView: view, Parent: &parent}, err
		})
	addTool(s, "list_projects", "The projects of the board: the repositories cards work on. A card's project_id is one of their ids; cards created from your output name their project.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, agentProjectsOutput, error) {
			projects, err := board.ListProjects(ctx)
			return nil, agentProjectsOutput{mapAll(projects, func(p domain.Project) namedView {
				return namedView{ID: p.ID, Name: p.Name}
			})}, err
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
	addCards(ctx, s, board, run, cfg)
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
