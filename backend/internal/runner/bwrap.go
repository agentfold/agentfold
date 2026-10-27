package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"orquestrator/internal/workspace"
)

// Sandbox paths of the card directories.
const (
	SandboxWorkspace = "/workspace"
	SandboxClaude    = "/claude"
	sandboxHome      = "/home/agent"
)

// Bwrap runs the process in a bubblewrap sandbox: the host /usr and /etc
// read-only, a private home and /tmp, the network shared, and writable only
// the card workspace (/workspace), its claude directory (/claude) and the
// git directory of the project (at its host path, which the worktree
// points to).
type Bwrap struct {
	// Path of the bwrap binary.
	Path string
	// ReadOnly lists host paths mounted read-only at the same place, e.g.
	// the install directory of the agent CLI (see CLIMounts).
	ReadOnly []string
}

// Layout returns the sandbox paths of the card directories.
func (Bwrap) Layout(workspace.Workspace) Layout {
	return Layout{Workspace: SandboxWorkspace, Claude: SandboxClaude}
}

// Args builds the bwrap command line for spec.
func (b Bwrap) Args(spec Spec) []string {
	args := []string{
		"--die-with-parent", "--new-session",
		"--unshare-all", "--share-net",
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/etc", "/etc",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--dir", sandboxHome,
	}
	args = append(args, rootDirs("/")...)
	args = append(args, resolvConf("/")...)
	for _, path := range b.ReadOnly {
		args = append(args, "--ro-bind", path, path)
	}
	args = append(args,
		"--bind", spec.Workspace.Dir, SandboxWorkspace,
		"--bind", spec.Workspace.ClaudeDir, SandboxClaude,
	)
	if spec.Workspace.GitDir != "" {
		args = append(args, "--bind", spec.Workspace.GitDir, spec.Workspace.GitDir)
	}
	args = append(args, "--chdir", SandboxWorkspace, "--clearenv")
	for _, kv := range append(append(baseEnv(), "HOME="+sandboxHome), spec.Env...) {
		key, value := splitEnv(kv)
		args = append(args, "--setenv", key, value)
	}
	return append(append(args, "--", spec.Binary), spec.Args...)
}

// Run starts the sandboxed process and waits for it.
func (b Bwrap) Run(ctx context.Context, spec Spec, onLine func([]byte)) error {
	cmd := exec.CommandContext(ctx, b.Path, b.Args(spec)...) //nolint:gosec // fixed sandbox binary
	return start(cmd, onLine)
}

// rootDirs mounts /bin, /lib… as on the host under root: merged-/usr
// systems link them into /usr, others have them as directories.
func rootDirs(root string) []string {
	var args []string
	for _, dir := range []string{"/bin", "/sbin", "/lib", "/lib32", "/lib64"} {
		host := filepath.Join(root, dir)
		if target, err := os.Readlink(host); err == nil {
			args = append(args, "--symlink", target, dir)
		} else if _, err := os.Stat(host); err == nil {
			args = append(args, "--ro-bind", host, dir)
		}
	}
	return args
}

// resolvConf mounts the file /etc/resolv.conf links to under root, when it
// lives outside /etc and /usr (systemd-resolved keeps it in /run): without
// it the agent cannot resolve its API host.
func resolvConf(root string) []string {
	target, err := filepath.EvalSymlinks(filepath.Join(root, "etc", "resolv.conf"))
	if err != nil {
		return nil
	}
	rel, _ := filepath.Rel(root, target) // both are under root
	inside := "/" + rel
	for _, dir := range []string{"/etc/", "/usr/"} {
		if strings.HasPrefix(inside, dir) {
			return nil
		}
	}
	return []string{"--ro-bind", target, inside}
}

func splitEnv(kv string) (string, string) {
	for i := range len(kv) {
		if kv[i] == '=' {
			return kv[:i], kv[i+1:]
		}
	}
	return kv, ""
}
