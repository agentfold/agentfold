package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/store/sqlcgen"
)

func toAgentProfile(r sqlcgen.AgentProfile) domain.AgentProfile {
	p := domain.AgentProfile{
		ID:        r.ID,
		Kind:      domain.AgentKind(r.Kind),
		Name:      r.Name,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if r.CredentialKind != nil {
		p.CredentialKind = domain.CredentialKind(*r.CredentialKind)
	}
	return p
}

// ListAgentProfiles returns the profiles ordered by name.
func (s *Store) ListAgentProfiles(ctx context.Context) ([]domain.AgentProfile, error) {
	rows, err := s.q.ListAgentProfiles(ctx)
	return mapRows(rows, err, toAgentProfile)
}

// GetAgentProfile returns one profile.
func (s *Store) GetAgentProfile(ctx context.Context, id uuid.UUID) (domain.AgentProfile, error) {
	row, err := s.q.GetAgentProfile(ctx, id)
	return mapRow(row, err, toAgentProfile)
}

// GetAgentProfileByName returns the profile with that name.
func (s *Store) GetAgentProfileByName(ctx context.Context, name string) (domain.AgentProfile, error) {
	row, err := s.q.GetAgentProfileByName(ctx, name)
	return mapRow(row, err, toAgentProfile)
}

// CreateAgentProfile stores a profile without credential.
func (s *Store) CreateAgentProfile(ctx context.Context, kind domain.AgentKind, name string) (domain.AgentProfile, error) {
	row, err := s.q.InsertAgentProfile(ctx, sqlcgen.InsertAgentProfileParams{Kind: string(kind), Name: name})
	return mapRow(row, err, toAgentProfile)
}

// RenameAgentProfile changes the name of a profile.
func (s *Store) RenameAgentProfile(ctx context.Context, id uuid.UUID, name string) (domain.AgentProfile, error) {
	row, err := s.q.RenameAgentProfile(ctx, sqlcgen.RenameAgentProfileParams{ID: id, Name: name})
	return mapRow(row, err, toAgentProfile)
}

// SetAgentCredential stores the sealed credential of a profile.
func (s *Store) SetAgentCredential(ctx context.Context, id uuid.UUID, kind domain.CredentialKind, sealed []byte) (domain.AgentProfile, error) {
	k := string(kind)
	row, err := s.q.SetAgentCredential(ctx, sqlcgen.SetAgentCredentialParams{ID: id, Credential: sealed, CredentialKind: &k})
	return mapRow(row, err, toAgentProfile)
}

// AgentCredential returns the sealed credential of a profile; nil when the
// profile has none.
func (s *Store) AgentCredential(ctx context.Context, id uuid.UUID) ([]byte, error) {
	row, err := s.q.GetAgentProfile(ctx, id)
	return row.Credential, mapErr(err)
}

// DeleteAgentProfile removes a profile no session uses.
func (s *Store) DeleteAgentProfile(ctx context.Context, id uuid.UUID) error {
	return requireDeleted(s.q.DeleteAgentProfile(ctx, id))
}

// mcpStored is the config column of mcp_servers.
type mcpStored struct {
	domain.McpConfig
	SecretNames []string `json:"secret_names,omitempty"`
}

func toMcpServer(r sqlcgen.McpServer) (domain.McpServer, error) {
	var c mcpStored
	err := json.Unmarshal(r.Config, &c)
	return domain.McpServer{
		ID:          r.ID,
		Name:        r.Name,
		Transport:   domain.McpTransport(r.Transport),
		McpConfig:   c.McpConfig,
		SecretNames: c.SecretNames,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}, err
}

func mcpConfigJSON(r domain.McpServerRecord) []byte {
	data, _ := json.Marshal(mcpStored{McpConfig: r.Config, SecretNames: r.SecretNames}) // plain strings and maps
	return data
}

// ListMcpServers returns the MCP servers ordered by name.
func (s *Store) ListMcpServers(ctx context.Context) ([]domain.McpServer, error) {
	rows, err := s.q.ListMcpServers(ctx)
	return mapParsed(rows, err, toMcpServer)
}

// GetMcpServer returns one MCP server.
func (s *Store) GetMcpServer(ctx context.Context, id uuid.UUID) (domain.McpServer, error) {
	row, err := s.q.GetMcpServer(ctx, id)
	if err != nil {
		return domain.McpServer{}, mapErr(err)
	}
	return toMcpServer(row)
}

// McpSecrets returns the sealed secrets of an MCP server; nil when it has none.
func (s *Store) McpSecrets(ctx context.Context, id uuid.UUID) ([]byte, error) {
	row, err := s.q.GetMcpServer(ctx, id)
	return row.Secrets, mapErr(err)
}

// CreateMcpServer stores an MCP server.
func (s *Store) CreateMcpServer(ctx context.Context, r domain.McpServerRecord) (domain.McpServer, error) {
	row, err := s.q.InsertMcpServer(ctx, sqlcgen.InsertMcpServerParams{
		Name: r.Name, Transport: string(r.Transport), Config: mcpConfigJSON(r), Secrets: r.Sealed,
	})
	if err != nil {
		return domain.McpServer{}, mapErr(err)
	}
	return toMcpServer(row)
}

// UpdateMcpServer replaces an MCP server.
func (s *Store) UpdateMcpServer(ctx context.Context, id uuid.UUID, r domain.McpServerRecord) (domain.McpServer, error) {
	row, err := s.q.UpdateMcpServer(ctx, sqlcgen.UpdateMcpServerParams{
		ID: id, Name: r.Name, Transport: string(r.Transport), Config: mcpConfigJSON(r), Secrets: r.Sealed,
	})
	if err != nil {
		return domain.McpServer{}, mapErr(err)
	}
	return toMcpServer(row)
}

// DeleteMcpServer removes an MCP server.
func (s *Store) DeleteMcpServer(ctx context.Context, id uuid.UUID) error {
	return requireDeleted(s.q.DeleteMcpServer(ctx, id))
}
