package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

const opencodeAuth = `{"openrouter":{"type":"api","key":"k"}}`

// opencodeSetup is newSetup with an opencode profile.
func opencodeSetup(t *testing.T) setup {
	t.Helper()
	s := newSetup(t)
	s.store.profile.Kind = domain.AgentOpencode
	s.store.profile.CredentialKind = domain.CredentialOpencodeAuth
	s.store.cred = seal(t, s.box, opencodeAuth)
	return s
}

// opencodeCall reads what the fake CLI recorded about its last opencode run.
func (s setup) opencodeCall(t *testing.T) (string, map[string]any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.cardDir(), "workspace", "fake-opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var call map[string]any
	if err := json.Unmarshal(data, &call); err != nil {
		t.Fatal(err)
	}
	return argsOf(call), call
}

func TestOpencodeRun(t *testing.T) {
	s := opencodeSetup(t)
	id := withMcp(s)
	http := uuid.New()
	s.store.mcp[id] = domain.McpServer{Name: "fs", Transport: domain.McpStdio, McpConfig: domain.McpConfig{Command: "srv", Args: []string{"-v"}, Env: map[string]string{"A": "1"}}}
	s.store.mcp[http] = domain.McpServer{Name: "sg", Transport: domain.McpHTTP, McpConfig: domain.McpConfig{URL: "https://sg/mcp"}}
	s.store.mcpSeals = map[uuid.UUID][]byte{http: seal(t, s.box, `{"Authorization":"token x"}`)}
	turns := 7
	s.store.version.Graph.Nodes[0].Config, _ = json.Marshal(domain.AgentConfig{
		ProfileID: &s.store.profile.ID, Prompt: "Review it", Model: "openrouter/m", ReadOnly: true,
		MaxTurns: &turns, McpServerIDs: []uuid.UUID{id, http},
	})

	if err := s.handle(t, ""); err != nil {
		t.Fatal(err)
	}
	r := s.store.result
	want := domain.Usage{TokensIn: 10, TokensOut: 20, CacheRead: 30, CacheWrite: 40, CostUSD: 0.5, Turns: 2, DurationMS: 1200}
	if r.Status != domain.NodeDone || r.OutputHandle != "done" || r.Usage != want || string(r.Output) != `{"result":"did it"}` {
		t.Fatalf("result %+v", r)
	}
	args, call := s.opencodeCall(t)
	session := OpencodeSession(*s.store.session)
	if !strings.HasPrefix(args, "run --standalone --format json --auto --agent build --session "+session+" --model openrouter/m -- Review it\n\n## Card") {
		t.Fatalf("args: %s", args)
	}
	home := filepath.Join(s.cardDir(), "claude")
	if call["home"] != home {
		t.Fatalf("home %v", call["home"])
	}
	config := call["config"].(map[string]any)
	mcp := config["mcp"].(map[string]any)
	board := mcp[boardServer].(map[string]any)
	if board["type"] != "remote" || board["url"] != "http://board:8080/mcp" || !strings.HasPrefix(board["headers"].(map[string]any)["Authorization"].(string), "Bearer ") {
		t.Fatalf("board %v", board)
	}
	if fs := mcp["fs"].(map[string]any); fs["type"] != "local" || len(fs["command"].([]any)) != 2 || fs["environment"].(map[string]any)["A"] != "1" {
		t.Fatalf("stdio %v", fs)
	}
	if sg := mcp["sg"].(map[string]any); sg["headers"].(map[string]any)["Authorization"] != "token x" {
		t.Fatalf("http %v", sg)
	}
	permission := config["permission"].(map[string]any)
	if permission["edit"] != "deny" || permission["websearch"] != "deny" || permission["bash"].(map[string]any)["git push*"] != "deny" {
		t.Fatalf("permission %v", permission)
	}
	if steps := config["agent"].(map[string]any)["build"].(map[string]any)["steps"]; steps != float64(7) {
		t.Fatalf("steps %v", steps)
	}
	if instructions := config["instructions"].([]any)[0].(string); !strings.HasPrefix(instructions, home+"/orquestrator-") {
		t.Fatalf("instructions %v", instructions)
	}
	if imported, _ := os.ReadFile(filepath.Join(home, "fake-auth.json")); string(imported) != opencodeAuth {
		t.Fatalf("credentials not imported: %q", imported)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(home, "*-"+s.store.run.ID.String()+".*")); len(leftovers) != 0 {
		t.Fatalf("run files left behind: %v", leftovers)
	}
	kinds := make([]domain.OutputKind, 0, len(s.store.outputs))
	for _, o := range s.store.outputs {
		kinds = append(kinds, o.Kind)
	}
	if len(kinds) != 3 || kinds[0] != domain.OutputToolUse || kinds[1] != domain.OutputToolResult || kinds[2] != domain.OutputText {
		t.Fatalf("outputs %+v", s.store.outputs)
	}

	// A resumed session keeps its id and gets the new prompt alone.
	if err := s.handle(t, "add tests"); err != nil {
		t.Fatal(err)
	}
	if args, _ := s.opencodeCall(t); !strings.HasSuffix(args, "--session "+session+" --model openrouter/m -- add tests") {
		t.Fatalf("resume args: %s", args)
	}
}

func TestOpencodeFailures(t *testing.T) {
	for prompt, want := range map[string]string{
		"[fail]":  "provider.free-limit: Free usage limit reached. (try again in 1h0m0s, or switch the node to a paid model)",
		"[crash]": "the agent ended without a result: exit status 2: boom",
	} {
		s := opencodeSetup(t)
		session := uuid.New()
		s.store.session = &session
		if err := s.handle(t, prompt); err != nil {
			t.Fatalf("%s: the job must not fail: %v", prompt, err)
		}
		if r := s.store.result; r.Status != domain.NodeFailed || r.Error != want {
			t.Fatalf("%s: %+v", prompt, r)
		}
	}

	tests := []struct {
		name  string
		setup func(s setup)
		want  string
	}{
		{"no CLI", func(s setup) { delete(s.exec.Binaries, domain.AgentOpencode) }, "this worker has no opencode CLI: install it, or set ORQ_OPENCODE_BIN"},
		{"credentials", func(s setup) { s.store.cred = seal(t, s.box, `{"bad":{}}`) }, `import the opencode credentials of profile "dev"`},
		{"instructions file", func(s setup) {
			_ = os.MkdirAll(filepath.Join(s.cardDir(), "claude", "orquestrator-"+s.store.run.ID.String()+".md"), 0o700)
		}, "is a directory"},
		{"credentials file", func(s setup) {
			_ = os.MkdirAll(filepath.Join(s.cardDir(), "claude", "auth-"+s.store.run.ID.String()+".json"), 0o700)
		}, "is a directory"},
		{"mcp server", func(s setup) { withMcp(s); s.store.errs["GetMcpServer"] = errStore }, "mcp server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := opencodeSetup(t)
			tt.setup(s)
			if err := s.handle(t, ""); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}

func TestOpencodeStream(t *testing.T) {
	var s OpencodeStream
	for _, line := range []string{
		`not json`, `{}`,
		`{"type":"step_start","timestamp":5}`,
		`{"type":"tool_use","timestamp":6,"part":{"tool":"bash","state":{"status":"error","input":{"command":"x"},"error":"denied"}}}`,
		`{"type":"error","timestamp":9,"error":{"type":"provider.no-route","message":"Model unavailable: x/y"}}`,
	} {
		s.Line([]byte(line))
	}
	r := s.Outcome(errors.New("exit status 1"))
	if r.Status != domain.NodeFailed || !strings.Contains(r.Error, "log it in again with `orquestrator agent login opencode`") || r.DurationMS != 4 {
		t.Fatalf("no route: %+v", r)
	}

	s = OpencodeStream{}
	steps := s.Line([]byte(`{"type":"tool_use","part":{"tool":"bash","state":{"status":"error","error":"denied"}}}`))
	if len(steps) != 2 || !steps[1].IsError || steps[1].Text != "denied" {
		t.Fatalf("tool error steps %+v", steps)
	}
	s.Line([]byte(`{"type":"step_start","timestamp":1}`))
	if r := s.Outcome(errors.New("exit status 1")); r.Status != domain.NodeFailed || r.Error != "exit status 1 (last failed tool: bash: denied)" {
		t.Fatalf("process error after a failed tool: %+v", r)
	}
	s = OpencodeStream{}
	s.Line([]byte(`{"type":"step_start","timestamp":1}`))
	if r := s.Outcome(errors.New("killed")); r.Status != domain.NodeFailed || r.Error != "killed" {
		t.Fatalf("process error: %+v", r)
	}
	if r := (&OpencodeStream{}).Outcome(nil); r.Error != "the agent ended without a result" {
		t.Fatalf("empty: %+v", r)
	}
	if OpencodeImportArgs("f")[3] != "f" {
		t.Fatal("import args")
	}
}
