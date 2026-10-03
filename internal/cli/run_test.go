package cli

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"
)

func noEnv(string) string { return "" }

func TestRun(t *testing.T) {
	wantVersion := BinaryName + " " + resolveVersion(version, debug.ReadBuildInfo) + "\n"
	usage := "usage: " + BinaryName + " <command>\n\ncommands:\n  version  print the version\n"

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "version prints the version and succeeds",
			args:       []string{"version"},
			wantCode:   0,
			wantStdout: wantVersion,
		},
		{
			name:       "unknown command prints usage to standard error",
			args:       []string{"frobnicate"},
			wantCode:   2,
			wantStderr: BinaryName + ": unknown command \"frobnicate\"\n" + usage,
		},
		{
			name:       "no command prints usage to standard error",
			args:       nil,
			wantCode:   2,
			wantStderr: BinaryName + ": no command given\n" + usage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tt.args, strings.NewReader(""), &stdout, &stderr, noEnv)
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

func TestResolveVersion(t *testing.T) {
	info := func(v string, ok bool) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			if !ok {
				return nil, false
			}
			return &debug.BuildInfo{Main: debug.Module{Version: v}}, true
		}
	}

	tests := []struct {
		name     string
		override string
		read     func() (*debug.BuildInfo, bool)
		want     string
	}{
		{"linker override wins over build info", "1.2.3", info("v9.9.9", true), "1.2.3"},
		{"linker override without build info", "1.2.3", info("", false), "1.2.3"},
		{"build info module version", "", info("v0.4.0", true), "v0.4.0"},
		{"build info without a module version", "", info("", true), unknownVersion},
		{"no build info", "", info("", false), unknownVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.override, tt.read); got != tt.want {
				t.Errorf("resolveVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNames(t *testing.T) {
	// ADR-0010: no vendor mark in any name the product publishes.
	names := []string{ProductName, BinaryName, PluginName, ToolEvent, ToolSummary, ToolStatus, ToolPause}
	for _, n := range names {
		lower := strings.ToLower(n)
		if n == "" || strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic") {
			t.Errorf("name %q is empty or carries a vendor mark", n)
		}
	}
}
