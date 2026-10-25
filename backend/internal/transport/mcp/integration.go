package mcp

import (
	"context"
	"errors"
	"math"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"orquestrator/internal/domain"
	"orquestrator/internal/service"
)

type (
	laneView struct {
		ID       uuid.UUID `json:"id"`
		Name     string    `json:"name"`
		Position int       `json:"position"`
	}
	namedView struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}
	projectView struct {
		ID        uuid.UUID `json:"id"`
		Name      string    `json:"name"`
		LocalPath string    `json:"local_path"`
	}
	flowView struct {
		ID            uuid.UUID `json:"id"`
		Name          string    `json:"name"`
		Enabled       bool      `json:"enabled"`
		LatestVersion int       `json:"latest_version"`
	}
	nodeRunView struct {
		NodeKey      string  `json:"node_key"`
		NodeType     string  `json:"node_type"`
		Iteration    int     `json:"iteration"`
		Status       string  `json:"status"`
		OutputHandle string  `json:"output_handle,omitempty"`
		Error        string  `json:"error,omitempty"`
		Turns        int     `json:"turns"`
		CostUSD      float64 `json:"cost_usd"`
	}
	runView struct {
		ID       uuid.UUID     `json:"id"`
		FlowName string        `json:"flow_name"`
		Status   string        `json:"status"`
		Error    string        `json:"error,omitempty"`
		Nodes    []nodeRunView `json:"nodes"`
	}

	lanesOutput struct {
		Lanes []laneView `json:"lanes"`
	}
	cardTypesOutput struct {
		CardTypes []namedView `json:"card_types"`
	}
	projectsOutput struct {
		Projects []projectView `json:"projects"`
	}
	cardsOutput struct {
		Cards []cardView `json:"cards"`
	}
	flowsOutput struct {
		Flows []flowView `json:"flows"`
	}
	questionsOutput struct {
		Questions []questionView `json:"questions"`
	}

	listCardsInput struct {
		LaneID *uuid.UUID `json:"lane_id,omitempty" jsonschema:"only the cards of this lane"`
	}
	createCardInput struct {
		Title        string     `json:"title"`
		Description  string     `json:"description,omitempty"`
		LaneID       uuid.UUID  `json:"lane_id"`
		CardTypeID   uuid.UUID  `json:"card_type_id"`
		ProjectID    *uuid.UUID `json:"project_id,omitempty"`
		ParentCardID *uuid.UUID `json:"parent_card_id,omitempty"`
	}
	moveCardInput struct {
		CardID   uuid.UUID `json:"card_id"`
		LaneID   uuid.UUID `json:"lane_id"`
		Position *int      `json:"position,omitempty" jsonschema:"position in the lane, from 0; the end when absent"`
	}
	cardInput struct {
		CardID uuid.UUID `json:"card_id"`
	}
	answer struct {
		QuestionID uuid.UUID `json:"question_id"`
		Answer     string    `json:"answer"`
	}
	answersInput struct {
		Answers []answer `json:"answers"`
	}
)

// ErrNoRun means the card never ran a flow.
var ErrNoRun = errors.New("no flow ran for this card")

// integrationServer exposes the board to other tools.
func integrationServer(board Board) *sdk.Server {
	s := sdk.NewServer(implementation, nil)
	addTool(s, "list_lanes", "The lanes of the board, in order.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, lanesOutput, error) {
			lanes, err := board.ListLanes(ctx)
			return nil, lanesOutput{mapAll(lanes, func(l domain.Lane) laneView {
				return laneView{ID: l.ID, Name: l.Name, Position: l.Position}
			})}, err
		})
	addTool(s, "list_card_types", "The card types.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, cardTypesOutput, error) {
			types, err := board.ListCardTypes(ctx)
			return nil, cardTypesOutput{mapAll(types, func(t domain.CardType) namedView {
				return namedView{ID: t.ID, Name: t.Name}
			})}, err
		})
	addTool(s, "list_projects", "The projects cards can work on.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, projectsOutput, error) {
			projects, err := board.ListProjects(ctx)
			return nil, projectsOutput{mapAll(projects, func(p domain.Project) projectView {
				return projectView{ID: p.ID, Name: p.Name, LocalPath: p.LocalPath}
			})}, err
		})
	addTool(s, "list_cards", "The cards of the board, or of one lane.",
		func(ctx context.Context, _ *sdk.CallToolRequest, in listCardsInput) (*sdk.CallToolResult, cardsOutput, error) {
			cards, err := board.ListCards(ctx)
			views := []cardView{}
			for _, c := range cards {
				if in.LaneID == nil || *in.LaneID == c.LaneID {
					views = append(views, toCard(c))
				}
			}
			return nil, cardsOutput{views}, err
		})
	addTool(s, "create_card", "Creates a card at the end of a lane; flows triggered by it start.",
		func(ctx context.Context, _ *sdk.CallToolRequest, in createCardInput) (*sdk.CallToolResult, cardView, error) {
			card, err := board.CreateCard(ctx, domain.NewCard{
				CardInput: domain.CardInput{Title: in.Title, Description: in.Description, CardTypeID: in.CardTypeID, ProjectID: in.ProjectID},
				LaneID:    in.LaneID, ParentCardID: in.ParentCardID,
			})
			return nil, toCard(card), err
		})
	addTool(s, "move_card", "Moves a card to a lane; cards held by a running flow cannot move.",
		func(ctx context.Context, _ *sdk.CallToolRequest, in moveCardInput) (*sdk.CallToolResult, cardView, error) {
			position := math.MaxInt32
			if in.Position != nil {
				position = *in.Position
			}
			card, err := board.MoveCard(ctx, in.CardID, in.LaneID, position)
			return nil, toCard(card), err
		})
	addTool(s, "list_flows", "The flows and whether they are enabled.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, flowsOutput, error) {
			flows, err := board.ListFlows(ctx)
			return nil, flowsOutput{mapAll(flows, func(f domain.Flow) flowView {
				return flowView{ID: f.ID, Name: f.Name, Enabled: f.Enabled, LatestVersion: f.LatestVersion}
			})}, err
		})
	addTool(s, "list_questions", "The questions agents asked on a card, answered or not.",
		func(ctx context.Context, _ *sdk.CallToolRequest, in cardInput) (*sdk.CallToolResult, questionsOutput, error) {
			qs, err := board.ListCardQuestions(ctx, in.CardID)
			return nil, questionsOutput{toQuestions(qs)}, err
		})
	addTool(s, "answer_questions", "Answers questions; an agent resumes once all its questions are answered.",
		func(ctx context.Context, _ *sdk.CallToolRequest, in answersInput) (*sdk.CallToolResult, questionsOutput, error) {
			var answered []domain.Question
			for _, a := range in.Answers {
				q, err := board.AnswerQuestion(ctx, a.QuestionID, a.Answer)
				if err != nil {
					return nil, questionsOutput{}, err
				}
				answered = append(answered, q)
			}
			return nil, questionsOutput{toQuestions(answered)}, nil
		})
	addTool(s, "get_run", "The latest flow run of a card and its nodes.",
		func(ctx context.Context, _ *sdk.CallToolRequest, in cardInput) (*sdk.CallToolResult, runView, error) {
			run, err := latestRun(ctx, board, in.CardID)
			return nil, run, err
		})
	return s
}

func latestRun(ctx context.Context, board Board, cardID uuid.UUID) (runView, error) {
	runs, err := board.ListLatestFlowRuns(ctx)
	if err != nil {
		return runView{}, err
	}
	for _, r := range runs {
		if r.CardID != cardID {
			continue
		}
		detail, err := board.GetRunDetail(ctx, r.ID)
		return runView{
			ID: detail.ID, FlowName: detail.FlowName, Status: string(detail.Status), Error: detail.Error,
			Nodes: mapAll(detail.Nodes, func(n service.RunNode) nodeRunView {
				return nodeRunView{
					NodeKey: n.NodeKey, NodeType: string(n.Type), Iteration: n.Iteration, Status: string(n.Status),
					OutputHandle: n.OutputHandle, Error: n.Error, Turns: n.Turns, CostUSD: n.CostUSD,
				}
			}),
		}, err
	}
	return runView{}, ErrNoRun
}
