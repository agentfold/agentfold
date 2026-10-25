// Command orquestrator is the single binary for the board server, workers and admin tasks.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"orquestrator/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
