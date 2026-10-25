package rest

import (
	"encoding/json"

	"orquestrator/internal/domain"
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
