// Package runner starts agent CLIs, directly on the host or in a sandbox.
// Inside a sandbox the card directories have fixed paths (/workspace,
// /claude), so sessions resume the same way on any host.
package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"orquestrator/internal/workspace"
)

// Spec is a process to run for a card.
type Spec struct {
	// Binary is the path of the CLI on the host.
	Binary string
	Args   []string
	// Env holds KEY=VALUE pairs added to a minimal environment.
	Env       []string
	Workspace workspace.Workspace
}

// Layout tells where the card directories are as the process sees them.
type Layout struct {
	Workspace string
	Claude    string
}

// Runner runs a process for a card and streams its stdout, line by line.
type Runner interface {
	Layout(ws workspace.Workspace) Layout
	Run(ctx context.Context, spec Spec, onLine func(line []byte)) error
}

// ExitError reports a process that ended unsuccessfully, with the end of
// its stderr.
type ExitError struct {
	Code   int
	Stderr string
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d: %s", e.Code, e.Stderr)
}

// maxLine bounds a stdout line (stream-json events can be large). A
// variable so tests can lower it.
var maxLine = 64 << 20

// stderrTail keeps the end of stderr for error messages.
const stderrTail = 4 << 10

// baseEnv is the host environment passed to agents: just enough to run.
func baseEnv() []string {
	var env []string
	for _, key := range []string{"PATH", "LANG", "LC_ALL", "TZ"} {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	return env
}

// start runs cmd, sending each stdout line to onLine, and waits for it.
func start(cmd *exec.Cmd, onLine func([]byte)) error {
	stdout, _ := cmd.StdoutPipe() // fails only if Stdout is already set
	var stderr tail
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, min(64<<10, maxLine)), maxLine)
	for scanner.Scan() {
		onLine(scanner.Bytes())
	}
	scanErr := scanner.Err()
	if scanErr != nil {
		_, _ = io.Copy(io.Discard, stdout) // let the process finish writing
	}
	err := cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return &ExitError{Code: exit.ExitCode(), Stderr: stderr.String()}
	}
	return errors.Join(err, scanErr)
}

// CLIMounts lists the host directories an agent CLI needs inside a
// sandbox: the directory of its (resolved) binary and, for npm installs,
// the package tree around it, since npm shims exec files elsewhere in it.
func CLIMounts(binary string) ([]string, error) {
	resolved, err := filepath.EvalSymlinks(binary)
	if err != nil {
		return nil, err
	}
	mounts := []string{filepath.Dir(resolved)}
	for dir := filepath.Dir(resolved); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if filepath.Base(dir) == "node_modules" {
			mounts = append(mounts, filepath.Dir(dir))
			break
		}
	}
	return mounts, nil
}

// tail is an io.Writer that keeps the last stderrTail bytes.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > stderrTail {
		t.buf = t.buf[len(t.buf)-stderrTail:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(bytes.TrimSpace(t.buf))
}
