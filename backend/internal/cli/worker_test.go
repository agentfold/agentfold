package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

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
// TestFlowEndToEnd runs the server and a worker on one database: moving a
// card into a lane starts a flow whose agent (the fake CLI) runs and whose
// MoveCard node moves the card on, which the event stream shows live.
func TestFlowEndToEnd(t *testing.T) {
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
	b := newFlowBoard(t, board)

	t.Setenv(envAddr, "127.0.0.1:0")
	t.Setenv(envRunner, "local")
	t.Setenv(envClaudeBin, "")
	t.Setenv(envDataDir, "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	sctx, stopServer := context.WithCancel(ctx)
	var serverOut, serverErr syncBuffer
	serverDone := make(chan int)
	go func() { serverDone <- Run(sctx, []string{"server"}, &serverOut, &serverErr) }()
	base := waitListening(t, &serverOut)
	stream := openEvents(t, base)

	wctx, stopWorker := context.WithCancel(ctx)
	var workerOut, workerErr syncBuffer
	workerDone := make(chan int)
	go func() { workerDone <- Run(wctx, []string{"worker"}, &workerOut, &workerErr) }()

	card := post(t, base+"/api/cards", fmt.Sprintf(`{"title":"Add login","lane_id":%q,"card_type_id":%q}`, b.todo, b.cardType))
	cardID := card["id"].(string)
	moved := postStatus(t, base+"/api/cards/"+cardID+"/move", fmt.Sprintf(`{"lane_id":%q,"position":0}`, b.doing), http.StatusOK)
	if moved["lane_id"] != b.doing.String() {
		t.Fatalf("moved %v", moved)
	}
	// The card is created and moved by hand, then the flow starts, its agent
	// runs, it moves the card to Done and ends. The server and the worker
	// publish concurrently, so only the flow events keep their order.
	var seen []string
	for len(seen) == 0 || seen[len(seen)-1] != "event: flow_run.updated" {
		seen = append(seen, nextEventOf(t, stream, "card_event.created"))
	}
	slices.Sort(seen)
	if want := []string{"event: card.created", "event: card.moved", "event: card.moved", "event: flow_run.created", "event: flow_run.updated"}; !slices.Equal(seen, want) {
		t.Fatalf("events %q, want %q; worker log: %s", seen, want, workerErr.String())
	}
	got, err := st.GetCard(ctx, uuid.MustParse(cardID))
	if err != nil || got.LaneID != b.done {
		t.Fatalf("card %+v, %v", got, err)
	}
	runs, err := st.ListLatestFlowRuns(ctx)
	if err != nil || len(runs) != 1 || runs[0].Status != domain.RunDone || runs[0].FlowName != "dev" {
		t.Fatalf("runs %+v, %v", runs, err)
	}
	timeline, err := st.ListCardEvents(ctx, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, e := range timeline {
		types = append(types, string(e.Type))
	}
	want := "card.created card.moved flow.started node.started node.finished node.started card.moved node.finished node.started node.finished flow.finished"
	if strings.Join(types, " ") != want {
		t.Fatalf("timeline %q", types)
	}
	var turns int
	if err := pool.QueryRow(ctx, `SELECT turns FROM node_runs WHERE node_key = 'dev'`).Scan(&turns); err != nil || turns != 3 {
		t.Fatalf("agent turns %d, %v", turns, err)
	}

	stopWorker()
	if code := <-workerDone; code != ExitOK {
		t.Fatalf("worker exit %d: %s", code, workerErr.String())
	}
	host, _ := os.Hostname()
	if out := workerOut.String(); !strings.HasPrefix(out, "worker "+host+"-") || !strings.Contains(out, filepath.Join(".local", "share", "orquestrator")) {
		t.Fatalf("worker stdout %q", out)
	}
	stopServer()
	if code := <-serverDone; code != ExitOK {
		t.Fatalf("server exit %d: %s", code, serverErr.String())
	}
}

type flowBoard struct {
	todo, doing, done, cardType uuid.UUID
}

// newFlowBoard creates lanes Todo, Doing and Done and the enabled flow "dev":
// a card entering Doing is implemented by an agent and moved to Done.
func newFlowBoard(t *testing.T, board *service.Service) flowBoard {
	t.Helper()
	ctx := context.Background()
	var b flowBoard
	for _, lane := range []struct {
		name string
		id   *uuid.UUID
	}{{"Todo", &b.todo}, {"Doing", &b.doing}, {"Done", &b.done}} {
		l, err := board.CreateLane(ctx, domain.LaneInput{Name: lane.name})
		if err != nil {
			t.Fatal(err)
		}
		*lane.id = l.ID
	}
	cardType, err := board.CreateCardType(ctx, domain.CardTypeInput{Name: "task"})
	if err != nil {
		t.Fatal(err)
	}
	b.cardType = cardType.ID
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
			{ID: "t", Type: domain.NodeTrigger, Config: fmt.Appendf(nil, `{"event":"card_entered_lane","lane_id":%q}`, b.doing)},
			{ID: "dev", Type: domain.NodeAgent, Config: fmt.Appendf(nil, `{"profile_id":%q,"prompt":"Implement it"}`, profile.ID)},
			{ID: "move", Type: domain.NodeMoveCard, Config: fmt.Appendf(nil, `{"lane_id":%q}`, b.done)},
			{ID: "end", Type: domain.NodeEnd, Config: []byte(`{}`)},
		},
		Edges: []domain.Edge{
			{ID: "e1", Source: "t", SourceHandle: "next", Target: "dev"},
			{ID: "e2", Source: "dev", SourceHandle: "done", Target: "move"},
			{ID: "e3", Source: "move", SourceHandle: "next", Target: "end"},
		},
	}
	if _, err := board.SaveFlowDraft(ctx, flow.ID, graph); err != nil {
		t.Fatal(err)
	}
	if _, err := board.PublishFlow(ctx, flow.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := board.UpdateFlow(ctx, flow.ID, domain.FlowInput{Name: "dev", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return b
}

// nextEventOf returns the next "event:" line of the stream, skipping the
// events of type skip.
func nextEventOf(t *testing.T, lines chan string, skip string) string {
	t.Helper()
	for {
		if line := nextEvent(t, lines); line != "event: "+skip {
			return line
		}
	}
}

type failingNotifier struct{}

func (failingNotifier) NotifyChange(context.Context, string, uuid.UUID) error {
	return errors.New("database down")
}

func TestNotifierLogsFailures(t *testing.T) {
	var log bytes.Buffer
	n := notifier{store: failingNotifier{}, log: slog.New(slog.NewTextHandler(&log, nil))}
	n.Publish(events.Event{Type: "card.moved", ID: uuid.New()})
	if !strings.Contains(log.String(), "cannot announce a change") || !strings.Contains(log.String(), "database down") {
		t.Fatalf("log %q", log.String())
	}
}
