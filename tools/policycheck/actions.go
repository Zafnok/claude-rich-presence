package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

// uses matches a step or job that uses an action or a reusable workflow, and
// captures its value, quoted or not. A comment line does not match.
var uses = regexp.MustCompile(`^\s*(?:-\s+)?uses:\s*(?:"([^"]*)"|'([^']*)'|([^\s#]+))`)

// commit is a full SHA-1 commit hash, the only ref that cannot be moved.
var commit = regexp.MustCompile(`^[0-9a-f]{40}$`)

// digest is the pin of a container image.
var digest = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)

// pinned reports whether a uses value is fixed to content that cannot change.
// An action in the same repository is as pinned as the commit that runs it.
func pinned(value string) bool {
	switch {
	case strings.HasPrefix(value, "./"):
		return true
	case strings.HasPrefix(value, "docker://"):
		return digest.MatchString(value)
	}
	_, ref, found := strings.Cut(value, "@")
	return found && commit.MatchString(ref)
}

// unpinned returns a message for each uses line of a workflow that is not
// pinned to a full commit hash.
func unpinned(name string, workflow []byte) []string {
	var bad []string
	for i, line := range strings.Split(string(workflow), "\n") {
		match := uses.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		value := match[1] + match[2] + match[3]
		if !pinned(value) {
			bad = append(bad, fmt.Sprintf("%s:%d: %s is not pinned to a full commit hash. "+
				"Use the 40-character hash with the tag in a comment. See ADR-0003.", name, i+1, value))
		}
	}
	return bad
}

func runActions(args []string, stdout, stderr io.Writer, readFile func(string) ([]byte, error)) int {
	if len(args) == 0 {
		return usageError(stderr, "actions needs at least one workflow file")
	}
	violated := false
	for _, name := range args {
		data, err := readFile(name)
		if err != nil {
			fmt.Fprintf(stderr, "policycheck: %v\n", err)
			return exitUsage
		}
		for _, message := range unpinned(name, data) {
			fmt.Fprintln(stdout, message)
			violated = true
		}
	}
	if violated {
		return exitViolated
	}
	fmt.Fprintf(stdout, "actions: every action in %d workflow files is pinned\n", len(args))
	return exitOK
}
