// Command fakeclaude stands in for the claude CLI in tests. It records how
// it was called in fake-claude.json (in its working directory) and prints a
// scripted stream-json run, chosen by markers in the prompt:
//
//	[fail]  the result event reports an error
//	[crash] exits with status 2 before any result
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]
	prompt := ""
	mcp := ""
	for i, a := range args {
		if a == "-p" && i+1 < len(args) {
			prompt = args[i+1]
		}
		if a == "--mcp-config" && i+1 < len(args) {
			data, _ := os.ReadFile(args[i+1])
			mcp = string(data)
		}
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
