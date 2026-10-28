package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
	"orquestrator/internal/service"
)

var (
	answerText = "Postgres"
	id1        = uuid.MustParse("00000000-0000-7000-8000-000000000001")
	id2        = uuid.MustParse("00000000-0000-7000-8000-000000000002")
	stamp      = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
)

func newStub() *stubBoard {
	return &stubBoard{
		lane:      domain.Lane{ID: id1, Name: "Todo", Color: "#000000", Position: 0, CreatedAt: stamp},
		project:   domain.Project{ID: id1, Name: "web", SourceKind: domain.SourceLocal, LocalPath: "/src/web", CreatedAt: stamp},
		cardType:  domain.CardType{ID: id1, Name: "bug", CreatedAt: stamp},
		card:      domain.Card{ID: id1, Title: "t", CardTypeID: id2, LaneID: id2, ParentCardID: &id2, CreatedAt: stamp, UpdatedAt: stamp},
		cardEvent: domain.CardEvent{ID: id1, CardID: id2, Type: domain.CardMoved, Payload: []byte(`{"position":1}`), CreatedAt: stamp},
		question: domain.Question{
			ID: id1, CardID: id2, NodeRunID: id2, Text: "DB?", Answer: &answerText, AnsweredAt: &stamp, DeliveredAt: &stamp, CreatedAt: stamp,
		},
		nodeRun: domain.NodeRun{
			ID: id1, FlowRunID: id2, NodeKey: "dev", Iteration: 2, Status: domain.NodeDone, OutputHandle: "done",
			Summary: "did it", Output: []byte(`{"result":"ok"}`), CreatedAt: stamp, StartedAt: &stamp, FinishedAt: &stamp,
			Usage: domain.Usage{TokensIn: 1, TokensOut: 2, CacheRead: 3, CacheWrite: 4, CostUSD: 0.5, Turns: 6, DurationMS: 7},
		},
		run: domain.FlowRun{
			ID: id1, FlowID: id2, FlowVersionID: id2, FlowName: "dev", CardID: id2, Trigger: domain.EventCardCreated,
			Status: domain.RunFailed, Error: "boom", StartedAt: stamp, FinishedAt: &stamp,
		},
		flow: service.FlowDetail{
			Flow:     domain.Flow{ID: id1, Name: "dev", LatestVersion: 2, Draft: stubGraph(), CreatedAt: stamp, UpdatedAt: stamp},
			Problems: []domain.GraphProblem{{NodeID: "t", Message: "on node"}, {EdgeID: "e", Message: "on edge"}},
		},
		version: domain.FlowVersion{ID: id2, FlowID: id1, Version: 2, Graph: stubGraph(), PublishedAt: stamp},
		profile: domain.AgentProfile{ID: id1, Kind: domain.AgentClaude, Name: "dev", CredentialKind: domain.CredentialOAuthToken, CreatedAt: stamp, UpdatedAt: stamp},
		artifact: service.ArtifactDetail{
			Artifact: domain.Artifact{ID: id1, CardID: id2, Name: "Plano", NodeRunID: id2, FrozenAt: &stamp, CreatedAt: stamp, UpdatedAt: stamp},
			Versions: []domain.ArtifactVersion{{ID: id1, ArtifactID: id1, Version: 1, Path: "plan.md", Content: "# Plano", NodeRunID: id2, CreatedAt: stamp}},
			Messages: []domain.ArtifactMessage{{ID: id1, ArtifactID: id1, Version: 1, Role: domain.RoleHuman, Text: "more", FlowRunID: &id2, CreatedAt: stamp}},
			Revising: true,
		},
		mcp: domain.McpServer{
			ID: id1, Name: "sg", Transport: domain.McpHTTP, McpConfig: domain.McpConfig{URL: "https://sg/mcp"},
			SecretNames: []string{"Authorization"}, CreatedAt: stamp, UpdatedAt: stamp,
		},
	}
}

// stubGraph is trigger -> end, as sent by graphJSON.
func stubGraph() domain.Graph {
	return domain.Graph{
		Nodes: []domain.Node{
			{ID: "t", Type: domain.NodeTrigger, Position: domain.Position{X: 1, Y: 2}, Config: []byte(`{"event":"card_created"}`)},
			{ID: "x", Type: domain.NodeEnd, Config: []byte(`{}`)},
		},
		Edges: []domain.Edge{{ID: "e", Source: "t", SourceHandle: "next", Target: "x"}},
	}
}

func newHandler(b Board, log *bytes.Buffer) http.Handler {
	return NewHandler(b, events.NewHub(1), Options{
		Log:       slog.New(slog.NewTextHandler(log, nil)),
		Heartbeat: time.Hour,
		MCP: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
	})
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// decode parses a JSON response into a generic value for comparisons.
func decode(t *testing.T, data []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	return v
}

const (
	laneJSON     = `{"id":"00000000-0000-7000-8000-000000000001","name":"Todo","color":"#000000","position":0,"created_at":"2026-10-25T12:00:00Z"}`
	projectJSON  = `{"id":"00000000-0000-7000-8000-000000000001","name":"web","source_kind":"local","local_path":"/src/web","created_at":"2026-10-25T12:00:00Z"}`
	cardTypeJSON = `{"id":"00000000-0000-7000-8000-000000000001","name":"bug","color":"","created_at":"2026-10-25T12:00:00Z"}`
	cardJSON     = `{"id":"00000000-0000-7000-8000-000000000001","title":"t","description":"",` +
		`"card_type_id":"00000000-0000-7000-8000-000000000002","project_id":null,` +
		`"parent_card_id":"00000000-0000-7000-8000-000000000002","lane_id":"00000000-0000-7000-8000-000000000002",` +
		`"position":0,"created_at":"2026-10-25T12:00:00Z","updated_at":"2026-10-25T12:00:00Z"}`
	graphJSON = `{"nodes":[{"id":"t","type":"trigger","position":{"x":1,"y":2},"config":{"event":"card_created"}},` +
		`{"id":"x","type":"end","position":{"x":0,"y":0},"config":{}}],` +
		`"edges":[{"id":"e","source":"t","source_handle":"next","target":"x"}]}`
	flowSummaryJSON = `{"id":"00000000-0000-7000-8000-000000000001","name":"dev","enabled":false,"latest_version":2,` +
		`"created_at":"2026-10-25T12:00:00Z","updated_at":"2026-10-25T12:00:00Z"}`
	flowJSON = `{"id":"00000000-0000-7000-8000-000000000001","name":"dev","enabled":false,"latest_version":2,` +
		`"created_at":"2026-10-25T12:00:00Z","updated_at":"2026-10-25T12:00:00Z","draft":` + graphJSON + `,` +
		`"problems":[{"node_id":"t","message":"on node"},{"edge_id":"e","message":"on edge"}]}`
	versionJSON = `{"id":"00000000-0000-7000-8000-000000000002","flow_id":"00000000-0000-7000-8000-000000000001",` +
		`"version":2,"graph":` + graphJSON + `,"published_at":"2026-10-25T12:00:00Z"}`
	profileJSON = `{"id":"00000000-0000-7000-8000-000000000001","kind":"claude","name":"dev","ready":true,` +
		`"credential_kind":"oauth_token","created_at":"2026-10-25T12:00:00Z","updated_at":"2026-10-25T12:00:00Z"}`
	mcpJSON = `{"id":"00000000-0000-7000-8000-000000000001","name":"sg","transport":"http","url":"https://sg/mcp",` +
		`"args":[],"env":{},"headers":{},"secret_names":["Authorization"],` +
		`"created_at":"2026-10-25T12:00:00Z","updated_at":"2026-10-25T12:00:00Z"}`
	questionJSON = `{"id":"00000000-0000-7000-8000-000000000001","card_id":"00000000-0000-7000-8000-000000000002",` +
		`"node_run_id":"00000000-0000-7000-8000-000000000002","text":"DB?","answer":"Postgres",` +
		`"answered_at":"2026-10-25T12:00:00Z","delivered":true,"created_at":"2026-10-25T12:00:00Z"}`
	nodeRunJSON = `{"id":"00000000-0000-7000-8000-000000000001","flow_run_id":"00000000-0000-7000-8000-000000000002",` +
		`"node_key":"dev","node_type":"agent","iteration":2,"status":"done","output_handle":"done","error":"","summary":"did it",` +
		`"decision":"","feedback":"",` +
		`"output":{"result":"ok"},"tokens_in":1,"tokens_out":2,"cache_read":3,"cache_write":4,"cost_usd":0.5,"turns":6,` +
		`"duration_ms":7,"created_at":"2026-10-25T12:00:00Z","started_at":"2026-10-25T12:00:00Z","finished_at":"2026-10-25T12:00:00Z"}`
	messageJSON = `{"id":"00000000-0000-7000-8000-000000000001","artifact_id":"00000000-0000-7000-8000-000000000001","version":1,` +
		`"role":"human","text":"more","flow_run_id":"00000000-0000-7000-8000-000000000002","created_at":"2026-10-25T12:00:00Z"}`
	artifactJSON = `{"id":"00000000-0000-7000-8000-000000000001","card_id":"00000000-0000-7000-8000-000000000002","name":"Plano",` +
		`"frozen":true,"revising":true,"versions":[{"version":1,"path":"plan.md","content":"# Plano",` +
		`"node_run_id":"00000000-0000-7000-8000-000000000002","created_at":"2026-10-25T12:00:00Z"}],"messages":[` + messageJSON + `],` +
		`"created_at":"2026-10-25T12:00:00Z","updated_at":"2026-10-25T12:00:00Z"}`
	outputJSON = `{"id":"00000000-0000-7000-8000-000000000001","node_run_id":"00000000-0000-7000-8000-000000000002",` +
		`"kind":"tool_use","text":"{}","tool":"Read","is_error":false,"created_at":"2026-10-25T12:00:00Z"}`
	runJSON = `{"id":"00000000-0000-7000-8000-000000000001","flow_id":"00000000-0000-7000-8000-000000000002",` +
		`"flow_version_id":"00000000-0000-7000-8000-000000000002","flow_name":"dev",` +
		`"card_id":"00000000-0000-7000-8000-000000000002","trigger":"card_created","status":"failed","error":"boom",` +
		`"started_at":"2026-10-25T12:00:00Z","finished_at":"2026-10-25T12:00:00Z"}`
	cardEventJSON = `{"id":"00000000-0000-7000-8000-000000000001","card_id":"00000000-0000-7000-8000-000000000002",` +
		`"type":"card.moved","payload":{"position":1},"created_at":"2026-10-25T12:00:00Z"}`
)

func TestRoutes(t *testing.T) {
	p1 := "/" + id1.String()
	tests := []struct {
		method, path, body string
		status             int
		call               string
		args               []any
		response           string
	}{
		{"GET", "/api/lanes", "", 200, "ListLanes", nil, "[" + laneJSON + "]"},
		{"POST", "/api/lanes", `{"name":"Todo","color":"#000000"}`, 201, "CreateLane",
			[]any{domain.LaneInput{Name: "Todo", Color: "#000000"}}, laneJSON},
		{"GET", "/api/lanes" + p1, "", 200, "GetLane", []any{id1}, laneJSON},
		{"PUT", "/api/lanes" + p1, `{"name":"Doing"}`, 200, "UpdateLane",
			[]any{id1, domain.LaneInput{Name: "Doing"}}, laneJSON},
		{"POST", "/api/lanes" + p1 + "/move", `{"position":3}`, 200, "MoveLane", []any{id1, 3}, laneJSON},
		{"DELETE", "/api/lanes" + p1, "", 204, "DeleteLane", []any{id1}, ""},

		{"GET", "/api/projects", "", 200, "ListProjects", nil, "[" + projectJSON + "]"},
		{"POST", "/api/projects", `{"name":"web","local_path":"/src/web"}`, 201, "CreateProject",
			[]any{domain.ProjectInput{Name: "web", LocalPath: "/src/web"}}, projectJSON},
		{"GET", "/api/projects" + p1, "", 200, "GetProject", []any{id1}, projectJSON},
		{"PUT", "/api/projects" + p1, `{"name":"api","local_path":"/src/api"}`, 200, "UpdateProject",
			[]any{id1, domain.ProjectInput{Name: "api", LocalPath: "/src/api"}}, projectJSON},
		{"DELETE", "/api/projects" + p1, "", 204, "DeleteProject", []any{id1}, ""},

		{"GET", "/api/card-types", "", 200, "ListCardTypes", nil, "[" + cardTypeJSON + "]"},
		{"POST", "/api/card-types", `{"name":"bug"}`, 201, "CreateCardType",
			[]any{domain.CardTypeInput{Name: "bug"}}, cardTypeJSON},
		{"GET", "/api/card-types" + p1, "", 200, "GetCardType", []any{id1}, cardTypeJSON},
		{"PUT", "/api/card-types" + p1, `{"name":"bug","color":"#ffffff"}`, 200, "UpdateCardType",
			[]any{id1, domain.CardTypeInput{Name: "bug", Color: "#ffffff"}}, cardTypeJSON},
		{"DELETE", "/api/card-types" + p1, "", 204, "DeleteCardType", []any{id1}, ""},

		{"GET", "/api/cards", "", 200, "ListCards", nil, "[" + cardJSON + "]"},
		{"POST", "/api/cards",
			fmt.Sprintf(`{"title":"t","description":"d","card_type_id":%q,"project_id":%q,"lane_id":%q,"parent_card_id":%q}`, id2, id1, id2, id1),
			201, "CreateCard",
			[]any{domain.NewCard{
				CardInput:    domain.CardInput{Title: "t", Description: "d", CardTypeID: id2, ProjectID: &id1},
				LaneID:       id2,
				ParentCardID: &id1,
			}}, cardJSON},
		{"GET", "/api/cards" + p1, "", 200, "GetCard", []any{id1}, cardJSON},
		{"PUT", "/api/cards" + p1, fmt.Sprintf(`{"title":"t","card_type_id":%q}`, id2), 200, "UpdateCard",
			[]any{id1, domain.CardInput{Title: "t", CardTypeID: id2}}, cardJSON},
		{"POST", "/api/cards" + p1 + "/move", fmt.Sprintf(`{"lane_id":%q,"position":2}`, id2), 200, "MoveCard",
			[]any{id1, id2, 2}, cardJSON},
		{"DELETE", "/api/cards" + p1, "", 204, "DeleteCard", []any{id1}, ""},
		{"GET", "/api/cards" + p1 + "/children", "", 200, "ListChildCards", []any{id1}, "[" + cardJSON + "]"},
		{"GET", "/api/cards" + p1 + "/events", "", 200, "ListCardEvents", []any{id1}, "[" + cardEventJSON + "]"},
		{"GET", "/api/flow-runs/latest", "", 200, "ListLatestFlowRuns", nil, "[" + runJSON + "]"},
		{"GET", "/api/cards" + p1 + "/runs", "", 200, "ListCardRuns", []any{id1}, "[" + runJSON[:len(runJSON)-1] + `,"nodes":[` + nodeRunJSON + "]}]"},
		{"POST", "/api/flow-runs" + p1 + "/cancel", "", 200, "CancelFlowRun", []any{id1}, runJSON},
		{"GET", "/api/node-runs" + p1 + "/output", "", 200, "ListNodeOutput", []any{id1}, "[" + outputJSON + "]"},
		{"GET", "/api/cards" + p1 + "/questions", "", 200, "ListCardQuestions", []any{id1}, "[" + questionJSON + "]"},
		{"POST", "/api/questions" + p1 + "/answer", `{"answer":"Postgres"}`, 200, "AnswerQuestion", []any{id1, "Postgres"}, questionJSON},
		{"GET", "/api/cards" + p1 + "/artifacts", "", 200, "ListCardArtifacts", []any{id1}, "[" + artifactJSON + "]"},
		{"GET", "/api/artifacts" + p1, "", 200, "GetArtifact", []any{id1}, artifactJSON},
		{"POST", "/api/artifacts" + p1 + "/messages", `{"text":"more","version":1}`, 201, "ReviseArtifact", []any{id1, 1, "more"}, messageJSON},

		{"GET", "/api/flows", "", 200, "ListFlows", nil, "[" + flowSummaryJSON + "]"},
		{"POST", "/api/flows", `{"name":"dev"}`, 201, "CreateFlow", []any{"dev"}, flowJSON},
		{"GET", "/api/flows" + p1, "", 200, "GetFlow", []any{id1}, flowJSON},
		{"PUT", "/api/flows" + p1, `{"name":"dev","enabled":true}`, 200, "UpdateFlow",
			[]any{id1, domain.FlowInput{Name: "dev", Enabled: true}}, flowJSON},
		{"DELETE", "/api/flows" + p1, "", 204, "DeleteFlow", []any{id1}, ""},
		{"PUT", "/api/flows" + p1 + "/draft", graphJSON, 200, "SaveFlowDraft", []any{id1, stubGraph()}, flowJSON},
		{"POST", "/api/flows" + p1 + "/publish", "", 201, "PublishFlow", []any{id1}, versionJSON},
		{"GET", "/api/flows" + p1 + "/versions", "", 200, "ListFlowVersions", []any{id1}, "[" + versionJSON + "]"},

		{"GET", "/api/agent-profiles", "", 200, "ListAgentProfiles", nil, "[" + profileJSON + "]"},
		{"POST", "/api/agent-profiles", `{"name":"dev"}`, 201, "CreateAgentProfile", []any{domain.AgentProfileInput{Name: "dev"}}, profileJSON},
		{"POST", "/api/agent-profiles", `{"name":"oc","kind":"opencode"}`, 201, "CreateAgentProfile", []any{domain.AgentProfileInput{Kind: domain.AgentOpencode, Name: "oc"}}, profileJSON},
		{"GET", "/api/agent-profiles" + p1, "", 200, "GetAgentProfile", []any{id1}, profileJSON},
		{"PUT", "/api/agent-profiles" + p1, `{"name":"rev"}`, 200, "UpdateAgentProfile", []any{id1, domain.AgentProfileInput{Name: "rev"}}, profileJSON},
		{"DELETE", "/api/agent-profiles" + p1, "", 204, "DeleteAgentProfile", []any{id1}, ""},

		{"GET", "/api/mcp-servers", "", 200, "ListMcpServers", nil, "[" + mcpJSON + "]"},
		{"POST", "/api/mcp-servers", `{"name":"sg","transport":"http","url":"https://sg/mcp","headers":{"X":"1"},"secrets":{"Authorization":"t"}}`,
			201, "CreateMcpServer", []any{domain.McpServerInput{
				Name: "sg", Transport: domain.McpHTTP,
				McpConfig: domain.McpConfig{URL: "https://sg/mcp", Headers: map[string]string{"X": "1"}},
				Secrets:   map[string]string{"Authorization": "t"},
			}}, mcpJSON},
		{"GET", "/api/mcp-servers" + p1, "", 200, "GetMcpServer", []any{id1}, mcpJSON},
		{"PUT", "/api/mcp-servers" + p1, `{"name":"fs","transport":"stdio","command":"srv","args":["-v"],"env":{"A":"1"}}`,
			200, "UpdateMcpServer", []any{id1, domain.McpServerInput{
				Name: "fs", Transport: domain.McpStdio,
				McpConfig: domain.McpConfig{Command: "srv", Args: []string{"-v"}, Env: map[string]string{"A": "1"}},
			}}, mcpJSON},
		{"DELETE", "/api/mcp-servers" + p1, "", 204, "DeleteMcpServer", []any{id1}, ""},
		{"POST", "/api/mcp-servers/test", `{"server":{"name":"fs","transport":"stdio","command":"srv"}}`,
			200, "TestMcpServer", []any{domain.McpServerTest{
				Server: domain.McpServerInput{Name: "fs", Transport: domain.McpStdio, McpConfig: domain.McpConfig{Command: "srv"}},
			}}, `{"ok":false,"tools":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			b := newStub()
			rec := do(t, newHandler(b, &bytes.Buffer{}), tt.method, tt.path, tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tt.status, rec.Body)
			}
			if b.call != tt.call || !reflect.DeepEqual(b.args, tt.args) {
				t.Fatalf("called %s%+v, want %s%+v", b.call, b.args, tt.call, tt.args)
			}
			if tt.response == "" {
				if rec.Body.Len() != 0 {
					t.Fatalf("unexpected body %s", rec.Body)
				}
				return
			}
			if got, want := decode(t, rec.Body.Bytes()), decode(t, []byte(tt.response)); !reflect.DeepEqual(got, want) {
				t.Fatalf("body %s\nwant %s", rec.Body, tt.response)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"validation", &domain.ValidationError{Field: "name", Message: "is required"}, 422,
			`{"code":"validation","message":"name: is required","field":"name"}`},
		{"not found", domain.ErrNotFound, 404, `{"code":"not_found","message":"not found"}`},
		{"conflict", fmt.Errorf("%w: lanes_name_key", domain.ErrConflict), 409,
			`{"code":"conflict","message":"conflict: lanes_name_key"}`},
		{"in use", domain.ErrInUse, 409, `{"code":"in_use","message":"in use"}`},
		{"card busy", domain.ErrCardBusy, 409, `{"code":"card_busy","message":"a flow is running for the card"}`},
		{"question closed", domain.ErrQuestionClosed, 409, `{"code":"question_closed","message":"the answer was already delivered to the agent"}`},
		{"artifact frozen", domain.ErrArtifactFrozen, 409, `{"code":"artifact_frozen","message":"the artifact is frozen: the card left its lane"}`},
		{"invalid reference", domain.ErrInvalidReference, 422, `{"code":"invalid_reference","message":"invalid reference"}`},
		{"invalid graph", &domain.GraphError{Problems: []domain.GraphProblem{{Message: "no trigger"}}}, 422,
			`{"code":"invalid_graph","message":"the draft has problems","problems":[{"message":"no trigger"}]}`},
		{"internal", errors.New("db down"), 500, `{"code":"internal","message":"internal error"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newStub()
			b.err = tt.err
			var log bytes.Buffer
			rec := do(t, newHandler(b, &log), "GET", "/api/lanes", "")
			if rec.Code != tt.status {
				t.Fatalf("status %d, want %d", rec.Code, tt.status)
			}
			if got, want := decode(t, rec.Body.Bytes()), decode(t, []byte(tt.body)); !reflect.DeepEqual(got, want) {
				t.Fatalf("body %s, want %s", rec.Body, tt.body)
			}
			if logged := strings.Contains(log.String(), "db down"); logged != (tt.status == 500) {
				t.Fatalf("log = %q", log.String())
			}
		})
	}
}

func TestBadRequests(t *testing.T) {
	tests := map[string][3]string{
		"malformed body": {"POST", "/api/lanes", `{"name":`},
		"malformed id":   {"GET", "/api/lanes/not-a-uuid", ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			b := newStub()
			rec := do(t, newHandler(b, &bytes.Buffer{}), tt[0], tt[1], tt[2])
			if rec.Code != http.StatusBadRequest || b.call != "" {
				t.Fatalf("status %d, call %q", rec.Code, b.call)
			}
			var body struct{ Code string }
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != "bad_request" {
				t.Fatalf("body %s", rec.Body)
			}
		})
	}
}

func TestMalformedGraphNode(t *testing.T) {
	b := newStub()
	rec := do(t, newHandler(b, &bytes.Buffer{}), "PUT", "/api/flows/"+id1.String()+"/draft", `{"nodes":[5],"edges":[]}`)
	if rec.Code != http.StatusUnprocessableEntity || b.call != "" {
		t.Fatalf("status %d, call %q, body %s", rec.Code, b.call, rec.Body)
	}
	var body struct{ Code, Field string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != "validation" || body.Field != "nodes" {
		t.Fatalf("body %s", rec.Body)
	}
}

func TestConversionsOfEmptyValues(t *testing.T) {
	profile := toAgentProfile(domain.AgentProfile{Name: "new"})
	if profile.Ready || profile.CredentialKind != nil {
		t.Errorf("profile %+v", profile)
	}
	stdio := toMcpServer(domain.McpServer{Transport: domain.McpStdio, McpConfig: domain.McpConfig{Command: "srv", Env: map[string]string{"A": "1"}}})
	if *stdio.Command != "srv" || stdio.Url != nil || stdio.Args == nil || stdio.Env["A"] != "1" || stdio.Headers == nil || stdio.SecretNames == nil {
		t.Errorf("mcp %+v", stdio)
	}
}

func TestToEntity(t *testing.T) {
	b := newStub()
	tests := []struct {
		in   any
		want string
	}{
		{b.lane, laneJSON},
		{b.project, projectJSON},
		{b.cardType, cardTypeJSON},
		{b.card, cardJSON},
		{b.flow.Flow, flowSummaryJSON},
		{b.profile, profileJSON},
		{b.mcp, mcpJSON},
		{b.cardEvent, cardEventJSON},
		{b.run, runJSON},
		{b.question, questionJSON},
		{b.nodeRun, strings.Replace(nodeRunJSON, `"node_type":"agent"`, `"node_type":""`, 1)},
		{nil, "null"},
	}
	for _, tt := range tests {
		data, err := json.Marshal(toEntity(tt.in))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := decode(t, data), decode(t, []byte(tt.want)); !reflect.DeepEqual(got, want) {
			t.Errorf("toEntity(%T) = %s, want %s", tt.in, data, tt.want)
		}
	}
}

func TestMCPMount(t *testing.T) {
	var log bytes.Buffer
	if rec := do(t, newHandler(newStub(), &log), "POST", "/mcp", "{}"); rec.Code != http.StatusTeapot {
		t.Fatalf("got %d", rec.Code)
	}
	without := NewHandler(newStub(), events.NewHub(1), Options{Log: slog.New(slog.NewTextHandler(&log, nil))})
	if rec := do(t, without, "POST", "/mcp", "{}"); rec.Code != http.StatusNotFound {
		t.Fatalf("without MCP: got %d", rec.Code)
	}
}

func TestTestMcpServerResult(t *testing.T) {
	b := newStub()
	b.mcpTest = domain.McpTestResult{Tools: []string{"denied"}, Error: "a tool denied devolveu um erro", ToolResult: "401"}
	body := `{"server":{"name":"sg","transport":"http","url":"https://sg/mcp"},"server_id":"` + id1.String() + `","tool":"denied","arguments":{"n":1}}`
	rec := do(t, newHandler(b, &bytes.Buffer{}), "POST", "/api/mcp-servers/test", body)
	want := domain.McpServerTest{
		Server:   domain.McpServerInput{Name: "sg", Transport: domain.McpHTTP, McpConfig: domain.McpConfig{URL: "https://sg/mcp"}},
		ServerID: &id1, Tool: "denied", Arguments: map[string]any{"n": float64(1)},
	}
	if rec.Code != 200 || !reflect.DeepEqual(b.args, []any{want}) {
		t.Fatalf("status %d, args %+v", rec.Code, b.args)
	}
	if got, want := decode(t, rec.Body.Bytes()), decode(t, []byte(`{"ok":false,"tools":["denied"],"error":"a tool denied devolveu um erro","tool_result":"401"}`)); !reflect.DeepEqual(got, want) {
		t.Fatalf("body %s", rec.Body)
	}
}
