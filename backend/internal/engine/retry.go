package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"orquestrator/internal/domain"
)

// RetryTrigger is the trigger of the runs flow.retry jobs start.
const RetryTrigger = "retry"

// HandleRetry runs flow.retry jobs: it runs again, from its trigger and in
// its latest version, the flow of the last run of the card.
func (e *Engine) HandleRetry(ctx context.Context, payload json.RawMessage) error {
	var p domain.RetryJob
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("flow.retry payload: %w", err)
	}
	runs, err := e.Store.ListCardFlowRuns(ctx, p.CardID)
	if err != nil || len(runs) == 0 {
		return err // a card without runs has nothing to retry
	}
	flows, err := e.Store.RunnableFlows(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(flows, func(f domain.RunnableFlow) bool { return f.Version.FlowID == runs[0].FlowID })
	if i < 0 {
		return e.timeline(ctx, p.CardID, domain.FlowSkipped, flowSkipped{
			FlowID: runs[0].FlowID, FlowName: runs[0].FlowName, Reason: "disabled",
		})
	}
	flow := flows[i]
	j := slices.IndexFunc(flow.Version.Graph.Nodes, func(n domain.Node) bool { return n.Type == domain.NodeTrigger })
	return e.start(ctx, flow, p.CardID, flow.Version.Graph.Nodes[j].ID, RetryTrigger) // published graphs have a trigger
}
