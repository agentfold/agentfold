package mcp

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"orquestrator/internal/domain"
	"orquestrator/internal/service"
)

type (
	childCardInput struct {
		Ref         string   `json:"ref,omitempty" jsonschema:"a short name for the card, for depends_on of the other cards of this call"`
		Title       string   `json:"title"`
		Description string   `json:"description,omitempty" jsonschema:"everything whoever works on the card needs (context, what to do, decisions, contracts with the other cards): it must stand on its own"`
		Project     string   `json:"project,omitempty" jsonschema:"the exact name of the project the card works on (list_projects)"`
		Type        string   `json:"type"`
		DependsOn   []string `json:"depends_on,omitempty" jsonschema:"what must be done first: refs of cards of this call or ids of existing cards"`
	}
	createCardsInput struct {
		Cards []childCardInput `json:"cards" jsonschema:"every card, in one call"`
	}
	createdCardView struct {
		Ref   string    `json:"ref,omitempty"`
		ID    uuid.UUID `json:"id"`
		Title string    `json:"title"`
	}
	createCardsOutput struct {
		Cards []createdCardView `json:"cards"`
	}
	childRunView struct {
		Flow   string `json:"flow"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
		Result string `json:"result,omitempty" jsonschema:"the last message an agent of the run handed on"`
	}
	childView struct {
		cardView
		Type     string        `json:"type"`
		Lane     string        `json:"lane"`
		Canceled bool          `json:"canceled,omitempty"`
		LastRun  *childRunView `json:"last_run,omitempty"`
	}
	listChildrenOutput struct {
		Cards []childView `json:"cards"`
	}
	retryCardInput struct {
		CardID      uuid.UUID `json:"card_id"`
		Description string    `json:"description,omitempty" jsonschema:"a new description for the card, telling what to do differently; empty keeps it"`
	}
)

// addCards adds the child card tools the allowed tools of the node list.
func addCards(ctx context.Context, s *sdk.Server, board Board, run service.AgentRun, cfg domain.AgentConfig) {
	if cfg.CreatesCards() {
		types, _ := board.CardTypesOf(ctx, run) // on an error, create_cards tells it
		addCreateCards(s, board, run, types)
	}
	if cfg.AllowsBoardTool("list_cards") {
		addTool(s, "list_cards", "The child cards of your card: where each one is, its properties and how its last flow run went.",
			func(ctx context.Context, _ *sdk.CallToolRequest, _ noInput) (*sdk.CallToolResult, listChildrenOutput, error) {
				children, err := board.ListChildren(ctx, run)
				return nil, listChildrenOutput{mapAll(children, toChild)}, err
			})
	}
	if cfg.AllowsBoardTool("retry_card") {
		addTool(s, "retry_card", "Runs again the flow whose last run of a child card failed or was canceled, optionally with a new description.",
			func(ctx context.Context, _ *sdk.CallToolRequest, in retryCardInput) (*sdk.CallToolResult, any, error) {
				if _, err := board.RetryChild(ctx, run, in.CardID, in.Description); err != nil {
					return nil, nil, err
				}
				return text("The card will run again."), nil, nil
			})
	}
	if cfg.AllowsBoardTool("cancel_card") {
		addTool(s, "cancel_card", "Cancels a child card: its flow stops and the flow of your card no longer waits for it.",
			func(ctx context.Context, _ *sdk.CallToolRequest, in cardInput) (*sdk.CallToolResult, any, error) {
				if _, err := board.CancelChild(ctx, run, in.CardID); err != nil {
					return nil, nil, err
				}
				return text("Card canceled."), nil, nil
			})
	}
}

// addCreateCards adds create_cards, whose type parameter takes the names
// of the card types of the node; with a single type, it has none.
func addCreateCards(s *sdk.Server, board Board, run service.AgentRun, types []domain.CardType) {
	input := mustSchema[createCardsInput]()
	item := input.Properties["cards"].Items
	names := mapAll(types, func(t domain.CardType) string { return t.Name })
	description := "Creates child cards of your card, each run by the flows of its type. Plan every card first, then create them all in one call. " +
		"Creating them does not wait for them: finish with a summary and end your turn; the flow takes it from there."
	if len(names) == 1 {
		delete(item.Properties, "type")
		item.Required = slices.DeleteFunc(item.Required, func(p string) bool { return p == "type" })
		description += " The cards are of the type " + names[0] + "."
	} else {
		item.Properties["type"].Description = "the card type of the card"
		for _, n := range names {
			item.Properties["type"].Enum = append(item.Properties["type"].Enum, n)
		}
		description += " The cards are of the types " + strings.Join(names, ", ") + "."
	}
	sdk.AddTool(s, &sdk.Tool{Name: "create_cards", Description: description, InputSchema: input, OutputSchema: mustSchema[createCardsOutput]()},
		func(ctx context.Context, _ *sdk.CallToolRequest, in createCardsInput) (*sdk.CallToolResult, createCardsOutput, error) {
			specs := mapAll(in.Cards, func(c childCardInput) service.CardSpec {
				return service.CardSpec{Type: c.Type, ChildCardSpec: domain.ChildCardSpec{
					Ref: c.Ref, Title: c.Title, Description: c.Description, Project: c.Project, DependsOn: c.DependsOn,
				}}
			})
			cards, err := board.CreateCards(ctx, run, specs)
			out := createCardsOutput{Cards: make([]createdCardView, len(cards))}
			for i, c := range cards {
				out.Cards[i] = createdCardView{Ref: in.Cards[i].Ref, ID: c.ID, Title: c.Title}
			}
			return nil, out, err
		})
}

func toChild(c service.Child) childView {
	v := childView{cardView: toCard(c.Card), Type: c.TypeName, Lane: c.LaneName, Canceled: c.Card.CanceledAt != nil}
	if r := c.LastRun; r != nil {
		v.LastRun = &childRunView{Flow: r.FlowName, Status: string(r.Status), Error: r.Error, Result: r.Result}
	}
	return v
}
