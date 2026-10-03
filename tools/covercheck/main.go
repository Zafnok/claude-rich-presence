// Command covercheck is the coverage gate of ADR-0009. It holds the one
// definition of the measured set, which is every package of the module except
// those under internal/testutil.
//
//	covercheck packages
//
// reads import paths from standard input, one per line, and prints the
// measured ones joined by commas, ready for the -coverpkg flag of go test.
//
//	covercheck check [-module path] profile...
//
// reads Go coverage profiles in text form, treats a block as covered if any
// profile covers it, prints every uncovered block as file:line, and fails
// unless every measured statement is covered.
package main

import (
	"bufio"
	"cmp"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
)

// Exit codes returned by run.
const (
	exitOK    = 0
	exitBelow = 1
	exitUsage = 2
)

const usage = "usage: covercheck packages\n       covercheck check [-module path] profile...\n"

// unmeasured is the only directory left out of the measured set. It holds
// test helpers, which are test code (ADR-0009).
const unmeasured = "/internal/testutil/"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.ReadFile))
}

// run executes one command and returns the process exit code.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, readFile func(string) ([]byte, error)) int {
	if len(args) == 0 {
		return usageError(stderr, "no command given")
	}
	switch args[0] {
	case "packages":
		return runPackages(stdin, stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr, readFile)
	default:
		return usageError(stderr, fmt.Sprintf("unknown command %q", args[0]))
	}
}

func usageError(stderr io.Writer, problem string) int {
	fmt.Fprintf(stderr, "covercheck: %s\n%s", problem, usage)
	return exitUsage
}

// measured reports whether the package with this import path is in the
// measured set.
func measured(pkg string) bool {
	return !strings.Contains("/"+pkg+"/", unmeasured)
}

func runPackages(stdin io.Reader, stdout, stderr io.Writer) int {
	var keep []string
	lines := bufio.NewScanner(stdin)
	for lines.Scan() {
		if pkg := strings.TrimSpace(lines.Text()); pkg != "" && measured(pkg) {
			keep = append(keep, pkg)
		}
	}
	if err := lines.Err(); err != nil {
		fmt.Fprintf(stderr, "covercheck: reading the package list: %v\n", err)
		return exitUsage
	}
	// An empty -coverpkg would make go test fall back to its default and
	// measure less than the gate claims.
	if len(keep) == 0 {
		fmt.Fprintln(stderr, "covercheck: no measured package on standard input")
		return exitUsage
	}
	fmt.Fprintln(stdout, strings.Join(keep, ","))
	return exitOK
}

func runCheck(args []string, stdout, stderr io.Writer, readFile func(string) ([]byte, error)) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	module := flags.String("module", "", "module path, trimmed from the file names printed")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() == 0 {
		return usageError(stderr, "check needs at least one profile")
	}
	merged := profile{}
	for _, name := range flags.Args() {
		data, err := readFile(name)
		if err == nil {
			err = merged.add(data)
		}
		if err != nil {
			fmt.Fprintf(stderr, "covercheck: %s: %v\n", name, err)
			return exitUsage
		}
	}
	if !merged.report(stdout, *module) {
		return exitBelow
	}
	return exitOK
}

// block is one basic block of a source file, as a coverage profile names it.
type block struct {
	file                string
	startLine, startCol uint32
	endLine, endCol     uint32
	statements          uint32
}

// profile records, for each block seen, whether any profile covered it.
type profile map[block]bool

// add merges one coverage profile in Go's text form into p.
func (p profile) add(data []byte) error {
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		b, count, err := parseBlock(line)
		if err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
		// A block with no statements has nothing to cover.
		if measured(path.Dir(b.file)) && b.statements > 0 {
			p[b] = p[b] || count > 0
		}
	}
	return nil
}

// parseBlock decodes one profile line, which has the form
//
//	file:startLine.startCol,endLine.endCol statements count
func parseBlock(line string) (block, uint64, error) {
	colon := strings.LastIndexByte(line, ':')
	if colon <= 0 {
		return block{}, 0, fmt.Errorf("no file name in %q", line)
	}
	b := block{file: line[:colon]}
	var count uint64
	_, err := fmt.Sscanf(line[colon+1:], "%d.%d,%d.%d %d %d",
		&b.startLine, &b.startCol, &b.endLine, &b.endCol, &b.statements, &count)
	if err != nil {
		return block{}, 0, fmt.Errorf("malformed block %q", line)
	}
	return b, count, nil
}

// totals returns the number of covered statements and of all statements.
func (p profile) totals() (covered, total uint64) {
	for b, hit := range p {
		total += uint64(b.statements)
		if hit {
			covered += uint64(b.statements)
		}
	}
	return covered, total
}

// report prints every uncovered block as file:line, then the total, and
// reports whether every statement is covered. A profile with no statements
// does not pass: nothing was measured.
func (p profile) report(w io.Writer, module string) bool {
	var missed []block
	for b, hit := range p {
		if !hit {
			missed = append(missed, b)
		}
	}
	slices.SortFunc(missed, func(a, b block) int {
		return cmp.Or(
			cmp.Compare(a.file, b.file),
			cmp.Compare(a.startLine, b.startLine),
			cmp.Compare(a.startCol, b.startCol),
			cmp.Compare(a.endLine, b.endLine),
			cmp.Compare(a.endCol, b.endCol),
			cmp.Compare(a.statements, b.statements),
		)
	})
	for _, b := range missed {
		fmt.Fprintf(w, "%s:%d: not covered\n", strings.TrimPrefix(b.file, module+"/"), b.startLine)
	}
	covered, total := p.totals()
	if total == 0 {
		fmt.Fprintln(w, "coverage: no statements were measured")
		return false
	}
	// Tenths of a percent, rounded down, so that anything short of full
	// coverage never prints as 100.0%.
	tenths := covered * 1000 / total
	fmt.Fprintf(w, "coverage: %d.%d%% of statements (%d of %d)\n", tenths/10, tenths%10, covered, total)
	return covered == total
}
