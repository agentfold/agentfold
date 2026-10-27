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

	"orquestrator/internal/domain"
	"orquestrator/internal/events"
	"orquestrator/internal/gitrepo"
	"orquestrator/internal/service"
	"orquestrator/internal/store"
)

// stdin is where `agent login` reads the pasted token; tests replace it.
var stdin io.Reader = os.Stdin

const agentUsage = "usage: orquestrator agent login claude [--profile name]\n" +
	"       orquestrator agent login opencode [--profile name] [--integration id]"

func runAgent(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 2 || args[0] != "login" || (args[1] != "claude" && args[1] != "opencode") {
		return errors.New(agentUsage)
	}
	flags := flag.NewFlagSet("agent login "+args[1], flag.ContinueOnError)
	flags.SetOutput(stderr)
	profile := flags.String("profile", "default", "name of the agent profile to log in")
	integration := ""
	if args[1] == "opencode" {
		flags.StringVar(&integration, "integration", "", "opencode integration to copy (all when empty), e.g. openrouter")
	}
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return errors.New(agentUsage)
	}
	if args[1] == "opencode" {
		return loginOpencode(ctx, *profile, integration, stdout, stderr)
	}
	return loginClaude(ctx, *profile, stdout, stderr)
}

// boardFor opens the board to store a profile credential.
func boardFor(ctx context.Context) (*service.Service, func(), error) {
	box, err := secretBox()
	if err != nil {
		return nil, nil, err
	}
	pool, err := connect(ctx)
	if err != nil {
		return nil, nil, err
	}
	return service.New(store.New(pool), gitrepo.Inspector{}, events.NewHub(1), box), pool.Close, nil
}

// loginOpencode copies the credentials of the user's own opencode login
// (`opencode auth export`) into the profile, sealed: log in to the
// provider with `opencode auth login` first. The secrets never show on
// the terminal.
func loginOpencode(ctx context.Context, profile, integration string, stdout, stderr io.Writer) error {
	opencode, err := opencodeBinary()
	if err != nil {
		return err
	}
	board, closeBoard, err := boardFor(ctx)
	if err != nil {
		return err
	}
	defer closeBoard()

	args := []string{"auth", "export"}
	if integration != "" {
		args = append(args, integration)
	}
	cmd := exec.CommandContext(ctx, opencode, args...) //nolint:gosec // the configured opencode CLI
	cmd.Stderr = stderr
	exported, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("opencode auth export: %w", err)
	}
	p, err := board.LoginAgent(ctx, domain.AgentOpencode, profile, string(exported))
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "Agent profile %q is ready (opencode credentials copied).\n", p.Name)
	return nil
}

// loginClaude runs `claude setup-token` in a throwaway config directory (so
// the user's own Claude login is untouched), then stores the token the user
// pastes, sealed, in the profile.
func loginClaude(ctx context.Context, profile string, stdout, stderr io.Writer) error {
	claude, err := claudeBinary()
	if err != nil {
		return err
	}
	board, closeBoard, err := boardFor(ctx)
	if err != nil {
		return err
	}
	defer closeBoard()

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
	p, err := board.LoginAgent(ctx, domain.AgentClaude, profile, line)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "Agent profile %q is ready (%s).\n", p.Name, p.CredentialKind)
	return nil
}
