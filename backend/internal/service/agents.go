package service

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

// Sealer encrypts and decrypts stored secrets. *crypto.Box implements it.
type Sealer interface {
	Seal(plaintext []byte) ([]byte, error)
	Open(sealed []byte) ([]byte, error)
}

// ListAgentProfiles returns the profiles ordered by name.
func (s *Service) ListAgentProfiles(ctx context.Context) ([]domain.AgentProfile, error) {
	return s.repo.ListAgentProfiles(ctx)
}

// GetAgentProfile returns one profile.
func (s *Service) GetAgentProfile(ctx context.Context, id uuid.UUID) (domain.AgentProfile, error) {
	return s.repo.GetAgentProfile(ctx, id)
}

// CreateAgentProfile registers a profile; it gets its credential through
// `orquestrator agent login <kind>`.
func (s *Service) CreateAgentProfile(ctx context.Context, in domain.AgentProfileInput) (domain.AgentProfile, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.AgentProfile{}, err
	}
	profile, err := s.repo.CreateAgentProfile(ctx, in.Kind, in.Name)
	return changed(s, "agent_profile.created", profile, err)
}

// UpdateAgentProfile renames a profile.
func (s *Service) UpdateAgentProfile(ctx context.Context, id uuid.UUID, in domain.AgentProfileInput) (domain.AgentProfile, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.AgentProfile{}, err
	}
	profile, err := s.repo.RenameAgentProfile(ctx, id, in.Name)
	return changed(s, "agent_profile.updated", profile, err)
}

// DeleteAgentProfile removes a profile no agent session used.
func (s *Service) DeleteAgentProfile(ctx context.Context, id uuid.UUID) error {
	return s.deleted("agent_profile.deleted", id, s.repo.DeleteAgentProfile(ctx, id))
}

// LoginAgent stores the credential of the profile of kind called name,
// creating the profile if needed. secret is a `claude setup-token` token or
// an API key, or what `opencode auth export` prints.
func (s *Service) LoginAgent(ctx context.Context, kind domain.AgentKind, name, secret string) (domain.AgentProfile, error) {
	secret, credKind, err := domain.Credential(kind, secret)
	if err != nil {
		return domain.AgentProfile{}, err
	}
	profile, err := s.repo.GetAgentProfileByName(ctx, name)
	if errors.Is(err, domain.ErrNotFound) {
		profile, err = s.CreateAgentProfile(ctx, domain.AgentProfileInput{Kind: kind, Name: name})
	}
	if err != nil {
		return domain.AgentProfile{}, err
	}
	if profile.Kind != kind {
		return domain.AgentProfile{}, &domain.ValidationError{Field: "kind", Message: "profile " + name + " runs " + string(profile.Kind) + ", not " + string(kind)}
	}
	sealed, err := s.secrets.Seal([]byte(secret))
	if err != nil {
		return domain.AgentProfile{}, err
	}
	profile, err = s.repo.SetAgentCredential(ctx, profile.ID, credKind, sealed)
	return changed(s, "agent_profile.updated", profile, err)
}

// ListMcpServers returns the MCP servers ordered by name.
func (s *Service) ListMcpServers(ctx context.Context) ([]domain.McpServer, error) {
	return s.repo.ListMcpServers(ctx)
}

// GetMcpServer returns one MCP server, without its secret values.
func (s *Service) GetMcpServer(ctx context.Context, id uuid.UUID) (domain.McpServer, error) {
	return s.repo.GetMcpServer(ctx, id)
}

// CreateMcpServer registers an MCP server; its secrets are sealed.
func (s *Service) CreateMcpServer(ctx context.Context, in domain.McpServerInput) (domain.McpServer, error) {
	record, err := s.mcpRecord(in)
	if err != nil {
		return domain.McpServer{}, err
	}
	server, err := s.repo.CreateMcpServer(ctx, record)
	return changed(s, "mcp_server.created", server, err)
}

// UpdateMcpServer replaces an MCP server. Nil secrets keep the current ones.
func (s *Service) UpdateMcpServer(ctx context.Context, id uuid.UUID, in domain.McpServerInput) (domain.McpServer, error) {
	record, err := s.mcpRecord(in)
	if err != nil {
		return domain.McpServer{}, err
	}
	if in.Secrets == nil {
		current, err := s.repo.GetMcpServer(ctx, id)
		if err != nil {
			return domain.McpServer{}, err
		}
		if record.Sealed, err = s.repo.McpSecrets(ctx, id); err != nil {
			return domain.McpServer{}, err
		}
		record.SecretNames = current.SecretNames
	}
	server, err := s.repo.UpdateMcpServer(ctx, id, record)
	return changed(s, "mcp_server.updated", server, err)
}

// DeleteMcpServer removes an MCP server.
func (s *Service) DeleteMcpServer(ctx context.Context, id uuid.UUID) error {
	return s.deleted("mcp_server.deleted", id, s.repo.DeleteMcpServer(ctx, id))
}

// TestMcpServer tries an MCP server out as typed in the form. With no
// secrets in the input, it uses those stored for the server being edited.
func (s *Service) TestMcpServer(ctx context.Context, in domain.McpServerTest) (domain.McpTestResult, error) {
	server, err := in.Server.Normalize()
	if err != nil {
		return domain.McpTestResult{}, err
	}
	secrets := server.Secrets
	if secrets == nil && in.ServerID != nil {
		if secrets, err = s.storedMcpSecrets(ctx, *in.ServerID); err != nil {
			return domain.McpTestResult{}, err
		}
	}
	cfg := server.McpConfig
	if server.Transport == domain.McpStdio {
		cfg.Env = withSecrets(cfg.Env, secrets)
	} else {
		cfg.Headers = withSecrets(cfg.Headers, secrets)
	}
	return s.checkMcp(ctx, server.Transport, cfg, strings.TrimSpace(in.Tool), in.Arguments), nil
}

// storedMcpSecrets opens the secrets stored for an MCP server.
func (s *Service) storedMcpSecrets(ctx context.Context, id uuid.UUID) (map[string]string, error) {
	sealed, err := s.repo.McpSecrets(ctx, id)
	if err != nil || sealed == nil {
		return nil, err
	}
	plain, err := s.secrets.Open(sealed)
	if err != nil {
		return nil, err
	}
	var secrets map[string]string
	err = json.Unmarshal(plain, &secrets)
	return secrets, err
}

func withSecrets(plain, secrets map[string]string) map[string]string {
	out := make(map[string]string, len(plain)+len(secrets))
	maps.Copy(out, plain)
	maps.Copy(out, secrets)
	return out
}

// mcpRecord validates in and seals its secrets.
func (s *Service) mcpRecord(in domain.McpServerInput) (domain.McpServerRecord, error) {
	in, err := in.Normalize()
	if err != nil {
		return domain.McpServerRecord{}, err
	}
	record := domain.McpServerRecord{Name: in.Name, Transport: in.Transport, Config: in.McpConfig}
	if len(in.Secrets) == 0 {
		return record, nil
	}
	for name := range in.Secrets {
		record.SecretNames = append(record.SecretNames, name)
	}
	slices.Sort(record.SecretNames)
	plain, _ := json.Marshal(in.Secrets) // a string map
	record.Sealed, err = s.secrets.Seal(plain)
	return record, err
}
