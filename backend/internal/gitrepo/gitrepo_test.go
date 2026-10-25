package gitrepo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRoot(t *testing.T) {
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{repo, sub} {
		root, err := Inspector{}.Root(context.Background(), path)
		if err != nil || root != repo {
			t.Errorf("Root(%s) = %q, %v; want %q", path, root, err, repo)
		}
	}
}

func TestRootOutsideRepository(t *testing.T) {
	if _, err := (Inspector{}).Root(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected an error outside a repository")
	}
}
