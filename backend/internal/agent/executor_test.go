package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/crypto"
	"orquestrator/internal/domain"
	"orquestrator/internal/events"
	"orquestrator/internal/runner"
	"orquestrator/internal/workspace"
)

// fakeClaude is the path of the compiled testdata/fakeclaude.
var fakeClaude string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeclaude")
	if err != nil {
		panic(err)
	}
	fakeClaude = filepath.Join(dir, "claude")
	if out, err := exec.Command("go", "build", "-o", fakeClaude, "./testdata/fakeclaude").CombinedOutput(); err != nil {
		panic(fmt.Sprintf("build fake claude: %v: %s", err, out))
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

var errStore = errors.New("store failed")

// fakeStore holds one node run of one flow run; errs fails methods by name.
type fakeStore struct {
	errs     map[string]error
	run      domain.NodeRun
	flowRun  domain.FlowRun
	version  domain.FlowVersion
	card     domain.Card
	project  domain.Project
	profile  domain.AgentProfile
	cred     []byte
	mcp      map[uuid.UUID]domain.McpServer
	mcpSeals map[uuid.UUID][]byte
	session  *uuid.UUID
	started  bool
	result   *domain.NodeResult
	// tokenHash is what the run started with; pending and comments are what
	// the agent left through the board tools.
	tokenHash []byte
	pending   int
	comments  []domain.ReviewComment
	outputs   []domain.NodeOutput
}

func (f *fakeStore) AddNodeOutput(_ context.Context, _ uuid.UUID, entries []domain.NodeOutput) error {
	if err := f.err("AddNodeOutput"); err != nil {
		return err
	}
	f.outputs = append(f.outputs, entries...)
	return nil
}

// recorder keeps the published event types.
type recorder struct{ types []string }

func (r *recorder) Publish(e events.Event) { r.types = append(r.types, e.Type) }

func (f *fakeStore) err(name string) error { return f.errs[name] }

func (f *fakeStore) GetNodeRun(context.Context, uuid.UUID) (domain.NodeRun, error) {
	if f.started { // reading what the agent reported
		return f.run, f.err("GetNodeRun after")
	}
	return f.run, f.err("GetNodeRun")
}

func (f *fakeStore) PendingQuestions(context.Context, uuid.UUID) (int, error) {
	return f.pending, f.err("PendingQuestions")
}

func (f *fakeStore) ListReviewComments(context.Context, uuid.UUID) ([]domain.ReviewComment, error) {
	return f.comments, f.err("ListReviewComments")
}

func (f *fakeStore) GetFlowRun(context.Context, uuid.UUID) (domain.FlowRun, error) {
	return f.flowRun, f.err("GetFlowRun")
}

func (f *fakeStore) GetFlowVersion(context.Context, uuid.UUID) (domain.FlowVersion, error) {
	return f.version, f.err("GetFlowVersion")
}

func (f *fakeStore) GetCard(context.Context, uuid.UUID) (domain.Card, error) {
	return f.card, f.err("GetCard")
}

func (f *fakeStore) GetProject(context.Context, uuid.UUID) (domain.Project, error) {
	return f.project, f.err("GetProject")
}

func (f *fakeStore) GetAgentProfile(context.Context, uuid.UUID) (domain.AgentProfile, error) {
	return f.profile, f.err("GetAgentProfile")
}

func (f *fakeStore) AgentCredential(context.Context, uuid.UUID) ([]byte, error) {
	return f.cred, f.err("AgentCredential")
}

func (f *fakeStore) GetMcpServer(_ context.Context, id uuid.UUID) (domain.McpServer, error) {
	return f.mcp[id], f.err("GetMcpServer")
}

func (f *fakeStore) McpSecrets(_ context.Context, id uuid.UUID) ([]byte, error) {
	return f.mcpSeals[id], f.err("McpSecrets")
}

func (f *fakeStore) AgentSession(context.Context, uuid.UUID, string) (uuid.UUID, error) {
	if err := f.err("AgentSession"); err != nil {
		return uuid.Nil, err
	}
	if f.session == nil {
		return uuid.Nil, domain.ErrNotFound
	}
	return *f.session, nil
}

func (f *fakeStore) CreateAgentSession(_ context.Context, _ uuid.UUID, _ string, _, session uuid.UUID) error {
	f.session = &session
	return f.err("CreateAgentSession")
}

func (f *fakeStore) StartNodeRun(_ context.Context, _, _ uuid.UUID, tokenHash []byte) (domain.NodeRun, error) {
	f.started = true
	f.tokenHash = tokenHash
	return f.run, f.err("StartNodeRun")
}

func (f *fakeStore) FinishNodeRun(_ context.Context, _ uuid.UUID, r domain.NodeResult) (domain.NodeRun, error) {
	f.result = &r
	return f.run, f.err("FinishNodeRun")
}

type setup struct {
	pub   *recorder
	store *fakeStore
	exec  *Executor
	box   *crypto.Box
	data  string
}

func seal(t *testing.T, box *crypto.Box, plain string) []byte {
	t.Helper()
	sealed, err := box.Seal([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

// newSetup is an agent node "dev" on a card without project, run by a
// logged-in profile through the local runner and the fake CLI.
func newSetup(t *testing.T) setup {
	t.Helper()
	box, err := crypto.NewBox(crypto.NewKey())
	if err != nil {
		t.Fatal(err)
	}
	profile := domain.AgentProfile{ID: uuid.New(), Name: "dev", CredentialKind: domain.CredentialOAuthToken}
	config := `{"profile_id":"` + profile.ID.String() + `","prompt":"Implement it"}`
	flowRun := domain.FlowRun{ID: uuid.New(), FlowVersionID: uuid.New(), CardID: uuid.New()}
	store := &fakeStore{
		errs:    map[string]error{},
		run:     domain.NodeRun{ID: uuid.New(), FlowRunID: flowRun.ID, NodeKey: "dev", Status: domain.NodeQueued},
		flowRun: flowRun,
		version: domain.FlowVersion{Graph: domain.Graph{Nodes: []domain.Node{
			{ID: "dev", Type: domain.NodeAgent, Config: json.RawMessage(config)},
		}}},
		card:    domain.Card{ID: flowRun.CardID, Title: "Add login", Description: "With OAuth."},
		profile: profile,
		cred:    seal(t, box, "sk-ant-oat-secret"),
	}
	data := t.TempDir()
	pub := &recorder{}
	return setup{
		pub:   pub,
		store: store,
		box:   box,
		data:  data,
		exec: &Executor{
			Store:      store,
			Secrets:    box,
			Workspaces: workspace.Local{DataDir: data},
			Runner:     runner.Local{Home: t.TempDir()},
			Binary:     fakeClaude,
			BoardURL:   "http://board:8080",
			Pub:        pub,
		},
	}
}

func (s setup) handle(t *testing.T, prompt string) error {
	t.Helper()
	payload, _ := json.Marshal(Payload{NodeRunID: s.store.run.ID, Prompt: prompt})
	return s.exec.Handle(context.Background(), payload)
}

func (s setup) cardDir() string {
	return filepath.Join(s.data, "cards", s.store.card.ID.String())
}

// call reads what the fake CLI recorded about its last run.
func (s setup) call(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.cardDir(), "workspace", "fake-claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	var call map[string]any
	if err := json.Unmarshal(data, &call); err != nil {
		t.Fatal(err)
	}
	return call
}

func argsOf(call map[string]any) string {
	var parts []string
	for _, a := range call["args"].([]any) {
		parts = append(parts, a.(string))
	}
	return strings.Join(parts, " ")
}

func TestFirstRun(t *testing.T) {
	s := newSetup(t)
	if err := s.handle(t, "ignored on the first run"); err != nil {
		t.Fatal(err)
	}
	r := s.store.result
	if r.Status != domain.NodeDone || r.OutputHandle != "done" || r.Turns != 3 || r.CostUSD != 0.5 || r.TokensIn != 10 || r.CacheWrite != 40 {
		t.Fatalf("result %+v", r)
	}
	if !s.store.started || s.store.session == nil {
		t.Fatal("the node run did not start in a new session")
	}

	call := s.call(t)
	args := argsOf(call)
	if !strings.Contains(args, "--session-id "+s.store.session.String()) || !strings.Contains(args, "Implement it\n\n## Card\n\n# Add login\n\nWith OAuth.") {
		t.Fatalf("args: %s", args)
	}
	if call["oauth_token"] != "sk-ant-oat-secret" || call["api_key"] != "" || call["config_dir"] != filepath.Join(s.cardDir(), "claude") {
		t.Fatalf("env: %v", call)
	}
	if string(r.Output) != `{"result":"did it"}` {
		t.Fatalf("output %s", r.Output)
	}
	board := mcpServers(t, call)[boardServer]
	token, ok := strings.CutPrefix(board["headers"].(map[string]any)["Authorization"].(string), "Bearer ")
	sum := sha256.Sum256([]byte(token))
	if !ok || board["url"] != "http://board:8080/mcp" || !bytes.Equal(sum[:], s.store.tokenHash) {
		t.Fatalf("board MCP entry %v", board)
	}
	if !strings.Contains(args, "--allowedTools mcp__orquestrator ") || !strings.Contains(args, "call report_done") {
		t.Fatalf("args: %s", args)
	}
	if len(s.store.outputs) != 1 || s.store.outputs[0].Kind != domain.OutputResult || s.store.outputs[0].Text != "did it" {
		t.Fatalf("outputs %+v", s.store.outputs)
	}
	if !slices.Equal(s.pub.types, []string{"node_run.updated", "node_output.created"}) {
		t.Fatalf("published %v", s.pub.types)
	}
	log, err := os.ReadFile(filepath.Join(s.cardDir(), "logs", s.store.run.ID.String()+".jsonl"))
	if err != nil || strings.Count(string(log), "\n") != 3 {
		t.Fatalf("log: %q, %v", log, err)
	}
}

func TestResume(t *testing.T) {
	s := newSetup(t)
	session := uuid.New()
	s.store.session = &session
	s.store.profile.CredentialKind = domain.CredentialAPIKey
	if err := s.handle(t, "Fix the review comments"); err != nil {
		t.Fatal(err)
	}
	call := s.call(t)
	args := argsOf(call)
	if !strings.Contains(args, "-p Fix the review comments") || !strings.Contains(args, "--resume "+session.String()) {
		t.Fatalf("args: %s", args)
	}
	if call["api_key"] != "sk-ant-oat-secret" || call["oauth_token"] != "" {
		t.Fatalf("env: %v", call)
	}

	// Without a new prompt the session starts over from the node prompt.
	if err := s.handle(t, ""); err != nil {
		t.Fatal(err)
	}
	if args := argsOf(s.call(t)); !strings.Contains(args, "-p Implement it") {
		t.Fatalf("args: %s", args)
	}
}

func TestProjectWorktreeAndMcp(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, out)
		}
	}
	stdio, http := uuid.New(), uuid.New()
	s := newSetup(t)
	var config domain.AgentConfig
	_ = json.Unmarshal(s.store.version.Graph.Nodes[0].Config, &config)
	config.McpServerIDs = []uuid.UUID{stdio, http}
	s.store.version.Graph.Nodes[0].Config, _ = json.Marshal(config)
	projectID := uuid.New()
	s.store.card.ProjectID = &projectID
	s.store.project = domain.Project{ID: projectID, LocalPath: repo}
	s.store.mcp = map[uuid.UUID]domain.McpServer{
		stdio: {Name: "fs", Transport: domain.McpStdio, McpConfig: domain.McpConfig{Command: "srv", Args: []string{"-v"}, Env: map[string]string{"A": "1"}}},
		http:  {Name: "sg", Transport: domain.McpHTTP, McpConfig: domain.McpConfig{URL: "https://sg/mcp"}},
	}
	s.store.mcpSeals = map[uuid.UUID][]byte{http: seal(t, s.box, `{"Authorization":"Bearer x"}`)}

	if err := s.handle(t, ""); err != nil {
		t.Fatal(err)
	}
	call := s.call(t)
	mcp := mcpServers(t, call)
	if mcp["fs"]["command"] != "srv" || mcp["fs"]["env"].(map[string]any)["A"] != "1" {
		t.Fatalf("stdio entry: %v", mcp)
	}
	if mcp["sg"]["headers"].(map[string]any)["Authorization"] != "Bearer x" || mcp["sg"]["type"] != "http" {
		t.Fatalf("http entry: %v", mcp)
	}
	if args := argsOf(call); !strings.Contains(args, "--allowedTools mcp__orquestrator,mcp__fs,mcp__sg ") {
		t.Fatalf("the node MCP servers are not allowed: %s", args)
	}
	leftovers, _ := filepath.Glob(filepath.Join(s.cardDir(), "claude", "mcp-*.json"))
	if len(leftovers) != 0 {
		t.Fatalf("MCP config with secrets left behind: %v", leftovers)
	}
	if out, _ := exec.Command("git", "-C", filepath.Join(s.cardDir(), "workspace"), "branch", "--show-current").Output(); !strings.HasPrefix(string(out), "orq/card-") {
		t.Fatalf("workspace is not the card worktree: %q", out)
	}
}

func TestAgentFailures(t *testing.T) {
	for prompt, want := range map[string]string{
		"[fail]":  "error_during_execution: did it",
		"[crash]": "the agent ended without a result: exit status 2: boom",
	} {
		s := newSetup(t)
		session := uuid.New()
		s.store.session = &session
		if err := s.handle(t, prompt); err != nil {
			t.Fatalf("%s: the job must not fail: %v", prompt, err)
		}
		if r := s.store.result; r.Status != domain.NodeFailed || r.OutputHandle != "failed" || r.Error != want {
			t.Fatalf("%s: %+v", prompt, r)
		}
	}
}

func TestBadPayload(t *testing.T) {
	s := newSetup(t)
	if err := s.exec.Handle(context.Background(), json.RawMessage(`[]`)); err == nil {
		t.Fatal("expected error")
	}
	s.store.errs["GetNodeRun"] = errStore
	if err := s.handle(t, ""); !errors.Is(err, errStore) || s.store.result != nil {
		t.Fatalf("missing node run: %v", err)
	}
}

// TestRunFailures checks that a node that cannot run fails, and so does its job.
func TestRunFailures(t *testing.T) {
	tests := []struct {
		name  string
		setup func(s setup)
		want  string
	}{
		{"flow run", func(s setup) { s.store.errs["GetFlowRun"] = errStore }, "store failed"},
		{"version", func(s setup) { s.store.errs["GetFlowVersion"] = errStore }, "store failed"},
		{"node missing", func(s setup) { s.store.run.NodeKey = "other" }, `node "other" not found`},
		{"not an agent", func(s setup) { s.store.version.Graph.Nodes[0].Type = domain.NodeEnd }, `is a end node, not an agent`},
		{"bad review config", func(s setup) {
			s.store.version.Graph.Nodes[0] = domain.Node{ID: "dev", Type: domain.NodeReview, Config: json.RawMessage(`[]`)}
		}, "cannot unmarshal"},
		{"bad config", func(s setup) { s.store.version.Graph.Nodes[0].Config = json.RawMessage(`[]`) }, "cannot unmarshal"},
		{"card", func(s setup) { s.store.errs["GetCard"] = errStore }, "store failed"},
		{"project", func(s setup) {
			id := uuid.New()
			s.store.card.ProjectID = &id
			s.store.errs["GetProject"] = errStore
		}, "store failed"},
		{"no profile", func(s setup) { s.store.version.Graph.Nodes[0].Config = json.RawMessage(`{"prompt":"p"}`) }, "no agent profile"},
		{"profile", func(s setup) { s.store.errs["GetAgentProfile"] = errStore }, "store failed"},
		{"credential", func(s setup) { s.store.errs["AgentCredential"] = errStore }, "store failed"},
		{"not logged in", func(s setup) { s.store.cred = nil }, `agent profile "dev" is not logged in`},
		{"wrong key", func(s setup) { s.store.cred = []byte("garbage, not sealed by this key") }, "cannot decrypt"},
		{"workspace", func(s setup) {
			s.exec.Workspaces = workspace.Local{DataDir: "/dev/null/nope"}
		}, "prepare workspace"},
		{"session lookup", func(s setup) { s.store.errs["AgentSession"] = errStore }, "store failed"},
		{"session create", func(s setup) { s.store.errs["CreateAgentSession"] = errStore }, "store failed"},
		{"mcp server", func(s setup) { withMcp(s); s.store.errs["GetMcpServer"] = errStore }, "mcp server"},
		{"mcp secrets", func(s setup) { withMcp(s); s.store.errs["McpSecrets"] = errStore }, "store failed"},
		{"mcp secrets key", func(s setup) {
			id := withMcp(s)
			s.store.mcpSeals = map[uuid.UUID][]byte{id: []byte("garbage, not sealed by this key")}
		}, "cannot decrypt"},
		{"mcp secrets json", func(s setup) {
			id := withMcp(s)
			s.store.mcpSeals = map[uuid.UUID][]byte{id: seal(t, s.box, "[]")}
		}, "cannot unmarshal"},
		{"mcp file", func(s setup) {
			withMcp(s)
			_ = os.MkdirAll(filepath.Join(s.cardDir(), "claude", "mcp-"+s.store.run.ID.String()+".json"), 0o700)
		}, "is a directory"},
		{"start", func(s setup) { s.store.errs["StartNodeRun"] = errStore }, "store failed"},
		{"reports", func(s setup) { s.store.errs["GetNodeRun after"] = errStore }, "store failed"},
		{"questions", func(s setup) { s.store.errs["PendingQuestions"] = errStore }, "store failed"},
		{"comments", func(s setup) {
			s.store.version.Graph.Nodes[0].Type = domain.NodeReview
			s.store.errs["ListReviewComments"] = errStore
		}, "store failed"},
		{"log file", func(s setup) {
			_ = os.MkdirAll(filepath.Join(s.cardDir(), "logs", s.store.run.ID.String()+".jsonl"), 0o700)
		}, "is a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSetup(t)
			tt.setup(s)
			err := s.handle(t, "")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
			if r := s.store.result; r == nil || r.Status != domain.NodeFailed || !strings.Contains(r.Error, tt.want) {
				t.Fatalf("node result %+v", r)
			}
		})
	}
}

// withMcp gives the node one stdio MCP server and returns its id.
func withMcp(s setup) uuid.UUID {
	id := uuid.New()
	var config domain.AgentConfig
	_ = json.Unmarshal(s.store.version.Graph.Nodes[0].Config, &config)
	config.McpServerIDs = []uuid.UUID{id}
	s.store.version.Graph.Nodes[0].Config, _ = json.Marshal(config)
	s.store.mcp = map[uuid.UUID]domain.McpServer{id: {Name: "fs", Transport: domain.McpStdio, McpConfig: domain.McpConfig{Command: "srv"}}}
	return id
}

func TestFinishFailure(t *testing.T) {
	s := newSetup(t)
	s.store.errs["FinishNodeRun"] = errStore
	if err := s.handle(t, ""); !errors.Is(err, errStore) {
		t.Fatalf("got %v", err)
	}
}

func TestMerge(t *testing.T) {
	if merge(nil, nil) != nil {
		t.Fatal("empty maps should merge to nil")
	}
	got := merge(map[string]string{"a": "1", "b": "1"}, map[string]string{"b": "2"})
	if got["a"] != "1" || got["b"] != "2" {
		t.Fatalf("got %v", got)
	}
}

func TestFinishedNodeRunsAreNotRunAgain(t *testing.T) {
	for _, status := range []domain.NodeStatus{domain.NodeDone, domain.NodeFailed, domain.NodeAwaitingInput} {
		s := newSetup(t)
		s.store.run.Status = status
		if err := s.handle(t, ""); err != nil {
			t.Fatal(err)
		}
		if s.store.started || s.store.result != nil {
			t.Fatalf("%s: the node ran again", status)
		}
	}
}

// mcpServers reads the servers of the --mcp-config the fake CLI got.
func mcpServers(t *testing.T, call map[string]any) map[string]map[string]any {
	t.Helper()
	var mcp struct {
		McpServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(call["mcp_config"].(string)), &mcp); err != nil {
		t.Fatalf("mcp config %q: %v", call["mcp_config"], err)
	}
	return mcp.McpServers
}

func TestWaitingForAnswers(t *testing.T) {
	s := newSetup(t)
	s.store.pending = 1
	if err := s.handle(t, ""); err != nil {
		t.Fatal(err)
	}
	if r := s.store.result; r.Status != domain.NodeAwaitingInput || r.OutputHandle != "" || r.Output != nil || r.Turns != 3 {
		t.Fatalf("result %+v", r)
	}
}

func TestReviewNode(t *testing.T) {
	s := newSetup(t)
	cfg := `{"profile_id":"` + s.store.profile.ID.String() + `","prompt":"Review it","max_rounds":2,"disallowed_tools":["WebFetch"]}`
	s.store.version.Graph.Nodes[0] = domain.Node{ID: "dev", Type: domain.NodeReview, Config: json.RawMessage(cfg)}
	approved := false
	s.store.run.ReviewApproved = &approved
	s.store.run.Iteration = 1
	s.store.comments = []domain.ReviewComment{{File: "a.go", Body: "add tests"}}
	if err := s.handle(t, ""); err != nil {
		t.Fatal(err)
	}
	if r := s.store.result; r.OutputHandle != "changes_requested" || !strings.Contains(string(r.Output), "a.go: add tests") {
		t.Fatalf("result %+v (%s)", r, r.Output)
	}
	args := argsOf(s.call(t))
	if !strings.Contains(args, "--disallowedTools WebFetch,Edit,MultiEdit,Write,NotebookEdit,") || !strings.Contains(args, "calling submit_review") {
		t.Fatalf("args: %s", args)
	}
}

func TestOutputThatCannotBeKept(t *testing.T) {
	s := newSetup(t)
	s.store.errs["AddNodeOutput"] = errStore
	if err := s.handle(t, ""); err != nil {
		t.Fatal(err)
	}
	if s.store.result.Status != domain.NodeDone || slices.Contains(s.pub.types, "node_output.created") {
		t.Fatalf("result %+v, published %v", s.store.result, s.pub.types)
	}
}

func TestInterruptedAgent(t *testing.T) {
	s := newSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	payload, _ := json.Marshal(Payload{NodeRunID: s.store.run.ID})
	s.exec.Runner = cancelling{Runner: s.exec.Runner, cancel: cancel}
	if err := s.exec.Handle(ctx, payload); err != nil {
		t.Fatal(err)
	}
	if r := s.store.result; r.Status != domain.NodeFailed || !strings.Contains(r.Error, "interrupted") {
		t.Fatalf("result %+v", r)
	}
}

// cancelling cancels the job context while the agent runs.
type cancelling struct {
	runner.Runner
	cancel context.CancelFunc
}

func (c cancelling) Run(ctx context.Context, spec runner.Spec, line func([]byte)) error {
	c.cancel()
	return c.Runner.Run(ctx, spec, line)
}
