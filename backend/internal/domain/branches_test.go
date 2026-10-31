package domain

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func branch(name string, status BranchStatus, output string) Branch {
	b := Branch{Name: name, Label: "s." + name, Status: status, Output: json.RawMessage(output)}
	if status == BranchFailed {
		b.Error = "boom"
	}
	return b
}

func TestJoinReleases(t *testing.T) {
	active, arrived, failed, done := branch("a", BranchActive, ""), branch("b", BranchArrived, `{"output":1}`), branch("c", BranchFailed, ""), branch("d", BranchDone, "")
	for _, tc := range []struct {
		cfg      JoinConfig
		branches []Branch
		want     bool
	}{
		{JoinConfig{}, []Branch{active, arrived}, false},
		{JoinConfig{}, []Branch{done, arrived, failed}, true},
		{JoinConfig{}, []Branch{active, failed}, false},
		{JoinConfig{FailFast: true}, []Branch{active, failed}, true},
		{JoinConfig{FailFast: true}, []Branch{active, arrived}, false},
		{JoinConfig{Mode: JoinFirst}, []Branch{active, arrived}, true},
		{JoinConfig{Mode: JoinFirst}, []Branch{active, failed}, false},
	} {
		if got := tc.cfg.Releases(tc.branches); got != tc.want {
			t.Errorf("%+v on %v = %v", tc.cfg, tc.branches, got)
		}
	}
}

func TestJoinResult(t *testing.T) {
	arrived, failed := branch("b", BranchArrived, `{"result":"text"}`), branch("c", BranchFailed, "")
	r := JoinConfig{}.Result([]Branch{arrived, branch("d", BranchCanceled, "")})
	if r.Status != NodeDone || r.OutputHandle != "all_done" || string(r.Output) != `{"output":{"branches":{"b":"text"}}}` {
		t.Errorf("all done %+v (%s)", r, r.Output)
	}
	r = JoinConfig{}.Result([]Branch{arrived, failed})
	if r.OutputHandle != "some_failed" || string(r.Output) != `{"output":{"branches":{"b":"text"},"failed":{"c":"boom"}}}` {
		t.Errorf("some failed %+v (%s)", r, r.Output)
	}
	if r = (JoinConfig{Mode: JoinFirst}).Result([]Branch{arrived, failed}); r.OutputHandle != "all_done" {
		t.Errorf("first arrived %+v", r)
	}
	if r = (JoinConfig{Mode: JoinFirst}).Result([]Branch{failed}); r.OutputHandle != "some_failed" {
		t.Errorf("first, none arrived %+v", r)
	}
	if got := BranchFailures([]Branch{arrived, failed, failed}); got != "o ramo s.c falhou: boom; o ramo s.c falhou: boom" {
		t.Errorf("failures %q", got)
	}
}

// branchGraph is t -> s (a, b) -> a1 / b1 -> j -> end.
func branchGraph() Graph {
	return Graph{
		Nodes: []Node{
			node("t", NodeTrigger, `{"event":"card_created"}`),
			node("s", NodeSplit, `{"branches":["a","b"]}`),
			node("a1", NodeTransform, `{"output":"=1"}`),
			node("b1", NodeHTTPRequest, `{"method":"GET","url":"http://x"}`),
			node("j", NodeJoin, `{"mode":"first"}`),
			node("end", NodeEnd, ``),
		},
		Edges: []Edge{
			edge("e1", "t", "next", "s"), edge("e2", "s", "a", "a1"), edge("e3", "s", "b", "b1"),
			edge("e4", "a1", "next", "j"), edge("e5", "b1", "success", "j"), edge("e6", "j", "all_done", "end"),
		},
	}
}

func TestSplitGraphs(t *testing.T) {
	if problems := branchGraph().Problems(refs); len(problems) != 0 {
		t.Fatalf("problems %q", messages(problems))
	}
	if got := branchGraph().SplitJoin("s"); got != "j" {
		t.Errorf("join %q", got)
	}
	if got := NodeOutputs(branchGraph().Nodes[1]); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("outputs %q", got)
	}
	tests := []struct {
		name   string
		change func(g *Graph)
		want   []string
	}{
		{"nested split", func(g *Graph) {
			g.Nodes = append(g.Nodes, node("in", NodeSplit, `{"branches":["x","y"]}`), node("x", NodeEnd, ``), node("y", NodeSetProperty, `{"key":"k","value":"v"}`), node("ij", NodeJoin, ``))
			g.Edges[3] = edge("e4", "a1", "next", "in")
			g.Edges = append(g.Edges, edge("e7", "in", "x", "x"), edge("e8", "in", "y", "y"), edge("e9", "y", "next", "ij"), edge("e10", "ij", "all_done", "j"))
		}, nil},
		{"nested split ending in Ends", func(g *Graph) {
			g.Nodes = append(g.Nodes, node("in", NodeSplit, `{"branches":["x","y"]}`), node("x", NodeEnd, ``), node("y", NodeEnd, ``))
			g.Edges[3] = edge("e4", "a1", "next", "in")
			g.Edges = append(g.Edges, edge("e7", "in", "x", "x"), edge("e8", "in", "y", "y"))
		}, nil},
		{"branch to a missing node", func(g *Graph) {
			g.Edges = append(g.Edges, edge("e7", "a1", "failed", "ghost"))
		}, []string{"e7: ligação com nó inexistente"}},
		{"branch ending in End", func(g *Graph) {
			g.Nodes = append(g.Nodes, node("stop", NodeEnd, ``))
			g.Edges[3] = edge("e4", "a1", "next", "stop")
		}, nil},
		{"agent and waiting check in a branch", func(g *Graph) {
			g.Nodes[2] = node("a1", NodeAgent, `{`+profile+`,"prompt":"x"}`)
			g.Edges[3] = edge("e4", "a1", "done", "j")
			g.Nodes[3] = node("b1", NodeCheckProperty, `{"key":"k","value":"v","wait":true}`)
			g.Edges[4] = edge("e5", "b1", "satisfied", "j")
		}, []string{"a1: o nó agent não roda num ramo paralelo (split s)", "b1: num ramo paralelo (split s) o Conferir propriedade não pode esperar"}},
		{"branches reaching two joins", func(g *Graph) {
			g.Nodes = append(g.Nodes, node("j2", NodeJoin, ``))
			g.Edges[4] = edge("e5", "b1", "success", "j2")
		}, []string{"s: os ramos precisam chegar ao mesmo join (chegam a j, j2)"}},
		{"edge between branches", func(g *Graph) {
			g.Edges = append(g.Edges, edge("e7", "b1", "failed", "a1"))
		}, []string{"a1: o nó está nos ramos a e b do split s", "a1: a ligação e7 entra no ramo a do split s por fora"}},
		{"edge into a branch from outside", func(g *Graph) {
			g.Edges = append(g.Edges, edge("e7", "j", "some_failed", "a1"))
		}, []string{"a1: a ligação e7 entra no ramo a do split s por fora"}},
		{"branch back to its split", func(g *Graph) {
			g.Edges = append(g.Edges, edge("e7", "b1", "failed", "s"))
		}, []string{"s: um ramo do split volta para ele"}},
		{"join reached from outside", func(g *Graph) {
			g.Nodes = append(g.Nodes, node("m", NodeSetProperty, `{"key":"k","value":"v"}`))
			g.Edges[0] = edge("e1", "t", "next", "m")
			g.Edges = append(g.Edges, edge("e7", "m", "next", "s"), edge("e8", "s", "nope", "j"))
		}, []string{`e8: o nó s não tem a saída "nope"`}},
		{"split linked straight to its join", func(g *Graph) {
			g.Edges[2] = edge("e3", "s", "b", "j")
			g.Nodes = slices.Delete(g.Nodes, 3, 4)
			g.Edges = slices.Delete(g.Edges, 4, 5)
		}, nil},
		{"unlinked split", func(g *Graph) {
			g.Edges = []Edge{edge("e1", "t", "next", "s")}
		}, []string{
			"a1: nó inalcançável a partir do gatilho", "b1: nó inalcançável a partir do gatilho", "end: nó inalcançável a partir do gatilho",
			"j: nó inalcançável a partir do gatilho", "s: ligue pelo menos um ramo do split", "j: o join não fecha os ramos de nenhum split",
		}},
		{"join from the main path", func(g *Graph) {
			g.Nodes = append(g.Nodes, node("m", NodeSetProperty, `{"key":"k","value":"v"}`))
			g.Edges[0] = edge("e1", "t", "next", "m")
			g.Edges = append(g.Edges, edge("e7", "m", "next", "j"), edge("e8", "j", "some_failed", "s"))
		}, []string{"j: a ligação e7 chega ao join por fora dos ramos do split s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := branchGraph()
			tt.change(&g)
			if got := messages(g.Problems(refs)); !reflect.DeepEqual(got, tt.want) && (len(got) != 0 || tt.want != nil) {
				t.Fatalf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestBranchConfigProblems(t *testing.T) {
	for config, want := range map[string][]string{
		`{"branches":["a"]}`:           {"o split precisa de pelo menos dois ramos"},
		`{"branches":["a","a","b c"]}`: {"o ramo a aparece mais de uma vez", "o nome do ramo 3 só aceita letras, números, _ e -"},
		`{"branches":["a","b"]}`:       nil,
	} {
		var c SplitConfig
		_ = json.Unmarshal([]byte(config), &c)
		if got := splitProblems(c); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %q", config, got)
		}
	}
	for config, want := range map[string][]string{
		`{}`:                                {},
		`{"mode":"all","fail_fast":true}`:   {},
		`{"mode":"some"}`:                   {`modo do join desconhecido: "some"`},
		`{"mode":"first","fail_fast":true}`: {"falhar no primeiro erro só vale no modo todos"},
	} {
		var c JoinConfig
		_ = json.Unmarshal([]byte(config), &c)
		if got := joinProblems(c); len(got) != len(want) || (len(want) > 0 && got[0] != want[0]) {
			t.Errorf("%s: %q", config, got)
		}
	}
	g := Graph{Nodes: []Node{node("s", NodeSplit, `[`), node("j", NodeJoin, `[`)}}
	invalid := 0
	for _, p := range g.Problems(refs) {
		if strings.HasPrefix(p.Message, "configuração inválida") {
			invalid++
		}
	}
	if invalid != 2 {
		t.Errorf("problems %q", messages(g.Problems(refs)))
	}
	if NodeOutputs(g.Nodes[0]) == nil || len(NodeOutputs(g.Nodes[0])) != 0 {
		t.Error("outputs of a bad split config")
	}
}
