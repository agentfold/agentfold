// Package rest serves the REST API described by api/openapi.yaml and the
// board event stream.
package rest

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
	"orquestrator/internal/service"
	"orquestrator/internal/transport/rest/openapi"
)

// Board is the set of use cases the API exposes. *service.Service implements it.
type Board interface {
	ListLanes(ctx context.Context) ([]domain.Lane, error)
	GetLane(ctx context.Context, id uuid.UUID) (domain.Lane, error)
	CreateLane(ctx context.Context, in domain.LaneInput) (domain.Lane, error)
	UpdateLane(ctx context.Context, id uuid.UUID, in domain.LaneInput) (domain.Lane, error)
	MoveLane(ctx context.Context, id uuid.UUID, position int) (domain.Lane, error)
	DeleteLane(ctx context.Context, id uuid.UUID) error

	ListProjects(ctx context.Context) ([]domain.Project, error)
	GetProject(ctx context.Context, id uuid.UUID) (domain.Project, error)
	CreateProject(ctx context.Context, in domain.ProjectInput) (domain.Project, error)
	UpdateProject(ctx context.Context, id uuid.UUID, in domain.ProjectInput) (domain.Project, error)
	DeleteProject(ctx context.Context, id uuid.UUID) error

	ListCardTypes(ctx context.Context) ([]domain.CardType, error)
	GetCardType(ctx context.Context, id uuid.UUID) (domain.CardType, error)
	CreateCardType(ctx context.Context, in domain.CardTypeInput) (domain.CardType, error)
	UpdateCardType(ctx context.Context, id uuid.UUID, in domain.CardTypeInput) (domain.CardType, error)
	DeleteCardType(ctx context.Context, id uuid.UUID) error

	ListCards(ctx context.Context) ([]domain.Card, error)
	ListChildCards(ctx context.Context, parentID uuid.UUID) ([]domain.Card, error)
	GetCard(ctx context.Context, id uuid.UUID) (domain.Card, error)
	CreateCard(ctx context.Context, in domain.NewCard) (domain.Card, error)
	UpdateCard(ctx context.Context, id uuid.UUID, in domain.CardInput) (domain.Card, error)
	MoveCard(ctx context.Context, id, laneID uuid.UUID, position int) (domain.Card, error)
	DeleteCard(ctx context.Context, id uuid.UUID) error
	ListCardEvents(ctx context.Context, cardID uuid.UUID) ([]domain.CardEvent, error)

	ListLatestFlowRuns(ctx context.Context) ([]domain.FlowRun, error)

	ListCardRuns(ctx context.Context, cardID uuid.UUID) ([]service.RunDetail, error)
	CancelFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error)
	ListNodeOutput(ctx context.Context, nodeRunID uuid.UUID) ([]domain.NodeOutput, error)

	ListCardQuestions(ctx context.Context, cardID uuid.UUID) ([]domain.Question, error)
	AnswerQuestion(ctx context.Context, id uuid.UUID, answer string) (domain.Question, error)

	ListCardArtifacts(ctx context.Context, cardID uuid.UUID) ([]service.ArtifactDetail, error)
	GetArtifact(ctx context.Context, id uuid.UUID) (service.ArtifactDetail, error)
	ReviseArtifact(ctx context.Context, id uuid.UUID, version int, text string) (domain.ArtifactMessage, error)

	ListFlows(ctx context.Context) ([]domain.Flow, error)
	GetFlow(ctx context.Context, id uuid.UUID) (service.FlowDetail, error)
	CreateFlow(ctx context.Context, name string) (service.FlowDetail, error)
	UpdateFlow(ctx context.Context, id uuid.UUID, in domain.FlowInput) (service.FlowDetail, error)
	SaveFlowDraft(ctx context.Context, id uuid.UUID, draft domain.Graph) (service.FlowDetail, error)
	PublishFlow(ctx context.Context, id uuid.UUID) (domain.FlowVersion, error)
	DeleteFlow(ctx context.Context, id uuid.UUID) error
	ListFlowVersions(ctx context.Context, id uuid.UUID) ([]domain.FlowVersion, error)

	ListAgentProfiles(ctx context.Context) ([]domain.AgentProfile, error)
	GetAgentProfile(ctx context.Context, id uuid.UUID) (domain.AgentProfile, error)
	CreateAgentProfile(ctx context.Context, in domain.AgentProfileInput) (domain.AgentProfile, error)
	UpdateAgentProfile(ctx context.Context, id uuid.UUID, in domain.AgentProfileInput) (domain.AgentProfile, error)
	DeleteAgentProfile(ctx context.Context, id uuid.UUID) error

	ListMcpServers(ctx context.Context) ([]domain.McpServer, error)
	GetMcpServer(ctx context.Context, id uuid.UUID) (domain.McpServer, error)
	CreateMcpServer(ctx context.Context, in domain.McpServerInput) (domain.McpServer, error)
	UpdateMcpServer(ctx context.Context, id uuid.UUID, in domain.McpServerInput) (domain.McpServer, error)
	DeleteMcpServer(ctx context.Context, id uuid.UUID) error
	TestMcpServer(ctx context.Context, in domain.McpServerTest) (domain.McpTestResult, error)
}

// Subscriber gives access to the live board events. *events.Hub implements it.
type Subscriber interface {
	Subscribe() (<-chan events.Event, func())
}

// Options configures the handler.
type Options struct {
	Log *slog.Logger
	// Heartbeat is the interval of keep-alive comments on the event stream.
	Heartbeat time.Duration
	// MCP serves the MCP endpoint at /mcp; none when nil.
	MCP http.Handler
}

// NewHandler returns the API handler, mounted under /api, and the MCP one.
func NewHandler(board Board, hub Subscriber, opts Options) http.Handler {
	mux := http.NewServeMux()
	strict := openapi.NewStrictHandlerWithOptions(handlers{board: board}, nil, openapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  badRequest,
		ResponseErrorHandlerFunc: errorWriter(opts.Log),
	})
	openapi.HandlerWithOptions(strict, openapi.StdHTTPServerOptions{
		BaseURL:          "/api",
		BaseRouter:       mux,
		ErrorHandlerFunc: badRequest,
	})
	mux.Handle("GET /api/events", eventStream{hub: hub, heartbeat: opts.Heartbeat})
	if opts.MCP != nil {
		mux.Handle("/mcp", opts.MCP)
	}
	return mux
}
