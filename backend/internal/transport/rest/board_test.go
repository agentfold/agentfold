package rest

import (
	"context"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/service"
)

// stubBoard returns its fields from every use case and records the last call.
type stubBoard struct {
	err       error
	lane      domain.Lane
	project   domain.Project
	cardType  domain.CardType
	card      domain.Card
	cardEvent domain.CardEvent
	run       domain.FlowRun
	question  domain.Question
	nodeRun   domain.NodeRun
	flow      service.FlowDetail
	version   domain.FlowVersion
	profile   domain.AgentProfile
	mcp       domain.McpServer
	mcpTest   domain.McpTestResult
	artifact  service.ArtifactDetail

	call string
	args []any
}

func (b *stubBoard) record(call string, args ...any) error {
	b.call, b.args = call, args
	return b.err
}

func (b *stubBoard) ListLanes(context.Context) ([]domain.Lane, error) {
	return []domain.Lane{b.lane}, b.record("ListLanes")
}

func (b *stubBoard) GetLane(_ context.Context, id uuid.UUID) (domain.Lane, error) {
	return b.lane, b.record("GetLane", id)
}

func (b *stubBoard) CreateLane(_ context.Context, in domain.LaneInput) (domain.Lane, error) {
	return b.lane, b.record("CreateLane", in)
}

func (b *stubBoard) UpdateLane(_ context.Context, id uuid.UUID, in domain.LaneInput) (domain.Lane, error) {
	return b.lane, b.record("UpdateLane", id, in)
}

func (b *stubBoard) MoveLane(_ context.Context, id uuid.UUID, position int) (domain.Lane, error) {
	return b.lane, b.record("MoveLane", id, position)
}

func (b *stubBoard) DeleteLane(_ context.Context, id uuid.UUID) error {
	return b.record("DeleteLane", id)
}

func (b *stubBoard) ListProjects(context.Context) ([]domain.Project, error) {
	return []domain.Project{b.project}, b.record("ListProjects")
}

func (b *stubBoard) GetProject(_ context.Context, id uuid.UUID) (domain.Project, error) {
	return b.project, b.record("GetProject", id)
}

func (b *stubBoard) CreateProject(_ context.Context, in domain.ProjectInput) (domain.Project, error) {
	return b.project, b.record("CreateProject", in)
}

func (b *stubBoard) UpdateProject(_ context.Context, id uuid.UUID, in domain.ProjectInput) (domain.Project, error) {
	return b.project, b.record("UpdateProject", id, in)
}

func (b *stubBoard) DeleteProject(_ context.Context, id uuid.UUID) error {
	return b.record("DeleteProject", id)
}

func (b *stubBoard) ListCardTypes(context.Context) ([]domain.CardType, error) {
	return []domain.CardType{b.cardType}, b.record("ListCardTypes")
}

func (b *stubBoard) GetCardType(_ context.Context, id uuid.UUID) (domain.CardType, error) {
	return b.cardType, b.record("GetCardType", id)
}

func (b *stubBoard) CreateCardType(_ context.Context, in domain.CardTypeInput) (domain.CardType, error) {
	return b.cardType, b.record("CreateCardType", in)
}

func (b *stubBoard) UpdateCardType(_ context.Context, id uuid.UUID, in domain.CardTypeInput) (domain.CardType, error) {
	return b.cardType, b.record("UpdateCardType", id, in)
}

func (b *stubBoard) DeleteCardType(_ context.Context, id uuid.UUID) error {
	return b.record("DeleteCardType", id)
}

func (b *stubBoard) ListCards(context.Context) ([]domain.Card, error) {
	return []domain.Card{b.card}, b.record("ListCards")
}

func (b *stubBoard) ListChildCards(_ context.Context, parentID uuid.UUID) ([]domain.Card, error) {
	return []domain.Card{b.card}, b.record("ListChildCards", parentID)
}

func (b *stubBoard) GetCard(_ context.Context, id uuid.UUID) (domain.Card, error) {
	return b.card, b.record("GetCard", id)
}

func (b *stubBoard) CreateCard(_ context.Context, in domain.NewCard) (domain.Card, error) {
	return b.card, b.record("CreateCard", in)
}

func (b *stubBoard) UpdateCard(_ context.Context, id uuid.UUID, in domain.CardInput) (domain.Card, error) {
	return b.card, b.record("UpdateCard", id, in)
}

func (b *stubBoard) MoveCard(_ context.Context, id, laneID uuid.UUID, position int) (domain.Card, error) {
	return b.card, b.record("MoveCard", id, laneID, position)
}

func (b *stubBoard) DeleteCard(_ context.Context, id uuid.UUID) error {
	return b.record("DeleteCard", id)
}

func (b *stubBoard) ListCardRuns(_ context.Context, cardID uuid.UUID) ([]service.RunDetail, error) {
	return []service.RunDetail{{FlowRun: b.run, Nodes: []service.RunNode{{NodeRun: b.nodeRun, Type: domain.NodeAgent}}}}, b.record("ListCardRuns", cardID)
}

func (b *stubBoard) CancelFlowRun(_ context.Context, id uuid.UUID) (domain.FlowRun, error) {
	return b.run, b.record("CancelFlowRun", id)
}

func (b *stubBoard) ListNodeOutput(_ context.Context, id uuid.UUID) ([]domain.NodeOutput, error) {
	return []domain.NodeOutput{{ID: id1, NodeRunID: id2, Kind: domain.OutputToolUse, Tool: "Read", Text: "{}", CreatedAt: stamp}}, b.record("ListNodeOutput", id)
}

func (b *stubBoard) ListCardQuestions(_ context.Context, cardID uuid.UUID) ([]domain.Question, error) {
	return []domain.Question{b.question}, b.record("ListCardQuestions", cardID)
}

func (b *stubBoard) AnswerQuestion(_ context.Context, id uuid.UUID, answer string) (domain.Question, error) {
	return b.question, b.record("AnswerQuestion", id, answer)
}

func (b *stubBoard) ListCardArtifacts(_ context.Context, cardID uuid.UUID) ([]service.ArtifactDetail, error) {
	return []service.ArtifactDetail{b.artifact}, b.record("ListCardArtifacts", cardID)
}

func (b *stubBoard) GetArtifact(_ context.Context, id uuid.UUID) (service.ArtifactDetail, error) {
	return b.artifact, b.record("GetArtifact", id)
}

func (b *stubBoard) RequestArtifactChanges(_ context.Context, id uuid.UUID, text string) (domain.ArtifactMessage, error) {
	return b.artifact.Messages[0], b.record("RequestArtifactChanges", id, text)
}

func (b *stubBoard) ApproveArtifact(_ context.Context, id uuid.UUID) error {
	return b.record("ApproveArtifact", id)
}

func (b *stubBoard) ListLatestFlowRuns(context.Context) ([]domain.FlowRun, error) {
	return []domain.FlowRun{b.run}, b.record("ListLatestFlowRuns")
}

func (b *stubBoard) ListCardEvents(_ context.Context, cardID uuid.UUID) ([]domain.CardEvent, error) {
	return []domain.CardEvent{b.cardEvent}, b.record("ListCardEvents", cardID)
}

func (b *stubBoard) ListFlows(context.Context) ([]domain.Flow, error) {
	return []domain.Flow{b.flow.Flow}, b.record("ListFlows")
}

func (b *stubBoard) GetFlow(_ context.Context, id uuid.UUID) (service.FlowDetail, error) {
	return b.flow, b.record("GetFlow", id)
}

func (b *stubBoard) CreateFlow(_ context.Context, name string) (service.FlowDetail, error) {
	return b.flow, b.record("CreateFlow", name)
}

func (b *stubBoard) UpdateFlow(_ context.Context, id uuid.UUID, in domain.FlowInput) (service.FlowDetail, error) {
	return b.flow, b.record("UpdateFlow", id, in)
}

func (b *stubBoard) SaveFlowDraft(_ context.Context, id uuid.UUID, draft domain.Graph) (service.FlowDetail, error) {
	return b.flow, b.record("SaveFlowDraft", id, draft)
}

func (b *stubBoard) PublishFlow(_ context.Context, id uuid.UUID) (domain.FlowVersion, error) {
	return b.version, b.record("PublishFlow", id)
}

func (b *stubBoard) DeleteFlow(_ context.Context, id uuid.UUID) error {
	return b.record("DeleteFlow", id)
}

func (b *stubBoard) ListFlowVersions(_ context.Context, id uuid.UUID) ([]domain.FlowVersion, error) {
	return []domain.FlowVersion{b.version}, b.record("ListFlowVersions", id)
}

func (b *stubBoard) ListAgentProfiles(context.Context) ([]domain.AgentProfile, error) {
	return []domain.AgentProfile{b.profile}, b.record("ListAgentProfiles")
}

func (b *stubBoard) GetAgentProfile(_ context.Context, id uuid.UUID) (domain.AgentProfile, error) {
	return b.profile, b.record("GetAgentProfile", id)
}

func (b *stubBoard) CreateAgentProfile(_ context.Context, in domain.AgentProfileInput) (domain.AgentProfile, error) {
	return b.profile, b.record("CreateAgentProfile", in)
}

func (b *stubBoard) UpdateAgentProfile(_ context.Context, id uuid.UUID, in domain.AgentProfileInput) (domain.AgentProfile, error) {
	return b.profile, b.record("UpdateAgentProfile", id, in)
}

func (b *stubBoard) DeleteAgentProfile(_ context.Context, id uuid.UUID) error {
	return b.record("DeleteAgentProfile", id)
}

func (b *stubBoard) ListMcpServers(context.Context) ([]domain.McpServer, error) {
	return []domain.McpServer{b.mcp}, b.record("ListMcpServers")
}

func (b *stubBoard) GetMcpServer(_ context.Context, id uuid.UUID) (domain.McpServer, error) {
	return b.mcp, b.record("GetMcpServer", id)
}

func (b *stubBoard) CreateMcpServer(_ context.Context, in domain.McpServerInput) (domain.McpServer, error) {
	return b.mcp, b.record("CreateMcpServer", in)
}

func (b *stubBoard) UpdateMcpServer(_ context.Context, id uuid.UUID, in domain.McpServerInput) (domain.McpServer, error) {
	return b.mcp, b.record("UpdateMcpServer", id, in)
}

func (b *stubBoard) TestMcpServer(_ context.Context, in domain.McpServerTest) (domain.McpTestResult, error) {
	return b.mcpTest, b.record("TestMcpServer", in)
}

func (b *stubBoard) DeleteMcpServer(_ context.Context, id uuid.UUID) error {
	return b.record("DeleteMcpServer", id)
}
