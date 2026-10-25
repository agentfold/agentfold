package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func TestAgentProfiles(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	p, err := s.CreateAgentProfile(ctx, domain.AgentClaude, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if p.Ready() || p.Kind != domain.AgentClaude {
		t.Fatalf("unexpected profile %+v", p)
	}
	if _, err := s.CreateAgentProfile(ctx, domain.AgentClaude, "dev"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	cred, err := s.AgentCredential(ctx, p.ID)
	if err != nil || cred != nil {
		t.Fatalf("AgentCredential = %v, %v", cred, err)
	}

	p, err = s.SetAgentCredential(ctx, p.ID, domain.CredentialOAuthToken, []byte("sealed"))
	if err != nil || p.CredentialKind != domain.CredentialOAuthToken {
		t.Fatalf("SetAgentCredential = %+v, %v", p, err)
	}
	if cred, _ := s.AgentCredential(ctx, p.ID); string(cred) != "sealed" {
		t.Fatalf("credential %q", cred)
	}
	if p, err = s.RenameAgentProfile(ctx, p.ID, "reviewer"); err != nil || p.Name != "reviewer" {
		t.Fatalf("RenameAgentProfile = %+v, %v", p, err)
	}
	if got, err := s.GetAgentProfileByName(ctx, "reviewer"); err != nil || got.ID != p.ID {
		t.Fatalf("GetAgentProfileByName = %+v, %v", got, err)
	}
	if got, err := s.GetAgentProfile(ctx, p.ID); err != nil || !got.Ready() {
		t.Fatalf("GetAgentProfile = %+v, %v", got, err)
	}
	list, err := s.ListAgentProfiles(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListAgentProfiles = %+v, %v", list, err)
	}
	if err := s.DeleteAgentProfile(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentCredential(ctx, p.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AgentCredential after delete: %v", err)
	}
	if _, err := s.ListAgentProfiles(cancelled()); err == nil {
		t.Fatal("ListAgentProfiles: expected error")
	}
}

func TestMcpServers(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	record := domain.McpServerRecord{
		Name:        "sourcegraph",
		Transport:   domain.McpHTTP,
		Config:      domain.McpConfig{URL: "https://sg.example/mcp", Headers: map[string]string{"X-A": "1"}},
		SecretNames: []string{"Authorization"},
		Sealed:      []byte("sealed"),
	}
	m, err := s.CreateMcpServer(ctx, record)
	if err != nil {
		t.Fatal(err)
	}
	if m.URL != record.Config.URL || !reflect.DeepEqual(m.SecretNames, record.SecretNames) {
		t.Fatalf("unexpected server %+v", m)
	}
	if _, err := s.CreateMcpServer(ctx, record); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	if sealed, err := s.McpSecrets(ctx, m.ID); err != nil || string(sealed) != "sealed" {
		t.Fatalf("McpSecrets = %q, %v", sealed, err)
	}

	record.Name, record.Transport = "local", domain.McpStdio
	record.Config = domain.McpConfig{Command: "srv", Args: []string{"-v"}}
	record.SecretNames, record.Sealed = nil, nil
	if m, err = s.UpdateMcpServer(ctx, m.ID, record); err != nil || m.Command != "srv" || m.SecretNames != nil {
		t.Fatalf("UpdateMcpServer = %+v, %v", m, err)
	}
	if got, err := s.GetMcpServer(ctx, m.ID); err != nil || got.Name != "local" {
		t.Fatalf("GetMcpServer = %+v, %v", got, err)
	}
	if list, err := s.ListMcpServers(ctx); err != nil || len(list) != 1 {
		t.Fatalf("ListMcpServers = %+v, %v", list, err)
	}
	if err := s.DeleteMcpServer(ctx, m.ID); err != nil {
		t.Fatal(err)
	}

	missing := uuid.New()
	if _, err := s.GetMcpServer(ctx, missing); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetMcpServer: %v", err)
	}
	if _, err := s.UpdateMcpServer(ctx, missing, record); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UpdateMcpServer: %v", err)
	}
	if _, err := s.McpSecrets(ctx, missing); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("McpSecrets: %v", err)
	}
	if err := s.DeleteMcpServer(ctx, missing); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("DeleteMcpServer: %v", err)
	}
}

func TestCorruptedMcpConfig(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO mcp_servers (name, transport, config) VALUES ('bad', 'stdio', '"x"') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetMcpServer(ctx, id); err == nil {
		t.Error("GetMcpServer: expected error")
	}
	if _, err := s.ListMcpServers(ctx); err == nil {
		t.Error("ListMcpServers: expected error")
	}
}
