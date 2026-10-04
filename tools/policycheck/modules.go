package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// preapproved is the module path prefix that needs no ADR: the modules the Go
// project maintains (ADR-0003).
const preapproved = "golang.org/x/"

// adrReference is how an allowlist entry names the ADR that approved it.
var adrReference = regexp.MustCompile(`^ADR-[0-9]{4}$`)

// parseAllowlist reads an allowlist file into a map from module path to the
// ADR that approved it.
func parseAllowlist(data []byte) (map[string]string, error) {
	allowed := map[string]string{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || !adrReference.MatchString(fields[1]) {
			return nil, fmt.Errorf("line %d: want a module path and an ADR number such as ADR-0017, got %q", i+1, line)
		}
		allowed[fields[0]] = fields[1]
	}
	return allowed, nil
}

// unapproved decodes the output of go list -deps -test -json and returns the
// sorted paths of the modules that supply a package and are not approved. The
// standard library and the main module are not modules to approve.
func unapproved(listing io.Reader, allowed map[string]string) ([]string, error) {
	seen := map[string]bool{}
	decoder := json.NewDecoder(listing)
	for {
		var pkg struct {
			Module *struct {
				Path string
				Main bool
			}
		}
		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if pkg.Module == nil || pkg.Module.Main {
			continue
		}
		_, listed := allowed[pkg.Module.Path]
		if !listed && !strings.HasPrefix(pkg.Module.Path, preapproved) {
			seen[pkg.Module.Path] = true
		}
	}
	return slices.Sorted(maps.Keys(seen)), nil
}

func runModules(args []string, stdin io.Reader, stdout, stderr io.Writer, readFile func(string) ([]byte, error)) int {
	if len(args) != 1 {
		return usageError(stderr, "modules needs exactly one allowlist file")
	}
	data, err := readFile(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "policycheck: %v\n", err)
		return exitUsage
	}
	allowed, err := parseAllowlist(data)
	if err != nil {
		fmt.Fprintf(stderr, "policycheck: %s: %v\n", args[0], err)
		return exitUsage
	}
	bad, err := unapproved(stdin, allowed)
	if err != nil {
		fmt.Fprintf(stderr, "policycheck: reading the module list: %v\n", err)
		return exitUsage
	}
	for _, path := range bad {
		fmt.Fprintf(stdout, "%s: not approved. Only the standard library and %s* are pre-approved. "+
			"Anything else needs an accepted ADR and a line in %s naming it. "+
			"See ADR-0003 and the add-dependency skill.\n", path, preapproved, args[0])
	}
	if len(bad) > 0 {
		return exitViolated
	}
	fmt.Fprintln(stdout, "modules: every module is approved")
	return exitOK
}
