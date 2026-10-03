// Command main is the single binary of the product. It holds no logic:
// everything is in internal/cli, where it can be tested.
package main

import (
	"os"

	"github.com/Zafnok/claude-rich-presence/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}
