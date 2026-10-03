package cli

import (
	"fmt"
	"io"
	"runtime/debug"
)

// Exit codes returned by Run.
const (
	exitOK    = 0
	exitUsage = 2
)

const usage = "usage: " + BinaryName + " <command>\n\ncommands:\n  version  print the version\n"

// Run executes one command and returns the process exit code. It receives
// everything it needs from the process as parameters, so that tests call it
// directly and main stays a single statement.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		return usageError(stderr, "no command given")
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "%s %s\n", BinaryName, resolveVersion(version, debug.ReadBuildInfo))
		return exitOK
	default:
		return usageError(stderr, fmt.Sprintf("unknown command %q", args[0]))
	}
}

func usageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "%s: %s\n%s", BinaryName, problem, usage)
	return exitUsage
}
