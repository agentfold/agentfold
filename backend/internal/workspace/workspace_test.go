package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"orquestrator/internal/domain"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo returns a repository with one commit.
func newRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", ".")
	run(t, repo, "commit", "-q", "-m", "init")
	return repo
}

func TestEmptyWorkspace(t *testing.T) {
	l := Local{DataDir: t.TempDir()}
	card := domain.Card{ID: uuid.New()}
	ws, err := l.Prepare(context.Background(), card, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{ws.Dir, ws.ClaudeDir, ws.LogsDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("%s: %v", dir, err)
		}
	}
	if ws.GitDir != "" || !strings.HasSuffix(ws.Dir, filepath.Join(card.ID.String(), "workspace")) {
		t.Fatalf("unexpected workspace %+v", ws)
	}
}

func TestWorktree(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	l := Local{DataDir: t.TempDir()}
	card := domain.Card{ID: uuid.New()}
	project := &domain.Project{LocalPath: repo}

	ws, err := l.Prepare(ctx, card, project)
	if err != nil {
		t.Fatal(err)
	}
	if ws.GitDir != filepath.Join(repo, ".git") {
		t.Fatalf("GitDir = %q", ws.GitDir)
	}
	if got := run(t, ws.Dir, "branch", "--show-current"); got != Branch(card) {
		t.Fatalf("branch %q", got)
	}

	// Work left in the worktree survives the next preparation.
	if err := os.WriteFile(filepath.Join(ws.Dir, "work"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Prepare(ctx, card, project); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, "work")); err != nil {
		t.Fatal("the worktree was recreated")
	}

	// A removed worktree comes back on its existing branch.
	run(t, repo, "worktree", "remove", "--force", ws.Dir)
	if _, err := l.Prepare(ctx, card, project); err != nil {
		t.Fatal(err)
	}
	if got := run(t, ws.Dir, "branch", "--show-current"); got != Branch(card) {
		t.Fatalf("branch after re-adding %q", got)
	}
}

func TestPrepareErrors(t *testing.T) {
	ctx := context.Background()
	card := domain.Card{ID: uuid.New()}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Local{DataDir: file}).Prepare(ctx, card, nil); err == nil {
		t.Error("data dir is a file: expected error")
	}

	notRepo := &domain.Project{LocalPath: t.TempDir()}
	if _, err := (Local{DataDir: t.TempDir()}).Prepare(ctx, card, notRepo); err == nil {
		t.Error("project outside git: expected error")
	}

	// An existing workspace that is not a worktree fails on rev-parse.
	l := Local{DataDir: t.TempDir()}
	if _, err := l.Prepare(ctx, card, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Prepare(ctx, card, &domain.Project{LocalPath: newRepo(t)}); err == nil {
		t.Error("plain directory as worktree: expected error")
	}
}
