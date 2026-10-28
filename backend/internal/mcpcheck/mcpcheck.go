// Package mcpcheck tries an MCP server out the way agents use it: it
// connects, lists the tools and, when asked, calls one.
package mcpcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"orquestrator/internal/domain"
)

// Timeout bounds a whole check.
const Timeout = 15 * time.Second

// maxText bounds the tool result and the stderr of a stdio server.
const maxText = 2000

// Check connects to a server, secrets already in cfg, lists its tools and
// calls tool when set. Failing to reach or use the server is a result that
// is not OK. stdio commands run here, outside the sandbox agents run in.
func Check(ctx context.Context, transport domain.McpTransport, cfg domain.McpConfig, tool string, args map[string]any) domain.McpTestResult {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	stderr := &tail{}
	client := sdk.NewClient(&sdk.Implementation{Name: "orquestrator", Version: "check"}, nil)
	session, err := client.Connect(ctx, connector(ctx, transport, cfg, stderr), nil)
	if err != nil {
		return failure("não conectou: "+err.Error(), stderr)
	}
	defer session.Close() //nolint:errcheck // the check is over
	out := domain.McpTestResult{Tools: []string{}}
	for t, err := range session.Tools(ctx, nil) {
		if err != nil {
			return failure("não listou as tools: "+err.Error(), stderr)
		}
		out.Tools = append(out.Tools, t.Name)
	}
	if tool == "" {
		out.OK = true
		return out
	}
	res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		out.Error = fmt.Sprintf("a chamada de %s falhou: %s", tool, err)
		return out
	}
	out.ToolResult = shorten(contentText(res.Content))
	if res.IsError {
		out.Error = fmt.Sprintf("a tool %s devolveu um erro", tool)
		return out
	}
	out.OK = true
	return out
}

// connector reaches the server: the command, with the env vars added to
// ours, or the URL, with the headers on every request.
func connector(ctx context.Context, transport domain.McpTransport, cfg domain.McpConfig, stderr *tail) sdk.Transport {
	if transport == domain.McpStdio {
		cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...) //nolint:gosec // the command the user registered
		cmd.Env = os.Environ()
		for _, k := range slices.Sorted(maps.Keys(cfg.Env)) {
			cmd.Env = append(cmd.Env, k+"="+cfg.Env[k])
		}
		cmd.Stderr = stderr
		return &sdk.CommandTransport{Command: cmd, TerminateDuration: time.Second}
	}
	return &sdk.StreamableClientTransport{
		Endpoint:             cfg.URL,
		HTTPClient:           &http.Client{Transport: headers(cfg.Headers)},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}
}

// headers adds fixed headers to every request.
type headers map[string]string

func (h headers) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h {
		r.Header.Set(k, v)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// failure is a check that did not get to the tools; what a stdio server
// wrote to stderr usually says why.
func failure(msg string, stderr *tail) domain.McpTestResult {
	if s := strings.TrimSpace(stderr.String()); s != "" {
		msg += "\nstderr: " + s
	}
	return domain.McpTestResult{Tools: []string{}, Error: msg}
}

// contentText is the text of a tool result; other content shows as JSON.
func contentText(content []sdk.Content) string {
	parts := make([]string, 0, len(content))
	for _, c := range content {
		if t, ok := c.(*sdk.TextContent); ok {
			parts = append(parts, t.Text)
			continue
		}
		data, _ := json.Marshal(c) // content the SDK decoded
		parts = append(parts, string(data))
	}
	return strings.Join(parts, "\n")
}

func shorten(s string) string {
	if len(s) <= maxText {
		return s
	}
	cut := maxText
	for !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// tail keeps the last bytes written to it, safe for the command's copier.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > maxText {
		t.buf = t.buf[len(t.buf)-maxText:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
