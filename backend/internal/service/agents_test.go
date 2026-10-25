package service

import (
	"errors"
	"reflect"
	"testing"

	"orquestrator/internal/domain"
)

func TestAgentProfiles(t *testing.T) {
	s, repo, pub := newService()
	if _, err := s.ListAgentProfiles(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAgentProfile(ctx, id); err != nil || repo.ins["GetAgentProfile"] != id {
		t.Fatalf("GetAgentProfile: %v", err)
	}
	if _, err := s.CreateAgentProfile(ctx, domain.AgentProfileInput{Name: " dev "}); err != nil || repo.ins["CreateAgentProfile"] != "claude dev" {
		t.Fatalf("CreateAgentProfile: %v, %v", err, repo.ins)
	}
	if _, err := s.UpdateAgentProfile(ctx, id, domain.AgentProfileInput{Name: "rev"}); err != nil || repo.ins["RenameAgentProfile"] != "rev" {
		t.Fatalf("UpdateAgentProfile: %v", err)
	}
	if err := s.DeleteAgentProfile(ctx, id); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(eventTypes(pub), []string{"agent_profile.created", "agent_profile.updated", "agent_profile.deleted"}) {
		t.Fatalf("events %v", eventTypes(pub))
	}

	repo.calls = nil
	if _, err := s.CreateAgentProfile(ctx, domain.AgentProfileInput{}); err == nil {
		t.Fatal("blank name accepted")
	}
	if _, err := s.UpdateAgentProfile(ctx, id, domain.AgentProfileInput{}); err == nil || len(repo.calls) != 0 {
		t.Fatalf("blank rename: %v, calls %v", err, repo.calls)
	}
}

func TestLoginAgent(t *testing.T) {
	t.Run("existing profile", func(t *testing.T) {
		s, repo, pub := newService()
		if _, err := s.LoginAgent(ctx, "dev", " sk-ant-oat01-x "); err != nil {
			t.Fatal(err)
		}
		if repo.ins["GetAgentProfileByName"] != "dev" || repo.ins["SetAgentCredential"] != "oauth_token sealed:sk-ant-oat01-x" {
			t.Fatalf("ins %v", repo.ins)
		}
		if !reflect.DeepEqual(eventTypes(pub), []string{"agent_profile.updated"}) {
			t.Fatalf("events %v", eventTypes(pub))
		}
	})
	t.Run("new profile", func(t *testing.T) {
		s, repo, pub := newService()
		repo.errs = map[string]error{"GetAgentProfileByName": domain.ErrNotFound}
		if _, err := s.LoginAgent(ctx, "dev", "sk-ant-api03-y"); err != nil {
			t.Fatal(err)
		}
		if repo.ins["CreateAgentProfile"] != "claude dev" || repo.ins["SetAgentCredential"] != "api_key sealed:sk-ant-api03-y" {
			t.Fatalf("ins %v", repo.ins)
		}
		if !reflect.DeepEqual(eventTypes(pub), []string{"agent_profile.created", "agent_profile.updated"}) {
			t.Fatalf("events %v", eventTypes(pub))
		}
	})
	t.Run("failures", func(t *testing.T) {
		s, repo, _ := newService()
		if _, err := s.LoginAgent(ctx, "dev", "hunter2"); err == nil || len(repo.calls) != 0 {
			t.Fatalf("bad secret: %v", err)
		}
		repo.errs = map[string]error{"GetAgentProfileByName": errRepo}
		if _, err := s.LoginAgent(ctx, "dev", "sk-ant-api03-y"); !errors.Is(err, errRepo) {
			t.Fatalf("lookup: %v", err)
		}
		repo.errs = nil
		s.secrets = fakeSealer{err: errRepo}
		if _, err := s.LoginAgent(ctx, "dev", "sk-ant-api03-y"); !errors.Is(err, errRepo) {
			t.Fatalf("seal: %v", err)
		}
	})
}

func TestMcpServers(t *testing.T) {
	stdio := domain.McpServerInput{
		Name: "fs", Transport: domain.McpStdio, McpConfig: domain.McpConfig{Command: "srv"},
		Secrets: map[string]string{"TOKEN": "t", "A_KEY": "a"},
	}
	s, repo, pub := newService()
	if _, err := s.ListMcpServers(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetMcpServer(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMcpServer(ctx, stdio); err != nil {
		t.Fatal(err)
	}
	created := repo.ins["CreateMcpServer"].(domain.McpServerRecord)
	if !reflect.DeepEqual(created.SecretNames, []string{"A_KEY", "TOKEN"}) || string(created.Sealed) != `sealed:{"A_KEY":"a","TOKEN":"t"}` {
		t.Fatalf("record %+v", created)
	}

	// Replacing the secrets, or keeping them when none are given.
	if _, err := s.UpdateMcpServer(ctx, id, domain.McpServerInput{Name: "fs", Transport: domain.McpStdio, McpConfig: domain.McpConfig{Command: "x"}, Secrets: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if r := repo.ins["UpdateMcpServer"].(domain.McpServerRecord); r.SecretNames != nil || r.Sealed != nil {
		t.Fatalf("cleared secrets: %+v", r)
	}
	repo.mcp = domain.McpServer{SecretNames: []string{"TOKEN"}}
	repo.sealed = []byte("old")
	if _, err := s.UpdateMcpServer(ctx, id, domain.McpServerInput{Name: "fs", Transport: domain.McpStdio, McpConfig: domain.McpConfig{Command: "x"}}); err != nil {
		t.Fatal(err)
	}
	if r := repo.ins["UpdateMcpServer"].(domain.McpServerRecord); !reflect.DeepEqual(r.SecretNames, []string{"TOKEN"}) || string(r.Sealed) != "old" {
		t.Fatalf("kept secrets: %+v", r)
	}
	if err := s.DeleteMcpServer(ctx, id); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(eventTypes(pub), []string{"mcp_server.created", "mcp_server.updated", "mcp_server.updated", "mcp_server.deleted"}) {
		t.Fatalf("events %v", eventTypes(pub))
	}
}

func TestMcpServerFailures(t *testing.T) {
	keep := domain.McpServerInput{Name: "fs", Transport: domain.McpStdio, McpConfig: domain.McpConfig{Command: "x"}}
	s, repo, _ := newService()
	if _, err := s.CreateMcpServer(ctx, domain.McpServerInput{Name: "x"}); err == nil || len(repo.calls) != 0 {
		t.Fatalf("invalid create: %v", err)
	}
	if _, err := s.UpdateMcpServer(ctx, id, domain.McpServerInput{}); err == nil || len(repo.calls) != 0 {
		t.Fatalf("invalid update: %v", err)
	}
	for _, method := range []string{"GetMcpServer", "McpSecrets"} {
		repo.errs = map[string]error{method: errRepo}
		if _, err := s.UpdateMcpServer(ctx, id, keep); !errors.Is(err, errRepo) {
			t.Fatalf("%s: %v", method, err)
		}
	}
	s.secrets = fakeSealer{err: errRepo}
	keep.Secrets = map[string]string{"A": "b"}
	if _, err := s.CreateMcpServer(ctx, keep); !errors.Is(err, errRepo) {
		t.Fatalf("seal: %v", err)
	}
}
