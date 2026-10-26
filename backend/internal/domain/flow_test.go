package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

var (
	laneID    = uuid.New()
	typeID    = uuid.New()
	profileID = uuid.New()
	mcpID     = uuid.New()
	refs      = References{
		Lanes:      map[uuid.UUID]bool{laneID: true},
		CardTypes:  map[uuid.UUID]bool{typeID: true},
		Profiles:   map[uuid.UUID]bool{profileID: true},
		McpServers: map[uuid.UUID]bool{mcpID: true},
	}
	profile = `"profile_id":"` + profileID.String() + `"`
)

func node(id string, t NodeType, config string) Node {
	return Node{ID: id, Type: t, Config: json.RawMessage(config)}
}

func edge(id, source, handle, target string) Edge {
	return Edge{ID: id, Source: source, SourceHandle: handle, Target: target}
}

// validGraph is trigger -> dev <-> gateway -> move -> end.
func validGraph() Graph {
	return Graph{
		Nodes: []Node{
			node("t", NodeTrigger, `{"event":"card_entered_lane","lane_id":"`+laneID.String()+`","card_type_ids":["`+typeID.String()+`"]}`),
			node("dev", NodeAgent, `{`+profile+`,"mcp_server_ids":["`+mcpID.String()+`"],"prompt":"do it","max_turns":5,"output_schema":{"type":"object"}}`),
			node("rev", NodeAgent, `{`+profile+`,"prompt":"check","read_only":true,"gateway":true,"max_rounds":3,"artifacts":[{"name":"Plano","path":"docs/plan.md"}]}`),
			node("move", NodeMoveCard, `{"lane_id":"`+laneID.String()+`"}`),
			node("end", NodeEnd, ``),
		},
		Edges: []Edge{
			edge("e1", "t", "next", "dev"),
			edge("e2", "dev", "done", "rev"),
			edge("e3", "rev", "rejected", "dev"),
			edge("e4", "rev", "approved", "move"),
			edge("e5", "move", "next", "end"),
		},
	}
}

func messages(problems []GraphProblem) []string {
	out := make([]string, len(problems))
	for i, p := range problems {
		out[i] = p.NodeID + p.EdgeID + ": " + p.Message
	}
	return out
}

func TestValidGraph(t *testing.T) {
	if problems := validGraph().Problems(refs); len(problems) != 0 {
		t.Fatalf("unexpected problems %q", messages(problems))
	}
}

func TestGraphProblems(t *testing.T) {
	tests := []struct {
		name   string
		change func(g *Graph)
		want   []string
	}{
		{"no trigger", func(g *Graph) { g.Nodes[0].Type = NodeEnd; g.Edges = g.Edges[1:] }, []string{
			": o fluxo precisa de exatamente um gatilho (tem 0)",
		}},
		{"two triggers", func(g *Graph) { g.Nodes = append(g.Nodes, node("t2", NodeTrigger, `{"event":"card_created"}`)) }, []string{
			": o fluxo precisa de exatamente um gatilho (tem 2)",
		}},
		{"duplicate node id", func(g *Graph) { g.Nodes = append(g.Nodes, node("end", NodeEnd, `{}`)) }, []string{
			"end: id de nó vazio ou repetido",
		}},
		{"unknown node type", func(g *Graph) { g.Nodes = append(g.Nodes, node("x", "pr", `{}`)) }, []string{
			`x: tipo de nó desconhecido: "pr"`,
		}},
		{"unreachable node and unlinked trigger", func(g *Graph) { g.Edges = nil }, []string{
			"t: o gatilho precisa estar ligado a um nó",
			"dev: nó inalcançável a partir do gatilho",
			"end: nó inalcançável a partir do gatilho",
			"move: nó inalcançável a partir do gatilho",
			"rev: nó inalcançável a partir do gatilho",
		}},
		{"duplicate edge id", func(g *Graph) { g.Edges = append(g.Edges, edge("e1", "dev", "failed", "end")) }, []string{
			"e1: id de ligação vazio ou repetido",
		}},
		{"edge to missing node", func(g *Graph) { g.Edges = append(g.Edges, edge("e9", "dev", "failed", "nope")) }, []string{
			"e9: ligação com nó inexistente",
		}},
		{"unknown output", func(g *Graph) { g.Edges = append(g.Edges, edge("e9", "end", "next", "dev")) }, []string{
			`e9: o nó end não tem a saída "next"`,
		}},
		{"edge into the trigger", func(g *Graph) { g.Edges = append(g.Edges, edge("e9", "dev", "failed", "t")) }, []string{
			"e9: o gatilho não pode ser destino de ligação",
		}},
		{"self loop", func(g *Graph) { g.Edges = append(g.Edges, edge("e9", "dev", "failed", "dev")) }, []string{
			"e9: um nó não pode se ligar a si mesmo",
		}},
		{"output used twice", func(g *Graph) { g.Edges = append(g.Edges, edge("e9", "dev", "done", "end")) }, []string{
			`e9: a saída "done" do nó dev já está ligada`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := validGraph()
			tt.change(&g)
			if got := messages(g.Problems(refs)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestConfigProblems(t *testing.T) {
	missingLane := uuid.New().String()
	tests := []struct {
		name   string
		typ    NodeType
		config string
		want   []string
	}{
		{"trigger on creation", NodeTrigger, `{"event":"card_created"}`, nil},
		{"trigger without event", NodeTrigger, `{}`, []string{"escolha o evento do gatilho"}},
		{"trigger without lane", NodeTrigger, `{"event":"card_entered_lane"}`, []string{"escolha a baia que dispara o fluxo"}},
		{"trigger with missing lane", NodeTrigger, `{"event":"card_entered_lane","lane_id":"` + missingLane + `"}`, []string{"escolha a baia que dispara o fluxo"}},
		{"trigger with missing type", NodeTrigger, `{"event":"card_created","card_type_ids":["` + uuid.NewString() + `","` + uuid.NewString() + `"]}`,
			[]string{"o filtro usa um tipo de card que não existe mais"}},
		{"trigger with bad JSON", NodeTrigger, `[]`, []string{"configuração inválida"}},
		{"agent without config", NodeAgent, ``, []string{"escolha o perfil do agente", "o prompt inicial é obrigatório"}},
		{"agent with bad values", NodeAgent, `{` + profile + `,"prompt":" ","max_turns":0,"output_schema":[1]}`, []string{
			"o prompt inicial é obrigatório", "max_turns precisa ser pelo menos 1", "o schema de saída precisa ser um objeto JSON",
		}},
		{"agent with missing profile and MCP", NodeAgent, `{"profile_id":"` + uuid.NewString() + `","mcp_server_ids":["` + uuid.NewString() + `"],"prompt":"p"}`,
			[]string{"escolha o perfil do agente", "um dos servidores MCP não existe mais"}},
		{"agent with invalid schema", NodeAgent, `{` + profile + `,"prompt":"p","output_schema":{"type":1}}`, []string{"o schema de saída é inválido: "}},
		{"agent with null schema", NodeAgent, `{` + profile + `,"prompt":"p","output_schema":null}`, nil},
		{"agent with bad JSON", NodeAgent, `[]`, []string{"configuração inválida"}},
		{"gateway without rounds", NodeAgent, `{` + profile + `,"prompt":"p","gateway":true}`, []string{
			"o gateway precisa de um máximo de rodadas de pelo menos 1",
		}},
		{"agent with bad artifacts", NodeAgent, `{` + profile + `,"prompt":"p","artifacts":[{"name":" ","path":"a.md"},{"name":"A","path":"../a.md"},` +
			`{"name":"B","path":"/b.md"},{"name":"C","path":"c.md"},{"name":"C","path":"d.md"}]}`, []string{
			"todo artefato precisa de um nome",
			`o arquivo do artefato "A" precisa ser um caminho relativo dentro do workspace`,
			`o arquivo do artefato "B" precisa ser um caminho relativo dentro do workspace`,
			`o artefato "C" aparece mais de uma vez`,
		}},
		{"move without lane", NodeMoveCard, `{}`, []string{"escolha a baia de destino"}},
		{"move with bad JSON", NodeMoveCard, `[]`, []string{"configuração inválida"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// trigger -> n, with the config under test on n, or on the
			// trigger (n is then an End).
			g := Graph{
				Nodes: []Node{node("t", NodeTrigger, `{"event":"card_created"}`), node("n", tt.typ, tt.config)},
				Edges: []Edge{edge("e", "t", "next", "n")},
			}
			id := "n"
			if tt.typ == NodeTrigger {
				id = "t"
				g.Nodes[0].Config = json.RawMessage(tt.config)
				g.Nodes[1] = node("n", NodeEnd, `{}`)
			}
			var got []string
			for _, p := range g.Problems(refs) {
				if p.NodeID == id {
					got = append(got, p.Message)
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			for i := range got {
				if !strings.HasPrefix(got[i], tt.want[i]) {
					t.Fatalf("got %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestNewFlowGraph(t *testing.T) {
	g := NewFlowGraph()
	problems := g.Problems(refs)
	if len(g.Nodes) != 1 || g.Nodes[0].Type != NodeTrigger || len(problems) != 1 || problems[0].Message != "o gatilho precisa estar ligado a um nó" {
		t.Fatalf("graph %+v, problems %q", g, messages(problems))
	}
}

func TestFlowBasics(t *testing.T) {
	id := uuid.New()
	if (Flow{ID: id}).EntityID() != id {
		t.Fatal("EntityID")
	}
	if (&GraphError{Problems: make([]GraphProblem, 2)}).Error() != "graph has 2 problem(s)" {
		t.Fatal("GraphError message")
	}
	in, err := FlowInput{Name: " dev ", Enabled: true}.Normalize()
	if err != nil || in != (FlowInput{Name: "dev", Enabled: true}) {
		t.Fatalf("Normalize = %+v, %v", in, err)
	}
	if _, err := (FlowInput{}).Normalize(); fieldOf(t, err) != "name" {
		t.Fatal("blank name accepted")
	}
}

func TestNormalized(t *testing.T) {
	g := Graph{}.Normalized()
	if g.Nodes == nil || g.Edges == nil {
		t.Fatalf("got %+v", g)
	}
	full := validGraph()
	if !reflect.DeepEqual(full.Normalized(), full) {
		t.Fatal("a complete graph changed")
	}
}
