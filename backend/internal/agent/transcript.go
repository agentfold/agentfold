package agent

import (
	"encoding/json"
	"strings"

	"orquestrator/internal/domain"
)

// Limits of the text kept for each step shown on the card page.
const (
	maxText       = 8000
	maxToolInput  = 500
	maxToolResult = 1000
)

// streamEvent is the part of a stream-json event the card page shows.
type streamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type    string          `json:"type"`
			Text    string          `json:"text"`
			Name    string          `json:"name"`
			Input   json.RawMessage `json:"input"`
			Content json.RawMessage `json:"content"`
			IsError bool            `json:"is_error"`
		} `json:"content"`
	} `json:"message"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// Steps turns a stream-json line into the steps shown on the card page:
// assistant messages, tool calls and results, and the final result.
func Steps(line []byte) []domain.NodeOutput {
	var e streamEvent
	if json.Unmarshal(line, &e) != nil {
		return nil
	}
	var out []domain.NodeOutput
	switch e.Type {
	case "assistant":
		for _, c := range e.Message.Content {
			switch c.Type {
			case "text":
				out = append(out, domain.NodeOutput{Kind: domain.OutputText, Text: shorten(c.Text, maxText)})
			case "tool_use":
				out = append(out, domain.NodeOutput{Kind: domain.OutputToolUse, Tool: c.Name, Text: shorten(string(c.Input), maxToolInput)})
			}
		}
	case "user":
		for _, c := range e.Message.Content {
			if c.Type == "tool_result" {
				out = append(out, domain.NodeOutput{Kind: domain.OutputToolResult, Text: shorten(resultText(c.Content), maxToolResult), IsError: c.IsError})
			}
		}
	case "result":
		out = append(out, domain.NodeOutput{Kind: domain.OutputResult, Text: shorten(e.Result, maxText), IsError: e.IsError})
	}
	return out
}

// resultText is the text of a tool result: a string, or text blocks.
func resultText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(content, &blocks) // anything else shows as empty
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// shorten cuts s to at most n runes, marking the cut.
func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
