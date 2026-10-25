// Package workspace prepares the per-card directories agents run in:
//
//	<data_dir>/cards/<card_id>/
//	  workspace/  git worktree of the project (or empty), the agent's cwd
//	  claude/     CLAUDE_CONFIG_DIR, where sessions live for --resume
//	  logs/       raw stream-json of each node run
package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"orquestrator/internal/domain"
)

// Workspace is the set of directories of a card.
type Workspace struct {
	Dir       string
	ClaudeDir string
	LogsDir   string
	// GitDir is the .git directory of the project repository the worktree
	// points to; empty for cards without project. Sandboxes mount it at the
	// same path so git works inside the worktree.
	GitDir string
}

// Provider prepares the workspace of a card.
type Provider interface {
	Prepare(ctx context.Context, card domain.Card, project *domain.Project) (Workspace, error)
}

// Local keeps workspaces under DataDir on this host: a git worktree of the
// project on branch orq/card-<id>, or an empty directory for cards without
// project.
type Local struct {
	DataDir string
}

// Branch is the worktree branch of a card.
func Branch(card domain.Card) string {
	return "orq/card-" + card.ID.String()
}

// Prepare creates the card directories once and reuses them afterwards.
func (l Local) Prepare(ctx context.Context, card domain.Card, project *domain.Project) (Workspace, error) {
	root := filepath.Join(l.DataDir, "cards", card.ID.String())
	ws := Workspace{
		Dir:       filepath.Join(root, "workspace"),
		ClaudeDir: filepath.Join(root, "claude"),
		LogsDir:   filepath.Join(root, "logs"),
	}
	for _, dir := range []string{ws.ClaudeDir, ws.LogsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return ws, err
		}
	}
	if project == nil {
		return ws, os.MkdirAll(ws.Dir, 0o700)
	}
	if _, err := os.Stat(ws.Dir); os.IsNotExist(err) {
		if err := addWorktree(ctx, project.LocalPath, ws.Dir, Branch(card)); err != nil {
			return ws, err
		}
	}
	gitDir, err := git(ctx, ws.Dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	ws.GitDir = gitDir
	return ws, err
}

// addWorktree checks out branch in dir, creating the branch from the
// repository HEAD unless it exists (the worktree was removed before).
func addWorktree(ctx context.Context, repo, dir, branch string) error {
	if _, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		_, err = git(ctx, repo, "worktree", "add", dir, branch)
		return err
	}
	_, err := git(ctx, repo, "worktree", "add", "-b", branch, dir, "HEAD")
	return err
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // fixed git subcommands; paths are arguments
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
