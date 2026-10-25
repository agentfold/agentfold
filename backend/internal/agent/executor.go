package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/runner"
	"orquestrator/internal/workspace"
)

// JobKind is the queue job that runs an agent node.
const JobKind = "agent.run"

// Payload is the payload of an agent.run job.
type Payload struct {
	NodeRunID uuid.UUID `json:"node_run_id"`
	// Prompt continues an existing session of the node (answers, review
	// comments); the first run of a node uses its configured prompt.
	Prompt string `json:"prompt,omitempty"`
}

// Store is the persistence the executor needs. *store.Store implements it.
type Store interface {
	GetNodeRun(ctx context.Context, id uuid.UUID) (domain.NodeRun, error)
	GetFlowRun(ctx context.Context, id uuid.UUID) (domain.FlowRun, error)
	GetFlowVersion(ctx context.Context, id uuid.UUID) (domain.FlowVersion, error)
	GetCard(ctx context.Context, id uuid.UUID) (domain.Card, error)
	GetProject(ctx context.Context, id uuid.UUID) (domain.Project, error)
	GetAgentProfile(ctx context.Context, id uuid.UUID) (domain.AgentProfile, error)
	AgentCredential(ctx context.Context, id uuid.UUID) ([]byte, error)
	GetMcpServer(ctx context.Context, id uuid.UUID) (domain.McpServer, error)
	McpSecrets(ctx context.Context, id uuid.UUID) ([]byte, error)
	AgentSession(ctx context.Context, flowRunID uuid.UUID, nodeKey string) (uuid.UUID, error)
	CreateAgentSession(ctx context.Context, flowRunID uuid.UUID, nodeKey string, profileID, sessionID uuid.UUID) error
	StartNodeRun(ctx context.Context, id, sessionID uuid.UUID) (domain.NodeRun, error)
	FinishNodeRun(ctx context.Context, id uuid.UUID, r domain.NodeResult) (domain.NodeRun, error)
}

// Opener decrypts stored secrets. *crypto.Box implements it.
type Opener interface {
	Open(sealed []byte) ([]byte, error)
}

// Executor runs agent.run jobs.
type Executor struct {
	Store      Store
	Secrets    Opener
	Workspaces workspace.Provider
	Runner     runner.Runner
	// Binary is the path of the claude CLI on the host.
	Binary string
}

// Handle runs the node of an agent.run job and records its result. Agent
// failures fail the node, not the job; the job fails when the node could
// not be run at all.
func (e *Executor) Handle(ctx context.Context, payload json.RawMessage) error {
	var p Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("agent.run payload: %w", err)
	}
	run, err := e.Store.GetNodeRun(ctx, p.NodeRunID)
	if err != nil {
		return err
	}
	result, err := e.execute(ctx, run, p.Prompt)
	if err != nil {
		result = domain.NodeResult{Status: domain.NodeFailed, OutputHandle: "failed", Error: err.Error()}
	}
	_, finishErr := e.Store.FinishNodeRun(context.WithoutCancel(ctx), run.ID, result)
	return errors.Join(err, finishErr)
}

// job is everything a node run needs, loaded before starting the agent.
type job struct {
	run     domain.NodeRun
	flowRun domain.FlowRun
	config  domain.AgentConfig
	card    domain.Card
	project *domain.Project
	profile domain.AgentProfile
	secret  string
}

func (e *Executor) execute(ctx context.Context, run domain.NodeRun, prompt string) (domain.NodeResult, error) {
	j, err := e.load(ctx, run)
	if err != nil {
		return domain.NodeResult{}, err
	}
	ws, err := e.Workspaces.Prepare(ctx, j.card, j.project)
	if err != nil {
		return domain.NodeResult{}, fmt.Errorf("prepare workspace: %w", err)
	}
	session, resume, err := e.session(ctx, j)
	if err != nil {
		return domain.NodeResult{}, err
	}
	if !resume || prompt == "" {
		prompt = firstPrompt(j.config.Prompt, j.card)
	}
	layout := e.Runner.Layout(ws)
	inv := Invocation{Prompt: prompt, SessionID: session, Resume: resume, Config: j.config}
	if len(j.config.McpServerIDs) > 0 {
		name := "mcp-" + run.ID.String() + ".json"
		if err := e.writeMcpConfig(ctx, filepath.Join(ws.ClaudeDir, name), j.config.McpServerIDs); err != nil {
			return domain.NodeResult{}, err
		}
		defer func() { _ = os.Remove(filepath.Join(ws.ClaudeDir, name)) }()
		inv.McpConfig = layout.Claude + "/" + name
	}
	if _, err := e.Store.StartNodeRun(ctx, run.ID, session); err != nil {
		return domain.NodeResult{}, err
	}
	logFile, err := os.OpenFile(filepath.Join(ws.LogsDir, run.ID.String()+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return domain.NodeResult{}, err
	}
	defer func() { _ = logFile.Close() }()

	var stream Stream
	runErr := e.Runner.Run(ctx, runner.Spec{
		Binary:    e.Binary,
		Args:      Args(inv),
		Env:       Env(layout.Claude, j.profile.CredentialKind, j.secret),
		Workspace: ws,
	}, func(line []byte) {
		_, _ = logFile.Write(append(line, '\n'))
		stream.Line(line)
	})
	return stream.Outcome(runErr), nil
}

// load reads the node config, the card, its project and the agent profile.
func (e *Executor) load(ctx context.Context, run domain.NodeRun) (job, error) {
	j := job{run: run}
	var err error
	if j.flowRun, err = e.Store.GetFlowRun(ctx, run.FlowRunID); err != nil {
		return j, err
	}
	version, err := e.Store.GetFlowVersion(ctx, j.flowRun.FlowVersionID)
	if err != nil {
		return j, err
	}
	if j.config, err = agentConfig(version.Graph, run.NodeKey); err != nil {
		return j, err
	}
	if j.card, err = e.Store.GetCard(ctx, j.flowRun.CardID); err != nil {
		return j, err
	}
	if j.card.ProjectID != nil {
		project, err := e.Store.GetProject(ctx, *j.card.ProjectID)
		if err != nil {
			return j, err
		}
		j.project = &project
	}
	if j.config.ProfileID == nil {
		return j, errors.New("the node has no agent profile")
	}
	if j.profile, err = e.Store.GetAgentProfile(ctx, *j.config.ProfileID); err != nil {
		return j, err
	}
	sealed, err := e.Store.AgentCredential(ctx, j.profile.ID)
	if err != nil {
		return j, err
	}
	if sealed == nil {
		return j, fmt.Errorf("agent profile %q is not logged in: run `orquestrator agent login claude --profile %s`", j.profile.Name, j.profile.Name)
	}
	secret, err := e.Secrets.Open(sealed)
	j.secret = string(secret)
	return j, err
}

// agentConfig finds the config of an agent node in a graph.
func agentConfig(g domain.Graph, key string) (domain.AgentConfig, error) {
	var c domain.AgentConfig
	for _, n := range g.Nodes {
		if n.ID == key && n.Type == domain.NodeAgent {
			err := json.Unmarshal(n.Config, &c)
			return c, err
		}
	}
	return c, fmt.Errorf("agent node %q not found in the flow version", key)
}

// session returns the Claude session of the node in this run, creating it
// on the first run.
func (e *Executor) session(ctx context.Context, j job) (uuid.UUID, bool, error) {
	id, err := e.Store.AgentSession(ctx, j.flowRun.ID, j.run.NodeKey)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return uuid.Nil, false, err
	}
	id = uuid.New()
	return id, false, e.Store.CreateAgentSession(ctx, j.flowRun.ID, j.run.NodeKey, j.profile.ID, id)
}

// firstPrompt is the node prompt followed by the card it works on.
func firstPrompt(prompt string, card domain.Card) string {
	text := prompt + "\n\n## Card\n\n# " + card.Title
	if card.Description != "" {
		text += "\n\n" + card.Description
	}
	return text
}

// mcpEntry is a server in the --mcp-config file.
type mcpEntry struct {
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// writeMcpConfig writes the --mcp-config file of a run, secrets included;
// it is removed when the run ends.
func (e *Executor) writeMcpConfig(ctx context.Context, path string, ids []uuid.UUID) error {
	servers := map[string]mcpEntry{}
	for _, id := range ids {
		server, err := e.Store.GetMcpServer(ctx, id)
		if err != nil {
			return fmt.Errorf("mcp server %s: %w", id, err)
		}
		secrets, err := e.mcpSecrets(ctx, id)
		if err != nil {
			return err
		}
		entry := mcpEntry{Type: string(server.Transport), Command: server.Command, Args: server.Args, URL: server.URL}
		if server.Transport == domain.McpStdio {
			entry.Env = merge(server.Env, secrets)
		} else {
			entry.Headers = merge(server.Headers, secrets)
		}
		servers[server.Name] = entry
	}
	data, _ := json.Marshal(map[string]any{"mcpServers": servers}) // plain values
	return os.WriteFile(path, data, 0o600)
}

func (e *Executor) mcpSecrets(ctx context.Context, id uuid.UUID) (map[string]string, error) {
	sealed, err := e.Store.McpSecrets(ctx, id)
	if err != nil || sealed == nil {
		return nil, err
	}
	plain, err := e.Secrets.Open(sealed)
	if err != nil {
		return nil, err
	}
	var secrets map[string]string
	err = json.Unmarshal(plain, &secrets)
	return secrets, err
}

func merge(a, b map[string]string) map[string]string {
	if len(a)+len(b) == 0 {
		return nil
	}
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
