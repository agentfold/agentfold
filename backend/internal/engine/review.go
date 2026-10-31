package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
)

// review holds the run at a HumanReview node until the human asks for
// changes or approves, with its artifacts under review. What the node
// received is the agent answer when it comes back from changes. Without
// artifacts to review the node fails the run.
func (e *Engine) review(ctx context.Context, r *run, node domain.Node, nr domain.NodeRun, input json.RawMessage) error {
	var cfg domain.HumanReviewConfig
	_ = json.Unmarshal(node.Config, &cfg) // validated when published
	if cfg.ReviewsOutput() {
		return e.awaitApproval(ctx, r, nr)
	}
	names := make([]string, len(cfg.Artifacts))
	for i, name := range cfg.Artifacts {
		names[i] = strings.TrimSpace(name)
	}
	held, artifacts, err := e.Store.OpenReview(ctx, nr.ID, r.CardID, names, reviewReply(input))
	if errors.Is(err, domain.ErrNotFound) {
		return nil // the run ended meanwhile
	}
	if err != nil {
		return err
	}
	if len(artifacts) == 0 {
		msg := "não há artefatos para revisar: nenhum nó coletou artefatos neste run"
		if len(names) > 0 {
			msg = fmt.Sprintf("não há artefatos para revisar: o card não tem %s", strings.Join(names, ", "))
		}
		if nr, err = e.Store.FinishNodeRun(ctx, nr.ID, failed(msg)); err != nil {
			return err
		}
		if err := e.nodeFinished(ctx, r, nr, node.Type); err != nil {
			return err
		}
		return e.finish(ctx, r, domain.RunFailed, msg)
	}
	e.Pub.Publish(events.Event{Type: "node_run.updated", ID: held.ID, Data: held})
	fr := r.FlowRun
	fr.Status = domain.RunWaiting
	e.Pub.Publish(events.Event{Type: "flow_run.updated", ID: fr.ID, Data: fr})
	for _, a := range artifacts {
		e.Pub.Publish(events.Event{Type: "artifact.updated", ID: a.ID})
	}
	return nil
}

// awaitApproval holds the run at a HumanReview node until the human
// approves the output of the node before, what the node received, or asks
// for changes.
func (e *Engine) awaitApproval(ctx context.Context, r *run, nr domain.NodeRun) error {
	held, err := e.Store.AwaitApproval(ctx, nr.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil // the run ended meanwhile
	}
	if err != nil {
		return err
	}
	e.Pub.Publish(events.Event{Type: "node_run.updated", ID: held.ID, Data: held})
	fr := r.FlowRun
	fr.Status = domain.RunWaiting
	e.Pub.Publish(events.Event{Type: "flow_run.updated", ID: fr.ID, Data: fr})
	return nil
}

// reviewReply is what the review chat shows of the agent coming back from
// changes: its answer, or a note when it gave none.
func reviewReply(input json.RawMessage) string {
	if reply := strings.TrimSpace(domain.MessageFrom(input)); reply != "" {
		return reply
	}
	return "Revisão concluída."
}

// HandleContinue runs flow.continue jobs: it follows the output of a node
// run the board ended outside the engine, as a human review decided.
func (e *Engine) HandleContinue(ctx context.Context, payload json.RawMessage) error {
	var p domain.ContinueJob
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("flow.continue payload: %w", err)
	}
	return e.continueFrom(ctx, p.NodeRunID)
}
