package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateOutput(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","required":["n"],"properties":{"n":{"type":"integer"}}}`)
	if err := ValidateOutput(schema, json.RawMessage(`{"n": 1}`)); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		schema, output string
		want           string
	}{
		"mismatch":        {string(schema), `{"n": "x"}`, "integer"},
		"not json":        {string(schema), `{`, "not JSON"},
		"bad schema":      {`{"type": 1}`, `{}`, "schema is invalid"},
		"schema not json": {`{`, `{}`, "schema is invalid"},
	} {
		if err := ValidateOutput(json.RawMessage(tc.schema), json.RawMessage(tc.output)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestPrompts(t *testing.T) {
	answer := "Postgres"
	got := AnswersPrompt([]Question{{Text: "DB?", Answer: &answer}, {Text: "Port?"}})
	if want := "Answers to your questions:\n\n1. DB?\nAnswer: Postgres\n\n2. Port?\nAnswer: \n"; got != want {
		t.Errorf("AnswersPrompt = %q", got)
	}
	if got := ReviewPrompt(true, nil); got != "The review approved the changes." {
		t.Errorf("approved: %q", got)
	}
	line := 3
	got = ReviewPrompt(false, []ReviewComment{{File: "a.go", Line: &line, Body: "tests"}, {File: "b.go", Body: "name"}, {Body: "docs"}})
	if want := "The review asked for changes. Address every comment:\n\n- a.go:3: tests\n- b.go: name\n- docs"; got != want {
		t.Errorf("ReviewPrompt = %q", got)
	}
	if (Question{ID: [16]byte{1}}).EntityID() != [16]byte{1} {
		t.Error("EntityID")
	}
}
