// Package agent runs agent nodes: it prepares the card workspace, starts
// the agent CLI of the profile (Claude Code or opencode) through a runner
// and records the outcome.
package agent

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// alwaysDisallowed keeps agents from committing or pushing: the board does
// that (the PR node, later).
var alwaysDisallowed = []string{"Bash(git commit:*)", "Bash(git push:*)"}

// Invocation is one run of `claude -p`.
type Invocation struct {
	Prompt string
	// SessionID is the session to start, or to resume when Resume is set.
	SessionID uuid.UUID
	Resume    bool
	Config    domain.AgentConfig
	// SystemPrompt is appended to Claude's own.
	SystemPrompt string
	// McpConfig is the path, as the process sees it, of the --mcp-config
	// file, which declares McpServers; their tools are allowed.
	McpConfig  string
	McpServers []string
}

// Args builds the command line. The prompt comes right after -p: flags with
// several values (--allowedTools…) would take it otherwise.
func Args(inv Invocation) []string {
	args := []string{"-p", inv.Prompt, "--output-format", "stream-json", "--verbose", "--permission-mode", "acceptEdits"}
	if inv.Resume {
		args = append(args, "--resume", inv.SessionID.String())
	} else {
		args = append(args, "--session-id", inv.SessionID.String())
	}
	args = append(args, "--append-system-prompt", inv.SystemPrompt)
	c := inv.Config
	allowed := append([]string{}, c.AllowedTools...)
	for _, name := range inv.McpServers {
		allowed = append(allowed, "mcp__"+name)
	}
	if len(allowed) > 0 {
		args = append(args, "--allowedTools", strings.Join(allowed, ","))
	}
	args = append(args, "--disallowedTools", strings.Join(append(append([]string{}, c.DisallowedTools...), alwaysDisallowed...), ","))
	if c.MaxTurns != nil {
		args = append(args, "--max-turns", strconv.Itoa(*c.MaxTurns))
	}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if inv.McpConfig != "" {
		args = append(args, "--mcp-config", inv.McpConfig, "--strict-mcp-config")
	}
	return args
}

// Env authenticates the CLI and points it at the card's config directory,
// where its sessions live.
func Env(claudeDir string, kind domain.CredentialKind, secret string) []string {
	name := envOAuthToken
	if kind == domain.CredentialAPIKey {
		name = envAPIKey
	}
	return []string{"CLAUDE_CONFIG_DIR=" + claudeDir, name + "=" + secret}
}

// Environment variables the CLI reads its credential from.
const (
	envOAuthToken = "CLAUDE_CODE_OAUTH_TOKEN" //nolint:gosec // a variable name, not a credential
	envAPIKey     = "ANTHROPIC_API_KEY"       //nolint:gosec // a variable name, not a credential
)

// resultEvent is the last event of a stream-json run.
type resultEvent struct {
	Type         string  `json:"type"`
	Subtype      string  `json:"subtype"`
	IsError      bool    `json:"is_error"`
	Result       string  `json:"result"`
	NumTurns     int     `json:"num_turns"`
	DurationMS   int64   `json:"duration_ms"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Usage        struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

// Stream follows the stream-json output of a run.
type Stream struct {
	result *resultEvent
}

// Line reads one line of output and returns the steps it shows on the
// card page; the result event settles the outcome.
func (s *Stream) Line(line []byte) []domain.NodeOutput {
	var e resultEvent
	if json.Unmarshal(line, &e) == nil && e.Type == "result" {
		s.result = &e
	}
	return Steps(line)
}

// Outcome turns the stream and the process error into the node result.
func (s *Stream) Outcome(runErr error) domain.NodeResult {
	if s.result == nil {
		msg := "the agent ended without a result"
		if runErr != nil {
			msg += ": " + runErr.Error()
		}
		return domain.NodeResult{Status: domain.NodeFailed, OutputHandle: "failed", Error: msg}
	}
	r := s.result
	out := domain.NodeResult{
		Status:       domain.NodeDone,
		OutputHandle: "done",
		Usage: domain.Usage{
			TokensIn:   r.Usage.InputTokens,
			TokensOut:  r.Usage.OutputTokens,
			CacheRead:  r.Usage.CacheReadInputTokens,
			CacheWrite: r.Usage.CacheCreationInputTokens,
			CostUSD:    r.TotalCostUSD,
			Turns:      r.NumTurns,
			DurationMS: r.DurationMS,
		},
	}
	out.Output, _ = json.Marshal(map[string]string{"result": r.Result}) // a string map
	if r.IsError || runErr != nil {
		out.Status, out.OutputHandle = domain.NodeFailed, "failed"
		out.Error = strings.TrimSpace(r.Subtype + ": " + r.Result)
	}
	return out
}
