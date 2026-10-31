package rest

import (
	"context"
	"encoding/json"

	"orquestrator/internal/domain"
	"orquestrator/internal/service"
	"orquestrator/internal/transport/rest/openapi"
)

// Graphs share their JSON shape between the contract and the domain, whose
// node configs stay raw until validated; converting goes through JSON.

func toAPIGraph(g domain.Graph) openapi.FlowGraph {
	var out openapi.FlowGraph
	data, _ := json.Marshal(g.Normalized()) // domain graphs always marshal
	_ = json.Unmarshal(data, &out)          // and their JSON fits FlowGraph
	return out
}

func fromAPIGraph(g openapi.FlowGraph) (domain.Graph, error) {
	var out domain.Graph
	data, _ := json.Marshal(g) // decoded request bodies always marshal
	if err := json.Unmarshal(data, &out); err != nil {
		return out, &domain.ValidationError{Field: "nodes", Message: "invalid node: " + err.Error()}
	}
	return out, nil
}

func toProblems(problems []domain.GraphProblem) []openapi.GraphProblem {
	return mapAll(problems, func(p domain.GraphProblem) openapi.GraphProblem {
		out := openapi.GraphProblem{Message: p.Message}
		if p.NodeID != "" {
			out.NodeId = &p.NodeID
		}
		if p.EdgeID != "" {
			out.EdgeId = &p.EdgeID
		}
		if p.Warning {
			out.Warning = &p.Warning
		}
		return out
	})
}

func toFlowSummary(f domain.Flow) openapi.FlowSummary {
	return openapi.FlowSummary{
		Id:            f.ID,
		Name:          f.Name,
		Enabled:       f.Enabled,
		LatestVersion: f.LatestVersion,
		CreatedAt:     f.CreatedAt,
		UpdatedAt:     f.UpdatedAt,
	}
}

func toFlow(f service.FlowDetail) openapi.Flow {
	return openapi.Flow{
		Id:            f.ID,
		Name:          f.Name,
		Enabled:       f.Enabled,
		LatestVersion: f.LatestVersion,
		CreatedAt:     f.CreatedAt,
		UpdatedAt:     f.UpdatedAt,
		Draft:         toAPIGraph(f.Draft),
		Problems:      toProblems(f.Problems),
	}
}

func toFlowVersion(v domain.FlowVersion) openapi.FlowVersion {
	return openapi.FlowVersion{
		Id:          v.ID,
		FlowId:      v.FlowID,
		Version:     v.Version,
		Graph:       toAPIGraph(v.Graph),
		PublishedAt: v.PublishedAt,
	}
}

func (h handlers) ListFlows(ctx context.Context, _ openapi.ListFlowsRequestObject) (openapi.ListFlowsResponseObject, error) {
	flows, err := h.board.ListFlows(ctx)
	return openapi.ListFlows200JSONResponse(mapAll(flows, toFlowSummary)), err
}

func (h handlers) CreateFlow(ctx context.Context, req openapi.CreateFlowRequestObject) (openapi.CreateFlowResponseObject, error) {
	flow, err := h.board.CreateFlow(ctx, req.Body.Name)
	return openapi.CreateFlow201JSONResponse(toFlow(flow)), err
}

func (h handlers) GetFlow(ctx context.Context, req openapi.GetFlowRequestObject) (openapi.GetFlowResponseObject, error) {
	flow, err := h.board.GetFlow(ctx, req.Id)
	return openapi.GetFlow200JSONResponse(toFlow(flow)), err
}

func (h handlers) UpdateFlow(ctx context.Context, req openapi.UpdateFlowRequestObject) (openapi.UpdateFlowResponseObject, error) {
	flow, err := h.board.UpdateFlow(ctx, req.Id, domain.FlowInput{Name: req.Body.Name, Enabled: req.Body.Enabled})
	return openapi.UpdateFlow200JSONResponse(toFlow(flow)), err
}

func (h handlers) DeleteFlow(ctx context.Context, req openapi.DeleteFlowRequestObject) (openapi.DeleteFlowResponseObject, error) {
	return openapi.DeleteFlow204Response{}, h.board.DeleteFlow(ctx, req.Id)
}

func (h handlers) SaveFlowDraft(ctx context.Context, req openapi.SaveFlowDraftRequestObject) (openapi.SaveFlowDraftResponseObject, error) {
	draft, err := fromAPIGraph(*req.Body)
	if err != nil {
		return nil, err
	}
	flow, err := h.board.SaveFlowDraft(ctx, req.Id, draft)
	return openapi.SaveFlowDraft200JSONResponse(toFlow(flow)), err
}

func (h handlers) PublishFlow(ctx context.Context, req openapi.PublishFlowRequestObject) (openapi.PublishFlowResponseObject, error) {
	version, err := h.board.PublishFlow(ctx, req.Id)
	return openapi.PublishFlow201JSONResponse(toFlowVersion(version)), err
}

func (h handlers) ListFlowVersions(ctx context.Context, req openapi.ListFlowVersionsRequestObject) (openapi.ListFlowVersionsResponseObject, error) {
	versions, err := h.board.ListFlowVersions(ctx, req.Id)
	return openapi.ListFlowVersions200JSONResponse(mapAll(versions, toFlowVersion)), err
}
