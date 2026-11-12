// Command lacarta is the CLI, hooks, daemon and console of the lacarta framework.
package main

import (
	"os"

	"lacarta/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
