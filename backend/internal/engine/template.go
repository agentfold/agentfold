package engine

import (
	"context"
	"encoding/json"

	"orquestrator/internal/domain"
)

// templateData is what the placeholders of a node of fr read, given its
// input: the card and the last output of each node of the run.
func (e *Engine) templateData(ctx context.Context, fr domain.FlowRun, input json.RawMessage) (domain.TemplateData, error) {
	card, err := e.Store.GetCard(ctx, fr.CardID)
	if err != nil {
		return domain.TemplateData{}, err
	}
	runs, err := e.Store.ListNodeRuns(ctx, fr.ID)
	if err != nil {
		return domain.TemplateData{}, err
	}
	nodes := map[string]json.RawMessage{}
	for _, nr := range runs {
		if len(nr.Output) > 0 && string(nr.Output) != "null" {
			nodes[nr.NodeKey] = nr.Output
		}
	}
	return domain.TemplateData{Card: card, Input: input, Nodes: nodes}, nil
}

// transform builds the output of a Transform node; a failed expression
// leaves by failed, handing on the input.
func (e *Engine) transform(ctx context.Context, r *run, node domain.Node, input json.RawMessage) (domain.NodeResult, error) {
	var cfg domain.TransformConfig
	_ = json.Unmarshal(node.Config, &cfg) // validated when published
	data, err := e.templateData(ctx, r.FlowRun, input)
	if err != nil {
		return domain.NodeResult{}, err
	}
	out, err := cfg.Apply(data)
	if err != nil {
		return domain.NodeResult{Status: domain.NodeFailed, OutputHandle: "failed", Error: err.Error(), Output: input}, nil
	}
	return domain.NodeResult{Status: domain.NodeDone, OutputHandle: "next", Output: out}, nil
}

// choose sends the run on by the case of a Switch node that holds,
// handing on the input; a condition that fails leaves by failed.
func (e *Engine) choose(ctx context.Context, r *run, node domain.Node, input json.RawMessage) (domain.NodeResult, error) {
	var cfg domain.SwitchConfig
	_ = json.Unmarshal(node.Config, &cfg) // validated when published
	data, err := e.templateData(ctx, r.FlowRun, input)
	if err != nil {
		return domain.NodeResult{}, err
	}
	handle, err := cfg.Choose(data)
	if err != nil {
		return domain.NodeResult{Status: domain.NodeFailed, OutputHandle: "failed", Error: err.Error(), Output: input}, nil
	}
	return domain.NodeResult{Status: domain.NodeDone, OutputHandle: handle, Output: input}, nil
}
