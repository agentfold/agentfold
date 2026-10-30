package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
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
	NodeMoveCard NodeType = "move_card"
	NodeEnd      NodeType = "end"
	// NodeCreateCards creates child cards from the output of the node
	// before; NodeWaitChildren waits until the children are done.
	NodeCreateCards  NodeType = "create_cards"
	NodeWaitChildren NodeType = "wait_children"
	// NodeHumanReview waits for a human to review artifacts of the card.
	NodeHumanReview NodeType = "human_review"
	// NodeSetProperty sets a property of the card; NodeCheckProperty
	// checks it on the cards the card depends on.
	NodeSetProperty   NodeType = "set_property"
	NodeCheckProperty NodeType = "check_property"
)

// Outputs lists the outputs (edge source handles) of each node type. Agent
// nodes with the gateway on have GatewayOutputs instead.
var Outputs = map[NodeType][]string{
	NodeTrigger:  {"next"},
	NodeAgent:    {"done", "failed"},
	NodeMoveCard: {"next"},
	NodeEnd:      {},
	// failed: the input lists no valid cards, or names unknown projects.
	NodeCreateCards: {"next", "failed"},
	// some_failed: the last run of a child failed or was canceled before
	// the child got the done property.
	NodeWaitChildren: {"all_done", "some_failed"},
	// changes_requested: the human asked for changes, handed on to the
	// agent node it leads to.
	NodeHumanReview: {"approved", "changes_requested"},
	NodeSetProperty: {"next"},
	// pending: some dependency lacks the property (never with wait on).
	NodeCheckProperty: {"satisfied", "pending"},
}

// GatewayOutputs are the outputs of an agent node with the gateway on.
var GatewayOutputs = []string{"approved", "rejected", "max_rounds_exceeded", "failed"}

// NodeOutputs returns the outputs of a node of a known type.
func NodeOutputs(n Node) []string {
	if n.Type == NodeAgent {
		var c AgentConfig
		if json.Unmarshal(n.Config, &c) == nil && c.Gateway {
			return GatewayOutputs
		}
	}
	return Outputs[n.Type]
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
// external MCP servers. With the gateway on, the agent decides whether the
// work moves on (approve or reject); MaxRounds bounds the rejections.
type AgentConfig struct {
	ProfileID       *uuid.UUID      `json:"profile_id,omitempty"`
	McpServerIDs    []uuid.UUID     `json:"mcp_server_ids,omitempty"`
	Prompt          string          `json:"prompt"`
	AllowedTools    []string        `json:"allowed_tools,omitempty"`
	DisallowedTools []string        `json:"disallowed_tools,omitempty"`
	OutputSchema    json.RawMessage `json:"output_schema,omitempty"`
	MaxTurns        *int            `json:"max_turns,omitempty"`
	Model           string          `json:"model,omitempty"`
	ReadOnly        bool            `json:"read_only,omitempty"`
	Gateway         bool            `json:"gateway,omitempty"`
	MaxRounds       int             `json:"max_rounds,omitempty"`
	Artifacts       []ArtifactSpec  `json:"artifacts,omitempty"`
}

// BoardToolPrefix names the tools of the board MCP server in allowed_tools.
const BoardToolPrefix = "mcp__orquestrator__"

// TaskTools are the board tools an agent node only has when its allowed
// tools list them: with them the agent splits its card into child cards.
var TaskTools = []string{"create_tasks", "list_tasks", "retry_task", "cancel_task"}

// AllowsBoardTool tells whether the allowed tools of the node list the
// board tool name, or the whole board server.
func (c AgentConfig) AllowsBoardTool(name string) bool {
	return slices.Contains(c.AllowedTools, BoardToolPrefix+name) || slices.Contains(c.AllowedTools, strings.TrimSuffix(BoardToolPrefix, "__"))
}

// ArtifactSpec is a file of the card workspace an agent node collects into
// the card when it finishes. Path is relative to the workspace.
type ArtifactSpec struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// MoveCardConfig moves the card to a lane.
type MoveCardConfig struct {
	LaneID uuid.UUID `json:"lane_id"`
}

// CreateCardsConfig creates the child cards in a lane, with a card type.
type CreateCardsConfig struct {
	LaneID     uuid.UUID `json:"lane_id"`
	CardTypeID uuid.UUID `json:"card_type_id"`
}

// WaitChildren modes: all_succeed leaves by some_failed as soon as a child
// fails; all_finished waits until every child is done or failed.
const (
	WaitAllSucceed  = "all_succeed"
	WaitAllFinished = "all_finished"
)

// WaitChildrenConfig waits for the child cards (those not canceled): a
// child is done once it has the property PropertyKey set to PropertyValue.
// An empty Mode is WaitAllSucceed.
type WaitChildrenConfig struct {
	Mode          string `json:"mode,omitempty"`
	PropertyKey   string `json:"property_key,omitempty"`
	PropertyValue string `json:"property_value,omitempty"`
	// DoneLaneID is how versions published before properties tell a done
	// child: by the lane it is in.
	DoneLaneID *uuid.UUID `json:"done_lane_id,omitempty"`
}

// HumanReviewConfig names the artifacts the human reviews; none means
// those collected in the run.
type HumanReviewConfig struct {
	Artifacts []string `json:"artifacts,omitempty"`
}

// SetPropertyConfig sets the property Key of the card to Value.
type SetPropertyConfig struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// CheckPropertyConfig checks that every card the card depends on has the
// property Key set to Value. With Wait on, the node waits until they do
// instead of leaving by pending.
type CheckPropertyConfig struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Wait  bool   `json:"wait,omitempty"`
}

// ChildCardSpec is a card created as a child of another (by a CreateCards
// node or the create_tasks tool). Project is the name of a project,
// optional. DependsOn lists the refs of other cards of the same batch or
// the ids of existing cards.
type ChildCardSpec struct {
	Ref         string   `json:"ref,omitempty"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Project     string   `json:"project"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

func (c ChildCardSpec) normalized() ChildCardSpec {
	c.Ref, c.Title, c.Description, c.Project = strings.TrimSpace(c.Ref), strings.TrimSpace(c.Title), strings.TrimSpace(c.Description), strings.TrimSpace(c.Project)
	return c
}

// Placement is the lane and card type of a child card.
type Placement struct {
	LaneID     uuid.UUID
	CardTypeID uuid.UUID
}

// ChildCards turns specs into the child cards of parent: projects by name,
// dependencies by ref or id, and the lane and type place gives each one.
// The messages of its errors are meant for the agent that wrote the specs.
func ChildCards(parent uuid.UUID, specs []ChildCardSpec, projects []Project, place func(i int) (Placement, error)) ([]ChildCard, error) {
	byName := map[string]uuid.UUID{}
	names := make([]string, 0, len(projects))
	for _, p := range projects {
		byName[p.Name] = p.ID
		names = append(names, p.Name)
	}
	slices.Sort(names)
	refs := map[string]int{}
	for i := range specs {
		specs[i] = specs[i].normalized()
		spec := specs[i]
		if spec.Title == "" {
			return nil, fmt.Errorf("o card %d da lista não tem título", i+1)
		}
		if _, dup := refs[spec.Ref]; dup && spec.Ref != "" {
			return nil, fmt.Errorf("o ref %q aparece em mais de um card", spec.Ref)
		}
		if spec.Ref != "" {
			refs[spec.Ref] = i
		}
	}
	cards := make([]ChildCard, len(specs))
	for i, spec := range specs {
		at, err := place(i)
		if err != nil {
			return nil, err
		}
		cards[i].NewCard = NewCard{
			CardInput:    CardInput{Title: spec.Title, Description: spec.Description, CardTypeID: at.CardTypeID},
			LaneID:       at.LaneID,
			ParentCardID: &parent,
		}
		if spec.Project != "" {
			id, ok := byName[spec.Project]
			if !ok {
				return nil, fmt.Errorf("o card %q usa o projeto %q, que não existe (projetos: %s)", spec.Title, spec.Project, strings.Join(names, ", "))
			}
			cards[i].ProjectID = &id
		}
		for _, dep := range spec.DependsOn {
			dep = strings.TrimSpace(dep)
			if j, ok := refs[dep]; ok {
				cards[i].After = append(cards[i].After, j)
			} else if id, err := uuid.Parse(dep); err == nil {
				cards[i].DependsOn = append(cards[i].DependsOn, id)
			} else {
				return nil, fmt.Errorf("o card %q depende de %q, que não é o ref de um card da lista nem o id de um card", spec.Title, dep)
			}
		}
	}
	if _, err := CreationOrder(cards); err != nil {
		return nil, err
	}
	return cards, nil
}

// ChildCardsSchema is the output schema an agent node before a
// CreateCards node declares to hand on the cards to create.
const ChildCardsSchema = `{"type":"object","required":["cards"],"properties":{"cards":{"type":"array","minItems":1,"items":{` +
	`"type":"object","required":["title"],"properties":{"ref":{"type":"string"},"title":{"type":"string"},"description":{"type":"string"},` +
	`"project":{"type":"string"},"depends_on":{"type":"array","items":{"type":"string"}}}}}}}`

// ChildCardsFrom reads the cards to create from what the node before
// handed on: the cards list of an agent's structured output, or of the
// input itself.
func ChildCardsFrom(input json.RawMessage) ([]ChildCardSpec, error) {
	var in struct {
		Cards  []ChildCardSpec `json:"cards"`
		Output *struct {
			Cards []ChildCardSpec `json:"cards"`
		} `json:"output"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("a entrada não é um JSON com a lista cards: %w", err)
	}
	cards := in.Cards
	if in.Output != nil {
		cards = in.Output.Cards
	}
	if len(cards) == 0 {
		return nil, errors.New("a entrada não tem cards para criar: o nó anterior precisa entregar a lista cards na saída estruturada")
	}
	return cards, nil
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
	v.checkReviews(g.Edges)
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
		case !slices.Contains(NodeOutputs(source), e.SourceHandle):
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

// checkReviews requires every HumanReview node to send changes back to
// some node.
func (v *validator) checkReviews(edges []Edge) {
	for _, n := range v.sortedNodes() {
		if n.Type != NodeHumanReview {
			continue
		}
		linked := slices.ContainsFunc(edges, func(e Edge) bool {
			return e.Source == n.ID && e.SourceHandle == "changes_requested"
		})
		if !linked {
			v.node(n.ID, "ligue a saída changes_requested ao nó que refaz o trabalho")
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
	case NodeMoveCard:
		var c MoveCardConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return []string{invalidConfig(err)}
		}
		if !v.refs.Lanes[c.LaneID] {
			return []string{"escolha a baia de destino"}
		}
	case NodeCreateCards:
		var c CreateCardsConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return []string{invalidConfig(err)}
		}
		var out []string
		if !v.refs.Lanes[c.LaneID] {
			out = append(out, "escolha a baia dos cards criados")
		}
		if !v.refs.CardTypes[c.CardTypeID] {
			out = append(out, "escolha o tipo dos cards criados")
		}
		return out
	case NodeWaitChildren:
		var c WaitChildrenConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return []string{invalidConfig(err)}
		}
		out := propertyProblems(c.PropertyKey, c.PropertyValue)
		if c.Mode != "" && c.Mode != WaitAllSucceed && c.Mode != WaitAllFinished {
			out = append(out, fmt.Sprintf("modo de espera desconhecido: %q", c.Mode))
		}
		return out
	case NodeHumanReview:
		var c HumanReviewConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return []string{invalidConfig(err)}
		}
		return reviewProblems(c.Artifacts)
	case NodeSetProperty:
		var c SetPropertyConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return []string{invalidConfig(err)}
		}
		return propertyProblems(c.Key, c.Value)
	case NodeCheckProperty:
		var c CheckPropertyConfig
		if err := json.Unmarshal(config, &c); err != nil {
			return []string{invalidConfig(err)}
		}
		return propertyProblems(c.Key, c.Value)
	}
	return nil
}

func propertyProblems(key, value string) []string {
	var out []string
	if strings.TrimSpace(key) == "" {
		out = append(out, "informe a chave da propriedade")
	}
	if strings.TrimSpace(value) == "" {
		out = append(out, "informe o valor da propriedade")
	}
	return out
}

func reviewProblems(names []string) []string {
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return []string{"o nome de um artefato revisado está vazio"}
		}
		if seen[name] {
			return []string{fmt.Sprintf("o artefato %q aparece mais de uma vez", name)}
		}
		seen[name] = true
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
	if c.HasOutputSchema() {
		var schema map[string]any
		if json.Unmarshal(c.OutputSchema, &schema) != nil {
			out = append(out, "o schema de saída precisa ser um objeto JSON")
		} else if _, err := compileSchema(c.OutputSchema); err != nil {
			out = append(out, "o schema de saída é inválido: "+err.Error())
		}
	}
	if c.Gateway && c.MaxRounds < 1 {
		out = append(out, "o gateway precisa de um máximo de rodadas de pelo menos 1")
	}
	return append(out, artifactProblems(c.Artifacts)...)
}

func artifactProblems(specs []ArtifactSpec) []string {
	var out []string
	names := map[string]bool{}
	for _, a := range specs {
		name := strings.TrimSpace(a.Name)
		switch {
		case name == "":
			out = append(out, "todo artefato precisa de um nome")
		case names[name]:
			out = append(out, fmt.Sprintf("o artefato %q aparece mais de uma vez", name))
		case !filepath.IsLocal(a.Path):
			out = append(out, fmt.Sprintf("o arquivo do artefato %q precisa ser um caminho relativo dentro do workspace", name))
		}
		names[name] = true
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
