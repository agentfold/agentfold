package domain

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Flow is a graph of nodes that runs for cards. The draft is edited freely;
// publishing freezes it into a FlowVersion.
type Flow struct {
	ID      uuid.UUID
	Name    string
	Enabled bool
	Draft   Graph
	// LatestVersion is the number of the last published version, 0 if none.
	LatestVersion int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// EntityID returns the flow id.
func (f Flow) EntityID() uuid.UUID { return f.ID }

// FlowVersion is an immutable, published graph of a flow.
type FlowVersion struct {
	ID          uuid.UUID
	FlowID      uuid.UUID
	Version     int
	Graph       Graph
	PublishedAt time.Time
}

// FlowInput holds the editable fields of a flow.
type FlowInput struct {
	Name    string
	Enabled bool
}

// Normalize trims the fields and validates them.
func (in FlowInput) Normalize() (FlowInput, error) {
	name, err := requireText("name", in.Name)
	in.Name = name
	return in, err
}

// NodeType names what a node does.
type NodeType string

// Node types.
const (
	NodeTrigger  NodeType = "trigger"
	NodeAgent    NodeType = "agent"
	NodeReview   NodeType = "review"
	NodeMoveCard NodeType = "move_card"
	NodeEnd      NodeType = "end"
)

// Outputs lists the outputs (edge source handles) of each node type.
var Outputs = map[NodeType][]string{
	NodeTrigger:  {"next"},
	NodeAgent:    {"done", "failed"},
	NodeReview:   {"approved", "changes_requested", "max_rounds_exceeded"},
	NodeMoveCard: {"next"},
	NodeEnd:      {},
}

// Graph is the content of a flow: nodes and the edges between their outputs
// and other nodes. Node configs stay raw JSON until validated, so drafts can
// hold incomplete configs.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Position is where the editor draws a node.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Node is a step of a flow.
type Node struct {
	ID       string          `json:"id"`
	Type     NodeType        `json:"type"`
	Position Position        `json:"position"`
	Config   json.RawMessage `json:"config"`
}

// Edge connects an output of a node to another node.
type Edge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	SourceHandle string `json:"source_handle"`
	Target       string `json:"target"`
}

// Trigger events.
const (
	EventCardCreated     = "card_created"
	EventCardEnteredLane = "card_entered_lane"
)

// TriggerConfig starts the flow for cards: when created or when entering a
// lane, optionally only for some card types (empty means all).
type TriggerConfig struct {
	Event       string      `json:"event"`
	LaneID      *uuid.UUID  `json:"lane_id,omitempty"`
	CardTypeIDs []uuid.UUID `json:"card_type_ids,omitempty"`
}

// AgentConfig runs an agent session on the card as a profile, with optional
// external MCP servers.
type AgentConfig struct {
	ProfileID       *uuid.UUID      `json:"profile_id,omitempty"`
	McpServerIDs    []uuid.UUID     `json:"mcp_server_ids,omitempty"`
	Prompt          string          `json:"prompt"`
	AllowedTools    []string        `json:"allowed_tools,omitempty"`
	DisallowedTools []string        `json:"disallowed_tools,omitempty"`
	OutputSchema    json.RawMessage `json:"output_schema,omitempty"`
	MaxTurns        *int            `json:"max_turns,omitempty"`
	Model           string          `json:"model,omitempty"`
}

// ReviewConfig runs a reviewer agent on the work of the previous agent.
type ReviewConfig struct {
	ProfileID       *uuid.UUID `json:"profile_id,omitempty"`
	Prompt          string     `json:"prompt"`
	AllowedTools    []string   `json:"allowed_tools,omitempty"`
	DisallowedTools []string   `json:"disallowed_tools,omitempty"`
	MaxRounds       int        `json:"max_rounds"`
}

// MoveCardConfig moves the card to a lane.
type MoveCardConfig struct {
	LaneID uuid.UUID `json:"lane_id"`
}

// GraphProblem is a reason a graph cannot be published.
type GraphProblem struct {
	NodeID  string `json:"node_id,omitempty"`
	EdgeID  string `json:"edge_id,omitempty"`
	Message string `json:"message"`
}

// GraphError is returned when publishing a graph with problems.
type GraphError struct {
	Problems []GraphProblem
}

func (e *GraphError) Error() string {
	return fmt.Sprintf("graph has %d problem(s)", len(e.Problems))
}

// References tells which entities exist, to validate configs.
type References struct {
	Lanes      map[uuid.UUID]bool
	CardTypes  map[uuid.UUID]bool
	Profiles   map[uuid.UUID]bool
	McpServers map[uuid.UUID]bool
}

// Problems lists everything that prevents publishing g: the structure must
// have exactly one trigger, valid edges and every node reachable from the
// trigger, and every node config must be complete. Unconnected outputs other
// than the trigger's are allowed: the run ends there.
func (g Graph) Problems(refs References) []GraphProblem {
	v := validator{refs: refs, nodes: map[string]Node{}}
	v.checkNodes(g.Nodes)
	v.checkEdges(g.Edges)
	v.checkReachable(g.Edges)
	return v.problems
}

type validator struct {
	refs     References
	nodes    map[string]Node
	trigger  string
	problems []GraphProblem
}

func (v *validator) node(id, format string, args ...any) {
	v.problems = append(v.problems, GraphProblem{NodeID: id, Message: fmt.Sprintf(format, args...)})
}

func (v *validator) edge(id, format string, args ...any) {
	v.problems = append(v.problems, GraphProblem{EdgeID: id, Message: fmt.Sprintf(format, args...)})
}

func (v *validator) checkNodes(nodes []Node) {
	triggers := 0
	for _, n := range nodes {
		if _, dup := v.nodes[n.ID]; dup || n.ID == "" {
			v.node(n.ID, "id de nó vazio ou repetido")
			continue
		}
		if _, known := Outputs[n.Type]; !known {
			v.node(n.ID, "tipo de nó desconhecido: %q", n.Type)
			continue
		}
		v.nodes[n.ID] = n
		if n.Type == NodeTrigger {
			triggers++
			v.trigger = n.ID
		}
		for _, msg := range v.configProblems(n) {
			v.node(n.ID, "%s", msg)
		}
	}
	if triggers != 1 {
		v.problems = append(v.problems, GraphProblem{Message: fmt.Sprintf("o fluxo precisa de exatamente um gatilho (tem %d)", triggers)})
		v.trigger = ""
	}
}

func (v *validator) checkEdges(edges []Edge) {
	ids := map[string]bool{}
	used := map[string]bool{}
	for _, e := range edges {
		source, okSource := v.nodes[e.Source]
		target, okTarget := v.nodes[e.Target]
		switch {
		case e.ID == "" || ids[e.ID]:
			v.edge(e.ID, "id de ligação vazio ou repetido")
		case !okSource || !okTarget:
			v.edge(e.ID, "ligação com nó inexistente")
		case !hasOutput(source.Type, e.SourceHandle):
			v.edge(e.ID, "o nó %s não tem a saída %q", e.Source, e.SourceHandle)
		case target.Type == NodeTrigger:
			v.edge(e.ID, "o gatilho não pode ser destino de ligação")
		case e.Source == e.Target:
			v.edge(e.ID, "um nó não pode se ligar a si mesmo")
		case used[e.Source+"\x00"+e.SourceHandle]:
			v.edge(e.ID, "a saída %q do nó %s já está ligada", e.SourceHandle, e.Source)
		}
		ids[e.ID] = true
		used[e.Source+"\x00"+e.SourceHandle] = true
	}
}

func (v *validator) checkReachable(edges []Edge) {
	if v.trigger == "" {
		return
	}
	next := map[string][]string{}
	for _, e := range edges {
		next[e.Source] = append(next[e.Source], e.Target)
	}
	if len(next[v.trigger]) == 0 {
		v.node(v.trigger, "o gatilho precisa estar ligado a um nó")
	}
	seen := map[string]bool{v.trigger: true}
	queue := []string{v.trigger}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, t := range next[id] {
			if _, exists := v.nodes[t]; exists && !seen[t] {
				seen[t] = true
				queue = append(queue, t)
			}
		}
	}
	for _, n := range v.sortedNodes() {
		if !seen[n.ID] {
			v.node(n.ID, "nó inalcançável a partir do gatilho")
		}
	}
}

// sortedNodes returns the valid nodes in a stable order for messages.
func (v *validator) sortedNodes() []Node {
	out := make([]Node, 0, len(v.nodes))
	for _, n := range v.nodes {
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b Node) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func hasOutput(t NodeType, handle string) bool {
	for _, o := range Outputs[t] {
		if o == handle {
			return true
		}
	}
	return false
}

// configProblems validates the config of a node of a known type.
func (v *validator) configProblems(n Node) []string {
	config := n.Config
	if len(config) == 0 {
		config = json.RawMessage("{}")
	}
	switch n.Type {
	case NodeTrigger:
		var c TriggerConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return []string{invalidConfig(err)}
		}
		return v.triggerProblems(c)
	case NodeAgent:
		var c AgentConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return []string{invalidConfig(err)}
		}
		return append(v.profileProblems(c.ProfileID), append(v.mcpProblems(c.McpServerIDs), agentProblems(c)...)...)
	case NodeReview:
		var c ReviewConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return []string{invalidConfig(err)}
		}
		return append(v.profileProblems(c.ProfileID), reviewProblems(c)...)
	case NodeMoveCard:
		var c MoveCardConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return []string{invalidConfig(err)}
		}
		if !v.refs.Lanes[c.LaneID] {
			return []string{"escolha a baia de destino"}
		}
	}
	return nil
}

func invalidConfig(err error) string {
	return "configuração inválida: " + err.Error()
}

func (v *validator) triggerProblems(c TriggerConfig) []string {
	var out []string
	switch c.Event {
	case EventCardCreated:
	case EventCardEnteredLane:
		if c.LaneID == nil || !v.refs.Lanes[*c.LaneID] {
			out = append(out, "escolha a baia que dispara o fluxo")
		}
	default:
		out = append(out, "escolha o evento do gatilho")
	}
	for _, id := range c.CardTypeIDs {
		if !v.refs.CardTypes[id] {
			out = append(out, "o filtro usa um tipo de card que não existe mais")
			break
		}
	}
	return out
}

func (v *validator) profileProblems(id *uuid.UUID) []string {
	if id == nil || !v.refs.Profiles[*id] {
		return []string{"escolha o perfil do agente"}
	}
	return nil
}

func (v *validator) mcpProblems(ids []uuid.UUID) []string {
	for _, id := range ids {
		if !v.refs.McpServers[id] {
			return []string{"um dos servidores MCP não existe mais"}
		}
	}
	return nil
}

func agentProblems(c AgentConfig) []string {
	var out []string
	if strings.TrimSpace(c.Prompt) == "" {
		out = append(out, "o prompt inicial é obrigatório")
	}
	if c.MaxTurns != nil && *c.MaxTurns < 1 {
		out = append(out, "max_turns precisa ser pelo menos 1")
	}
	if len(c.OutputSchema) > 0 && string(c.OutputSchema) != "null" {
		var schema map[string]any
		if json.Unmarshal(c.OutputSchema, &schema) != nil {
			out = append(out, "o schema de saída precisa ser um objeto JSON")
		}
	}
	return out
}

func reviewProblems(c ReviewConfig) []string {
	var out []string
	if strings.TrimSpace(c.Prompt) == "" {
		out = append(out, "o prompt de review é obrigatório")
	}
	if c.MaxRounds < 1 {
		out = append(out, "max_rounds precisa ser pelo menos 1")
	}
	return out
}

// NewFlowGraph is the draft of a new flow: a lone trigger on card creation.
func NewFlowGraph() Graph {
	config, _ := json.Marshal(TriggerConfig{Event: EventCardCreated}) // always marshals
	return Graph{
		Nodes: []Node{{ID: "trigger", Type: NodeTrigger, Config: config}},
		Edges: []Edge{},
	}
}

// Normalized returns g with empty lists instead of missing ones.
func (g Graph) Normalized() Graph {
	if g.Nodes == nil {
		g.Nodes = []Node{}
	}
	if g.Edges == nil {
		g.Edges = []Edge{}
	}
	return g
}
