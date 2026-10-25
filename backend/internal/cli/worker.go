package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"orquestrator/internal/agent"
	"orquestrator/internal/crypto"
	"orquestrator/internal/runner"
	"orquestrator/internal/store"
	"orquestrator/internal/worker"
	"orquestrator/internal/workspace"
)

// Environment variables of the worker.
const (
	envDataDir   = "ORQ_DATA_DIR"
	envClaudeBin = "ORQ_CLAUDE_BIN"
	envRunner    = "ORQ_RUNNER"
)

const (
	jobLease = 2 * time.Minute
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
	if bin := os.Getenv(envClaudeBin); bin != "" {
		return bin, nil
	}
	bin, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("claude CLI not found: install it (mise install) or set %s", envClaudeBin)
	}
	return bin, nil
}

// newRunner builds the runner named by ORQ_RUNNER: bwrap (default) or local.
func newRunner(claude string) (runner.Runner, error) {
	switch name := os.Getenv(envRunner); name {
	case "", "bwrap":
		path, err := exec.LookPath("bwrap")
		if err != nil {
			return nil, fmt.Errorf("bwrap not found (install bubblewrap, or set %s=local to run agents unsandboxed)", envRunner)
		}
		mounts, err := runner.CLIMounts(claude)
		if err != nil {
			return nil, err
		}
		return runner.Bwrap{Path: path, ReadOnly: mounts}, nil
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
	claude, err := claudeBinary()
	if err != nil {
		return err
	}
	run, err := newRunner(claude)
	if err != nil {
		return err
	}
	pool, err := connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	st := store.New(pool)
	host, _ := os.Hostname() // empty only on broken systems; jobs then need no host
	executor := &agent.Executor{
		Store:      st,
		Secrets:    box,
		Workspaces: workspace.Local{DataDir: data},
		Runner:     run,
		Binary:     claude,
	}
	w := &worker.Worker{
		Queue:        st,
		Listen:       func(ctx context.Context) (worker.Listener, error) { return st.ListenJobs(ctx) },
		ID:           host + "-" + strconv.Itoa(os.Getpid()),
		Capabilities: map[string]string{"host": host},
		Handlers:     map[string]worker.Handler{agent.JobKind: executor.Handle},
		Lease:        jobLease,
		Idle:         jobIdle,
		Log:          slog.New(slog.NewTextHandler(stderr, nil)),
	}
	_, _ = fmt.Fprintf(stdout, "worker %s running agents with %T in %s\n", w.ID, run, data)
	return w.Run(ctx)
}
