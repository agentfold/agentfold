package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"orquestrator/internal/agent"
	"orquestrator/internal/crypto"
	"orquestrator/internal/domain"
	"orquestrator/internal/engine"
	"orquestrator/internal/events"
	"orquestrator/internal/runner"
	"orquestrator/internal/store"
	"orquestrator/internal/worker"
	"orquestrator/internal/workspace"
)

// Environment variables of the worker.
const (
	envDataDir     = "ORQ_DATA_DIR"
	envClaudeBin   = "ORQ_CLAUDE_BIN"
	envOpencodeBin = "ORQ_OPENCODE_BIN"
	envRunner      = "ORQ_RUNNER"
	envServerURL   = "ORQ_SERVER_URL"
)

// defaultServerURL is the server address agents reach the board MCP at.
const defaultServerURL = "http://" + defaultAddr

const (
	jobLease = 2 * time.Minute
	jobBeat  = 5 * time.Second
	jobIdle  = 30 * time.Second
)

func runSecretKey(_ context.Context, args []string, stdout, _ io.Writer) error {
	if len(args) > 0 {
		return errUnexpectedArgs
	}
	_, _ = fmt.Fprintln(stdout, crypto.NewKey())
	return nil
}

// dataDir is ORQ_DATA_DIR, or ~/.local/share/orquestrator.
func dataDir() (string, error) {
	if dir := os.Getenv(envDataDir); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%s is not set and there is no home directory: %w", envDataDir, err)
	}
	return filepath.Join(home, ".local", "share", "orquestrator"), nil
}

// claudeBinary is ORQ_CLAUDE_BIN, or the claude found in PATH.
func claudeBinary() (string, error) {
	return binary("claude", envClaudeBin)
}

// opencodeBinary is ORQ_OPENCODE_BIN, or the opencode found in PATH.
func opencodeBinary() (string, error) {
	return binary("opencode", envOpencodeBin)
}

func binary(name, env string) (string, error) {
	if bin := os.Getenv(env); bin != "" {
		return bin, nil
	}
	bin, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s CLI not found: install it (mise install) or set %s", name, env)
	}
	return bin, nil
}

// agentBinaries are the agent CLIs this worker can run; it needs one at
// least, and fails the nodes of profiles whose CLI it lacks.
func agentBinaries() (map[domain.AgentKind]string, error) {
	bins := map[domain.AgentKind]string{}
	var errs []error
	for kind, find := range map[domain.AgentKind]func() (string, error){
		domain.AgentClaude:   claudeBinary,
		domain.AgentOpencode: opencodeBinary,
	} {
		bin, err := find()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		bins[kind] = bin
	}
	if len(bins) == 0 {
		return nil, errors.Join(errs...)
	}
	return bins, nil
}

// serverURL is ORQ_SERVER_URL, or the server's default address.
func serverURL() string {
	if url := os.Getenv(envServerURL); url != "" {
		return strings.TrimSuffix(url, "/")
	}
	return defaultServerURL
}

// newRunner builds the runner named by ORQ_RUNNER: bwrap (default) or
// local. Sandboxes mount the agent CLIs read-only.
func newRunner(binaries map[domain.AgentKind]string) (runner.Runner, error) {
	switch name := os.Getenv(envRunner); name {
	case "", "bwrap":
		path, err := exec.LookPath("bwrap")
		if err != nil {
			return nil, fmt.Errorf("bwrap not found (install bubblewrap, or set %s=local to run agents unsandboxed)", envRunner)
		}
		var mounts []string
		for _, bin := range binaries {
			dirs, err := runner.CLIMounts(bin)
			if err != nil {
				return nil, err
			}
			mounts = append(mounts, dirs...)
		}
		slices.Sort(mounts)
		return runner.Bwrap{Path: path, ReadOnly: slices.Compact(mounts)}, nil
	case "local":
		home, _ := os.UserHomeDir() // only the agent's HOME; empty is fine
		return runner.Local{Home: home}, nil
	default:
		return nil, fmt.Errorf("%s must be bwrap or local, not %q", envRunner, name)
	}
}

func runWorker(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return errUnexpectedArgs
	}
	box, err := secretBox()
	if err != nil {
		return err
	}
	data, err := dataDir()
	if err != nil {
		return err
	}
	binaries, err := agentBinaries()
	if err != nil {
		return err
	}
	run, err := newRunner(binaries)
	if err != nil {
		return err
	}
	pool, err := connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	st := store.New(pool)
	log := slog.New(slog.NewTextHandler(stderr, nil))
	host, _ := os.Hostname() // empty only on broken systems; jobs then need no host
	pub := notifier{store: st, log: log}
	flows := &engine.Engine{
		Store: st,
		Agents: &agent.Executor{
			Store:      st,
			Secrets:    box,
			Workspaces: workspace.Local{DataDir: data},
			Runner:     run,
			Binaries:   binaries,
			BoardURL:   serverURL(),
			Pub:        pub,
		},
		Pub: pub,
	}
	w := &worker.Worker{
		Queue:        st,
		Listen:       func(ctx context.Context) (worker.Listener, error) { return st.ListenJobs(ctx) },
		ID:           host + "-" + strconv.Itoa(os.Getpid()),
		Capabilities: map[string]string{"host": host},
		Handlers: map[string]worker.Handler{
			domain.JobFlowTrigger:  flows.HandleTrigger,
			agent.JobKind:          flows.HandleAgent,
			domain.JobChildChanged: flows.HandleChildChanged,
		},
		Lease: jobLease,
		Beat:  jobBeat,
		Idle:  jobIdle,
		Log:   log,
	}
	_, _ = fmt.Fprintf(stdout, "worker %s running agents with %T in %s\n", w.ID, run, data)
	return w.Run(ctx)
}

// notifier announces the changes the engine makes to the server, which
// streams them to the board. A change that cannot be announced is only
// logged: clients catch up when they reload.
type notifier struct {
	store interface {
		NotifyChange(ctx context.Context, typ string, id uuid.UUID) error
	}
	log *slog.Logger
}

func (n notifier) Publish(e events.Event) {
	if err := n.store.NotifyChange(context.Background(), e.Type, e.ID); err != nil {
		n.log.Error("cannot announce a change", "type", e.Type, "id", e.ID, "error", err)
	}
}
