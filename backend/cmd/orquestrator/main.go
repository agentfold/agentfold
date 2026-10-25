// Command orquestrator is the single binary for the board server, workers and admin tasks.
package main

import (
	"os"

	"orquestrator/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
