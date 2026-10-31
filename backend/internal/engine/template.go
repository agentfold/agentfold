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
