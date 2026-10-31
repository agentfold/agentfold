package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func exprData() TemplateData {
	return TemplateData{
		Card:  Card{Title: "Login", Properties: map[string]string{"size": "3"}},
		Input: json.RawMessage(`{"result":"done"}`),
		Nodes: map[string]json.RawMessage{
			"a":     json.RawMessage(`{"output":{"status":200,"body":{"total":10,"items":[1,2,3]}}}`),
			"b-1":   json.RawMessage(`{"output":{"status":200,"body":{"total":5}}}`),
			"plain": json.RawMessage(`[1,2]`),
			"bad":   json.RawMessage(`nope`),
		},
	}
}

func TestEvaluate(t *testing.T) {
	data := exprData()
	for src, want := range map[string]any{
		`nodes.a.body.total + nodes["b-1"].body.total`: 15.0,
		`sum(nodes.a.body.items) * 2`:                  12.0,
		`card.title + "/" + card.properties.size`:      "Login/3",
		`input`:                                 "done",
		`nodes.plain[1]`:                        2.0,
		`nodes.bad == nil && nodes.gone == nil`: true,
		`filter(nodes.a.body.items, # > 1)`:     []any{2.0, 3.0},
	} {
		got, err := Evaluate(src, data)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, %v; want %#v", src, got, err, want)
		}
	}
	if _, err := Evaluate(`nodes.gone.body.total + 1`, data); err == nil {
		t.Error("a missing field did not fail")
	}
	if _, err := Evaluate(`unknown + 1`, data); err == nil || strings.Contains(err.Error(), "\n") {
		t.Errorf("unknown name: %v", err)
	}
	if err := CheckExpression(`card.title +`); err == nil {
		t.Error("bad syntax accepted")
	}
}

func TestExpressionReferences(t *testing.T) {
	got := ExpressionReferences(`nodes.a.body + nodes["b-1"].x + card.id + other.nodes + len(nodes)`)
	if !reflect.DeepEqual(got, []string{"a", "b-1"}) {
		t.Errorf("refs %q", got)
	}
	if ExpressionReferences(`(`) != nil {
		t.Error("refs of bad syntax")
	}
}

func TestTransformApply(t *testing.T) {
	cfg := TransformConfig{Output: json.RawMessage(`{
		"total": "= nodes.a.body.total + nodes[\"b-1\"].body.total",
		"cliente": "{{card.title}}",
		"list": ["=len(nodes.a.body.items)", 7, null, {"ok": "=true"}]
	}`)}
	out, err := cfg.Apply(exprData())
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"output":{"cliente":"Login","list":[3,7,null,{"ok":true}],"total":15}}` {
		t.Errorf("output %s", out)
	}
	for config, want := range map[string]string{
		`{"x":{"y":"=nodes.gone.v + 1"}}`: "a expressão em x.y: ",
		`["ok", "=1 +"]`:                  "a expressão em 1: ",
		`"=nodes.gone.v + 1"`:             "a expressão: ",
		`[`:                               "configuração inválida",
	} {
		if _, err := (TransformConfig{Output: json.RawMessage(config)}).Apply(exprData()); err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%s: %v", config, err)
		}
	}
	if _, err := (TransformConfig{Output: json.RawMessage(`"=1/0"`)}).Apply(exprData()); err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Errorf("an infinite result: %v", err)
	}
}
