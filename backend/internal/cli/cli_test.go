package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "no args prints usage", args: nil, wantCode: ExitUsage, wantStderr: "usage: orquestrator"},
		{name: "help", args: []string{"help"}, wantCode: ExitOK, wantStdout: "commands:"},
		{name: "short help flag", args: []string{"-h"}, wantCode: ExitOK, wantStdout: "server"},
		{name: "long help flag", args: []string{"--help"}, wantCode: ExitOK, wantStdout: "worker"},
		{name: "unknown command", args: []string{"nope"}, wantCode: ExitUsage, wantStderr: `unknown command "nope"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := Run(context.Background(), tt.args, &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRunSuccessfulCommand(t *testing.T) {
	original := commands
	t.Cleanup(func() { commands = original })

	var gotArgs []string
	commands = []command{{name: "ok", run: func(_ context.Context, args []string, _, _ io.Writer) error {
		gotArgs = args
		return nil
	}}}

	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"ok", "a", "b"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("exit code = %d, want %d", code, ExitOK)
	}
	if strings.Join(gotArgs, " ") != "a b" {
		t.Errorf("args = %v, want [a b]", gotArgs)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}
