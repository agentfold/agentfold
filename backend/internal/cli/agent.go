package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"

	"orquestrator/internal/events"
	"orquestrator/internal/gitrepo"
	"orquestrator/internal/service"
	"orquestrator/internal/store"
)

// stdin is where `agent login` reads the pasted token; tests replace it.
var stdin io.Reader = os.Stdin

const agentUsage = "usage: orquestrator agent login claude [--profile name]"

func runAgent(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 2 || args[0] != "login" || args[1] != "claude" {
		return errors.New(agentUsage)
	}
	flags := flag.NewFlagSet("agent login claude", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profile := flags.String("profile", "default", "name of the agent profile to log in")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return errors.New(agentUsage)
	}
	return loginClaude(ctx, *profile, stdout, stderr)
}

// loginClaude runs `claude setup-token` in a throwaway config directory (so
// the user's own Claude login is untouched), then stores the token the user
// pastes, sealed, in the profile.
func loginClaude(ctx context.Context, profile string, stdout, stderr io.Writer) error {
	box, err := secretBox()
	if err != nil {
		return err
	}
	claude, err := claudeBinary()
	if err != nil {
		return err
	}
	pool, err := connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	board := service.New(store.New(pool), gitrepo.Inspector{}, events.NewHub(1), box)

	config, err := os.MkdirTemp("", "orquestrator-login-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(config) }()

	_, _ = fmt.Fprintf(stdout, "Logging in agent profile %q. Follow `claude setup-token` below.\n\n", profile)
	cmd := exec.CommandContext(ctx, claude, "setup-token") //nolint:gosec // the configured claude CLI
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+config)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Fprintf(stderr, "\nclaude setup-token did not finish (%v); you can still paste an API key.\n", err)
	}

	_, _ = fmt.Fprint(stdout, "\nPaste the token shown above (or an Anthropic API key) and press Enter: ")
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("no token read: %w", err)
	}
	p, err := board.LoginAgent(ctx, profile, line)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "Agent profile %q is ready (%s).\n", p.Name, p.CredentialKind)
	return nil
}
