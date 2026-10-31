package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHTTPOutputs(t *testing.T) {
	wait := node("h", NodeHTTPRequest, `{"mode":"wait"}`)
	if got := NodeOutputs(wait); !reflect.DeepEqual(got, []string{"success", "failed"}) {
		t.Errorf("wait: %v", got)
	}
	fire := node("h", NodeHTTPRequest, `{"mode":"fire_and_forget"}`)
	if got := NodeOutputs(fire); !reflect.DeepEqual(got, HTTPOutputs) {
		t.Errorf("fire and forget: %v", got)
	}
	if got := NodeOutputs(node("h", NodeHTTPRequest, `[]`)); len(got) != 2 {
		t.Errorf("bad config: %v", got)
	}
}

func TestHTTPRequestConfig(t *testing.T) {
	var c HTTPRequestConfig
	if c.Timeout() != DefaultHTTPTimeout || c.Attempts() != 1 || c.RetryDelay(1) != DefaultRetryDelay {
		t.Errorf("defaults: %s, %d, %s", c.Timeout(), c.Attempts(), c.RetryDelay(1))
	}
	if !c.Succeeded(204) || c.Succeeded(301) || c.Succeeded(199) {
		t.Error("the default success is 2xx")
	}
	c = HTTPRequestConfig{TimeoutSeconds: 5, MaxAttempts: 4, RetryDelaySeconds: 10, SuccessStatuses: []int{404}}
	if c.Timeout() != 5*time.Second || c.Attempts() != 4 || c.RetryDelay(3) != 10*time.Second {
		t.Errorf("set: %s, %d, %s", c.Timeout(), c.Attempts(), c.RetryDelay(3))
	}
	if !c.Succeeded(404) || c.Succeeded(200) {
		t.Error("success statuses replace 2xx")
	}
	c.Backoff = BackoffExponential
	if c.RetryDelay(1) != 10*time.Second || c.RetryDelay(3) != 40*time.Second || c.RetryDelay(20) != MaxRetryDelay {
		t.Errorf("exponential: %s, %s, %s", c.RetryDelay(1), c.RetryDelay(3), c.RetryDelay(20))
	}
	c.Mode = HTTPModeFireAndForget
	if !c.FireAndForget() || c.Attempts() != 1 {
		t.Error("firing and forgetting tries once")
	}
	if !Retryable(429) || !Retryable(503) || Retryable(404) {
		t.Error("retryable: 429 and 5xx only")
	}
}

func TestSealSecrets(t *testing.T) {
	g := Graph{Nodes: []Node{
		node("h", NodeHTTPRequest, `{"method":"GET","headers":[{"name":"A","value":"plain","sealed":"AAEC"},`+
			`{"name":"B","value":"s3cret","secret":true},{"name":"C","secret":true,"sealed":"AAEC"}]}`),
		node("bad", NodeHTTPRequest, `[]`),
		node("a", NodeAgent, `{"prompt":"p"}`),
	}}
	seal := func(b []byte) ([]byte, error) { return append([]byte("sealed:"), b...), nil }
	got, err := g.SealSecrets(seal)
	if err != nil {
		t.Fatal(err)
	}
	var c HTTPRequestConfig
	if err := json.Unmarshal(got.Nodes[0].Config, &c); err != nil {
		t.Fatal(err)
	}
	want := []HTTPHeader{
		{Name: "A", Value: "plain"},
		{Name: "B", Secret: true, Sealed: []byte("sealed:s3cret")},
		{Name: "C", Secret: true, Sealed: []byte{0, 1, 2}},
	}
	if !reflect.DeepEqual(c.Headers, want) {
		t.Errorf("headers %+v", c.Headers)
	}
	if string(got.Nodes[1].Config) != `[]` || string(got.Nodes[2].Config) != `{"prompt":"p"}` {
		t.Errorf("other nodes changed: %s, %s", got.Nodes[1].Config, got.Nodes[2].Config)
	}
	if string(g.Nodes[0].Config) == string(got.Nodes[0].Config) {
		t.Error("the graph sealed in place")
	}

	boom := errors.New("boom")
	if _, err := g.SealSecrets(func([]byte) ([]byte, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Errorf("seal failure: %v", err)
	}
}

func TestRenderTemplate(t *testing.T) {
	project := uuid.New()
	card := Card{
		ID: uuid.New(), Title: `Say "hi"`, Description: "a\nb", LaneID: laneID, CardTypeID: typeID,
		ProjectID: &project, Properties: map[string]string{"dev": "done"},
	}
	plain := func(s string, _ bool) string { return s }
	structured := json.RawMessage(`{"result":"text","output":{"plan":{"steps":["a","b"]},"n":2,"name":"x","none":null}}`)
	tests := []struct {
		text  string
		input json.RawMessage
		want  string
	}{
		{"{{card.id}} {{ card.title }} {{card.description}}", nil, card.ID.String() + ` Say "hi" a` + "\nb"},
		{"{{card.lane_id}}/{{card.card_type_id}}/{{card.project_id}}/{{card.parent_card_id}}", nil, laneID.String() + "/" + typeID.String() + "/" + project.String() + "/"},
		{"{{card.properties.dev}}{{card.properties.missing}}{{card.nope}}{{other}}", nil, "done{{card.nope}}{{other}}"},
		{"{{nodes.dev}} {{nodes.dev.n}} [{{nodes.gone}}{{nodes.gone.x}}]", nil, "text 2 []"},
		{"{{input}}", json.RawMessage(`{"result":"text"}`), "text"},
		{"{{input.plan}} {{input.plan.steps.1}} {{input.n}} {{input.name}}", structured, `{"steps":["a","b"]} b 2 x`},
		{"[{{input.none}}{{input.plan.steps.9}}{{input.plan.steps.x}}{{input.n.deep}}]", structured, "[]"},
		{"{{input.result}}", json.RawMessage(`{"result":"text"}`), "text"},
		{"[{{input.a}}]", json.RawMessage(`not json`), "[]"},
	}
	for _, tt := range tests {
		data := TemplateData{Card: card, Input: tt.input, Nodes: map[string]json.RawMessage{"dev": json.RawMessage(`{"result":"text","output":null,"n":2}`)}}
		if got := RenderTemplate(tt.text, data, plain); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.text, got, tt.want)
		}
	}

	body := RenderTemplate(`{"title":"{{card.title}}","plan":{{input.plan}}}`, TemplateData{Card: card, Input: structured}, EscapeJSON)
	if !json.Valid([]byte(body)) || body != `{"title":"Say \"hi\"","plan":{"steps":["a","b"]}}` {
		t.Errorf("body %s", body)
	}
	if got := RenderTemplate("/q?t={{card.title}}", TemplateData{Card: card}, EscapeURL); got != "/q?t=Say%20%22hi%22" {
		t.Errorf("url %s", got)
	}
	if got := RenderTemplate("{{card.description}}", TemplateData{Card: card}, EscapeHeader); got != "a b" {
		t.Errorf("header %q", got)
	}
	if !HasPlaceholders("a {{card.id}}") || HasPlaceholders("a {b}") || EscapeNone("a\n", true) != "a\n" {
		t.Error("HasPlaceholders or EscapeNone")
	}
	if got := NodeReferences("{{nodes.a.b}} {{nodes.}} {{ nodes.c }} {{nodes.a}}"); !reflect.DeepEqual(got, []string{"a", "c", "a"}) {
		t.Errorf("references %q", got)
	}
}

func TestValidHeaderName(t *testing.T) {
	for name, want := range map[string]bool{"X-Token": true, "": false, "a b": false, "a:b": false, "ç": false} {
		if validHeaderName(name) != want {
			t.Errorf("%q: want %v", name, want)
		}
	}
}
