package rest

import (
	"context"

	"orquestrator/internal/domain"
	"orquestrator/internal/transport/rest/openapi"
)

func toAgentProfile(p domain.AgentProfile) openapi.AgentProfile {
	out := openapi.AgentProfile{
		Id:        p.ID,
		Kind:      openapi.AgentProfileKind(p.Kind),
		Name:      p.Name,
		Ready:     p.Ready(),
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
	if p.Ready() {
		kind := openapi.AgentProfileCredentialKind(p.CredentialKind)
		out.CredentialKind = &kind
	}
	return out
}

func toMcpServer(m domain.McpServer) openapi.McpServer {
	out := openapi.McpServer{
		Id:          m.ID,
		Name:        m.Name,
		Transport:   openapi.McpServerTransport(m.Transport),
		Args:        orEmpty(m.Args),
		Env:         orEmptyMap(m.Env),
		Headers:     orEmptyMap(m.Headers),
		SecretNames: orEmpty(m.SecretNames),
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
	if m.Command != "" {
		out.Command = &m.Command
	}
	if m.URL != "" {
		out.Url = &m.URL
	}
	return out
}

func mcpServerInput(b *openapi.McpServerInput) domain.McpServerInput {
	in := domain.McpServerInput{
		Name:      b.Name,
		Transport: domain.McpTransport(b.Transport),
		McpConfig: domain.McpConfig{Command: deref(b.Command), URL: deref(b.Url)},
	}
	if b.Args != nil {
		in.Args = *b.Args
	}
	if b.Env != nil {
		in.Env = *b.Env
	}
	if b.Headers != nil {
		in.Headers = *b.Headers
	}
	if b.Secrets != nil {
		in.Secrets = *b.Secrets
	}
	return in
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func orEmptyMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// Agent profiles

func (h handlers) ListAgentProfiles(ctx context.Context, _ openapi.ListAgentProfilesRequestObject) (openapi.ListAgentProfilesResponseObject, error) {
	profiles, err := h.board.ListAgentProfiles(ctx)
	return openapi.ListAgentProfiles200JSONResponse(mapAll(profiles, toAgentProfile)), err
}

func (h handlers) CreateAgentProfile(ctx context.Context, req openapi.CreateAgentProfileRequestObject) (openapi.CreateAgentProfileResponseObject, error) {
	profile, err := h.board.CreateAgentProfile(ctx, domain.AgentProfileInput{Name: req.Body.Name})
	return openapi.CreateAgentProfile201JSONResponse(toAgentProfile(profile)), err
}

func (h handlers) GetAgentProfile(ctx context.Context, req openapi.GetAgentProfileRequestObject) (openapi.GetAgentProfileResponseObject, error) {
	profile, err := h.board.GetAgentProfile(ctx, req.Id)
	return openapi.GetAgentProfile200JSONResponse(toAgentProfile(profile)), err
}

func (h handlers) UpdateAgentProfile(ctx context.Context, req openapi.UpdateAgentProfileRequestObject) (openapi.UpdateAgentProfileResponseObject, error) {
	profile, err := h.board.UpdateAgentProfile(ctx, req.Id, domain.AgentProfileInput{Name: req.Body.Name})
	return openapi.UpdateAgentProfile200JSONResponse(toAgentProfile(profile)), err
}

func (h handlers) DeleteAgentProfile(ctx context.Context, req openapi.DeleteAgentProfileRequestObject) (openapi.DeleteAgentProfileResponseObject, error) {
	return openapi.DeleteAgentProfile204Response{}, h.board.DeleteAgentProfile(ctx, req.Id)
}

// MCP servers

func (h handlers) ListMcpServers(ctx context.Context, _ openapi.ListMcpServersRequestObject) (openapi.ListMcpServersResponseObject, error) {
	servers, err := h.board.ListMcpServers(ctx)
	return openapi.ListMcpServers200JSONResponse(mapAll(servers, toMcpServer)), err
}

func (h handlers) CreateMcpServer(ctx context.Context, req openapi.CreateMcpServerRequestObject) (openapi.CreateMcpServerResponseObject, error) {
	server, err := h.board.CreateMcpServer(ctx, mcpServerInput(req.Body))
	return openapi.CreateMcpServer201JSONResponse(toMcpServer(server)), err
}

func (h handlers) GetMcpServer(ctx context.Context, req openapi.GetMcpServerRequestObject) (openapi.GetMcpServerResponseObject, error) {
	server, err := h.board.GetMcpServer(ctx, req.Id)
	return openapi.GetMcpServer200JSONResponse(toMcpServer(server)), err
}

func (h handlers) UpdateMcpServer(ctx context.Context, req openapi.UpdateMcpServerRequestObject) (openapi.UpdateMcpServerResponseObject, error) {
	server, err := h.board.UpdateMcpServer(ctx, req.Id, mcpServerInput(req.Body))
	return openapi.UpdateMcpServer200JSONResponse(toMcpServer(server)), err
}

func (h handlers) DeleteMcpServer(ctx context.Context, req openapi.DeleteMcpServerRequestObject) (openapi.DeleteMcpServerResponseObject, error) {
	return openapi.DeleteMcpServer204Response{}, h.board.DeleteMcpServer(ctx, req.Id)
}
