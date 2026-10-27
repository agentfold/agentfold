package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"orquestrator/internal/workspace"
)

// script writes an executable shell script and returns its path.
func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func newWorkspace(t *testing.T) workspace.Workspace {
	t.Helper()
	root := t.TempDir()
	ws := workspace.Workspace{
		Dir:       filepath.Join(root, "workspace"),
		ClaudeDir: filepath.Join(root, "claude"),
		LogsDir:   filepath.Join(root, "logs"),
	}
	for _, d := range []string{ws.Dir, ws.ClaudeDir, ws.LogsDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return ws
}

func collect(lines *[]string) func([]byte) {
	return func(b []byte) { *lines = append(*lines, string(b)) }
}

func TestLocalRunsInTheWorkspace(t *testing.T) {
	t.Setenv("SECRET_FROM_HOST", "leak")
	ws := newWorkspace(t)
	bin := script(t, `pwd; echo "$HOME"; echo "$FOO"; echo "${SECRET_FROM_HOST:-clean}"; echo "$1"`)
	r := Local{Home: "/tmp/home"}
	if got := r.Layout(ws); got != (Layout{Workspace: ws.Dir, Claude: ws.ClaudeDir}) {
		t.Fatalf("Layout = %+v", got)
	}
	var lines []string
	err := r.Run(context.Background(), Spec{Binary: bin, Args: []string{"arg"}, Env: []string{"FOO=bar"}, Workspace: ws}, collect(&lines))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ws.Dir, "/tmp/home", "bar", "clean", "arg"}
	if !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestExitErrorsKeepTheEndOfStderr(t *testing.T) {
	ws := newWorkspace(t)
	bin := script(t, `i=0; while [ $i -lt 2000 ]; do echo "line $i" >&2; i=$((i+1)); done; echo out; exit 3`)
	var lines []string
	err := Local{}.Run(context.Background(), Spec{Binary: bin, Workspace: ws}, collect(&lines))
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 3 {
		t.Fatalf("got %v", err)
	}
	if len(exit.Stderr) > stderrTail || !strings.HasSuffix(exit.Stderr, "line 1999") || strings.Contains(exit.Stderr, "line 0\n") {
		t.Fatalf("stderr tail %d bytes: %q…", len(exit.Stderr), exit.Stderr[:40])
	}
	if !strings.HasPrefix(err.Error(), "exit status 3: ") || !slices.Equal(lines, []string{"out"}) {
		t.Fatalf("message %q, lines %q", err.Error(), lines)
	}
}

func TestStartFailures(t *testing.T) {
	ws := newWorkspace(t)
	if err := (Local{}).Run(context.Background(), Spec{Binary: "/does/not/exist", Workspace: ws}, func([]byte) {}); err == nil {
		t.Error("missing binary: expected error")
	}

	old := maxLine
	maxLine = 16
	t.Cleanup(func() { maxLine = old })
	bin := script(t, `echo short; echo this-line-is-much-longer-than-sixteen-bytes; echo after`)
	var lines []string
	err := Local{}.Run(context.Background(), Spec{Binary: bin, Workspace: ws}, collect(&lines))
	if err == nil || !slices.Equal(lines, []string{"short"}) {
		t.Fatalf("long line: %v, lines %q", err, lines)
	}
}

func TestCLIMounts(t *testing.T) {
	native := script(t, "")
	mounts, err := CLIMounts(native)
	if err != nil || !slices.Equal(mounts, []string{filepath.Dir(native)}) {
		t.Fatalf("native: %v, %v", mounts, err)
	}

	root := t.TempDir()
	bin := filepath.Join(root, "pkg", "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bin, "claude")
	if err := os.Symlink(native, link); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(bin, "shim")
	if err := os.WriteFile(shim, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if mounts, _ := CLIMounts(link); !slices.Equal(mounts, []string{filepath.Dir(native)}) {
		t.Fatalf("symlink: %v", mounts)
	}
	if mounts, _ := CLIMounts(shim); !slices.Equal(mounts, []string{bin, filepath.Join(root, "pkg")}) {
		t.Fatalf("npm shim: %v", mounts)
	}
	if _, err := CLIMounts(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing binary: expected error")
	}
}

func TestBwrapArgs(t *testing.T) {
	ws := newWorkspace(t)
	ws.GitDir = "/repo/.git"
	b := Bwrap{Path: "bwrap", ReadOnly: []string{"/opt/cli"}}
	if got := b.Layout(ws); got != (Layout{Workspace: "/workspace", Claude: "/claude"}) {
		t.Fatalf("Layout = %+v", got)
	}
	args := strings.Join(b.Args(Spec{Binary: "/opt/cli/claude", Args: []string{"-p", "x"}, Env: []string{"A=1", "NOVALUE"}, Workspace: ws}), " ")
	for _, want := range []string{
		"--unshare-all --share-net",
		"--ro-bind /usr /usr",
		"--ro-bind /opt/cli /opt/cli",
		"--bind " + ws.Dir + " /workspace",
		"--bind " + ws.ClaudeDir + " /claude",
		"--bind /repo/.git /repo/.git",
		"--chdir /workspace --clearenv",
		"--setenv HOME /home/agent",
		"--setenv A 1",
		"--setenv NOVALUE ",
		"-- /opt/cli/claude -p x",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in %s", want, args)
		}
	}
	ws.GitDir = ""
	if args := strings.Join(b.Args(Spec{Binary: "/bin/true", Workspace: ws}), " "); strings.Contains(args, ".git") {
		t.Errorf("unexpected git mount: %s", args)
	}
}

func TestRootDirs(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("usr/bin", filepath.Join(root, "bin")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "lib"), 0o700); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(rootDirs(root), " ")
	want := "--symlink usr/bin /bin --ro-bind " + filepath.Join(root, "lib") + " /lib"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolvConf(t *testing.T) {
	root := t.TempDir()
	if got := resolvConf(root); got != nil {
		t.Fatalf("no resolv.conf: %q", got)
	}
	for _, dir := range []string{"etc", "run/resolve"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stub := filepath.Join(root, "run", "resolve", "stub.conf")
	if err := os.WriteFile(stub, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "etc", "resolv.conf")
	if err := os.Symlink("../run/resolve/stub.conf", link); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(resolvConf(root), " "); got != "--ro-bind "+stub+" /run/resolve/stub.conf" {
		t.Fatalf("linked: %q", got)
	}
	_ = os.Remove(link)
	if err := os.WriteFile(link, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := resolvConf(root); got != nil {
		t.Fatalf("plain file under /etc: %q", got)
	}
}

func TestBwrapSandbox(t *testing.T) {
	path, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("bwrap not installed")
	}
	ws := newWorkspace(t)
	bin := script(t, `pwd; echo "$HOME"; touch /workspace/made; ls /claude >/dev/null && echo claude-ok; touch /usr/x 2>/dev/null || echo usr-ro`)
	var lines []string
	b := Bwrap{Path: path, ReadOnly: []string{filepath.Dir(bin)}}
	if err := b.Run(context.Background(), Spec{Binary: bin, Workspace: ws}, collect(&lines)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/workspace", "/home/agent", "claude-ok", "usr-ro"}; !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, "made")); err != nil {
		t.Fatal("the workspace is not writable through the sandbox")
	}
}

func TestBwrapMissing(t *testing.T) {
	b := Bwrap{Path: "/does/not/exist"}
	if err := b.Run(context.Background(), Spec{Binary: "/bin/true", Workspace: newWorkspace(t)}, func([]byte) {}); err == nil {
		t.Fatal("expected error")
	}
}
