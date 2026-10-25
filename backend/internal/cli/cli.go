// Package cli dispatches the orquestrator subcommands.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Exit codes returned by Run.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string, stdout, stderr io.Writer) error
}

var commands = []command{
	{name: "server", summary: "run the API, MCP server and flow engine", run: runServer},
	{name: "worker", summary: "run a worker that executes agent jobs", run: runWorker},
	{name: "migrate", summary: "apply database migrations", run: runMigrate},
	{name: "agent", summary: "log in agent profiles (agent login claude --profile name)", run: runAgent},
	{name: "secret-key", summary: "print a new random ORQ_SECRET_KEY", run: runSecretKey},
}

// Run executes the subcommand named by args[0] and returns the process exit
// code. Long-running subcommands stop when ctx is cancelled.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return ExitUsage
	}

	name := args[0]
	if name == "help" || name == "-h" || name == "--help" {
		writeUsage(stdout)
		return ExitOK
	}

	for _, c := range commands {
		if c.name != name {
			continue
		}
		if err := c.run(ctx, args[1:], stdout, stderr); err != nil {
			_, _ = fmt.Fprintf(stderr, "orquestrator %s: %v\n", name, err)
			return ExitError
		}
		return ExitOK
	}

	_, _ = fmt.Fprintf(stderr, "orquestrator: unknown command %q\n\n", name)
	writeUsage(stderr)
	return ExitUsage
}

func writeUsage(w io.Writer) {
	var b strings.Builder
	b.WriteString("usage: orquestrator <command> [args]\n\ncommands:\n")
	for _, c := range commands {
		_, _ = fmt.Fprintf(&b, "  %-8s %s\n", c.name, c.summary)
	}
	_, _ = io.WriteString(w, b.String())
}
