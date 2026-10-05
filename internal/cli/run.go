package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// Exit codes returned by Run.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
	// exitNoHost is what status returns when no host is running.
	exitNoHost = 3
)

const usage = "usage: " + BinaryName + " <command>\n\n" +
	"commands:\n" +
	"  mcp      serve the presence tools to Claude over standard input and output\n" +
	"  status   show the running presence host's summary; exits 3 if none runs\n" +
	"  doctor   check the setup and report what to fix\n" +
	"  version  print the version\n" +
	"  help     print this text\n"

// Run executes one command and returns the process exit code. It receives
// everything it needs from the process as parameters, so that tests call it
// directly and main stays a single statement.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	return realSystem().run(args, stdin, stdout, stderr, getenv)
}

// run is Run over a system. A panic that reaches here ends the command with
// a fixed message: what it carried may be work content, and in the mcp
// command standard output belongs to the protocol (ADR-0008).
func (s system) run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) (code int) {
	defer func() {
		if recover() != nil {
			fmt.Fprintf(stderr, "%s: internal error\n", BinaryName)
			code = exitFailure
		}
	}()
	if len(args) == 0 {
		return help(stdout)
	}
	rest := args[1:]
	switch args[0] {
	case "mcp":
		return s.runMCP(rest, stdin, stdout, stderr, getenv)
	case "status":
		return s.runStatus(rest, stdout, stderr, getenv)
	case "doctor":
		return s.runDoctor(rest, stdout, stderr, getenv)
	case "version":
		return s.runVersion(rest, stdout, stderr)
	case "help", "-h", "-help", "--help":
		return help(stdout)
	}
	return usageError(stderr, fmt.Sprintf("unknown command %q", args[0]))
}

func help(stdout io.Writer) int {
	fmt.Fprint(stdout, usage)
	return exitOK
}

func (s system) runVersion(args []string, stdout, stderr io.Writer) int {
	if code, ok := parseFlags("version", args, stderr); !ok {
		return code
	}
	fmt.Fprintf(stdout, "%s %s\n", BinaryName, s.version)
	return exitOK
}

func usageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "%s: %s\n%s", BinaryName, problem, usage)
	return exitUsage
}

// parseFlags reads the flags of a command, of which there are none yet. It
// returns false, with the exit code, when the command is not to go on: the
// arguments were wrong, or help was asked for.
func parseFlags(command string, args []string, stderr io.Writer) (code int, ok bool) {
	flags := flag.NewFlagSet(BinaryName+" "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprintf(stderr, "usage: %s %s\n", BinaryName, command) }
	err := flags.Parse(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return exitOK, false
	case err != nil:
		return exitUsage, false
	case flags.NArg() > 0:
		return usageError(stderr, fmt.Sprintf("%s takes no arguments", command)), false
	}
	return exitOK, true
}
