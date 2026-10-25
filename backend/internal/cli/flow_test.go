package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"orquestrator/internal/crypto"
	"orquestrator/internal/domain"
	"orquestrator/internal/events"
	"orquestrator/internal/gitrepo"
	"orquestrator/internal/service"
	"orquestrator/internal/store"
)

// stack is a server and a worker running on one fresh database, with the
// fake claude CLI, and a board with lanes Todo, Doing and Done.
type stack struct {
	pool                   *pgxpool.Pool
	store                  *store.Store
	board                  *service.Service
	base                   string
	stream                 chan string
	todo, doing, done, typ uuid.UUID
	profile                uuid.UUID
	workerOut, workerErr   *syncBuffer
	stopServer, stopWorker context.CancelFunc
	serverDone, workerDone chan int
	serverErr              *syncBuffer
}

func newStack(t *testing.T) *stack {
	t.Helper()
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
	t.Cleanup(pool.Close)
	box, _ := crypto.NewBox(key)
	s := &stack{pool: pool, store: store.New(pool)}
	s.board = service.New(s.store, gitrepo.Inspector{}, events.NewHub(1), box)
	s.setUpBoard(t)

	t.Setenv(envAddr, "127.0.0.1:0")
	t.Setenv(envRunner, "local")
	t.Setenv(envClaudeBin, "")
	t.Setenv(envDataDir, "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var sctx, wctx context.Context
	sctx, s.stopServer = context.WithCancel(ctx)
	var serverOut syncBuffer
	s.serverErr = &syncBuffer{}
	s.serverDone = make(chan int)
	go func() { s.serverDone <- Run(sctx, []string{"server"}, &serverOut, s.serverErr) }()
	s.base = waitListening(t, &serverOut)
	s.stream = openEvents(t, s.base)

	t.Setenv(envServerURL, s.base+"/")
	wctx, s.stopWorker = context.WithCancel(ctx)
	s.workerOut, s.workerErr = &syncBuffer{}, &syncBuffer{}
	s.workerDone = make(chan int)
	go func() { s.workerDone <- Run(wctx, []string{"worker"}, s.workerOut, s.workerErr) }()
	return s
}

func (s *stack) setUpBoard(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, lane := range []struct {
		name string
		id   *uuid.UUID
	}{{"Todo", &s.todo}, {"Doing", &s.doing}, {"Done", &s.done}} {
		l, err := s.board.CreateLane(ctx, domain.LaneInput{Name: lane.name})
		if err != nil {
			t.Fatal(err)
		}
		*lane.id = l.ID
	}
	cardType, err := s.board.CreateCardType(ctx, domain.CardTypeInput{Name: "task"})
	if err != nil {
		t.Fatal(err)
	}
	s.typ = cardType.ID
	profile, err := s.board.LoginAgent(ctx, "dev", "sk-ant-oat01-x")
	if err != nil {
		t.Fatal(err)
	}
	s.profile = profile.ID
}

// flow publishes and enables a flow triggered by cards entering Doing:
// trigger -> nodes in order (by their "next"/"done"/"approved" outputs),
// plus the extra edges.
func (s *stack) flow(t *testing.T, nodes []domain.Node, extra ...domain.Edge) {
	t.Helper()
	ctx := context.Background()
	graph := domain.Graph{Nodes: append([]domain.Node{{
		ID: "t", Type: domain.NodeTrigger, Config: fmt.Appendf(nil, `{"event":"card_entered_lane","lane_id":%q}`, s.doing),
	}}, nodes...)}
	for i := 1; i < len(graph.Nodes); i++ {
		prev := graph.Nodes[i-1]
		handle := map[domain.NodeType]string{domain.NodeAgent: "done", domain.NodeReview: "approved"}[prev.Type]
		if handle == "" {
			handle = "next"
		}
		graph.Edges = append(graph.Edges, domain.Edge{ID: prev.ID + "." + handle, Source: prev.ID, SourceHandle: handle, Target: graph.Nodes[i].ID})
	}
	graph.Edges = append(graph.Edges, extra...)
	f, err := s.board.CreateFlow(ctx, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.board.SaveFlowDraft(ctx, f.ID, graph); err != nil {
		t.Fatal(err)
	}
	if _, err := s.board.PublishFlow(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.board.UpdateFlow(ctx, f.ID, domain.FlowInput{Name: "dev", Enabled: true}); err != nil {
		t.Fatal(err)
	}
}

func (s *stack) agent(id, prompt string) domain.Node {
	return domain.Node{ID: id, Type: domain.NodeAgent, Config: fmt.Appendf(nil, `{"profile_id":%q,"prompt":%q}`, s.profile, prompt)}
}

func (s *stack) moveTo(id string, lane uuid.UUID) domain.Node {
	return domain.Node{ID: id, Type: domain.NodeMoveCard, Config: fmt.Appendf(nil, `{"lane_id":%q}`, lane)}
}

// startCard creates a card in Todo and moves it into Doing by hand.
func (s *stack) startCard(t *testing.T) uuid.UUID {
	t.Helper()
	card := post(t, s.base+"/api/cards", fmt.Sprintf(`{"title":"Add login","lane_id":%q,"card_type_id":%q}`, s.todo, s.typ))
	moved := postStatus(t, s.base+"/api/cards/"+card["id"].(string)+"/move", fmt.Sprintf(`{"lane_id":%q,"position":0}`, s.doing), http.StatusOK)
	if moved["lane_id"] != s.doing.String() {
		t.Fatalf("moved %v", moved)
	}
	return uuid.MustParse(card["id"].(string))
}

// waitRun waits until the latest run of the card has status.
func (s *stack) waitRun(t *testing.T, card uuid.UUID, status domain.RunStatus) domain.FlowRun {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := s.store.ListLatestFlowRuns(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range runs {
			if r.CardID == card && r.Status == status {
				return r
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the run did not get %s; worker log: %s", status, s.workerErr.String())
	return domain.FlowRun{}
}

func (s *stack) timeline(t *testing.T, card uuid.UUID) []string {
	t.Helper()
	events, err := s.store.ListCardEvents(context.Background(), card)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, e := range events {
		types = append(types, string(e.Type))
	}
	return types
}

func (s *stack) stop(t *testing.T) {
	t.Helper()
	s.stopWorker()
	if code := <-s.workerDone; code != ExitOK {
		t.Fatalf("worker exit %d: %s", code, s.workerErr.String())
	}
	s.stopServer()
	if code := <-s.serverDone; code != ExitOK {
		t.Fatalf("server exit %d: %s", code, s.serverErr.String())
	}
}

// TestFlowEndToEnd: moving a card into a lane starts a flow whose agent (the
// fake CLI) runs and whose MoveCard node moves the card on, which the event
// stream shows live.
func TestFlowEndToEnd(t *testing.T) {
	s := newStack(t)
	s.flow(t, []domain.Node{s.agent("dev", "Implement it"), s.moveTo("move", s.done), {ID: "end", Type: domain.NodeEnd, Config: []byte(`{}`)}})
	card := s.startCard(t)
	// The card is created and moved by hand, then the flow starts, its agent
	// runs, it moves the card to Done and ends. The server and the worker
	// publish concurrently, so only the flow events keep their order.
	var seen []string
	for len(seen) == 0 || seen[len(seen)-1] != "event: flow_run.updated" {
		seen = append(seen, nextEventOf(t, s.stream, "card_event.created"))
	}
	slices.Sort(seen)
	if want := []string{"event: card.created", "event: card.moved", "event: card.moved", "event: flow_run.created", "event: flow_run.updated"}; !slices.Equal(seen, want) {
		t.Fatalf("events %q, want %q; worker log: %s", seen, want, s.workerErr.String())
	}
	got, err := s.store.GetCard(context.Background(), card)
	if err != nil || got.LaneID != s.done {
		t.Fatalf("card %+v, %v", got, err)
	}
	if run := s.waitRun(t, card, domain.RunDone); run.FlowName != "dev" {
		t.Fatalf("run %+v", run)
	}
	want := "card.created card.moved flow.started node.started node.finished node.started card.moved node.finished node.started node.finished flow.finished"
	if got := strings.Join(s.timeline(t, card), " "); got != want {
		t.Fatalf("timeline %q", got)
	}
	var turns int
	if err := s.pool.QueryRow(context.Background(), `SELECT turns FROM node_runs WHERE node_key = 'dev'`).Scan(&turns); err != nil || turns != 3 {
		t.Fatalf("agent turns %d, %v", turns, err)
	}
	s.stop(t)
	host, _ := os.Hostname()
	if out := s.workerOut.String(); !strings.HasPrefix(out, "worker "+host+"-") || !strings.Contains(out, filepath.Join(".local", "share", "orquestrator")) {
		t.Fatalf("worker stdout %q", out)
	}
}

// TestReviewCycleEndToEnd: the developer agent asks a question and waits;
// once answered it resumes and finishes; the reviewer asks for changes, the
// developer resumes with the comments, and the second review approves.
func TestReviewCycleEndToEnd(t *testing.T) {
	s := newStack(t)
	review := domain.Node{ID: "rev", Type: domain.NodeReview, Config: fmt.Appendf(nil,
		`{"profile_id":%q,"prompt":"Review it [review:reject,approve]","max_rounds":3}`, s.profile)}
	s.flow(t, []domain.Node{s.agent("dev", "Implement it [ask:Which database?] [done:implemented]"), review, s.moveTo("move", s.done)},
		domain.Edge{ID: "rev.changes", Source: "rev", SourceHandle: "changes_requested", Target: "dev"})
	card := s.startCard(t)

	s.waitRun(t, card, domain.RunWaiting)
	questions := getJSON[[]map[string]any](t, s.base+"/api/cards/"+card.String()+"/questions")
	if len(questions) != 1 || questions[0]["text"] != "Which database?" || questions[0]["answer"] != nil {
		t.Fatalf("questions %v", questions)
	}
	resp, err := http.Post(s.base+"/api/cards/"+card.String()+"/move", "application/json", strings.NewReader(fmt.Sprintf(`{"lane_id":%q,"position":0}`, s.todo)))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("moving a waiting card: %d", resp.StatusCode)
	}
	postStatus(t, s.base+"/api/questions/"+questions[0]["id"].(string)+"/answer", `{"answer":"Postgres"}`, http.StatusOK)

	s.waitRun(t, card, domain.RunDone)
	got, err := s.store.GetCard(context.Background(), card)
	if err != nil || got.LaneID != s.done {
		t.Fatalf("card %+v, %v", got, err)
	}
	types := s.timeline(t, card)
	for typ, n := range map[string]int{"question.asked": 1, "question.answered": 1, "review.submitted": 2} {
		if c := countOf(types, typ); c != n {
			t.Fatalf("%d %s in %q", c, typ, types)
		}
	}
	rows, err := s.pool.Query(context.Background(), `SELECT node_key, iteration, status, coalesce(output_handle, ''), turns FROM node_runs ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	var runs []string
	for rows.Next() {
		var key, status, handle string
		var iteration, turns int
		if err := rows.Scan(&key, &iteration, &status, &handle, &turns); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, fmt.Sprintf("%s#%d %s %s %d", key, iteration, status, handle, turns))
	}
	// The first developer run ran twice (asked, then resumed with the answer).
	want := []string{"dev#1 done done 6", "rev#1 done changes_requested 3", "dev#2 done done 3", "rev#2 done approved 3", "move#1 done next 0"}
	if !slices.Equal(runs, want) {
		t.Fatalf("node runs %q", runs)
	}
	var prompt string
	if err := s.pool.QueryRow(context.Background(), `SELECT payload->>'prompt' FROM jobs WHERE kind = 'agent.run' ORDER BY created_at OFFSET 1 LIMIT 1`).Scan(&prompt); err != nil || !strings.Contains(prompt, "Answer: Postgres") {
		t.Fatalf("resume prompt %q, %v", prompt, err)
	}
	s.stop(t)
}

func countOf(items []string, item string) int {
	n := 0
	for _, i := range items {
		if i == item {
			n++
		}
	}
	return n
}

func getJSON[T any](t *testing.T, url string) T {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("GET %s: %s", url, data)
	}
	return v
}
