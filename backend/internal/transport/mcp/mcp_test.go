package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"orquestrator/internal/domain"
	"orquestrator/internal/service"
)

var errBoard = errors.New("board failed")

// fakeBoard records calls; errs fails them by name.
type fakeBoard struct {
	runs      map[string]service.AgentRun
	calls     []string
	args      map[string]any
	errs      map[string]error
	card      domain.Card
	latest    []domain.FlowRun
	artifacts []service.ArtifactDetail
}

func newBoard() *fakeBoard {
	return &fakeBoard{runs: map[string]service.AgentRun{}, args: map[string]any{}, errs: map[string]error{}}
}

func (b *fakeBoard) call(name string, arg any) error {
	b.calls = append(b.calls, name)
	b.args[name] = arg
	return b.errs[name]
}

func (b *fakeBoard) AgentRun(_ context.Context, token string) (service.AgentRun, error) {
	run, ok := b.runs[token]
	if !ok {
		return run, domain.ErrNotFound
	}
	return run, nil
}

func (b *fakeBoard) AskHuman(_ context.Context, _ service.AgentRun, texts []string) ([]domain.Question, error) {
	return nil, b.call("AskHuman", texts)
}

func (b *fakeBoard) ReportDone(_ context.Context, _ service.AgentRun, summary string) error {
	return b.call("ReportDone", summary)
}

func (b *fakeBoard) SubmitOutput(_ context.Context, _ service.AgentRun, output json.RawMessage) error {
	return b.call("SubmitOutput", string(output))
}

func (b *fakeBoard) Decide(_ context.Context, _ service.AgentRun, decision, feedback string) error {
	return b.call("Decide", decision+":"+feedback)
}

func (b *fakeBoard) ListLanes(context.Context) ([]domain.Lane, error) {
	return []domain.Lane{{ID: uuid.Nil, Name: "Todo"}}, b.call("ListLanes", nil)
}

func (b *fakeBoard) ListCardTypes(context.Context) ([]domain.CardType, error) {
	return []domain.CardType{{Name: "task"}}, b.call("ListCardTypes", nil)
}

func (b *fakeBoard) ListProjects(context.Context) ([]domain.Project, error) {
	return []domain.Project{{Name: "web", LocalPath: "/src/web"}}, b.call("ListProjects", nil)
}

func (b *fakeBoard) ListCards(context.Context) ([]domain.Card, error) {
	other := b.card
	other.LaneID = uuid.New()
	return []domain.Card{b.card, other}, b.call("ListCards", nil)
}

func (b *fakeBoard) GetCard(_ context.Context, id uuid.UUID) (domain.Card, error) {
	return b.card, b.call("GetCard", id)
}

func (b *fakeBoard) ListCardArtifacts(_ context.Context, id uuid.UUID) ([]service.ArtifactDetail, error) {
	return b.artifacts, b.call("ListCardArtifacts", id)
}

func (b *fakeBoard) CreateCard(_ context.Context, in domain.NewCard) (domain.Card, error) {
	return b.card, b.call("CreateCard", in)
}

func (b *fakeBoard) MoveCard(_ context.Context, _, _ uuid.UUID, position int) (domain.Card, error) {
	return b.card, b.call("MoveCard", position)
}

func (b *fakeBoard) ListFlows(context.Context) ([]domain.Flow, error) {
	return []domain.Flow{{Name: "dev", Enabled: true}}, b.call("ListFlows", nil)
}

func (b *fakeBoard) ListCardQuestions(_ context.Context, cardID uuid.UUID) ([]domain.Question, error) {
	answer := "yes"
	return []domain.Question{{Text: "Q?", Answer: &answer}}, b.call("ListCardQuestions", cardID)
}

func (b *fakeBoard) AnswerQuestion(_ context.Context, _ uuid.UUID, answer string) (domain.Question, error) {
	return domain.Question{Text: "Q?", Answer: &answer}, b.call("AnswerQuestion", answer)
}

func (b *fakeBoard) ListLatestFlowRuns(context.Context) ([]domain.FlowRun, error) {
	return b.latest, b.call("ListLatestFlowRuns", nil)
}

func (b *fakeBoard) GetRunDetail(_ context.Context, id uuid.UUID) (service.RunDetail, error) {
	return service.RunDetail{
		FlowRun: domain.FlowRun{ID: id, FlowName: "dev", Status: domain.RunWaiting},
		Nodes: []service.RunNode{{
			NodeRun: domain.NodeRun{NodeKey: "dev", Iteration: 1, Status: domain.NodeAwaitingInput, Usage: domain.Usage{Turns: 3}},
			Type:    domain.NodeAgent,
		}},
	}, b.call("GetRunDetail", id)
}

// session connects an MCP client to the handler, with token when not empty.
func session(t *testing.T, board *fakeBoard, token string) *sdk.ClientSession {
	t.Helper()
	srv := httptest.NewServer(NewHandler(board, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
	httpClient := srv.Client()
	if token != "" {
		httpClient = &http.Client{Transport: bearer{token}}
	}
	cs, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint: srv.URL, HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// call calls a tool and returns its text, failing on protocol errors.
func call(t *testing.T, cs *sdk.ClientSession, name string, args any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var text strings.Builder
	for _, c := range res.Content {
		text.WriteString(c.(*sdk.TextContent).Text)
	}
	return text.String(), res.IsError
}

func toolNames(t *testing.T, cs *sdk.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

func agentRun(typ domain.NodeType, cfg string) service.AgentRun {
	return service.AgentRun{
		NodeRun: domain.NodeRun{ID: uuid.New(), NodeKey: "dev"},
		FlowRun: domain.FlowRun{CardID: uuid.New()},
		Node:    domain.Node{ID: "dev", Type: typ, Config: json.RawMessage(cfg)},
	}
}

func TestAgentTools(t *testing.T) {
	board := newBoard()
	board.card = domain.Card{Title: "Login"}
	board.runs["tok"] = agentRun(domain.NodeAgent, `{}`)
	cs := session(t, board, "Bearer tok")
	if got := toolNames(t, cs); !slices.Equal(got, []string{"ask_human", "get_card", "report_done"}) {
		t.Fatalf("tools %v", got)
	}
	if out, isErr := call(t, cs, "get_card", map[string]any{}); isErr || !strings.Contains(out, `"title":"Login"`) || !strings.Contains(out, `"artifacts":[]`) || strings.Contains(out, `"parent"`) {
		t.Fatalf("get_card: %s", out)
	}
	frozen := time.Now()
	board.artifacts = []service.ArtifactDetail{{
		Artifact: domain.Artifact{Name: "Plano", FrozenAt: &frozen},
		Versions: []domain.ArtifactVersion{{Version: 1, Content: "old"}, {Version: 2, Path: "plano.md", Content: "# Plano"}},
	}}
	parent := uuid.New()
	board.card.ParentCardID = &parent
	out, isErr := call(t, cs, "get_card", map[string]any{})
	if isErr || !strings.Contains(out, `{"content":"# Plano","frozen":true,"name":"Plano","path":"plano.md","version":2}`) || !strings.Contains(out, `"parent":{`) {
		t.Fatalf("get_card with artifacts and parent: %s", out)
	}
	board.errs["ListCardArtifacts"] = errBoard
	if out, isErr := call(t, cs, "get_card", map[string]any{}); !isErr {
		t.Fatalf("get_card error: %s", out)
	}
	board.errs["ListCardArtifacts"] = nil
	board.errs["GetCard"] = errBoard
	if out, isErr := call(t, cs, "get_card", map[string]any{}); !isErr {
		t.Fatalf("get_card without the card: %s", out)
	}
	board.errs["GetCard"] = nil
	board.card.ParentCardID = nil
	if out, isErr := call(t, cs, "ask_human", map[string]any{"questions": []string{"DB?"}}); isErr || !strings.Contains(out, "End your turn") {
		t.Fatalf("ask_human: %s", out)
	}
	if out, isErr := call(t, cs, "report_done", map[string]any{"summary": "done"}); isErr || board.args["ReportDone"] != "done" {
		t.Fatalf("report_done: %s", out)
	}
	board.errs["AskHuman"], board.errs["ReportDone"] = errBoard, errBoard
	for name, args := range map[string]map[string]any{"ask_human": {"questions": []string{"x"}}, "report_done": {"summary": "x"}} {
		if out, isErr := call(t, cs, name, args); !isErr || !strings.Contains(out, "board failed") {
			t.Fatalf("%s error: %s", name, out)
		}
	}
}

func TestSubmitOutputTool(t *testing.T) {
	board := newBoard()
	board.runs["tok"] = agentRun(domain.NodeAgent, `{"output_schema":{"type":"object","properties":{"n":{"type":"integer"}}}}`)
	cs := session(t, board, "Bearer tok")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(res.Tools, func(tool *sdk.Tool) bool { return tool.Name == "submit_output" })
	if i < 0 {
		t.Fatal("no submit_output tool")
	}
	if schema, _ := json.Marshal(res.Tools[i].InputSchema); !strings.Contains(string(schema), `"n":{"type":"integer"}`) {
		t.Fatalf("input schema %s", schema)
	}
	if out, isErr := call(t, cs, "submit_output", map[string]any{"output": map[string]int{"n": 1}}); isErr || board.args["SubmitOutput"] != `{"n":1}` {
		t.Fatalf("submit_output: %s", out)
	}
	if out, isErr := call(t, cs, "submit_output", map[string]any{"other": 1}); !isErr || !strings.Contains(out, "output property") {
		t.Fatalf("missing output: %s", out)
	}
	board.errs["SubmitOutput"] = errBoard
	if out, isErr := call(t, cs, "submit_output", map[string]any{"output": 1}); !isErr || !strings.Contains(out, "board failed") {
		t.Fatalf("board error: %s", out)
	}
}

func TestGatewayTools(t *testing.T) {
	board := newBoard()
	board.runs["tok"] = agentRun(domain.NodeAgent, `{"gateway":true}`)
	cs := session(t, board, "Bearer tok")
	if got := toolNames(t, cs); !slices.Equal(got, []string{"approve", "ask_human", "get_card", "reject", "report_done"}) {
		t.Fatalf("tools %v", got)
	}
	if out, isErr := call(t, cs, "reject", map[string]any{"feedback": "add tests"}); isErr || board.args["Decide"] != "rejected:add tests" {
		t.Fatalf("reject: %s, %v", out, board.args["Decide"])
	}
	if out, isErr := call(t, cs, "approve", map[string]any{"reason": "fine"}); isErr || board.args["Decide"] != "approved:fine" {
		t.Fatalf("approve: %s, %v", out, board.args["Decide"])
	}
	board.errs["Decide"] = errBoard
	if _, isErr := call(t, cs, "reject", map[string]any{"feedback": "x"}); !isErr {
		t.Fatal("reject: expected a tool error")
	}
	if _, isErr := call(t, cs, "approve", map[string]any{}); !isErr {
		t.Fatal("approve: expected a tool error")
	}
}

func TestRefusedTokens(t *testing.T) {
	srv := httptest.NewServer(NewHandler(newBoard(), slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer srv.Close()
	for _, auth := range []string{"Bearer unknown", "Basic x"} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{}`))
		req.Header.Set("Authorization", auth)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s: got %d", auth, resp.StatusCode)
		}
	}
}

func TestIntegrationTools(t *testing.T) {
	board := newBoard()
	board.card = domain.Card{ID: uuid.New(), Title: "Login", LaneID: uuid.New()}
	cs := session(t, board, "")
	lane, id := board.card.LaneID.String(), uuid.New().String()
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"list_lanes", nil, `"name":"Todo"`},
		{"list_card_types", nil, `"name":"task"`},
		{"list_projects", nil, `"local_path":"/src/web"`},
		{"list_flows", nil, `"enabled":true`},
		{"list_cards", nil, `"title":"Login"`},
		{"create_card", map[string]any{"title": "New", "lane_id": id, "card_type_id": id}, `"title":"Login"`},
		{"move_card", map[string]any{"card_id": id, "lane_id": id}, `"title":"Login"`},
		{"list_questions", map[string]any{"card_id": id}, `"answered":true`},
		{"answer_questions", map[string]any{"answers": []map[string]any{{"question_id": id, "answer": "Postgres"}}}, `"answer":"Postgres"`},
	}
	for _, tt := range tests {
		if tt.args == nil {
			tt.args = map[string]any{}
		}
		if out, isErr := call(t, cs, tt.name, tt.args); isErr || !strings.Contains(out, tt.want) {
			t.Fatalf("%s: %s", tt.name, out)
		}
	}
	if board.args["MoveCard"] != 1<<31-1 {
		t.Fatalf("move to the end: %v", board.args["MoveCard"])
	}
	if out, _ := call(t, cs, "list_cards", map[string]any{"lane_id": lane}); strings.Count(out, `"title"`) != 1 {
		t.Fatalf("list_cards of a lane: %s", out)
	}
	if _, isErr := call(t, cs, "move_card", map[string]any{"card_id": id, "lane_id": id, "position": 2}); isErr || board.args["MoveCard"] != 2 {
		t.Fatalf("move to a position: %v", board.args["MoveCard"])
	}

	board.errs["AnswerQuestion"] = errBoard
	if _, isErr := call(t, cs, "answer_questions", map[string]any{"answers": []map[string]any{{"question_id": id, "answer": "x"}}}); !isErr {
		t.Fatal("answer_questions: expected a tool error")
	}
}

func TestGetRun(t *testing.T) {
	board := newBoard()
	cs := session(t, board, "")
	card := uuid.New()
	if out, isErr := call(t, cs, "get_run", map[string]any{"card_id": card.String()}); !isErr || !strings.Contains(out, "no flow ran") {
		t.Fatalf("no run: %s", out)
	}
	board.latest = []domain.FlowRun{{ID: uuid.New(), CardID: uuid.New()}, {ID: uuid.New(), CardID: card}}
	if out, isErr := call(t, cs, "get_run", map[string]any{"card_id": card.String()}); isErr || !strings.Contains(out, `"status":"awaiting_input"`) || !strings.Contains(out, `"turns":3`) {
		t.Fatalf("get_run: %s", out)
	}
	board.errs["ListLatestFlowRuns"] = errBoard
	if _, isErr := call(t, cs, "get_run", map[string]any{"card_id": card.String()}); !isErr {
		t.Fatal("expected a tool error")
	}
}

func TestMustSchemaPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for a type without schema")
		}
	}()
	mustSchema[chan int]()
}
