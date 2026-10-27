// Command fakeclaude stands in for the claude CLI in tests, and for the
// opencode CLI when called as `fakeclaude run|auth …`. It records how it
// was called in fake-claude.json or fake-opencode.json (in its working
// directory) and prints a scripted run, chosen by markers in the prompt:
//
//	[fail]  the result event reports an error (opencode: a free-limit error)
//	[crash] exits with status 2 before any result
//	[sleep] waits a minute before answering, when resumed (to be interrupted)
//
// As opencode, `auth import F` copies F to $HOME/fake-auth.json (failing
// when it holds "bad"), and `auth export` prints $FAKE_OPENCODE_AUTH.
//
// Markers in the first prompt of a session script calls to the board MCP
// tools, made on the runs of that session:
//
//	[ask:Q]          ask_human([Q]) on the first run
//	[done:S]         report_done(S) on every run without questions
//	[output:JSON]    submit_output(JSON) on every run without questions
//	[gate:a,b,…]     approve or reject on run i, by verdict i (the last
//	                 one repeats); rejections ask to "add tests"
//	[write:F:T]      writes T and the run number to the file F of the
//	                 working directory on every run without questions
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "run" || args[0] == "auth") {
		opencode(args)
		return
	}
	prompt, mcp, session := "", "", ""
	for i, a := range args {
		if a == "-p" && i+1 < len(args) {
			prompt = args[i+1]
		}
		if a == "--mcp-config" && i+1 < len(args) {
			data, _ := os.ReadFile(args[i+1])
			mcp = string(data)
		}
		if (a == "--session-id" || a == "--resume") && i+1 < len(args) {
			session = args[i+1]
		}
	}
	run, first, err := script(prompt, os.Getenv("CLAUDE_CONFIG_DIR"), session, func() (*sdk.ClientSession, error) { return connect(mcp) })
	if err != nil {
		fmt.Fprintln(os.Stderr, "board tools:", err)
		os.Exit(3)
	}
	cwd, _ := os.Getwd()
	record, _ := json.Marshal(map[string]any{
		"args":        args,
		"cwd":         cwd,
		"config_dir":  os.Getenv("CLAUDE_CONFIG_DIR"),
		"oauth_token": os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"),
		"api_key":     os.Getenv("ANTHROPIC_API_KEY"),
		"mcp_config":  mcp,
	})
	_ = os.WriteFile("fake-claude.json", record, 0o600)

	fmt.Println(`{"type":"system","subtype":"init","session_id":"s"}`)
	fmt.Println(`not json`)
	if strings.Contains(first, "[sleep]") && run > 1 {
		time.Sleep(time.Minute)
	}
	if strings.Contains(prompt, "[crash]") {
		fmt.Fprintln(os.Stderr, "boom")
		os.Exit(2)
	}
	isError := strings.Contains(prompt, "[fail]")
	subtype := "success"
	if isError {
		subtype = "error_during_execution"
	}
	result, _ := json.Marshal(map[string]any{
		"type": "result", "subtype": subtype, "is_error": isError, "result": "did it",
		"num_turns": 3, "duration_ms": 1200, "total_cost_usd": 0.5,
		"usage": map[string]int{
			"input_tokens": 10, "output_tokens": 20,
			"cache_read_input_tokens": 30, "cache_creation_input_tokens": 40,
		},
	})
	fmt.Println(string(result))
}

var marker = regexp.MustCompile(`\[(ask|done|output|gate|write):([^\]]*)\]`)

// opencode plays `opencode run` and `opencode auth`.
func opencode(args []string) {
	if args[0] == "auth" {
		file := args[len(args)-1]
		switch data, _ := os.ReadFile(file); {
		case args[1] == "export":
			fmt.Print(os.Getenv("FAKE_OPENCODE_AUTH"))
		case strings.Contains(string(data), "bad"):
			fmt.Fprintln(os.Stderr, "invalid credentials")
			os.Exit(1)
		default:
			_ = os.WriteFile(filepath.Join(os.Getenv("HOME"), "fake-auth.json"), data, 0o600)
		}
		return
	}
	prompt, session := args[len(args)-1], ""
	for i, a := range args {
		if a == "--session" && i+1 < len(args) {
			session = args[i+1]
		}
	}
	config := os.Getenv("OPENCODE_CONFIG_CONTENT")
	run, first, err := script(prompt, os.Getenv("HOME"), session, func() (*sdk.ClientSession, error) { return connectOpencode(config) })
	if err != nil {
		fmt.Fprintln(os.Stderr, "board tools:", err)
		os.Exit(3)
	}
	cwd, _ := os.Getwd()
	record, _ := json.Marshal(map[string]any{"args": args, "cwd": cwd, "home": os.Getenv("HOME"), "config": json.RawMessage(config)})
	_ = os.WriteFile("fake-opencode.json", record, 0o600)

	event := func(typ string, ts int, part map[string]any) {
		line, _ := json.Marshal(map[string]any{"type": typ, "timestamp": ts, "sessionID": session, "part": part})
		fmt.Println(string(line))
	}
	fmt.Println(`not json`)
	if strings.Contains(first, "[sleep]") && run > 1 {
		time.Sleep(time.Minute)
	}
	if strings.Contains(prompt, "[crash]") {
		fmt.Fprintln(os.Stderr, "boom")
		os.Exit(2)
	}
	event("step_start", 1000, map[string]any{"type": "step-start"})
	if strings.Contains(prompt, "[fail]") {
		fmt.Println(`{"type":"error","timestamp":1100,"error":{"type":"provider.free-limit","message":"Free usage limit reached.","retryAfterMs":3600000}}`)
		os.Exit(1)
	}
	event("tool_use", 1100, map[string]any{"type": "tool", "tool": "write", "state": map[string]any{
		"status": "completed", "input": map[string]string{"path": "a.txt"}, "output": "Created file",
	}})
	event("step_finish", 1200, map[string]any{"cost": 0.25, "tokens": map[string]any{
		"input": 10, "output": 15, "reasoning": 5, "cache": map[string]int{"read": 30, "write": 40},
	}})
	event("step_start", 1300, map[string]any{"type": "step-start"})
	event("text", 1400, map[string]any{"text": "did it"})
	event("step_finish", 2200, map[string]any{"cost": 0.25})
}

// script runs the board tool calls the first prompt of the session asks for
// and returns which run of the session this is, and that first prompt. The
// session state lives in dir.
func script(prompt, dir, session string, connect func() (*sdk.ClientSession, error)) (int, string, error) {
	if dir == "" || session == "" {
		return 1, prompt, nil
	}
	first := filepath.Join(dir, "fake-"+session+".prompt")
	if _, err := os.Stat(first); err != nil {
		_ = os.WriteFile(first, []byte(prompt), 0o600)
	}
	original, _ := os.ReadFile(first)
	counter := filepath.Join(dir, "fake-"+session+".runs")
	data, _ := os.ReadFile(counter)
	run, _ := strconv.Atoi(string(data))
	run++
	_ = os.WriteFile(counter, []byte(strconv.Itoa(run)), 0o600)

	calls := marker.FindAllStringSubmatch(string(original), -1)
	if len(calls) == 0 {
		return run, string(original), nil
	}
	cs, err := connect()
	if err != nil {
		return run, string(original), err
	}
	defer func() { _ = cs.Close() }()
	ctx := context.Background()
	call := func(name string, args map[string]any) error {
		res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err == nil && res.IsError {
			err = fmt.Errorf("%s: %v", name, res.Content)
		}
		return err
	}
	for _, c := range calls {
		if c[1] == "ask" && run == 1 {
			return run, string(original), call("ask_human", map[string]any{"questions": []string{c[2]}})
		}
	}
	for _, c := range calls {
		var err error
		switch c[1] {
		case "done":
			err = call("report_done", map[string]any{"summary": c[2]})
		case "output":
			err = call("submit_output", map[string]any{"output": json.RawMessage(c[2])})
		case "gate":
			verdicts := strings.Split(c[2], ",")
			if verdicts[min(run, len(verdicts))-1] == "approve" {
				err = call("approve", map[string]any{"reason": "looks good"})
			} else {
				err = call("reject", map[string]any{"feedback": "add tests"})
			}
		case "write":
			file, text, _ := strings.Cut(c[2], ":")
			err = os.WriteFile(file, fmt.Appendf(nil, "%s\n\nrun %d\n", text, run), 0o600)
		}
		if err != nil {
			return run, string(original), err
		}
	}
	return run, string(original), nil
}

// server is an MCP server entry of either CLI config.
type server struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// connect opens the board MCP server of the --mcp-config file.
func connect(mcpConfig string) (*sdk.ClientSession, error) {
	var cfg struct {
		McpServers map[string]server `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(mcpConfig), &cfg); err != nil {
		return nil, err
	}
	return open(cfg.McpServers["orquestrator"])
}

// connectOpencode opens the board MCP server of the opencode config.
func connectOpencode(config string) (*sdk.ClientSession, error) {
	var cfg struct {
		Mcp map[string]server `json:"mcp"`
	}
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		return nil, err
	}
	return open(cfg.Mcp["orquestrator"])
}

func open(board server) (*sdk.ClientSession, error) {
	client := sdk.NewClient(&sdk.Implementation{Name: "fakeclaude", Version: "0"}, nil)
	transport := &sdk.StreamableClientTransport{
		Endpoint:             board.URL,
		HTTPClient:           &http.Client{Transport: headers{board.Headers}},
		DisableStandaloneSSE: true,
	}
	return client.Connect(context.Background(), transport, nil)
}

// headers adds the configured headers to every request.
type headers struct{ values map[string]string }

func (h headers) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h.values {
		r.Header.Set(k, v)
	}
	return http.DefaultTransport.RoundTrip(r)
}
