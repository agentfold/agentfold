package domain

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AgentKind names the CLI an agent profile runs.
type AgentKind string

// Agent CLIs.
const (
	// AgentClaude runs Claude Code.
	AgentClaude AgentKind = "claude"
	// AgentOpencode runs opencode.
	AgentOpencode AgentKind = "opencode"
)

// CredentialKind tells how a profile authenticates.
type CredentialKind string

// Credential kinds.
const (
	// CredentialOAuthToken is a long-lived token from `claude setup-token`
	// (CLAUDE_CODE_OAUTH_TOKEN).
	CredentialOAuthToken CredentialKind = "oauth_token" //nolint:gosec // a kind name, not a credential
	// CredentialAPIKey is an Anthropic API key (ANTHROPIC_API_KEY).
	CredentialAPIKey CredentialKind = "api_key"
	// CredentialOpencodeAuth is the JSON `opencode auth export` prints.
	CredentialOpencodeAuth CredentialKind = "opencode_auth" //nolint:gosec // a kind name, not a credential
)

// AgentProfile is a configured agent account that nodes run as.
type AgentProfile struct {
	ID   uuid.UUID
	Kind AgentKind
	Name string
	// CredentialKind is empty until the profile is logged in.
	CredentialKind CredentialKind
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// EntityID returns the profile id.
func (p AgentProfile) EntityID() uuid.UUID { return p.ID }

// Ready reports whether the profile has a credential.
func (p AgentProfile) Ready() bool { return p.CredentialKind != "" }

// AgentProfileInput holds the editable fields of a profile. Kind is only
// read on creation; empty means claude.
type AgentProfileInput struct {
	Kind AgentKind
	Name string
}

// Normalize trims the fields and validates them.
func (in AgentProfileInput) Normalize() (AgentProfileInput, error) {
	switch in.Kind {
	case "":
		in.Kind = AgentClaude
	case AgentClaude, AgentOpencode:
	default:
		return in, &ValidationError{Field: "kind", Message: "must be claude or opencode"}
	}
	name, err := requireText("name", in.Name)
	return AgentProfileInput{Kind: in.Kind, Name: name}, err
}

// DetectCredential tells a setup-token token from an API key by its prefix.
func DetectCredential(secret string) (string, CredentialKind, error) {
	secret = strings.TrimSpace(secret)
	switch {
	case strings.HasPrefix(secret, "sk-ant-oat"):
		return secret, CredentialOAuthToken, nil
	case strings.HasPrefix(secret, "sk-ant-api"):
		return secret, CredentialAPIKey, nil
	}
	return "", "", &ValidationError{Field: "credential", Message: "expected a token from `claude setup-token` (sk-ant-oat…) or an API key (sk-ant-api…)"}
}

// Credential checks the secret of a profile of kind and tells its
// credential kind: a Claude token or API key, or for opencode the
// credentials `opencode auth export` prints: JSON, not empty. Its shape is
// opencode's, which reads it back with `opencode auth import`.
func Credential(kind AgentKind, secret string) (string, CredentialKind, error) {
	if kind != AgentOpencode {
		return DetectCredential(secret)
	}
	var auth any
	if err := json.Unmarshal([]byte(secret), &auth); err != nil || isEmpty(auth) {
		return "", "", &ValidationError{Field: "credential", Message: "expected the JSON `opencode auth export` prints, with at least one integration: log in with `opencode auth login` first"}
	}
	return strings.TrimSpace(secret), CredentialOpencodeAuth, nil
}

func isEmpty(v any) bool {
	switch v := v.(type) {
	case map[string]any:
		return len(v) == 0
	case []any:
		return len(v) == 0
	}
	return v == nil
}

// McpTransport tells how an MCP server is reached.
type McpTransport string

// MCP transports.
const (
	McpStdio McpTransport = "stdio"
	McpHTTP  McpTransport = "http"
)

// McpServer is an external MCP server agent nodes can use. Secret values are
// never exposed; only their names.
type McpServer struct {
	ID        uuid.UUID
	Name      string
	Transport McpTransport
	McpConfig
	SecretNames []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EntityID returns the server id.
func (s McpServer) EntityID() uuid.UUID { return s.ID }

// McpConfig is the non-secret configuration of an MCP server.
type McpConfig struct {
	// stdio
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	// http
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// McpServerInput creates or replaces an MCP server. Secrets are env vars
// (stdio) or headers (http) whose values are stored encrypted; nil keeps the
// current secrets on updates.
type McpServerInput struct {
	Name      string
	Transport McpTransport
	McpConfig
	Secrets map[string]string
}

// Normalize trims the fields and validates them.
func (in McpServerInput) Normalize() (McpServerInput, error) {
	name, err := requireText("name", in.Name)
	if err != nil {
		return in, err
	}
	in.Name = name
	switch in.Transport {
	case McpStdio:
		if in.Command, err = requireText("command", in.Command); err != nil {
			return in, err
		}
		in.URL, in.Headers = "", nil
	case McpHTTP:
		u, err := url.Parse(strings.TrimSpace(in.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return in, &ValidationError{Field: "url", Message: "must be an http(s) URL"}
		}
		in.URL = u.String()
		in.Command, in.Args, in.Env = "", nil, nil
	default:
		return in, &ValidationError{Field: "transport", Message: "must be stdio or http"}
	}
	return in, nil
}

// McpServerTest tries an MCP server out as typed, before it is saved. With
// no secrets in Server, those stored for ServerID are used. Tool, when set,
// is called after listing the tools, since some servers only check their
// credentials on a call.
type McpServerTest struct {
	Server    McpServerInput
	ServerID  *uuid.UUID
	Tool      string
	Arguments map[string]any
}

// McpTestResult is what a test of an MCP server found.
type McpTestResult struct {
	OK         bool
	Tools      []string
	Error      string
	ToolResult string
}

// McpServerRecord is what is stored for an MCP server: its config, the
// names of its secrets and their sealed values.
type McpServerRecord struct {
	Name        string
	Transport   McpTransport
	Config      McpConfig
	SecretNames []string
	Sealed      []byte
}
