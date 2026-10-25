package domain

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestAgentProfile(t *testing.T) {
	id := uuid.New()
	p := AgentProfile{ID: id}
	if p.EntityID() != id || p.Ready() {
		t.Fatalf("unexpected %+v", p)
	}
	p.CredentialKind = CredentialAPIKey
	if !p.Ready() {
		t.Fatal("profile with credential not ready")
	}
	in, err := AgentProfileInput{Name: " dev "}.Normalize()
	if err != nil || in.Name != "dev" {
		t.Fatalf("Normalize = %+v, %v", in, err)
	}
	if _, err := (AgentProfileInput{}).Normalize(); fieldOf(t, err) != "name" {
		t.Fatal("blank name accepted")
	}
}

func TestDetectCredential(t *testing.T) {
	tests := []struct {
		in   string
		kind CredentialKind
	}{
		{" sk-ant-oat01-abc\n", CredentialOAuthToken},
		{"sk-ant-api03-xyz", CredentialAPIKey},
	}
	for _, tt := range tests {
		secret, kind, err := DetectCredential(tt.in)
		if err != nil || kind != tt.kind || secret[0] != 's' {
			t.Errorf("DetectCredential(%q) = %q, %q, %v", tt.in, secret, kind, err)
		}
	}
	if _, _, err := DetectCredential("hunter2"); fieldOf(t, err) != "credential" {
		t.Fatal("unknown credential accepted")
	}
}

func TestMcpServerInput(t *testing.T) {
	stdio, err := McpServerInput{
		Name: " fs ", Transport: McpStdio,
		McpConfig: McpConfig{Command: " npx ", Args: []string{"server"}, URL: "x", Headers: map[string]string{"a": "b"}},
	}.Normalize()
	if err != nil || stdio.Name != "fs" || stdio.Command != "npx" || stdio.URL != "" || stdio.Headers != nil {
		t.Fatalf("stdio = %+v, %v", stdio, err)
	}
	http, err := McpServerInput{
		Name: "sg", Transport: McpHTTP,
		McpConfig: McpConfig{URL: " https://sg.example/mcp ", Command: "x", Args: []string{"y"}, Env: map[string]string{"a": "b"}},
	}.Normalize()
	want := McpConfig{URL: "https://sg.example/mcp"}
	if err != nil || !reflect.DeepEqual(http.McpConfig, want) {
		t.Fatalf("http = %+v, %v", http, err)
	}
	if (McpServer{ID: uuid.Nil}).EntityID() != uuid.Nil {
		t.Fatal("EntityID")
	}

	invalid := map[string]McpServerInput{
		"name":      {Transport: McpStdio, McpConfig: McpConfig{Command: "x"}},
		"command":   {Name: "a", Transport: McpStdio},
		"url":       {Name: "a", Transport: McpHTTP, McpConfig: McpConfig{URL: "ftp://x"}},
		"transport": {Name: "a", Transport: "ws"},
	}
	for field, in := range invalid {
		if _, err := in.Normalize(); fieldOf(t, err) != field {
			t.Errorf("%s not validated", field)
		}
	}
	for _, url := range []string{"https://", "%zz"} {
		if _, err := (McpServerInput{Name: "a", Transport: McpHTTP, McpConfig: McpConfig{URL: url}}).Normalize(); fieldOf(t, err) != "url" {
			t.Errorf("url %q accepted", url)
		}
	}
}
