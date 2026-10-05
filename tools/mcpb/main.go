// Command mcpb assembles and checks the MCPB bundle of ADR-0007: the one
// archive that Claude Code fetches through the plugin and Claude Desktop
// installs as an extension.
//
//	mcpb build -version V -manifest F -icon F -license F -notices F \
//	    -windows F -darwin F -linux F -out F
//	mcpb check -version V bundle
//
// build checks what it assembled before it writes anything, so a bundle that
// fails check is never produced. The archive is deterministic: entries are in
// a fixed order, with fixed times and modes, so the same inputs give the same
// bytes.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Exit codes returned by run.
const (
	exitOK       = 0
	exitProblems = 1
	exitUsage    = 2
)

const usage = "usage: mcpb build -version V -manifest F -icon F -license F -notices F -windows F -darwin F -linux F -out F\n" +
	"       mcpb check -version V bundle\n"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.ReadFile, writeFile))
}

func writeFile(name string, data []byte) error {
	return os.WriteFile(name, data, 0o644)
}

// run executes one command and returns the process exit code.
func run(args []string, stdout, stderr io.Writer, readFile func(string) ([]byte, error), write func(string, []byte) error) int {
	if len(args) == 0 {
		return usageError(stderr, "no command given")
	}
	switch args[0] {
	case "build":
		return runBuild(args[1:], stdout, stderr, readFile, write)
	case "check":
		return runCheck(args[1:], stdout, stderr, readFile)
	default:
		return usageError(stderr, fmt.Sprintf("unknown command %q", args[0]))
	}
}

func usageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "mcpb: %s\n%s", problem, usage)
	return exitUsage
}

// reportProblems prints each problem and returns the exit code for them.
func reportProblems(stderr io.Writer, what string, problems []string) int {
	for _, p := range problems {
		fmt.Fprintf(stderr, "mcpb: %s: %s\n", what, p)
	}
	return exitProblems
}

func runBuild(args []string, stdout, stderr io.Writer, readFile func(string) ([]byte, error), write func(string, []byte) error) int {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	version := flags.String("version", "", "version written into the manifest")
	out := flags.String("out", "", "bundle file to write")
	sources := map[string]*string{}
	for _, name := range sourceNames {
		sources[name] = flags.String(name, "", "source file of the "+name)
	}
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() != 0 {
		return usageError(stderr, "build takes flags only")
	}
	if *version == "" || *out == "" {
		return usageError(stderr, "build needs -version and -out")
	}
	files := map[string][]byte{}
	for _, name := range sourceNames {
		path := sources[name]
		if *path == "" {
			return usageError(stderr, "build needs -"+name)
		}
		data, err := readFile(*path)
		if err != nil {
			fmt.Fprintf(stderr, "mcpb: %s: %v\n", name, err)
			return exitUsage
		}
		files[name] = data
	}
	archive, problems := assemble(*version, files)
	if len(problems) > 0 {
		return reportProblems(stderr, "build", problems)
	}
	if err := write(*out, archive); err != nil {
		fmt.Fprintf(stderr, "mcpb: %v\n", err)
		return exitUsage
	}
	fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", *out, len(archive))
	return exitOK
}

func runCheck(args []string, stdout, stderr io.Writer, readFile func(string) ([]byte, error)) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	version := flags.String("version", "", "version the bundle must carry")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() != 1 || *version == "" {
		return usageError(stderr, "check needs -version and one bundle")
	}
	name := flags.Arg(0)
	data, err := readFile(name)
	if err != nil {
		fmt.Fprintf(stderr, "mcpb: %v\n", err)
		return exitUsage
	}
	if problems := check(data, *version); len(problems) > 0 {
		return reportProblems(stderr, name, problems)
	}
	fmt.Fprintf(stdout, "%s: ok\n", name)
	return exitOK
}

// substituteVersion returns the manifest with the version token replaced, or
// a problem if the token is not there exactly once.
func substituteVersion(manifest []byte, version string) ([]byte, string) {
	text := string(manifest)
	if strings.Count(text, versionToken) != 1 {
		return nil, "the manifest must contain " + versionToken + " exactly once"
	}
	return []byte(strings.Replace(text, versionToken, version, 1)), ""
}
