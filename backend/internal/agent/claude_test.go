package agent

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func TestArgs(t *testing.T) {
	session := uuid.New()
	turns := 7
	args := Args(Invocation{
		Prompt:    "do it",
		SessionID: session,
		Config: domain.AgentConfig{
			AllowedTools:    []string{"Read", "Edit"},
			DisallowedTools: []string{"WebFetch"},
			MaxTurns:        &turns,
			Model:           "opus",
		},
		McpConfig: "/claude/mcp.json",
	})
	line := strings.Join(args, " ")
	for _, want := range []string{
		"-p do it --output-format stream-json --verbose --permission-mode acceptEdits",
		"--session-id " + session.String(),
		"--allowedTools Read,Edit",
		"--disallowedTools WebFetch,Bash(git commit:*),Bash(git push:*)",
		"--max-turns 7",
		"--model opus",
		"--mcp-config /claude/mcp.json --strict-mcp-config",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %s", want, line)
		}
	}
	if !slices.Contains(args, "--append-system-prompt") {
		t.Error("missing the system prompt")
	}

	minimal := strings.Join(Args(Invocation{Prompt: "go on", SessionID: session, Resume: true}), " ")
	if !strings.Contains(minimal, "--resume "+session.String()) {
		t.Errorf("resume: %s", minimal)
	}
	for _, absent := range []string{"--allowedTools", "--max-turns", "--model", "--mcp-config", "--session-id"} {
		if strings.Contains(minimal, absent) {
			t.Errorf("unexpected %s in %s", absent, minimal)
		}
	}
}

func TestEnv(t *testing.T) {
	if got := Env("/claude", domain.CredentialOAuthToken, "tok"); !slices.Equal(got, []string{"CLAUDE_CONFIG_DIR=/claude", "CLAUDE_CODE_OAUTH_TOKEN=tok"}) {
		t.Errorf("oauth: %q", got)
	}
	if got := Env("/claude", domain.CredentialAPIKey, "key"); got[1] != "ANTHROPIC_API_KEY=key" {
		t.Errorf("api key: %q", got)
	}
}

func TestOutcome(t *testing.T) {
	var none Stream
	if got := none.Outcome(nil); got.Status != domain.NodeFailed || got.Error != "the agent ended without a result" {
		t.Errorf("no result: %+v", got)
	}
	if got := none.Outcome(errors.New("exit 2")); got.Error != "the agent ended without a result: exit 2" {
		t.Errorf("no result with error: %+v", got)
	}

	var ok Stream
	ok.Line([]byte(`{"type":"system"}`))
	ok.Line([]byte(`garbage`))
	ok.Line([]byte(`{"type":"result","subtype":"success","result":"done","num_turns":2,"duration_ms":5,"total_cost_usd":0.1,"usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}}`))
	got := ok.Outcome(nil)
	want := domain.Usage{TokensIn: 1, TokensOut: 2, CacheRead: 3, CacheWrite: 4, CostUSD: 0.1, Turns: 2, DurationMS: 5}
	if got.Status != domain.NodeDone || got.OutputHandle != "done" || got.Usage != want || string(got.Output) != `{"result":"done"}` {
		t.Errorf("success: %+v", got)
	}
	if got := ok.Outcome(errors.New("killed")); got.Status != domain.NodeFailed || got.Error != "success: done" {
		t.Errorf("result but process failed: %+v", got)
	}

	var failed Stream
	failed.Line([]byte(`{"type":"result","subtype":"error_max_turns","is_error":true,"result":""}`))
	if got := failed.Outcome(nil); got.Status != domain.NodeFailed || got.Error != "error_max_turns:" {
		t.Errorf("error result: %+v", got)
	}
}
