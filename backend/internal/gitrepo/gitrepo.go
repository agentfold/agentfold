// Package gitrepo inspects git repositories on the local filesystem.
package gitrepo

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Inspector runs the git binary found in PATH.
type Inspector struct{}

// Root returns the top-level directory of the repository that contains path.
func (Inspector) Root(ctx context.Context, path string) (string, error) {
	// path is the value of -C, never parsed as a flag or command.
	out, err := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--show-toplevel").Output() //nolint:gosec // fixed binary and arguments
	if err != nil {
		return "", fmt.Errorf("%s is not inside a git repository: %w", path, err)
	}
	return strings.TrimSpace(string(out)), nil
}
