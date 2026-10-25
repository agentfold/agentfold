package rest

import (
	"encoding/json"

	"orquestrator/internal/domain"
	"orquestrator/internal/service"
	"orquestrator/internal/transport/rest/openapi"
)

func toLane(l domain.Lane) openapi.Lane {
	return openapi.Lane{Id: l.ID, Name: l.Name, Color: l.Color, Position: l.Position, CreatedAt: l.CreatedAt}
}

func toProject(p domain.Project) openapi.Project {
	return openapi.Project{
		Id:         p.ID,
		Name:       p.Name,
		SourceKind: openapi.ProjectSourceKind(p.SourceKind),
		LocalPath:  p.LocalPath,
		CreatedAt:  p.CreatedAt,
	}
}

func toCardType(c domain.CardType) openapi.CardType {
	return openapi.CardType{Id: c.ID, Name: c.Name, Color: c.Color, CreatedAt: c.CreatedAt}
}

func toCard(c domain.Card) openapi.Card {
	return openapi.Card{
		Id:           c.ID,
		Title:        c.Title,
		Description:  c.Description,
		CardTypeId:   c.CardTypeID,
		ProjectId:    c.ProjectID,
		ParentCardId: c.ParentCardID,
		LaneId:       c.LaneID,
		Position:     c.Position,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
	}
}

func toCardEvent(e domain.CardEvent) openapi.CardEvent {
	payload := map[string]any{}
	_ = json.Unmarshal(e.Payload, &payload) // the store only holds JSON objects
	return openapi.CardEvent{Id: e.ID, CardId: e.CardID, Type: string(e.Type), Payload: payload, CreatedAt: e.CreatedAt}
}

func toFlowRun(r domain.FlowRun) openapi.FlowRun {
	return openapi.FlowRun{
		Id:            r.ID,
		FlowId:        r.FlowID,
		FlowVersionId: r.FlowVersionID,
		FlowName:      r.FlowName,
		CardId:        r.CardID,
		Trigger:       openapi.FlowRunTrigger(r.Trigger),
		Status:        openapi.FlowRunStatus(r.Status),
		Error:         r.Error,
		StartedAt:     r.StartedAt,
		FinishedAt:    r.FinishedAt,
	}
}

func toQuestion(q domain.Question) openapi.Question {
	return openapi.Question{
		Id: q.ID, CardId: q.CardID, NodeRunId: q.NodeRunID, Text: q.Text, Answer: q.Answer,
		AnsweredAt: q.AnsweredAt, Delivered: q.DeliveredAt != nil, CreatedAt: q.CreatedAt,
	}
}

func toNodeRun(n service.RunNode) openapi.NodeRun {
	out := openapi.NodeRun{
		Id: n.ID, FlowRunId: n.FlowRunID, NodeKey: n.NodeKey, NodeType: string(n.Type), Iteration: n.Iteration,
		Status: openapi.NodeRunStatus(n.Status), OutputHandle: n.OutputHandle, Error: n.Error, Summary: n.Summary,
		TokensIn: n.TokensIn, TokensOut: n.TokensOut, CacheRead: n.CacheRead, CacheWrite: n.CacheWrite,
		CostUsd: n.CostUSD, Turns: n.Turns, DurationMs: n.DurationMS,
		CreatedAt: n.CreatedAt, StartedAt: n.StartedAt, FinishedAt: n.FinishedAt,
	}
	var output map[string]any
	if json.Unmarshal(n.Output, &output) == nil && output != nil {
		out.Output = &output
	}
	return out
}

func toRunDetail(r service.RunDetail) openapi.FlowRunDetail {
	run := toFlowRun(r.FlowRun)
	return openapi.FlowRunDetail{
		Id: run.Id, FlowId: run.FlowId, FlowVersionId: run.FlowVersionId, FlowName: run.FlowName, CardId: run.CardId,
		Trigger: openapi.FlowRunDetailTrigger(run.Trigger), Status: openapi.FlowRunDetailStatus(run.Status),
		Error: run.Error, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		Nodes: mapAll(r.Nodes, toNodeRun),
	}
}

func toNodeOutput(o domain.NodeOutput) openapi.NodeOutput {
	return openapi.NodeOutput{
		Id: o.ID, NodeRunId: o.NodeRunID, Kind: openapi.NodeOutputKind(o.Kind), Text: o.Text, Tool: o.Tool,
		IsError: o.IsError, CreatedAt: o.CreatedAt,
	}
}

// toEntity converts the domain value carried by a board event.
func toEntity(v any) any {
	switch v := v.(type) {
	case domain.Lane:
		return toLane(v)
	case domain.Project:
		return toProject(v)
	case domain.CardType:
		return toCardType(v)
	case domain.Card:
		return toCard(v)
	case domain.CardEvent:
		return toCardEvent(v)
	case domain.FlowRun:
		return toFlowRun(v)
	case domain.Question:
		return toQuestion(v)
	case domain.NodeRun:
		return toNodeRun(service.RunNode{NodeRun: v})
	case domain.Flow:
		return toFlowSummary(v)
	case domain.AgentProfile:
		return toAgentProfile(v)
	case domain.McpServer:
		return toMcpServer(v)
	}
	return nil
}

func mapAll[D, A any](items []D, conv func(D) A) []A {
	out := make([]A, len(items))
	for i, item := range items {
		out[i] = conv(item)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
