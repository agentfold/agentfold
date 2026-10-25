package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"orquestrator/internal/agent"
	"orquestrator/internal/crypto"
	"orquestrator/internal/domain"
	"orquestrator/internal/events"
	"orquestrator/internal/gitrepo"
	"orquestrator/internal/service"
	"orquestrator/internal/store"
)

// script writes an executable shell script named name in a new directory.
func script(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSecretKey(t *testing.T) {
	code, stdout, _ := run("secret-key")
	if code != ExitOK {
		t.Fatalf("code %d", code)
	}
	if _, err := crypto.NewBox(strings.TrimSpace(stdout)); err != nil {
		t.Fatalf("printed key %q: %v", stdout, err)
	}
	if code, _, _ := run("secret-key", "x"); code != ExitError {
		t.Fatalf("extra args: %d", code)
	}
}

func TestAgentUsage(t *testing.T) {
	for _, args := range [][]string{
		{"agent"},
		{"agent", "logout", "claude"},
		{"agent", "login", "claude", "extra"},
		{"agent", "login", "claude", "--nope"},
	} {
		if code, _, stderr := run(args...); code != ExitError || stderr == "" {
			t.Errorf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
}

// loginEnv sets up a database and a fake claude whose setup-token runs body.
func loginEnv(t *testing.T, body string) *pgxpool.Pool {
	t.Helper()
	url := freshDatabase(t)
	t.Setenv(envDatabaseURL, url)
	t.Setenv(envSecretKey, crypto.NewKey())
	t.Setenv(envClaudeBin, script(t, "claude", body))
	if code, _, stderr := run("migrate"); code != ExitOK {
		t.Fatalf("migrate: %s", stderr)
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func paste(t *testing.T, text string) {
	t.Helper()
	old := stdin
	stdin = strings.NewReader(text)
	t.Cleanup(func() { stdin = old })
}

func TestAgentLogin(t *testing.T) {
	pool := loginEnv(t, `[ "$1" = setup-token ] && [ -n "$CLAUDE_CONFIG_DIR" ] && echo "token: sk-ant-oat01-abc"`)
	paste(t, "sk-ant-oat01-abc\n")
	code, stdout, stderr := run("agent", "login", "claude", "--profile", "dev")
	if code != ExitOK || !strings.Contains(stdout, "token: sk-ant-oat01-abc") || !strings.Contains(stdout, `Agent profile "dev" is ready (oauth_token)`) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	p, err := store.New(pool).GetAgentProfileByName(context.Background(), "dev")
	if err != nil || !p.Ready() {
		t.Fatalf("profile %+v, %v", p, err)
	}
}

func TestAgentLoginWithApiKeyAfterSetupFails(t *testing.T) {
	loginEnv(t, "exit 1")
	paste(t, "sk-ant-api03-key") // no newline: the end of input is fine
	code, stdout, stderr := run("agent", "login", "claude")
	if code != ExitOK || !strings.Contains(stderr, "setup-token did not finish") || !strings.Contains(stdout, `"default" is ready (api_key)`) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestAgentLoginErrors(t *testing.T) {
	t.Run("nothing pasted", func(t *testing.T) {
		loginEnv(t, "true")
		paste(t, "")
		if code, _, stderr := run("agent", "login", "claude"); code != ExitError || !strings.Contains(stderr, "no token read") {
			t.Fatalf("code %d, stderr %q", code, stderr)
		}
	})
	t.Run("not a token", func(t *testing.T) {
		loginEnv(t, "true")
		paste(t, "hunter2\n")
		if code, _, stderr := run("agent", "login", "claude"); code != ExitError || !strings.Contains(stderr, "credential:") {
			t.Fatalf("code %d, stderr %q", code, stderr)
		}
	})
	t.Run("no temp dir", func(t *testing.T) {
		loginEnv(t, "true")
		t.Setenv("TMPDIR", "/does/not/exist")
		if code, _, _ := run("agent", "login", "claude"); code != ExitError {
			t.Fatalf("code %d", code)
		}
	})
	t.Run("configuration", func(t *testing.T) {
		t.Setenv(envSecretKey, "")
		if code, _, stderr := run("agent", "login", "claude"); code != ExitError || !strings.Contains(stderr, "ORQ_SECRET_KEY") {
			t.Fatalf("no key: %q", stderr)
		}
		t.Setenv(envSecretKey, crypto.NewKey())
		t.Setenv(envClaudeBin, "")
		t.Setenv("PATH", t.TempDir())
		if code, _, stderr := run("agent", "login", "claude"); code != ExitError || !strings.Contains(stderr, "claude CLI not found") {
			t.Fatalf("no claude: %q", stderr)
		}
		t.Setenv(envClaudeBin, "/bin/true")
		t.Setenv(envDatabaseURL, "")
		if code, _, stderr := run("agent", "login", "claude"); code != ExitError || !strings.Contains(stderr, "ORQ_DATABASE_URL") {
			t.Fatalf("no database: %q", stderr)
		}
	})
}

func TestWorkerConfigurationErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	tests := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"args", nil, []string{"x"}, "unexpected arguments"},
		{"key", map[string]string{envSecretKey: ""}, nil, "ORQ_SECRET_KEY"},
		{"no home", map[string]string{envDataDir: "", "HOME": ""}, nil, "ORQ_DATA_DIR is not set"},
		{"no claude", map[string]string{envClaudeBin: "", "PATH": t.TempDir()}, nil, "claude CLI not found"},
		{"bad runner", map[string]string{envRunner: "docker"}, nil, `ORQ_RUNNER must be bwrap or local, not "docker"`},
		{"no bwrap", map[string]string{envRunner: "bwrap", "PATH": t.TempDir()}, nil, "bwrap not found"},
		{"claude missing for the sandbox", map[string]string{envClaudeBin: missing, "PATH": "/usr/bin:/bin"}, nil, "missing"},
		{"database with bwrap", map[string]string{"PATH": "/usr/bin:/bin"}, nil, "ORQ_DATABASE_URL is not set"},
		{"database with local", map[string]string{envRunner: "local"}, nil, "ORQ_DATABASE_URL is not set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envSecretKey, crypto.NewKey())
			t.Setenv(envDataDir, t.TempDir())
			t.Setenv(envClaudeBin, "/bin/true")
			t.Setenv(envRunner, "")
			t.Setenv(envDatabaseURL, "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			code, _, stderr := run(append([]string{"worker"}, tt.args...)...)
			if code != ExitError || !strings.Contains(stderr, tt.want) {
				t.Fatalf("code %d, stderr %q; want %q", code, stderr, tt.want)
			}
		})
	}
}

// TestWorkerEndToEnd runs an agent node through the queue: a worker with the
// local runner and the fake claude CLI (found in PATH) takes the job and
// records the run in the database.
func TestWorkerEndToEnd(t *testing.T) {
	ctx := context.Background()
	bin := t.TempDir()
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "claude"), "../agent/testdata/fakeclaude").CombinedOutput(); err != nil {
		t.Fatalf("build fake claude: %v: %s", err, out)
	}
	key := crypto.NewKey()
	url := freshDatabase(t)
	t.Setenv(envDatabaseURL, url)
	t.Setenv(envSecretKey, key)
	if code, _, stderr := run("migrate"); code != ExitOK {
		t.Fatalf("migrate: %s", stderr)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := crypto.NewBox(key)
	st := store.New(pool)
	board := service.New(st, gitrepo.Inspector{}, events.NewHub(1), box)
	nodeRun, host := agentJob(t, board, st)

	t.Setenv(envRunner, "local")
	t.Setenv(envClaudeBin, "")
	t.Setenv(envDataDir, "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	wctx, cancel := context.WithCancel(ctx)
	var stdout, stderr syncBuffer
	done := make(chan int)
	go func() { done <- Run(wctx, []string{"worker"}, &stdout, &stderr) }()

	deadline := time.Now().Add(30 * time.Second)
	var got domain.NodeRun
	for time.Now().Before(deadline) {
		if got, err = st.GetNodeRun(ctx, nodeRun); err == nil && got.Status == domain.NodeDone {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	if code := <-done; code != ExitOK {
		t.Fatalf("worker exit %d: %s", code, stderr.String())
	}
	if got.Status != domain.NodeDone || got.Turns != 3 || got.SessionID == nil {
		t.Fatalf("node run %+v; worker log: %s", got, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "worker "+host+"-") || !strings.Contains(stdout.String(), filepath.Join(".local", "share", "orquestrator")) {
		t.Fatalf("stdout %q", stdout.String())
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM jobs`).Scan(&status); err != nil || status != "done" {
		t.Fatalf("job status %q, %v", status, err)
	}
}

// agentJob builds a card and a published flow with one agent node, starts
// a run and enqueues the job of the node; it returns the node run id and
// the host the job requires.
func agentJob(t *testing.T, board *service.Service, st *store.Store) (nodeRunID [16]byte, host string) {
	t.Helper()
	ctx := context.Background()
	lane, err := board.CreateLane(ctx, domain.LaneInput{Name: "Todo"})
	if err != nil {
		t.Fatal(err)
	}
	cardType, err := board.CreateCardType(ctx, domain.CardTypeInput{Name: "task"})
	if err != nil {
		t.Fatal(err)
	}
	card, err := board.CreateCard(ctx, domain.NewCard{CardInput: domain.CardInput{Title: "Do it", CardTypeID: cardType.ID}, LaneID: lane.ID})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := board.LoginAgent(ctx, "dev", "sk-ant-oat01-x")
	if err != nil {
		t.Fatal(err)
	}
	flow, err := board.CreateFlow(ctx, "dev")
	if err != nil {
		t.Fatal(err)
	}
	graph := domain.Graph{
		Nodes: []domain.Node{
			{ID: "t", Type: domain.NodeTrigger, Config: []byte(`{"event":"card_created"}`)},
			{ID: "dev", Type: domain.NodeAgent, Config: []byte(`{"profile_id":"` + profile.ID.String() + `","prompt":"Implement it"}`)},
		},
		Edges: []domain.Edge{{ID: "e", Source: "t", SourceHandle: "next", Target: "dev"}},
	}
	if _, err := board.SaveFlowDraft(ctx, flow.ID, graph); err != nil {
		t.Fatal(err)
	}
	version, err := board.PublishFlow(ctx, flow.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateFlowRun(ctx, version.ID, card.ID, domain.EventCardCreated)
	if err != nil {
		t.Fatal(err)
	}
	nodeRun, err := st.CreateNodeRun(ctx, run.ID, "dev", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	host, _ = os.Hostname()
	if _, err := st.EnqueueJob(ctx, store.NewJob{
		Kind: agent.JobKind, Payload: agent.Payload{NodeRunID: nodeRun.ID}, Requirements: map[string]string{"host": host},
	}); err != nil {
		t.Fatal(err)
	}
	return nodeRun.ID, host
}
