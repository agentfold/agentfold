// Package mcp serves the board over the Model Context Protocol (streamable
// HTTP). Requests with a run token reach the tools of the agent running that
// node; requests without one reach the integration tools.
package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"orquestrator/internal/domain"
	"orquestrator/internal/service"
)

// Board is what the MCP tools use. *service.Service implements it.
type Board interface {
	AgentRun(ctx context.Context, token string) (service.AgentRun, error)
	AskHuman(ctx context.Context, run service.AgentRun, texts []string) ([]domain.Question, error)
	ReportDone(ctx context.Context, run service.AgentRun, summary string) error
	SubmitOutput(ctx context.Context, run service.AgentRun, output json.RawMessage) error
	Decide(ctx context.Context, run service.AgentRun, decision, feedback string) error

	ListLanes(ctx context.Context) ([]domain.Lane, error)
	ListCardTypes(ctx context.Context) ([]domain.CardType, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
	ListCards(ctx context.Context) ([]domain.Card, error)
	GetCard(ctx context.Context, id uuid.UUID) (domain.Card, error)
	CreateCard(ctx context.Context, in domain.NewCard) (domain.Card, error)
	MoveCard(ctx context.Context, id, laneID uuid.UUID, position int) (domain.Card, error)
	ListFlows(ctx context.Context) ([]domain.Flow, error)
	ListCardQuestions(ctx context.Context, cardID uuid.UUID) ([]domain.Question, error)
	AnswerQuestion(ctx context.Context, id uuid.UUID, answer string) (domain.Question, error)
	ListLatestFlowRuns(ctx context.Context) ([]domain.FlowRun, error)
	GetRunDetail(ctx context.Context, id uuid.UUID) (service.RunDetail, error)
	ListCardArtifacts(ctx context.Context, cardID uuid.UUID) ([]service.ArtifactDetail, error)
}

var implementation = &sdk.Implementation{Name: "orquestrator", Version: "0.1.0"}

type runKey struct{}

// NewHandler serves the MCP endpoint. A request whose Authorization header
// carries an invalid or expired run token is refused with 401.
func NewHandler(board Board, log *slog.Logger) http.Handler {
	integration := integrationServer(board)
	mcp := sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		if run, ok := r.Context().Value(runKey{}).(service.AgentRun); ok {
			return agentServer(board, run)
		}
		return integration
	}, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: log})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			token, ok := strings.CutPrefix(auth, "Bearer ")
			run, err := board.AgentRun(r.Context(), token)
			if !ok || err != nil {
				log.WarnContext(r.Context(), "MCP request refused", "error", err)
				http.Error(w, "invalid or expired run token", http.StatusUnauthorized)
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), runKey{}, run))
		}
		mcp.ServeHTTP(w, r)
	})
}

// schemaOptions describes uuids as strings: by default they would be byte
// arrays.
var schemaOptions = &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[uuid.UUID](): {Type: "string", Format: "uuid"},
}}

// addTool adds a typed tool whose input (and structured output, unless Out
// is any) schemas are inferred with schemaOptions.
func addTool[In, Out any](s *sdk.Server, name, description string, h sdk.ToolHandlerFor[In, Out]) {
	tool := &sdk.Tool{Name: name, Description: description, InputSchema: mustSchema[In]()}
	if reflect.TypeFor[Out]() != reflect.TypeFor[any]() {
		tool.OutputSchema = mustSchema[Out]()
	}
	sdk.AddTool(s, tool, h)
}

func mustSchema[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](schemaOptions)
	if err != nil {
		panic(err) // the tool types above always have schemas
	}
	return schema
}

// text is a tool result holding a message.
func text(msg string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: msg}}}
}

// cardView is a card as the tools show it.
type cardView struct {
	ID           uuid.UUID  `json:"id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	LaneID       uuid.UUID  `json:"lane_id"`
	CardTypeID   uuid.UUID  `json:"card_type_id"`
	ProjectID    *uuid.UUID `json:"project_id,omitempty"`
	ParentCardID *uuid.UUID `json:"parent_card_id,omitempty"`
	Position     int        `json:"position"`
	// Properties are set by the flows; DependsOn are the cards this one
	// depends on.
	Properties map[string]string `json:"properties,omitempty"`
	DependsOn  []uuid.UUID       `json:"depends_on,omitempty"`
}

func toCard(c domain.Card) cardView {
	return cardView{
		ID: c.ID, Title: c.Title, Description: c.Description, LaneID: c.LaneID, CardTypeID: c.CardTypeID,
		ProjectID: c.ProjectID, ParentCardID: c.ParentCardID, Position: c.Position,
		Properties: c.Properties, DependsOn: c.DependsOn,
	}
}

// questionView is a question as the tools show it.
type questionView struct {
	ID       uuid.UUID `json:"id"`
	CardID   uuid.UUID `json:"card_id"`
	Text     string    `json:"text"`
	Answer   *string   `json:"answer,omitempty"`
	Answered bool      `json:"answered"`
}

func toQuestions(qs []domain.Question) []questionView {
	out := make([]questionView, len(qs))
	for i, q := range qs {
		out[i] = questionView{ID: q.ID, CardID: q.CardID, Text: q.Text, Answer: q.Answer, Answered: q.Answer != nil}
	}
	return out
}

func mapAll[D, V any](items []D, conv func(D) V) []V {
	out := make([]V, len(items))
	for i, item := range items {
		out[i] = conv(item)
	}
	return out
}
