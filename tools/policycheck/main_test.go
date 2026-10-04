package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

const usageText = "usage: policycheck modules allowlist < go-list-json\n       policycheck actions workflow...\n"

const hash = "3d3c42e5aac5ba805825da76410c181273ba90b1"

const unpinnedAdvice = " is not pinned to a full commit hash. Use the 40-character hash with the tag in a comment. See ADR-0003.\n"

const notApprovedAdvice = ": not approved. Only the standard library and golang.org/x/* are pre-approved. " +
	"Anything else needs an accepted ADR and a line in allow.txt naming it. See ADR-0003 and the add-dependency skill.\n"

// files is a fake file reader: a name it does not hold cannot be read.
type files map[string]string

func (f files) read(name string) ([]byte, error) {
	data, ok := f[name]
	if !ok {
		return nil, errors.New("no such file")
	}
	return []byte(data), nil
}

// listing is go list -deps -test -json for a standard library package, a
// package of the main module and one package from each given module path.
func listing(paths ...string) string {
	out := `{"ImportPath":"fmt"}` + "\n" +
		`{"ImportPath":"example.com/m","Module":{"Path":"example.com/m","Main":true}}` + "\n"
	for _, p := range paths {
		out += `{"ImportPath":"` + p + `/pkg","Module":{"Path":"` + p + `","Version":"v1.0.0"}}` + "\n"
	}
	return out
}

func TestRun(t *testing.T) {
	allow := "# approved modules\n\ngithub.com/Microsoft/go-winio ADR-0017\n"
	tests := []struct {
		name       string
		args       []string
		stdin      string
		files      files
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "no command is a usage error",
			wantCode:   2,
			wantStderr: "policycheck: no command given\n" + usageText,
		},
		{
			name:       "unknown command is a usage error",
			args:       []string{"frobnicate"},
			wantCode:   2,
			wantStderr: "policycheck: unknown command \"frobnicate\"\n" + usageText,
		},
		{
			name:       "modules without an allowlist is a usage error",
			args:       []string{"modules"},
			wantCode:   2,
			wantStderr: "policycheck: modules needs exactly one allowlist file\n" + usageText,
		},
		{
			name:       "modules with an unreadable allowlist fails",
			args:       []string{"modules", "missing.txt"},
			wantCode:   2,
			wantStderr: "policycheck: no such file\n",
		},
		{
			name:       "modules with a line that has no ADR fails",
			args:       []string{"modules", "allow.txt"},
			files:      files{"allow.txt": "github.com/a/b\n"},
			wantCode:   2,
			wantStderr: "policycheck: allow.txt: line 1: want a module path and an ADR number such as ADR-0017, got \"github.com/a/b\"\n",
		},
		{
			name:       "modules with a bad ADR number fails",
			args:       []string{"modules", "allow.txt"},
			files:      files{"allow.txt": "ok ADR-0003\ngithub.com/a/b ADR-3\n"},
			wantCode:   2,
			wantStderr: "policycheck: allow.txt: line 2: want a module path and an ADR number such as ADR-0017, got \"github.com/a/b ADR-3\"\n",
		},
		{
			name:       "modules with a malformed listing fails",
			args:       []string{"modules", "allow.txt"},
			stdin:      "{",
			files:      files{"allow.txt": allow},
			wantCode:   2,
			wantStderr: "policycheck: reading the module list: unexpected EOF\n",
		},
		{
			name:       "modules passes the pre-approved and the allowlisted",
			args:       []string{"modules", "allow.txt"},
			stdin:      listing("golang.org/x/sys", "github.com/Microsoft/go-winio"),
			files:      files{"allow.txt": allow},
			wantCode:   0,
			wantStdout: "modules: every module is approved\n",
		},
		{
			name:       "modules fails an unapproved module and points to the policy",
			args:       []string{"modules", "allow.txt"},
			stdin:      listing("golang.org/xyz", "golang.org/x/sys", "github.com/spf13/cobra", "golang.org/xyz"),
			files:      files{"allow.txt": allow},
			wantCode:   1,
			wantStdout: "github.com/spf13/cobra" + notApprovedAdvice + "golang.org/xyz" + notApprovedAdvice,
		},
		{
			name:       "actions without a workflow is a usage error",
			args:       []string{"actions"},
			wantCode:   2,
			wantStderr: "policycheck: actions needs at least one workflow file\n" + usageText,
		},
		{
			name:       "actions with an unreadable workflow fails",
			args:       []string{"actions", "missing.yml"},
			wantCode:   2,
			wantStderr: "policycheck: no such file\n",
		},
		{
			name: "actions passes pinned, local and digest-pinned uses",
			args: []string{"actions", "a.yml", "b.yml"},
			files: files{
				"a.yml": "steps:\n" +
					"  - uses: actions/checkout@" + hash + " # v7.0.1\n" +
					"  - uses: \"actions/setup-go@" + hash + "\"\n" +
					"  # uses: actions/cache@v4\n" +
					"  - run: echo hi\n",
				"b.yml": "jobs:\n  x:\n    uses: ./.github/workflows/a.yml\n" +
					"    steps:\n      - uses: 'docker://alpine@sha256:" + strings.Repeat("a", 64) + "'\n",
			},
			wantCode:   0,
			wantStdout: "actions: every action in 2 workflow files is pinned\n",
		},
		{
			name: "actions fails a tag, a branch, a short hash, a bare name and an unpinned image",
			args: []string{"actions", "a.yml"},
			files: files{"a.yml": "steps:\n" +
				"  - uses: actions/checkout@v4\n" +
				"  - uses: actions/cache@main\n" +
				"  - uses: actions/x@3d3c42e\n" +
				"  - uses: actions/y\n" +
				"  - uses: docker://alpine:3\n" +
				"  - uses: actions/ok@" + hash + "\n"},
			wantCode: 1,
			wantStdout: "a.yml:2: actions/checkout@v4" + unpinnedAdvice +
				"a.yml:3: actions/cache@main" + unpinnedAdvice +
				"a.yml:4: actions/x@3d3c42e" + unpinnedAdvice +
				"a.yml:5: actions/y" + unpinnedAdvice +
				"a.yml:6: docker://alpine:3" + unpinnedAdvice,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, strings.NewReader(tt.stdin), &stdout, &stderr, tt.files.read)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if stdout.String() != tt.wantStdout {
				t.Errorf("stdout =\n%q\nwant\n%q", stdout.String(), tt.wantStdout)
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("stderr =\n%q\nwant\n%q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestUnapprovedReadError(t *testing.T) {
	_, err := unapproved(iotest.ErrReader(errors.New("boom")), nil)
	if err == nil || err.Error() != "boom" {
		t.Errorf("err = %v, want boom", err)
	}
}
