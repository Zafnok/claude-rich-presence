// Command policycheck enforces the parts of the dependency policy of
// ADR-0003 that no third-party tool checks.
//
//	policycheck modules allowlist
//
// reads the output of go list -deps -test -json from standard input and fails
// when a package comes from a module that is neither under golang.org/x/ nor
// named, with the ADR that approved it, in the allowlist file. Each line of
// the file is a module path and an ADR number, such as ADR-0017. Blank lines
// and lines starting with # are ignored.
//
//	policycheck actions workflow...
//
// reads GitHub workflow files and fails unless every action or reusable
// workflow they use is pinned to a full commit hash.
package main

import (
	"fmt"
	"io"
	"os"
)

// Exit codes returned by run.
const (
	exitOK       = 0
	exitViolated = 1
	exitUsage    = 2
)

const usage = "usage: policycheck modules allowlist < go-list-json\n       policycheck actions workflow...\n"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.ReadFile))
}

// run executes one command and returns the process exit code.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, readFile func(string) ([]byte, error)) int {
	if len(args) == 0 {
		return usageError(stderr, "no command given")
	}
	switch args[0] {
	case "modules":
		return runModules(args[1:], stdin, stdout, stderr, readFile)
	case "actions":
		return runActions(args[1:], stdout, stderr, readFile)
	default:
		return usageError(stderr, fmt.Sprintf("unknown command %q", args[0]))
	}
}

func usageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "policycheck: %s\n%s", problem, usage)
	return exitUsage
}
