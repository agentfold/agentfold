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
	// The fake plays opencode too.
	if err := os.Symlink(filepath.Join(bin, "claude"), filepath.Join(bin, "opencode")); err != nil {
		t.Fatal(err)
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
	t.Setenv(envOpencodeBin, "")
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
	profile, err := s.board.LoginAgent(ctx, domain.AgentClaude, "dev", "sk-ant-oat01-x")
	if err != nil {
		t.Fatal(err)
	}
	s.profile = profile.ID
}

// flow publishes and enables a flow triggered by cards entering Doing:
// trigger -> nodes in order (by their first output: "next", "done" or
// "approved"), plus the extra edges.
func (s *stack) flow(t *testing.T, nodes []domain.Node, extra ...domain.Edge) {
	t.Helper()
	s.flowOn(t, "dev", s.doing, nodes, extra...)
}

// flowOn is flow with its name and the lane that triggers it.
func (s *stack) flowOn(t *testing.T, name string, lane uuid.UUID, nodes []domain.Node, extra ...domain.Edge) {
	t.Helper()
	ctx := context.Background()
	graph := domain.Graph{Nodes: append([]domain.Node{{
		ID: "t", Type: domain.NodeTrigger, Config: fmt.Appendf(nil, `{"event":"card_entered_lane","lane_id":%q}`, lane),
	}}, nodes...)}
	for i := 1; i < len(graph.Nodes); i++ {
		prev := graph.Nodes[i-1]
		handle := domain.NodeOutputs(prev)[0]
		graph.Edges = append(graph.Edges, domain.Edge{ID: prev.ID + "." + handle, Source: prev.ID, SourceHandle: handle, Target: graph.Nodes[i].ID})
	}
	graph.Edges = append(graph.Edges, extra...)
	f, err := s.board.CreateFlow(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.board.SaveFlowDraft(ctx, f.ID, graph); err != nil {
		t.Fatal(err)
	}
	if _, err := s.board.PublishFlow(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.board.UpdateFlow(ctx, f.ID, domain.FlowInput{Name: name, Enabled: true}); err != nil {
		t.Fatal(err)
	}
}

// agent is the agent node "dev".
func (s *stack) agent(prompt string) domain.Node {
	return s.agentWith("dev", prompt, "")
}

// agentWith is an agent node with more config fields (`"k":v,…`).
func (s *stack) agentWith(id, prompt, more string) domain.Node {
	if more != "" {
		more = "," + more
	}
	return domain.Node{ID: id, Type: domain.NodeAgent, Config: fmt.Appendf(nil, `{"profile_id":%q,"prompt":%q%s}`, s.profile, prompt, more)}
}

func (s *stack) moveTo(lane uuid.UUID) domain.Node {
	return domain.Node{ID: "move", Type: domain.NodeMoveCard, Config: fmt.Appendf(nil, `{"lane_id":%q}`, lane)}
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
	s.flow(t, []domain.Node{s.agent("Implement it"), s.moveTo(s.done), {ID: "end", Type: domain.NodeEnd, Config: []byte(`{}`)}})
	card := s.startCard(t)
	// The card is created and moved by hand, then the flow starts, its agent
	// runs, it moves the card to Done and ends. The server and the worker
	// publish concurrently, so only the flow events keep their order.
	var seen []string
	for len(seen) == 0 || seen[len(seen)-1] != "event: flow_run.updated" {
		seen = append(seen, nextEventOf(t, s.stream, "card_event", "node_run", "node_output"))
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

// TestGatewayCycleEndToEnd: the developer agent asks a question and waits;
// once answered it resumes and finishes; the gateway rejects, the
// developer resumes with the feedback, and the second round approves.
func TestGatewayCycleEndToEnd(t *testing.T) {
	s := newStack(t)
	review := s.agentWith("rev", "Review it [gate:reject,approve]", `"gateway":true,"max_rounds":3,"read_only":true`)
	s.flow(t, []domain.Node{s.agent("Implement it [ask:Which database?] [done:implemented]"), review, s.moveTo(s.done)},
		domain.Edge{ID: "rev.rejected", Source: "rev", SourceHandle: "rejected", Target: "dev"})
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
	for typ, n := range map[string]int{"question.asked": 1, "question.answered": 1, "node.finished": 5} {
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
	want := []string{"dev#1 done done 6", "rev#1 done rejected 3", "dev#2 done done 3", "rev#2 done approved 3", "move#1 done next 0"}
	if !slices.Equal(runs, want) {
		t.Fatalf("node runs %q", runs)
	}
	// Each agent gets what the node before handed on: the answers, the
	// developer result, the gateway feedback.
	var prompts []string
	rows, err = s.pool.Query(context.Background(), `SELECT coalesce(payload->>'prompt', '') FROM jobs WHERE kind = 'agent.run' ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		prompts = append(prompts, p)
	}
	if len(prompts) != 5 || prompts[0] != "" || !strings.Contains(prompts[1], "Answer: Postgres") || prompts[2] != "did it" || prompts[3] != "add tests" {
		t.Fatalf("prompts %q", prompts)
	}
	s.stop(t)
}

// TestOpencodeEndToEnd: a developer on opencode and a gateway on Claude
// in the same flow: the board tools reach both through their own MCP
// config, and the rejected developer resumes its opencode session with the
// feedback.
func TestOpencodeEndToEnd(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	oc, err := s.board.LoginAgent(ctx, domain.AgentOpencode, "oc", `{"openrouter":{"type":"api","key":"k"}}`)
	if err != nil {
		t.Fatal(err)
	}
	dev := domain.Node{ID: "dev", Type: domain.NodeAgent, Config: fmt.Appendf(nil, `{"profile_id":%q,"prompt":"Implement it [done:implemented]","model":"openrouter/m"}`, oc.ID)}
	review := s.agentWith("rev", "Review it [gate:reject,approve]", `"gateway":true,"max_rounds":3`)
	s.flow(t, []domain.Node{dev, review, s.moveTo(s.done)},
		domain.Edge{ID: "rev.rejected", Source: "rev", SourceHandle: "rejected", Target: "dev"})
	card := s.startCard(t)

	s.waitRun(t, card, domain.RunDone)
	rows, err := s.pool.Query(ctx, `SELECT node_key, iteration, coalesce(output_handle, ''), turns, coalesce(summary, '') FROM node_runs ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	var runs []string
	for rows.Next() {
		var key, handle, summary string
		var iteration, turns int
		if err := rows.Scan(&key, &iteration, &handle, &turns, &summary); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, fmt.Sprintf("%s#%d %s %d %s", key, iteration, handle, turns, summary))
	}
	want := []string{"dev#1 done 2 implemented", "rev#1 rejected 3 ", "dev#2 done 2 implemented", "rev#2 approved 3 ", "move#1 next 0 "}
	if !slices.Equal(runs, want) {
		t.Fatalf("node runs %q; worker log: %s", runs, s.workerErr.String())
	}
	home := filepath.Join(os.Getenv("HOME"), ".local", "share", "orquestrator", "cards", card.String(), "claude")
	if imported, err := os.ReadFile(filepath.Join(home, "fake-auth.json")); err != nil || !strings.Contains(string(imported), "openrouter") {
		t.Fatalf("credentials not imported: %q, %v", imported, err)
	}
	sessions, _ := filepath.Glob(filepath.Join(home, "fake-ses_*.runs"))
	if runs, _ := os.ReadFile(sessions[0]); len(sessions) != 1 || string(runs) != "2" {
		t.Fatalf("the developer did not resume its session: %v %q", sessions, runs)
	}
	s.stop(t)
}

// TestChildCardsEndToEnd: a planner splits the card into two children,
// which run their own flow from their lane; the parent waits and moves on
// once both are done.
func TestChildCardsEndToEnd(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	kids, err := s.board.CreateLane(ctx, domain.LaneInput{Name: "Kids"})
	if err != nil {
		t.Fatal(err)
	}
	s.flowOn(t, "impl", kids.ID, []domain.Node{s.agent("Implement it [done:implemented]"), s.moveTo(s.done)})
	planner := s.agentWith("plan", "Split it [cards:API,Web] [done:split]", `"output_schema":`+domain.ChildCardsSchema)
	create := domain.Node{ID: "create", Type: domain.NodeCreateCards, Config: fmt.Appendf(nil, `{"lane_id":%q,"card_type_id":%q}`, kids.ID, s.typ)}
	wait := domain.Node{ID: "wait", Type: domain.NodeWaitChildren, Config: fmt.Appendf(nil, `{"done_lane_id":%q}`, s.done)}
	s.flow(t, []domain.Node{planner, create, wait, s.moveTo(s.done)})
	card := s.startCard(t)

	run := s.waitRun(t, card, domain.RunDone)
	if run.FlowName != "dev" {
		t.Fatalf("run %+v", run)
	}
	children, err := s.store.ListChildCards(ctx, card)
	if err != nil || len(children) != 2 || children[0].Title != "API" || children[1].Description != "Part Web" {
		t.Fatalf("children %+v, %v", children, err)
	}
	for _, c := range children {
		if c.LaneID != s.done {
			t.Fatalf("child %s is not done: %+v", c.Title, c)
		}
	}
	if got, _ := s.store.GetCard(ctx, card); got.LaneID != s.done {
		t.Fatalf("parent %+v", got)
	}
	var handle string
	if err := s.pool.QueryRow(ctx, `SELECT output_handle FROM node_runs WHERE node_key = 'wait'`).Scan(&handle); err != nil || handle != "all_done" {
		t.Fatalf("wait %q, %v", handle, err)
	}
	if c := countOf(s.timeline(t, card), "children.created"); c != 1 {
		t.Fatalf("timeline %q", s.timeline(t, card))
	}
	s.stop(t)
}

// TestArtifactRevisionEndToEnd: a planner writes plan.md, collected as the
// artifact "Plano"; a message in its review chat resumes the planner, which
// writes a new version, while the card stays in Done; moving the card on
// freezes the artifact.
func TestArtifactRevisionEndToEnd(t *testing.T) {
	s := newStack(t)
	planner := s.agentWith("plan", "Plan it [write:plan.md:# Plano]", `"artifacts":[{"name":"Plano","path":"plan.md"}]`)
	s.flow(t, []domain.Node{planner, s.moveTo(s.done)})
	card := s.startCard(t)
	s.waitRun(t, card, domain.RunDone)

	url := s.base + "/api/cards/" + card.String() + "/artifacts"
	artifacts := getJSON[[]map[string]any](t, url)
	if len(artifacts) != 1 || artifacts[0]["name"] != "Plano" || artifacts[0]["frozen"] != false {
		t.Fatalf("artifacts %v", artifacts)
	}
	versions := artifacts[0]["versions"].([]any)
	if len(versions) != 1 || versions[0].(map[string]any)["content"] != "# Plano\n\nrun 1\n" {
		t.Fatalf("versions %v", versions)
	}
	id := artifacts[0]["id"].(string)
	msg := postStatus(t, s.base+"/api/artifacts/"+id+"/messages", `{"text":"Citação sobre \"Plano\": detalhe mais","version":1}`, http.StatusCreated)
	if msg["role"] != "human" || msg["version"] != float64(1) {
		t.Fatalf("message %v", msg)
	}

	deadline := time.Now().Add(30 * time.Second)
	var artifact map[string]any
	for time.Now().Before(deadline) {
		artifact = getJSON[map[string]any](t, s.base+"/api/artifacts/"+id)
		if len(artifact["messages"].([]any)) == 2 && artifact["revising"] == false {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	messages := artifact["messages"].([]any)
	reply := messages[len(messages)-1].(map[string]any)
	versions = artifact["versions"].([]any)
	if reply["role"] != "agent" || reply["text"] != "did it" || reply["version"] != float64(2) || len(versions) != 2 ||
		versions[1].(map[string]any)["content"] != "# Plano\n\nrun 2\n" {
		t.Fatalf("after the revision: %v; worker log: %s", artifact, s.workerErr.String())
	}
	if got, _ := s.store.GetCard(context.Background(), card); got.LaneID != s.done {
		t.Fatal("the revision moved the card")
	}
	var sessions int
	if err := s.pool.QueryRow(context.Background(), `SELECT count(DISTINCT session_id) FROM node_runs WHERE node_key = 'plan'`).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("the revision did not resume the session: %d, %v", sessions, err)
	}
	var prompt string
	if err := s.pool.QueryRow(context.Background(), `SELECT payload->>'prompt' FROM jobs WHERE kind = 'agent.run' ORDER BY created_at DESC LIMIT 1`).Scan(&prompt); err != nil ||
		!strings.Contains(prompt, "detalhe mais") {
		t.Fatalf("revision prompt %q, %v", prompt, err)
	}

	postStatus(t, s.base+"/api/cards/"+card.String()+"/move", fmt.Sprintf(`{"lane_id":%q,"position":0}`, s.todo), http.StatusOK)
	if artifact = getJSON[map[string]any](t, s.base+"/api/artifacts/"+id); artifact["frozen"] != true {
		t.Fatalf("moving the card on did not freeze %v", artifact)
	}
	resp, err := http.Post(s.base+"/api/artifacts/"+id+"/messages", "application/json", strings.NewReader(`{"text":"x","version":2}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("revising a frozen artifact: %d", resp.StatusCode)
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

// TestCancelEndToEnd cancels a run waiting for answers and one whose agent
// is running: the cards are free again and the agent is interrupted.
func TestCancelEndToEnd(t *testing.T) {
	s := newStack(t)
	s.flow(t, []domain.Node{s.agent("Implement it [ask:Which database?] [sleep]")})
	waiting := s.startCard(t)
	run := s.waitRun(t, waiting, domain.RunWaiting)
	canceled := postStatus(t, s.base+"/api/flow-runs/"+run.ID.String()+"/cancel", "", http.StatusOK)
	if canceled["status"] != "canceled" {
		t.Fatalf("canceled %v", canceled)
	}
	if qs := getJSON[[]map[string]any](t, s.base+"/api/cards/"+waiting.String()+"/questions"); qs[0]["delivered"] != true {
		t.Fatalf("the question is still open: %v", qs)
	}
	postStatus(t, s.base+"/api/cards/"+waiting.String()+"/move", fmt.Sprintf(`{"lane_id":%q,"position":0}`, s.todo), http.StatusOK)

	// The answer resumes the agent, which now sleeps until interrupted.
	busy := s.startCard(t)
	run = s.waitRun(t, busy, domain.RunWaiting)
	qs := getJSON[[]map[string]any](t, s.base+"/api/cards/"+busy.String()+"/questions")
	postStatus(t, s.base+"/api/questions/"+qs[0]["id"].(string)+"/answer", `{"answer":"Postgres"}`, http.StatusOK)
	s.waitRun(t, busy, domain.RunRunning)
	deadline := time.Now().Add(10 * time.Second)
	for s.nodeStatus(t, run.ID) != domain.NodeRunning && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	start := time.Now()
	postStatus(t, s.base+"/api/flow-runs/"+run.ID.String()+"/cancel", "", http.StatusOK)
	for !strings.Contains(s.workerErr.String(), "job lease lost") && time.Since(start) < 20*time.Second {
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(s.workerErr.String(), "job lease lost") {
		t.Fatalf("the agent was not interrupted; worker log: %s", s.workerErr.String())
	}
	resp, err := http.Post(s.base+"/api/flow-runs/"+run.ID.String()+"/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("canceling an ended run: %d", resp.StatusCode)
	}
	var steps int
	if err := s.pool.QueryRow(context.Background(), `SELECT count(*) FROM node_run_output`).Scan(&steps); err != nil || steps == 0 {
		t.Fatalf("no agent output recorded: %d, %v", steps, err)
	}
	s.stop(t)
}

func (s *stack) nodeStatus(t *testing.T, flowRunID uuid.UUID) domain.NodeStatus {
	t.Helper()
	nodes, err := s.store.ListNodeRuns(context.Background(), flowRunID)
	if err != nil || len(nodes) == 0 {
		t.Fatalf("node runs %v, %v", nodes, err)
	}
	return nodes[len(nodes)-1].Status
}
