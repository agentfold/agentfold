package runner

import (
	"context"
	"os/exec"

	"orquestrator/internal/workspace"
)

// Local runs the process directly on the host, in the workspace directory.
// It does not isolate anything: for development and tests.
type Local struct {
	// Home is the HOME of the process; the CLI keeps its state in the
	// card's claude directory anyway.
	Home string
}

// Layout returns the host paths of the card directories.
func (Local) Layout(ws workspace.Workspace) Layout {
	return Layout{Workspace: ws.Dir, Claude: ws.ClaudeDir}
}

// Run starts the process and waits for it.
func (l Local) Run(ctx context.Context, spec Spec, onLine func([]byte)) error {
	cmd := exec.CommandContext(ctx, spec.Binary, spec.Args...) //nolint:gosec // the binary is the configured agent CLI
	cmd.Dir = spec.Workspace.Dir
	cmd.Env = append(append(baseEnv(), "HOME="+l.Home), spec.Env...)
	return start(cmd, onLine)
}
