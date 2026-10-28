package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// opencodeAgent is the opencode agent nodes run as: the built-in build
// agent, with the step limit of the node.
const opencodeAgent = "build"

// OpencodeSession is the opencode session id of a node session: opencode
// takes ids of its own shape, which `run --session` creates when missing.
func OpencodeSession(id uuid.UUID) string {
	return "ses_" + strings.ReplaceAll(id.String(), "-", "")
}

// OpencodeArgs builds the `opencode run` command line. --standalone keeps
// the run off the user's background service; --auto approves what the
// config does not deny, since nobody is there to answer.
func OpencodeArgs(inv Invocation) []string {
	args := []string{"run", "--standalone", "--format", "json", "--auto", "--agent", opencodeAgent, "--session", OpencodeSession(inv.SessionID)}
	if inv.Config.Model != "" {
		args = append(args, "--model", inv.Config.Model)
	}
	return append(args, "--", inv.Prompt)
}

// OpencodeImportArgs imports the profile credentials, from a file of the
// agent home, into the home of a card.
func OpencodeImportArgs(file string) []string {
	return []string{"auth", "import", "--standalone", file}
}

// OpencodeEnv points opencode at the card's agent home, where its sessions
// and credentials live, and passes the run config.
func OpencodeEnv(home, config string) []string {
	return []string{"HOME=" + home, "OPENCODE_CONFIG_CONTENT=" + config}
}

// opencodeMcp is a server in the mcp section of the opencode config.
type opencodeMcp struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// OpencodeConfig is the config of a run: the MCP servers, the board
// instructions (a file of the agent home), what the agent may not do and
// its step limit. Commits, pushes and web searches are always denied;
// read-only nodes cannot edit files.
func OpencodeConfig(inv Invocation, servers map[string]mcpEntry, instructions string) string {
	mcp := make(map[string]opencodeMcp, len(servers))
	for name, s := range servers {
		if s.Type == string(domain.McpStdio) {
			mcp[name] = opencodeMcp{Type: "local", Command: append([]string{s.Command}, s.Args...), Environment: s.Env}
		} else {
			mcp[name] = opencodeMcp{Type: "remote", URL: s.URL, Headers: s.Headers}
		}
	}
	edit := "allow"
	if inv.Config.ReadOnly {
		edit = "deny"
	}
	agent := map[string]any{}
	if inv.Config.MaxTurns != nil {
		agent["steps"] = *inv.Config.MaxTurns
	}
	data, _ := json.Marshal(map[string]any{ // plain values
		"$schema":      "https://opencode.ai/config.json",
		"autoupdate":   false,
		"share":        "disabled",
		"instructions": []string{instructions},
		"mcp":          mcp,
		"permission": map[string]any{
			"edit": edit,
			// web search needs an opencode account: standalone runs get it
			// cancelled, which fails the whole run
			"websearch": "deny",
			"bash":      map[string]string{"*": "allow", "git commit*": "deny", "git push*": "deny"},
		},
		"agent": map[string]any{opencodeAgent: agent},
	})
	return string(data)
}

// opencodeEvent is the part of an `opencode run --format json` event the
// executor reads.
type opencodeEvent struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	Part      struct {
		Text   string  `json:"text"`
		Tool   string  `json:"tool"`
		Cost   float64 `json:"cost"`
		Tokens struct {
			Input     int64 `json:"input"`
			Output    int64 `json:"output"`
			Reasoning int64 `json:"reasoning"`
			Cache     struct {
				Read  int64 `json:"read"`
				Write int64 `json:"write"`
			} `json:"cache"`
		} `json:"tokens"`
		State struct {
			Status string          `json:"status"`
			Input  json.RawMessage `json:"input"`
			Output string          `json:"output"`
			Error  string          `json:"error"`
		} `json:"state"`
	} `json:"part"`
	Error struct {
		Type         string `json:"type"`
		Message      string `json:"message"`
		RetryAfterMS int64  `json:"retryAfterMs"`
	} `json:"error"`
}

// OpencodeStream follows the JSON events of an opencode run. opencode has
// no result event: the result is the last text, usage adds up the steps.
type OpencodeStream struct {
	text        string
	steps       int
	usage       domain.Usage
	first, last int64
	err         string
	// toolErr is the last failed tool call: opencode may exit with an
	// error over one without saying so.
	toolErr string
}

// Line reads one event and returns the steps it shows on the card page.
func (s *OpencodeStream) Line(line []byte) []domain.NodeOutput {
	var e opencodeEvent
	if json.Unmarshal(line, &e) != nil || e.Type == "" {
		return nil
	}
	if s.first == 0 {
		s.first = e.Timestamp
	}
	s.last = e.Timestamp
	p := e.Part
	switch e.Type {
	case "step_start":
		s.steps++
	case "step_finish":
		s.usage.TokensIn += p.Tokens.Input
		s.usage.TokensOut += p.Tokens.Output + p.Tokens.Reasoning
		s.usage.CacheRead += p.Tokens.Cache.Read
		s.usage.CacheWrite += p.Tokens.Cache.Write
		s.usage.CostUSD += p.Cost
	case "text":
		s.text = p.Text
		return []domain.NodeOutput{{Kind: domain.OutputText, Text: shorten(p.Text, maxText)}}
	case "tool_use":
		result := p.State.Output
		if p.State.Status == "error" {
			result = p.State.Error
			s.toolErr = p.Tool + ": " + p.State.Error
		}
		return []domain.NodeOutput{
			{Kind: domain.OutputToolUse, Tool: p.Tool, Text: shorten(string(p.State.Input), maxToolInput)},
			{Kind: domain.OutputToolResult, Text: shorten(result, maxToolResult), IsError: p.State.Status == "error"},
		}
	case "error":
		s.err = opencodeError(e)
		return []domain.NodeOutput{{Kind: domain.OutputResult, Text: s.err, IsError: true}}
	}
	return nil
}

// opencodeError explains the errors a profile can fix.
func opencodeError(e opencodeEvent) string {
	msg := e.Error.Type + ": " + e.Error.Message
	switch e.Error.Type {
	case "provider.no-route":
		msg += " (the profile has no credentials for this model's provider: log it in again with `orquestrator agent login opencode`, or pick another model)"
	case "provider.free-limit":
		msg += fmt.Sprintf(" (try again in %s, or switch the node to a paid model)", (time.Duration(e.Error.RetryAfterMS) * time.Millisecond).Round(time.Minute))
	}
	return msg
}

// Outcome turns the events and the process error into the node result.
func (s *OpencodeStream) Outcome(runErr error) domain.NodeResult {
	if s.steps == 0 && s.err == "" {
		msg := "the agent ended without a result"
		if runErr != nil {
			msg += ": " + runErr.Error()
		}
		return domain.NodeResult{Status: domain.NodeFailed, OutputHandle: "failed", Error: msg}
	}
	out := domain.NodeResult{Status: domain.NodeDone, OutputHandle: "done", Usage: s.usage}
	out.Turns = s.steps
	out.DurationMS = s.last - s.first
	out.Output, _ = json.Marshal(map[string]string{"result": s.text}) // a string map
	switch {
	case s.err != "":
		out.Status, out.OutputHandle, out.Error = domain.NodeFailed, "failed", s.err
	case runErr != nil:
		out.Status, out.OutputHandle, out.Error = domain.NodeFailed, "failed", runErr.Error()
		if s.toolErr != "" {
			out.Error += " (last failed tool: " + s.toolErr + ")"
		}
	}
	return out
}
