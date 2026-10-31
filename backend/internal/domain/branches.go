package domain

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// SplitConfig names the branches of a Split node: each is an output, and
// the run walks the linked ones in parallel until their Join.
type SplitConfig struct {
	Branches []string `json:"branches,omitempty"`
}

// Join modes: all waits for every branch; first goes on with the first
// branch to arrive, canceling the others.
const (
	JoinAll   = "all"
	JoinFirst = "first"
)

// JoinConfig closes the branches of a Split. With FailFast, mode all
// leaves by some_failed at the first failed branch, canceling the others,
// instead of after every branch ends. An empty Mode is JoinAll.
type JoinConfig struct {
	Mode     string `json:"mode,omitempty"`
	FailFast bool   `json:"fail_fast,omitempty"`
}

// BranchStatus is the state of a branch of a Split.
type BranchStatus string

// Branch states: arrived at the Join, done at an End, failed by a node
// whose failure leads nowhere, or canceled when the Join went on without
// it.
const (
	BranchActive   BranchStatus = "active"
	BranchArrived  BranchStatus = "arrived"
	BranchDone     BranchStatus = "done"
	BranchFailed   BranchStatus = "failed"
	BranchCanceled BranchStatus = "canceled"
)

// Branch is a path a Split node opened in a flow run. Label names it in
// the run, with the branches it is nested in (split.a/inner.b).
type Branch struct {
	ID             uuid.UUID
	FlowRunID      uuid.UUID
	SplitNodeRunID uuid.UUID
	SplitKey       string
	ParentID       *uuid.UUID
	Name           string
	Label          string
	Status         BranchStatus
	// Output is what arrived at the Join; Error why the branch failed.
	Output json.RawMessage
	Error  string
}

// Settlement is what ending a branch did: whether it settled its split,
// the branches of the split, and the branch the split ran in (nil on the
// main path of the run).
type Settlement struct {
	Settled  bool
	Branches []Branch
	Parent   *Branch
}

// BranchEnd is how a branch ended.
type BranchEnd struct {
	Status BranchStatus
	Output json.RawMessage
	Error  string
}

// JobFlowBranch is the queue job that walks a branch from its first node.
const JobFlowBranch = "flow.branch"

// BranchJob is the payload of a flow.branch job.
type BranchJob struct {
	BranchID uuid.UUID       `json:"branch_id"`
	NodeKey  string          `json:"node_key"`
	Input    json.RawMessage `json:"input,omitempty"`
}

// Releases tells whether the branches of a Split settle its Join: when
// none is active, when one arrived in first mode, or when one failed with
// FailFast.
func (c JoinConfig) Releases(branches []Branch) bool {
	active, arrived, failed := false, false, false
	for _, b := range branches {
		active = active || b.Status == BranchActive
		arrived = arrived || b.Status == BranchArrived
		failed = failed || b.Status == BranchFailed
	}
	switch {
	case !active:
		return true
	case c.Mode == JoinFirst:
		return arrived
	}
	return c.FailFast && failed
}

// Result is how a released Join leaves: by some_failed when a branch
// failed (in first mode, only when none arrived), else by all_done, with
// {"output": {"branches": {<name>: <output>}, "failed": {<name>: <error>}}}.
func (c JoinConfig) Result(branches []Branch) NodeResult {
	arrived, failed := map[string]any{}, map[string]string{}
	for _, b := range branches {
		switch b.Status {
		case BranchArrived:
			arrived[b.Name] = outputData(b.Output)
		case BranchFailed:
			failed[b.Name] = b.Error
		}
	}
	out := map[string]any{"branches": arrived}
	handle := "all_done"
	if len(failed) > 0 {
		out["failed"] = failed
		if c.Mode != JoinFirst || len(arrived) == 0 {
			handle = "some_failed"
		}
	}
	output, _ := json.Marshal(map[string]any{"output": out}) // decoded JSON values: marshals
	return NodeResult{Status: NodeDone, OutputHandle: handle, Output: output}
}

// BranchFailures describes the failed branches, empty when none failed.
func BranchFailures(branches []Branch) string {
	var out []string
	for _, b := range branches {
		if b.Status == BranchFailed {
			out = append(out, fmt.Sprintf("o ramo %s falhou: %s", b.Label, b.Error))
		}
	}
	return strings.Join(out, "; ")
}

// branchTypes are the node types a branch may run: agents would share the
// workspace and git branch of the card, and nodes that wait for humans or
// other cards hold the whole run.
var branchTypes = []NodeType{
	NodeHTTPRequest, NodeTransform, NodeSwitch, NodeSetProperty, NodeCheckProperty,
	NodeMoveCard, NodeEnd, NodeSplit, NodeJoin,
}

// graphIndex finds the nodes and edges of a graph.
type graphIndex struct {
	nodes map[string]Node
	out   map[string][]Edge
	in    map[string][]Edge
}

func indexGraph(g Graph) graphIndex {
	idx := graphIndex{nodes: map[string]Node{}, out: map[string][]Edge{}, in: map[string][]Edge{}}
	for _, n := range g.Nodes {
		idx.nodes[n.ID] = n
	}
	for _, e := range g.Edges {
		idx.out[e.Source] = append(idx.out[e.Source], e)
		idx.in[e.Target] = append(idx.in[e.Target], e)
	}
	return idx
}

// region is what the branches of a Split reach: the nodes of each linked
// branch, by name, and the Joins they arrive at.
type region struct {
	branches map[string]map[string]bool
	joins    map[string]bool
}

// splitRegion explores the branches of split. A nested Split counts with
// its own region and goes on after its Join; visiting guards cycles.
func (idx graphIndex) splitRegion(split string, visiting map[string]bool) region {
	visiting[split] = true
	defer delete(visiting, split)
	r := region{branches: map[string]map[string]bool{}, joins: map[string]bool{}}
	for _, e := range idx.out[split] {
		nodes := map[string]bool{}
		queue := []string{e.Target}
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			n, ok := idx.nodes[id]
			switch {
			case !ok || nodes[id]:
				continue
			case n.Type == NodeJoin:
				r.joins[id] = true
				continue
			}
			nodes[id] = true
			next := idx.out[id]
			if n.Type == NodeSplit && visiting[id] {
				continue // a branch back to a split it is in: reported
			}
			if n.Type == NodeSplit {
				inner := idx.splitRegion(id, visiting)
				for _, set := range inner.branches {
					maps.Copy(nodes, set)
				}
				next = nil
				if join, ok := single(inner.joins); ok {
					nodes[join] = true
					next = idx.out[join]
				}
			}
			for _, out := range next {
				queue = append(queue, out.Target)
			}
		}
		r.branches[e.SourceHandle] = nodes
	}
	return r
}

// single returns the only key of set.
func single(set map[string]bool) (string, bool) {
	keys := slices.Collect(maps.Keys(set))
	if len(keys) != 1 {
		return "", false
	}
	return keys[0], true
}

// SplitJoin returns the Join node the branches of the Split node split
// arrive at in g, empty when they all end before one.
func (g Graph) SplitJoin(split string) string {
	join, _ := single(indexGraph(g).splitRegion(split, map[string]bool{}).joins)
	return join
}

// checkSplits checks the structure of parallel branches: every branch of
// a Split reaches the same Join or ends before one, no edge enters a
// branch from outside it, and branches only run the nodes they may.
func (v *validator) checkSplits(g Graph) {
	idx := indexGraph(g)
	closed := map[string]bool{}
	reported := map[string]bool{}
	report := func(node, msg string) {
		if !reported[node+"\x00"+msg] {
			reported[node+"\x00"+msg] = true
			v.node(node, "%s", msg)
		}
	}
	for _, s := range v.sortedNodes() {
		if s.Type != NodeSplit {
			continue
		}
		r := idx.splitRegion(s.ID, map[string]bool{})
		if len(r.branches) == 0 {
			report(s.ID, "ligue pelo menos um ramo do split")
		}
		joins := slices.Sorted(maps.Keys(r.joins))
		if len(joins) > 1 {
			report(s.ID, fmt.Sprintf("os ramos precisam chegar ao mesmo join (chegam a %s)", strings.Join(joins, ", ")))
		}
		owner := map[string]string{}
		for _, name := range slices.Sorted(maps.Keys(r.branches)) {
			for _, id := range slices.Sorted(maps.Keys(r.branches[name])) {
				n := idx.nodes[id]
				switch {
				case id == s.ID:
					report(id, "um ramo do split volta para ele")
					continue
				case owner[id] != "":
					report(id, fmt.Sprintf("o nó está nos ramos %s e %s do split %s", owner[id], name, s.ID))
					continue
				case !slices.Contains(branchTypes, n.Type):
					report(id, fmt.Sprintf("o nó %s não roda num ramo paralelo (split %s)", n.Type, s.ID))
				case n.Type == NodeCheckProperty && waits(n):
					report(id, fmt.Sprintf("num ramo paralelo (split %s) o Conferir propriedade não pode esperar", s.ID))
				}
				owner[id] = name
			}
		}
		for id, name := range owner {
			for _, e := range idx.in[id] {
				if owner[e.Source] != name && (e.Source != s.ID || e.SourceHandle != name) {
					report(id, fmt.Sprintf("a ligação %s entra no ramo %s do split %s por fora", e.ID, name, s.ID))
				}
			}
		}
		for _, join := range joins {
			closed[join] = true
			for _, e := range idx.in[join] {
				if owner[e.Source] == "" && e.Source != s.ID {
					report(join, fmt.Sprintf("a ligação %s chega ao join por fora dos ramos do split %s", e.ID, s.ID))
				}
			}
		}
	}
	for _, n := range v.sortedNodes() {
		if n.Type == NodeJoin && !closed[n.ID] {
			report(n.ID, "o join não fecha os ramos de nenhum split")
		}
	}
}

func waits(n Node) bool {
	var c CheckPropertyConfig
	return json.Unmarshal(n.Config, &c) == nil && c.Wait
}

func splitProblems(c SplitConfig) []string {
	if len(c.Branches) < 2 {
		return []string{"o split precisa de pelo menos dois ramos"}
	}
	var out []string
	seen := map[string]bool{}
	for i, name := range c.Branches {
		switch {
		case !nodeID.MatchString(name):
			out = append(out, fmt.Sprintf("o nome do ramo %d só aceita letras, números, _ e -", i+1))
		case seen[name]:
			out = append(out, fmt.Sprintf("o ramo %s aparece mais de uma vez", name))
		}
		seen[name] = true
	}
	return out
}

func joinProblems(c JoinConfig) []string {
	switch {
	case c.Mode != "" && c.Mode != JoinAll && c.Mode != JoinFirst:
		return []string{fmt.Sprintf("modo do join desconhecido: %q", c.Mode)}
	case c.Mode == JoinFirst && c.FailFast:
		return []string{"falhar no primeiro erro só vale no modo todos"}
	}
	return nil
}
