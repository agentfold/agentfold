package mcpcheck

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"orquestrator/internal/domain"
)

// TestMain doubles as the stdio server of the tests.
func TestMain(m *testing.M) {
	if os.Getenv("MCPCHECK_SERVER") == "1" {
		if err := server().Run(context.Background(), &sdk.StdioTransport{}); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type echoArgs struct {
	Text string `json:"text"`
}

// server has a tool of each kind of result.
func server() *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "fake", Version: "1"}, nil)
	text := func(t string) *sdk.CallToolResult {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: t}}}
	}
	sdk.AddTool(s, &sdk.Tool{Name: "echo"}, func(_ context.Context, _ *sdk.CallToolRequest, in echoArgs) (*sdk.CallToolResult, any, error) {
		return text(in.Text), nil, nil
	})
	sdk.AddTool(s, &sdk.Tool{Name: "denied"}, func(context.Context, *sdk.CallToolRequest, any) (*sdk.CallToolResult, any, error) {
		r := text("401 Unauthorized")
		r.IsError = true
		return r, nil, nil
	})
	sdk.AddTool(s, &sdk.Tool{Name: "image"}, func(context.Context, *sdk.CallToolRequest, any) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "a"}, &sdk.ImageContent{MIMEType: "image/png", Data: []byte("x")}}}, nil, nil
	})
	sdk.AddTool(s, &sdk.Tool{Name: "long"}, func(context.Context, *sdk.CallToolRequest, any) (*sdk.CallToolResult, any, error) {
		return text("a" + strings.Repeat("é", 1500)), nil, nil
	})
	return s
}

// httpServer serves server, refusing requests without the token and
// failing tools/list when told to.
func httpServer(t *testing.T, failList bool) string {
	t.Helper()
	s := server()
	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token good" {
			http.Error(w, "Invalid Authorization header", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if failList && bytes.Contains(body, []byte(`"tools/list"`)) {
			http.Error(w, "broken", http.StatusInternalServerError)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func httpConfig(url, token string) domain.McpConfig {
	return domain.McpConfig{URL: url, Headers: map[string]string{"Authorization": "token " + token}}
}

func TestCheckHTTP(t *testing.T) {
	url := httpServer(t, false)
	ctx := context.Background()

	r := Check(ctx, domain.McpHTTP, httpConfig(url, "good"), "", nil)
	if !r.OK || r.Error != "" || !slices.Equal(r.Tools, []string{"denied", "echo", "image", "long"}) {
		t.Errorf("listing = %+v", r)
	}
	r = Check(ctx, domain.McpHTTP, httpConfig(url, "good"), "echo", map[string]any{"text": "hi"})
	if !r.OK || r.ToolResult != "hi" {
		t.Errorf("call = %+v", r)
	}
	r = Check(ctx, domain.McpHTTP, httpConfig(url, "bad"), "", nil)
	if r.OK || !strings.HasPrefix(r.Error, "não conectou: ") || r.Tools == nil {
		t.Errorf("bad token = %+v", r)
	}
	r = Check(ctx, domain.McpHTTP, httpConfig(httpServer(t, true), "good"), "", nil)
	if r.OK || !strings.HasPrefix(r.Error, "não listou as tools: ") {
		t.Errorf("failed listing = %+v", r)
	}
}

func TestCheckToolResults(t *testing.T) {
	cfg := httpConfig(httpServer(t, false), "good")
	ctx := context.Background()

	r := Check(ctx, domain.McpHTTP, cfg, "denied", nil)
	if r.OK || r.Error != "a tool denied devolveu um erro" || r.ToolResult != "401 Unauthorized" || len(r.Tools) != 4 {
		t.Errorf("tool error = %+v", r)
	}
	r = Check(ctx, domain.McpHTTP, cfg, "missing", nil)
	if r.OK || !strings.HasPrefix(r.Error, "a chamada de missing falhou: ") {
		t.Errorf("unknown tool = %+v", r)
	}
	r = Check(ctx, domain.McpHTTP, cfg, "image", nil)
	if !r.OK || !strings.HasPrefix(r.ToolResult, "a\n{") || !strings.Contains(r.ToolResult, "image/png") {
		t.Errorf("image = %+v", r)
	}
	r = Check(ctx, domain.McpHTTP, cfg, "long", nil)
	if want := "a" + strings.Repeat("é", 999) + "…"; r.ToolResult != want {
		t.Errorf("long result has %d bytes", len(r.ToolResult))
	}
}

func TestCheckStdio(t *testing.T) {
	ctx := context.Background()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r := Check(ctx, domain.McpStdio, domain.McpConfig{Command: self, Env: map[string]string{"MCPCHECK_SERVER": "1", "OTHER": "x"}}, "echo", map[string]any{"text": "ok"})
	if !r.OK || r.ToolResult != "ok" || len(r.Tools) != 4 {
		t.Errorf("stdio = %+v", r)
	}

	r = Check(ctx, domain.McpStdio, domain.McpConfig{Command: "sh", Args: []string{"-c", "echo token missing >&2"}}, "", nil)
	if r.OK || !strings.HasPrefix(r.Error, "não conectou: ") || !strings.HasSuffix(r.Error, "\nstderr: token missing") {
		t.Errorf("dying server = %+v", r)
	}
	r = Check(ctx, domain.McpStdio, domain.McpConfig{Command: "/nonexistent/mcp"}, "", nil)
	if r.OK || strings.Contains(r.Error, "stderr") {
		t.Errorf("missing command = %+v", r)
	}
}

func TestTailKeepsTheEnd(t *testing.T) {
	var tl tail
	_, _ = tl.Write([]byte(strings.Repeat("a", maxText)))
	_, _ = tl.Write([]byte("end"))
	if s := tl.String(); len(s) != maxText || !strings.HasSuffix(s, "aend") {
		t.Errorf("tail = %d bytes", len(s))
	}
}
