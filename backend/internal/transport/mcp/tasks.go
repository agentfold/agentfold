package mcp

import (
	"context"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"orquestrator/internal/domain"
	"orquestrator/internal/service"
)

type (
	taskInput struct {
		Ref         string   `json:"ref,omitempty" jsonschema:"a short name for the task, for depends_on of the other tasks of this call"`
		Title       string   `json:"title"`
		Description string   `json:"description,omitempty" jsonschema:"everything whoever does the task needs (context, what to do, decisions, contracts with the other tasks): it must stand on its own"`
		Project     string   `json:"project,omitempty" jsonschema:"the exact name of the project the task works on (list_projects)"`
		Type        string   `json:"type" jsonschema:"the exact name of the card type of the task (list_card_types)"`
		DependsOn   []string `json:"depends_on,omitempty" jsonschema:"what must be done first: refs of tasks of this call or ids of existing cards"`
	}
	createTasksInput struct {
		Tasks []taskInput `json:"tasks" jsonschema:"every task, in one call"`
	}
	createdTaskView struct {
		Ref   string    `json:"ref,omitempty"`
		ID    uuid.UUID `json:"id"`
		Title string    `json:"title"`
	}
	createTasksOutput struct {
		Tasks []createdTaskView `json:"tasks"`
	}
	taskRunView struct {
		Flow   string `json:"flow"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
		Result string `json:"result,omitempty" jsonschema:"the last message an agent of the run handed on"`
	}
	taskView struct {
		cardView
		Type     string       `json:"type"`
		Lane     string       `json:"lane"`
		Canceled bool         `json:"canceled,omitempty"`
		LastRun  *taskRunView `json:"last_run,omitempty"`
	}
	listTasksOutput struct {
		Tasks []taskView `json:"tasks"`
	}
	retryTaskInput struct {
		CardID      uuid.UUID `json:"card_id"`
		Description string    `json:"description,omitempty" jsonschema:"a new description for the task, telling what to do differently; empty keeps it"`
	}
	cancelTaskInput struct {
		CardID uuid.UUID `json:"card_id"`
	}
	cardTypeView struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
		// CanCreateTasks is false for types without an initial lane.
		CanCreateTasks bool `json:"can_create_tasks"`
	}
	agentCardTypesOutput struct {
		CardTypes []cardTypeView `json:"card_types"`
	}
)

// addCardTypes adds the tool that lists the card types.
func addCardTypes(s *sdk.Server, board Board) {
	addTool(s, "list_card_types", "The card types of the board. Tasks are created by type name; only types that can_create_tasks take them.",
		func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, agentCardTypesOutput, error) {
			types, err := board.ListCardTypes(ctx)
			return nil, agentCardTypesOutput{mapAll(types, func(t domain.CardType) cardTypeView {
				return cardTypeView{ID: t.ID, Name: t.Name, CanCreateTasks: t.InitialLaneID != nil}
			})}, err
		})
}

// addTasks adds the task tools the allowed tools of the node list.
func addTasks(s *sdk.Server, board Board, run service.AgentRun, cfg domain.AgentConfig) {
	if cfg.AllowsBoardTool("create_tasks") {
		addTool(s, "create_tasks",
			"Creates tasks: child cards of your card, each run by the flows of its type. Plan every task first, then create them all in one call. "+
				"Creating them does not wait for them: finish with a summary and end your turn; the flow takes it from there.",
			func(ctx context.Context, _ *sdk.CallToolRequest, in createTasksInput) (*sdk.CallToolResult, createTasksOutput, error) {
				specs := mapAll(in.Tasks, func(t taskInput) service.TaskSpec {
					return service.TaskSpec{Type: t.Type, ChildCardSpec: domain.ChildCardSpec{
						Ref: t.Ref, Title: t.Title, Description: t.Description, Project: t.Project, DependsOn: t.DependsOn,
					}}
				})
				cards, err := board.CreateTasks(ctx, run, specs)
				out := createTasksOutput{Tasks: make([]createdTaskView, len(cards))}
				for i, c := range cards {
					out.Tasks[i] = createdTaskView{Ref: in.Tasks[i].Ref, ID: c.ID, Title: c.Title}
				}
				return nil, out, err
			})
	}
	if cfg.AllowsBoardTool("list_tasks") {
		addTool(s, "list_tasks", "The tasks (child cards) of your card: where each one is, its properties and how its last flow run went.",
			func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, listTasksOutput, error) {
				tasks, err := board.ListTasks(ctx, run)
				return nil, listTasksOutput{mapAll(tasks, toTask)}, err
			})
	}
	if cfg.AllowsBoardTool("retry_task") {
		addTool(s, "retry_task", "Runs again the flow whose last run of a task failed or was canceled, optionally with a new description.",
			func(ctx context.Context, _ *sdk.CallToolRequest, in retryTaskInput) (*sdk.CallToolResult, any, error) {
				if _, err := board.RetryTask(ctx, run, in.CardID, in.Description); err != nil {
					return nil, nil, err
				}
				return text("The task will run again."), nil, nil
			})
	}
	if cfg.AllowsBoardTool("cancel_task") {
		addTool(s, "cancel_task", "Cancels a task: its flow stops and the flow of your card no longer waits for it.",
			func(ctx context.Context, _ *sdk.CallToolRequest, in cancelTaskInput) (*sdk.CallToolResult, any, error) {
				if _, err := board.CancelTask(ctx, run, in.CardID); err != nil {
					return nil, nil, err
				}
				return text("Task canceled."), nil, nil
			})
	}
}

func toTask(t service.Task) taskView {
	v := taskView{cardView: toCard(t.Card), Type: t.TypeName, Lane: t.LaneName, Canceled: t.Card.CanceledAt != nil}
	if r := t.LastRun; r != nil {
		v.LastRun = &taskRunView{Flow: r.FlowName, Status: string(r.Status), Error: r.Error, Result: r.Result}
	}
	return v
}
