package service

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func (f *fakeRepo) AddCardEvent(_ context.Context, cardID uuid.UUID, typ domain.CardEventType, payload any) (domain.CardEvent, error) {
	data, _ := json.Marshal(payload)
	return domain.CardEvent{ID: uuid.New(), CardID: cardID, Type: typ, Payload: data}, f.call("AddCardEvent:"+string(typ), payload)
}

func (f *fakeRepo) GetFlowVersion(_ context.Context, id uuid.UUID) (domain.FlowVersion, error) {
	if f.version.Graph.Nodes == nil {
		return domain.FlowVersion{Graph: domain.Graph{Nodes: []domain.Node{{ID: "dev", Type: domain.NodeAgent}}}}, f.call("GetFlowVersion", id)
	}
	return f.version, f.call("GetFlowVersion", id)
}

func (f *fakeRepo) ListNodeRuns(_ context.Context, id uuid.UUID) ([]domain.NodeRun, error) {
	return []domain.NodeRun{f.nodeRun}, f.call("ListNodeRuns", id)
}

func (f *fakeRepo) RunningNodeRunByToken(_ context.Context, hash []byte) (domain.NodeRun, error) {
	return f.nodeRun, f.call("RunningNodeRunByToken", string(hash))
}

func (f *fakeRepo) SetNodeRunSummary(_ context.Context, _ uuid.UUID, summary string) (domain.NodeRun, error) {
	return f.nodeRun, f.call("SetNodeRunSummary", summary)
}

func (f *fakeRepo) SetNodeRunOutput(_ context.Context, _ uuid.UUID, output json.RawMessage) (domain.NodeRun, error) {
	return f.nodeRun, f.call("SetNodeRunOutput", string(output))
}

func (f *fakeRepo) SetNodeRunDecision(_ context.Context, _ uuid.UUID, decision, feedback string) (domain.NodeRun, error) {
	return f.nodeRun, f.call("SetNodeRunDecision", decision+":"+feedback)
}

func (f *fakeRepo) GetArtifact(_ context.Context, id uuid.UUID) (domain.Artifact, error) {
	return domain.Artifact{ID: id, CardID: id}, f.call("GetArtifact", id)
}

func (f *fakeRepo) ListCardArtifacts(_ context.Context, cardID uuid.UUID) ([]domain.Artifact, error) {
	return []domain.Artifact{{ID: uuid.New(), CardID: cardID}}, f.call("ListCardArtifacts", cardID)
}

func (f *fakeRepo) ListArtifactVersions(_ context.Context, id uuid.UUID) ([]domain.ArtifactVersion, error) {
	return []domain.ArtifactVersion{{ArtifactID: id, Version: 1}}, f.call("ListArtifactVersions", id)
}

func (f *fakeRepo) ListArtifactMessages(_ context.Context, id uuid.UUID) ([]domain.ArtifactMessage, error) {
	return []domain.ArtifactMessage{{ArtifactID: id, Version: 1}}, f.call("ListArtifactMessages", id)
}

func (f *fakeRepo) ArtifactReview(_ context.Context, id uuid.UUID) (domain.ReviewState, error) {
	return domain.ReviewOpen, f.call("ArtifactReview", id)
}

func (f *fakeRepo) RequestChanges(_ context.Context, id uuid.UUID, text string) (domain.ArtifactMessage, domain.NodeRun, error) {
	return domain.ArtifactMessage{ArtifactID: id, Version: 1, Role: domain.RoleHuman, Text: text}, f.nodeRun, f.call("RequestChanges", text)
}

func (f *fakeRepo) ApproveReview(_ context.Context, id uuid.UUID) (domain.NodeRun, error) {
	return f.nodeRun, f.call("ApproveReview", id)
}

func (f *fakeRepo) AddQuestions(_ context.Context, cardID, nodeRunID uuid.UUID, texts []string) ([]domain.Question, error) {
	out := make([]domain.Question, len(texts))
	for i, t := range texts {
		out[i] = domain.Question{ID: uuid.New(), CardID: cardID, NodeRunID: nodeRunID, Text: t}
	}
	return out, f.call("AddQuestions", len(texts))
}

func (f *fakeRepo) ListCardQuestions(_ context.Context, cardID uuid.UUID) ([]domain.Question, error) {
	return []domain.Question{f.question}, f.call("ListCardQuestions", cardID)
}

func (f *fakeRepo) AnswerQuestion(_ context.Context, _ uuid.UUID, answer string) (domain.Question, error) {
	q := f.question
	q.Answer = &answer
	return q, f.call("AnswerQuestion", answer)
}

func (f *fakeRepo) ResumeIfAnswered(_ context.Context, id uuid.UUID) (domain.FlowRun, bool, error) {
	return f.flowRun, f.resumed, f.call("ResumeIfAnswered", id)
}

func (f *fakeRepo) GetNodeRun(_ context.Context, id uuid.UUID) (domain.NodeRun, error) {
	return f.nodeRun, f.call("GetNodeRun", id)
}

func (f *fakeRepo) ListNodeOutput(_ context.Context, id uuid.UUID) ([]domain.NodeOutput, error) {
	return []domain.NodeOutput{{NodeRunID: id}}, f.call("ListNodeOutput", id)
}

func (f *fakeRepo) ListCardFlowRuns(_ context.Context, cardID uuid.UUID) ([]domain.FlowRun, error) {
	return []domain.FlowRun{f.flowRun}, f.call("ListCardFlowRuns", cardID)
}

func (f *fakeRepo) CancelFlowRun(_ context.Context, id uuid.UUID) (domain.FlowRun, error) {
	return domain.FlowRun{ID: id, Status: domain.RunCanceled, Error: "cancelado"}, f.call("CancelFlowRun", id)
}
