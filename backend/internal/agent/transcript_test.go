package agent

import (
	"strings"
	"testing"

	"orquestrator/internal/domain"
)

func TestSteps(t *testing.T) {
	long := strings.Repeat("é", maxToolInput+10)
	tests := []struct {
		line string
		want []domain.NodeOutput
	}{
		{`not json`, nil},
		{`{"type":"system"}`, nil},
		{`{"type":"assistant","message":{"content":[{"type":"text","text":"Looking"},{"type":"thinking"},` +
			`{"type":"tool_use","name":"Read","input":{"file":"a.go"}}]}}`, []domain.NodeOutput{
			{Kind: domain.OutputText, Text: "Looking"},
			{Kind: domain.OutputToolUse, Tool: "Read", Text: `{"file":"a.go"}`},
		}},
		{`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":"` + long + `"}]}}`, []domain.NodeOutput{
			{Kind: domain.OutputToolUse, Tool: "Bash", Text: `"` + string([]rune(long)[:maxToolInput-1]) + "…"},
		}},
		{`{"type":"user","message":{"content":[{"type":"tool_result","content":"ok","is_error":true},{"type":"text","text":"x"}]}}`,
			[]domain.NodeOutput{{Kind: domain.OutputToolResult, Text: "ok", IsError: true}}},
		{`{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"a"},{"type":"image"},{"type":"text","text":"b"}]}]}}`,
			[]domain.NodeOutput{{Kind: domain.OutputToolResult, Text: "a\nb"}}},
		{`{"type":"user","message":{"content":[{"type":"tool_result","content":7}]}}`,
			[]domain.NodeOutput{{Kind: domain.OutputToolResult}}},
		{`{"type":"result","result":"done","is_error":true}`, []domain.NodeOutput{{Kind: domain.OutputResult, Text: "done", IsError: true}}},
	}
	for _, tt := range tests {
		got := Steps([]byte(tt.line))
		if len(got) != len(tt.want) {
			t.Fatalf("%s: got %+v", tt.line, got)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("%s: step %d = %+v, want %+v", tt.line, i, got[i], tt.want[i])
			}
		}
	}
}
