package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"orquestrator/internal/domain"
	"orquestrator/internal/store"
)

// split opens a branch of the run per linked output of a Split node, each
// walked by its own flow.branch job, so workers run them in parallel.
func (e *Engine) split(ctx context.Context, r *run, br *domain.Branch, node domain.Node, nr domain.NodeRun, input json.RawMessage) error {
	var cfg domain.SplitConfig
	_ = json.Unmarshal(node.Config, &cfg) // validated when published
	targets := map[string]string{}
	var names []string
	for _, name := range cfg.Branches {
		for _, edge := range r.graph.Edges {
			if edge.Source == node.ID && edge.SourceHandle == name {
				names, targets[name] = append(names, name), edge.Target
			}
		}
	}
	nr, err := e.Store.FinishNodeRun(ctx, nr.ID, domain.NodeResult{Status: domain.NodeDone, Output: input})
	if err != nil {
		return err
	}
	if err := e.nodeFinished(ctx, r, nr, node.Type); err != nil {
		return err
	}
	branches, err := e.Store.StartSplit(ctx, nr, br, names)
	if err != nil {
		return err
	}
	for _, b := range branches {
		_, err := e.Store.EnqueueJob(ctx, store.NewJob{
			Kind: domain.JobFlowBranch, Payload: domain.BranchJob{BranchID: b.ID, NodeKey: targets[b.Name], Input: input},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// HandleBranch runs flow.branch jobs: it walks a branch from its first
// node, unless the branch or its run ended meanwhile.
func (e *Engine) HandleBranch(ctx context.Context, payload json.RawMessage) error {
	var p domain.BranchJob
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("flow.branch payload: %w", err)
	}
	br, err := e.Store.GetBranch(ctx, p.BranchID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil // the card was deleted meanwhile
	}
	if err != nil || br.Status != domain.BranchActive {
		return err
	}
	fr, err := e.Store.GetFlowRun(ctx, br.FlowRunID)
	if err != nil || !fr.Status.Active() {
		return err
	}
	version, err := e.Store.GetFlowVersion(ctx, fr.FlowVersionID)
	if err != nil {
		return err
	}
	return e.advance(ctx, &run{FlowRun: fr, graph: version.Graph}, &br, p.NodeKey, p.Input)
}

// arrive records that the branch br got to its Join, with input.
func (e *Engine) arrive(ctx context.Context, r *run, br *domain.Branch, input json.RawMessage) error {
	if br == nil {
		return e.finish(ctx, r, domain.RunFailed, "um join foi alcançado fora de um ramo")
	}
	return e.end(ctx, r, br, domain.BranchEnd{Status: domain.BranchArrived, Output: input})
}

// end ends the branch br, or the run on the main path (br nil). The
// branch that settles its split goes on: from the Join, in the branch the
// split ran in, or ending that one when the branches ended before a Join.
func (e *Engine) end(ctx context.Context, r *run, br *domain.Branch, end domain.BranchEnd) error {
	if br == nil {
		if end.Status == domain.BranchFailed {
			return e.finish(ctx, r, domain.RunFailed, end.Error)
		}
		return e.finish(ctx, r, domain.RunDone, "")
	}
	key := r.graph.SplitJoin(br.SplitKey)
	join, hasJoin := findNode(r.graph, key)
	var cfg domain.JoinConfig
	_ = json.Unmarshal(join.Config, &cfg) // validated when published
	st, err := e.Store.SettleBranch(ctx, br.ID, end, cfg.Releases)
	if err != nil || !st.Settled {
		return err
	}
	if !hasJoin {
		if msg := domain.BranchFailures(st.Branches); msg != "" {
			return e.end(ctx, r, st.Parent, domain.BranchEnd{Status: domain.BranchFailed, Error: msg})
		}
		return e.end(ctx, r, st.Parent, domain.BranchEnd{Status: domain.BranchDone})
	}
	nr, err := e.enter(ctx, r, st.Parent, join, nil)
	if err != nil {
		return err
	}
	if nr, err = e.Store.FinishNodeRun(ctx, nr.ID, cfg.Result(st.Branches)); err != nil {
		return err
	}
	if err := e.nodeFinished(ctx, r, nr, join.Type); err != nil {
		return err
	}
	next, err := e.route(ctx, r, st.Parent, key, nr.OutputHandle, nr.Error)
	if err != nil || next == "" {
		return err
	}
	return e.advance(ctx, r, st.Parent, next, nr.Output)
}
