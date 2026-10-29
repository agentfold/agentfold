package domain

import (
	"encoding/json"
	"reflect"
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
	got = ReviewPrompt("Plano", "plan.md", "more detail")
	if want := "A human reviewed the artifact \"Plano\" (the file plan.md) and asks:\n\nmore detail\n\n" +
		"Update plan.md to address it, then finish; the board collects the file again."; got != want {
		t.Errorf("ReviewPrompt = %q", got)
	}
	if (Question{ID: [16]byte{1}}).EntityID() != [16]byte{1} || (Artifact{ID: [16]byte{2}}).EntityID() != [16]byte{2} {
		t.Error("EntityID")
	}
}

func TestMessageFrom(t *testing.T) {
	for output, want := range map[string]string{
		`{"result":"did it"}`:                      "did it",
		`{"result":"did it","output":{"a":[1,2]}}`: "{\n  \"a\": [\n    1,\n    2\n  ]\n}",
		`{"result":"did it","output":null}`:        "did it",
		`{}`:                                       "",
		`null`:                                     "",
		`[1]`:                                      "",
	} {
		if got := MessageFrom(json.RawMessage(output)); got != want {
			t.Errorf("MessageFrom(%s) = %q, want %q", output, got, want)
		}
	}
	if got := MessageFrom(nil); got != "" {
		t.Errorf("MessageFrom(nil) = %q", got)
	}
}

func TestNodeOutputs(t *testing.T) {
	if got := NodeOutputs(node("a", NodeAgent, `{"gateway":true}`)); !reflect.DeepEqual(got, GatewayOutputs) {
		t.Errorf("gateway outputs %q", got)
	}
	if got := NodeOutputs(node("a", NodeAgent, `[]`)); !reflect.DeepEqual(got, []string{"done", "failed"}) {
		t.Errorf("agent outputs %q", got)
	}
	if got := NodeOutputs(node("m", NodeMoveCard, `{}`)); !reflect.DeepEqual(got, []string{"next"}) {
		t.Errorf("move outputs %q", got)
	}
}
