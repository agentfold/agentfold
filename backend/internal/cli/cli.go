// Package cli dispatches the orquestrator subcommands.
package cli

import (
	"errors"
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

// ErrNotImplemented is returned by subcommands that are not built yet.
var ErrNotImplemented = errors.New("not implemented yet")

type command struct {
	name    string
	summary string
	run     func(args []string, stdout io.Writer) error
}

func notImplemented([]string, io.Writer) error { return ErrNotImplemented }

var commands = []command{
	{name: "server", summary: "run the API, MCP server and flow engine", run: notImplemented},
	{name: "worker", summary: "run a worker that executes agent jobs", run: notImplemented},
	{name: "migrate", summary: "apply database migrations", run: notImplemented},
	{name: "agent", summary: "manage agent profiles (agent login claude)", run: notImplemented},
}

// Run executes the subcommand named by args[0] and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
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
		if err := c.run(args[1:], stdout); err != nil {
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
