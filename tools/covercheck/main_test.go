package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

const usageText = "usage: covercheck packages\n       covercheck check [-module path] profile...\n"

// files is a fake file reader: a name it does not hold cannot be read.
type files map[string]string

func (f files) read(name string) ([]byte, error) {
	data, ok := f[name]
	if !ok {
		return nil, errors.New("no such file")
	}
	return []byte(data), nil
}

func TestRun(t *testing.T) {
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
			wantStderr: "covercheck: no command given\n" + usageText,
		},
		{
			name:       "unknown command is a usage error",
			args:       []string{"frobnicate"},
			wantCode:   2,
			wantStderr: "covercheck: unknown command \"frobnicate\"\n" + usageText,
		},
		{
			name: "packages drops test helpers and joins the rest with commas",
			args: []string{"packages"},
			stdin: "example.com/m/cmd/app\n" +
				"example.com/m/internal/cli\r\n" +
				"\n" +
				"example.com/m/internal/testutil\n" +
				"example.com/m/internal/testutil/fakeclock\n" +
				"example.com/m/internal/testutilities\n" +
				"example.com/m/tools/covercheck\n",
			wantCode: 0,
			wantStdout: "example.com/m/cmd/app,example.com/m/internal/cli," +
				"example.com/m/internal/testutilities,example.com/m/tools/covercheck\n",
		},
		{
			name:       "packages with nothing measured fails rather than print an empty list",
			args:       []string{"packages"},
			stdin:      "example.com/m/internal/testutil/fakeclock\n",
			wantCode:   2,
			wantStderr: "covercheck: no measured package on standard input\n",
		},
		{
			name:       "check without a profile is a usage error",
			args:       []string{"check"},
			wantCode:   2,
			wantStderr: "covercheck: check needs at least one profile\n" + usageText,
		},
		{
			name:       "check with an unknown flag is a usage error",
			args:       []string{"check", "-frobnicate"},
			wantCode:   2,
			wantStderr: "flag provided but not defined: -frobnicate\n" + usageText,
		},
		{
			name:       "check with an unreadable profile fails",
			args:       []string{"check", "missing.txt"},
			wantCode:   2,
			wantStderr: "covercheck: missing.txt: no such file\n",
		},
		{
			name:       "check with a malformed profile names the file and line",
			args:       []string{"check", "p.txt"},
			files:      files{"p.txt": "mode: atomic\nexample.com/m/a/a.go:1.1,2.2 1 1\nnonsense\n"},
			wantCode:   2,
			wantStderr: "covercheck: p.txt: line 3: no file name in \"nonsense\"\n",
		},
		{
			name: "full coverage passes",
			args: []string{"check", "p.txt"},
			files: files{"p.txt": "mode: atomic\n" +
				"example.com/m/a/a.go:1.1,2.2 2 1\n" +
				"example.com/m/a/a.go:3.1,4.2 1 7\n"},
			wantCode:   0,
			wantStdout: "coverage: 100.0% of statements (3 of 3)\n",
		},
		{
			name: "an uncovered block fails and is named by file and line",
			args: []string{"check", "-module", "example.com/m", "p.txt"},
			files: files{"p.txt": "mode: atomic\n" +
				"example.com/m/b/b.go:30.2,31.3 1 0\n" +
				"example.com/m/a/a.go:12.14,14.3 2 0\n" +
				"example.com/m/a/a.go:5.2,5.9 1 0\n" +
				"example.com/m/a/a.go:5.20,5.30 1 0\n" +
				"example.com/m/a/a.go:1.1,2.2 2 1\n"},
			wantCode: 1,
			wantStdout: "a/a.go:5: not covered\n" +
				"a/a.go:5: not covered\n" +
				"a/a.go:12: not covered\n" +
				"b/b.go:30: not covered\n" +
				"coverage: 28.5% of statements (2 of 7)\n",
		},
		{
			name: "without -module the file is named as the profile names it",
			args: []string{"check", "p.txt"},
			files: files{"p.txt": "mode: set\n" +
				"example.com/m/a/a.go:5.2,5.9 1 0\n"},
			wantCode: 1,
			wantStdout: "example.com/m/a/a.go:5: not covered\n" +
				"coverage: 0.0% of statements (0 of 1)\n",
		},
		{
			name: "coverage just below the mark is never rounded up to 100.0%",
			args: []string{"check", "p.txt"},
			files: files{"p.txt": "mode: atomic\n" +
				"example.com/m/a/a.go:1.1,2.2 9999 1\n" +
				"example.com/m/a/a.go:3.1,4.2 1 0\n"},
			wantCode: 1,
			wantStdout: "example.com/m/a/a.go:3: not covered\n" +
				"coverage: 99.9% of statements (9999 of 10000)\n",
		},
		{
			name: "a block covered in any profile is covered, whatever the order",
			args: []string{"check", "unit.txt", "e2e.txt", "unit.txt"},
			files: files{
				"unit.txt": "mode: atomic\r\n" +
					"example.com/m/cmd/app/main.go:9.13,11.2 1 0\r\n" +
					"example.com/m/a/a.go:1.1,2.2 1 1\r\n" +
					"example.com/m/a/a.go:1.1,2.2 1 0\r\n",
				"e2e.txt": "mode: atomic\n" +
					"example.com/m/cmd/app/main.go:9.13,11.2 1 1\n",
			},
			wantCode:   0,
			wantStdout: "coverage: 100.0% of statements (2 of 2)\n",
		},
		{
			name: "test helpers are not measured",
			args: []string{"check", "p.txt"},
			files: files{"p.txt": "mode: atomic\n" +
				"example.com/m/internal/testutil/fakeclock/clock.go:1.1,2.2 5 0\n" +
				"example.com/m/a/a.go:1.1,2.2 1 1\n"},
			wantCode:   0,
			wantStdout: "coverage: 100.0% of statements (1 of 1)\n",
		},
		{
			name: "a block with no statements is not reported",
			args: []string{"check", "p.txt"},
			files: files{"p.txt": "mode: atomic\n" +
				"example.com/m/a/a.go:1.1,2.2 0 0\n" +
				"example.com/m/a/a.go:3.1,4.2 1 1\n"},
			wantCode:   0,
			wantStdout: "coverage: 100.0% of statements (1 of 1)\n",
		},
		{
			name:       "a profile that measured nothing fails",
			args:       []string{"check", "p.txt"},
			files:      files{"p.txt": "mode: atomic\n"},
			wantCode:   1,
			wantStdout: "coverage: no statements were measured\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, strings.NewReader(tt.stdin), &stdout, &stderr, tt.files.read)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if got := stdout.String(); got != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", got, tt.wantStdout)
			}
			if got := stderr.String(); got != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", got, tt.wantStderr)
			}
		})
	}
}

func TestRunPackagesReadError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"packages"}, iotest.ErrReader(errors.New("pipe closed")), &stdout, &stderr, files{}.read)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want nothing", got)
	}
	if got, want := stderr.String(), "covercheck: reading the package list: pipe closed\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

func TestMeasured(t *testing.T) {
	tests := []struct {
		pkg  string
		want bool
	}{
		{"example.com/m/cmd/app", true},
		{"example.com/m/internal/cli", true},
		{"example.com/m/tools/covercheck", true},
		{"example.com/m/internal/testutilities", true},
		{"example.com/m/testutil", true},
		{"example.com/m/internal/testutil", false},
		{"example.com/m/internal/testutil/fakeclock", false},
		{"internal/testutil", false},
	}
	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			if got := measured(tt.pkg); got != tt.want {
				t.Errorf("measured(%q) = %v, want %v", tt.pkg, got, tt.want)
			}
		})
	}
}

func TestProfileAddErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"no colon", "nonsense", "line 1: no file name in \"nonsense\""},
		{"colon first", ":1.1,2.2 1 1", "line 1: no file name in \":1.1,2.2 1 1\""},
		{"missing fields", "mode: set\na.go:1.1,2.2 1", "line 2: malformed block \"a.go:1.1,2.2 1\""},
		{"not a number", "a.go:1.x,2.2 1 1", "line 1: malformed block \"a.go:1.x,2.2 1 1\""},
		{"negative line", "a.go:-1.1,2.2 1 1", "line 1: malformed block \"a.go:-1.1,2.2 1 1\""},
		{"negative count", "a.go:1.1,2.2 1 -1", "line 1: malformed block \"a.go:1.1,2.2 1 -1\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := profile{}.add([]byte(tt.data))
			if err == nil || err.Error() != tt.want {
				t.Errorf("add() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func FuzzProfileAdd(f *testing.F) {
	f.Add([]byte("mode: atomic\nexample.com/m/a/a.go:1.1,2.2 2 1\nexample.com/m/a/a.go:3.1,4.2 1 0\n"))
	f.Add([]byte("mode: set\r\nexample.com/m/internal/testutil/x/x.go:1.1,2.2 5 0\r\n"))
	f.Add([]byte("a.go:1.1,2.2 1 1\na.go:1.1,2.2 1 0\n"))
	f.Add([]byte("nonsense"))
	f.Add([]byte(":1.1,2.2 1 1"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		p := profile{}
		if err := p.add(data); err != nil {
			return
		}
		var out bytes.Buffer
		passed := p.report(&out, "example.com/m")
		covered, total := p.totals()
		if covered > total {
			t.Fatalf("covered %d exceeds total %d", covered, total)
		}
		if passed != (total > 0 && covered == total) {
			t.Fatalf("passed = %v with %d of %d covered", passed, covered, total)
		}
		if passed != strings.HasPrefix(out.String(), "coverage: 100.0% ") {
			t.Fatalf("passed = %v but the report is %q", passed, out.String())
		}
	})
}
