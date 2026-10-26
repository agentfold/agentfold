package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
	"orquestrator/internal/runner"
	"orquestrator/internal/workspace"
)

// JobKind is the queue job that runs an agent node.
const JobKind = domain.JobAgentRun

// Payload is the payload of an agent.run job.
type Payload = domain.AgentJob

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
	StartNodeRun(ctx context.Context, id, sessionID uuid.UUID, tokenHash []byte) (domain.NodeRun, error)
	FinishNodeRun(ctx context.Context, id uuid.UUID, r domain.NodeResult) (domain.NodeRun, error)
	PendingQuestions(ctx context.Context, nodeRunID uuid.UUID) (int, error)
	AddNodeOutput(ctx context.Context, nodeRunID uuid.UUID, entries []domain.NodeOutput) error
	SaveArtifact(ctx context.Context, cardID, nodeRunID uuid.UUID, name, path, content string) (domain.Artifact, int, bool, error)
	AddCardEvent(ctx context.Context, cardID uuid.UUID, typ domain.CardEventType, payload any) (domain.CardEvent, error)
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
	// BoardURL is where agents reach the board MCP server (BoardURL/mcp).
	BoardURL string
	// Pub announces the node runs starting and their output growing.
	Pub Publisher
}

// Publisher announces changes to the board. The worker's announces them
// to the server.
type Publisher interface {
	Publish(e events.Event)
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
	if run.Status != domain.NodeQueued && run.Status != domain.NodeRunning {
		return nil // a retried job whose node already ran
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
	node    node
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
		prompt = firstPrompt(j.node.config.Prompt, j.card, prompt)
	}
	token, tokenHash := newToken()
	layout := e.Runner.Layout(ws)
	name := "mcp-" + run.ID.String() + ".json"
	servers, err := e.writeMcpConfig(ctx, filepath.Join(ws.ClaudeDir, name), token, j.node.config.McpServerIDs)
	if err != nil {
		return domain.NodeResult{}, err
	}
	defer func() { _ = os.Remove(filepath.Join(ws.ClaudeDir, name)) }()
	inv := Invocation{
		Prompt: prompt, SessionID: session, Resume: resume, Config: j.node.config,
		SystemPrompt: j.node.systemPrompt(), McpConfig: layout.Claude + "/" + name, McpServers: servers,
	}
	started, err := e.Store.StartNodeRun(ctx, run.ID, session, tokenHash)
	if err != nil {
		return domain.NodeResult{}, err
	}
	e.Pub.Publish(events.Event{Type: "node_run.updated", ID: run.ID, Data: started})
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
		e.record(ctx, run.ID, line)
	})
	if ctx.Err() != nil {
		return failure(stream.Outcome(runErr), "the agent was interrupted: its flow run was canceled, or another worker took the job over"), nil
	}
	return e.outcome(context.WithoutCancel(ctx), j, ws, stream.Outcome(runErr))
}

// record keeps the steps of a stream-json line for the card page. Steps
// that cannot be kept are lost to the page only: the log has them all.
func (e *Executor) record(ctx context.Context, nodeRunID uuid.UUID, line []byte) {
	steps := Steps(line)
	if len(steps) == 0 {
		return
	}
	if e.Store.AddNodeOutput(context.WithoutCancel(ctx), nodeRunID, steps) == nil {
		e.Pub.Publish(events.Event{Type: "node_output.created", ID: nodeRunID})
	}
}

// outcome reads what the agent reported through the board tools while it
// ran and settles the node result. A node done collects its artifacts.
func (e *Executor) outcome(ctx context.Context, j job, ws workspace.Workspace, cli domain.NodeResult) (domain.NodeResult, error) {
	nr, err := e.Store.GetNodeRun(ctx, j.run.ID)
	if err != nil {
		return domain.NodeResult{}, err
	}
	pending, err := e.Store.PendingQuestions(ctx, nr.ID)
	if err != nil {
		return domain.NodeResult{}, err
	}
	result := j.node.outcome(cli, nr, pending)
	if result.Status != domain.NodeDone {
		return result, nil
	}
	for _, spec := range j.node.config.Artifacts {
		if msg, err := e.collect(ctx, j, ws, spec); msg != "" || err != nil {
			return failure(result, msg), err
		}
	}
	return result, nil
}

// maxArtifactSize bounds the files collected as artifacts.
const maxArtifactSize = 1 << 20

// collect reads an artifact the node declares from the card workspace and
// saves it. It returns why the agent failed to provide it, if it did.
func (e *Executor) collect(ctx context.Context, j job, ws workspace.Workspace, spec domain.ArtifactSpec) (string, error) {
	name := strings.TrimSpace(spec.Name)
	root, err := os.OpenRoot(ws.Dir)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	content, err := readArtifact(root, spec.Path)
	if err != nil {
		return fmt.Sprintf("the artifact %q could not be collected from %s: %v", name, spec.Path, err), nil
	}
	artifact, version, created, err := e.Store.SaveArtifact(ctx, j.card.ID, j.run.ID, name, spec.Path, content)
	if err != nil {
		return "", err
	}
	e.Pub.Publish(events.Event{Type: "artifact.updated", ID: artifact.ID})
	if !created {
		return "", nil
	}
	event, err := e.Store.AddCardEvent(ctx, j.card.ID, domain.ArtifactCollected, artifactCollected{
		ArtifactID: artifact.ID, Name: name, Version: version, NodeKey: j.run.NodeKey,
	})
	if err == nil {
		e.Pub.Publish(events.Event{Type: "card_event.created", ID: event.ID, Data: event})
	}
	return "", err
}

// readArtifact reads a text file of the workspace; root keeps it inside.
func readArtifact(root *os.Root, path string) (string, error) {
	f, err := root.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", errors.New("the file does not exist")
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxArtifactSize+1))
	switch {
	case err != nil:
		return "", err
	case len(data) > maxArtifactSize:
		return "", fmt.Errorf("the file is larger than %d bytes", maxArtifactSize)
	case !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0:
		return "", errors.New("the file is not text")
	}
	return string(data), nil
}

// artifactCollected is the timeline payload of a new artifact version.
type artifactCollected struct {
	ArtifactID uuid.UUID `json:"artifact_id"`
	Name       string    `json:"name"`
	Version    int       `json:"version"`
	NodeKey    string    `json:"node_key"`
}

// newToken returns a random token for the board tools and its hash, which
// is all the board keeps.
func newToken() (string, []byte) {
	token := rand.Text()
	sum := sha256.Sum256([]byte(token))
	return token, sum[:]
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
	if j.node, err = findNode(version.Graph, run.NodeKey); err != nil {
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
	if j.node.config.ProfileID == nil {
		return j, errors.New("the node has no agent profile")
	}
	if j.profile, err = e.Store.GetAgentProfile(ctx, *j.node.config.ProfileID); err != nil {
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

// firstPrompt starts a session: the node prompt, the card it works on and
// what the board hands the node, if anything.
func firstPrompt(prompt string, card domain.Card, input string) string {
	text := prompt + "\n\n## Card\n\n# " + card.Title
	if card.Description != "" {
		text += "\n\n" + card.Description
	}
	if input != "" {
		text += "\n\n## Input\n\n" + input
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

// writeMcpConfig writes the --mcp-config file of a run: the board server,
// authenticated by the run token, and the node's servers, secrets included.
// The file is removed when the run ends. It returns the server names.
func (e *Executor) writeMcpConfig(ctx context.Context, path, token string, ids []uuid.UUID) ([]string, error) {
	servers := map[string]mcpEntry{
		boardServer: {Type: "http", URL: e.BoardURL + "/mcp", Headers: map[string]string{"Authorization": "Bearer " + token}},
	}
	names := []string{boardServer}
	for _, id := range ids {
		server, err := e.Store.GetMcpServer(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("mcp server %s: %w", id, err)
		}
		secrets, err := e.mcpSecrets(ctx, id)
		if err != nil {
			return nil, err
		}
		entry := mcpEntry{Type: string(server.Transport), Command: server.Command, Args: server.Args, URL: server.URL}
		if server.Transport == domain.McpStdio {
			entry.Env = merge(server.Env, secrets)
		} else {
			entry.Headers = merge(server.Headers, secrets)
		}
		servers[server.Name] = entry
		names = append(names, server.Name)
	}
	data, _ := json.Marshal(map[string]any{"mcpServers": servers}) // plain values
	return names, os.WriteFile(path, data, 0o600)
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
