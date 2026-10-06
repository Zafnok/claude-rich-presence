package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

func TestMCPWithPresenceOff(t *testing.T) {
	tests := []struct {
		name string
		env  []string
	}{
		{"disabled in the configuration", []string{"RICH_PRESENCE_ENABLED", "false"}},
		{"running remotely", []string{"CLAUDE_CODE_REMOTE", "true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.env = w.env.with(tt.env...)
			srv := w.discord(t)

			s := w.startSession(t)
			s.initialize()
			if got := strings.Join(s.toolNames(), ","); got != "presence_event,presence_status,presence_pause" {
				t.Errorf("tools = %q, want the event, status and pause tools", got)
			}
			if got := s.status(); !strings.Contains(got, "Role: off") || !strings.Contains(got, "Sessions: 0") {
				t.Errorf("status = %q, want role off and no sessions", got)
			}
			if code := s.finish(); code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}

			if _, err := os.Stat(w.runtime); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the runtime directory was made, or could not be read: %v", err)
			}
			if events := srv.Events(); len(events) != 0 {
				t.Errorf("Discord was contacted: %v", events)
			}
		})
	}
}

func TestMCPHostsPresence(t *testing.T) {
	w := newWorld(t)
	srv := w.discord(t)

	s := w.startSession(t)
	s.initialize()
	s.awaitStatus("Role: host")
	s.awaitStatus("Discord: connected")
	// The first update after a quiet period is not held back, but one that
	// came before the connection was ready is, for the rate limit's interval.
	eventually(t, "the activity to be set", func() bool {
		w.clock.Advance(time.Second)
		for _, ev := range srv.Events() {
			if ev.Kind == fakediscord.KindSetActivity {
				return true
			}
		}
		return false
	})
	if got := s.status(); !strings.Contains(got, "Sessions: 1") {
		t.Errorf("status = %q, want one session", got)
	}

	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	srv.Await(fakediscord.KindClearActivity, 1)
	if _, err := os.Stat(filepath.Join(w.runtime, "host.lock")); err != nil {
		t.Errorf("the lock file is part of the runtime directory: %v", err)
	}
}

func TestMCPSecondSessionFollows(t *testing.T) {
	w := newWorld(t)
	w.discord(t)

	first := w.startSession(t)
	first.initialize()
	first.awaitStatus("Role: host")

	// A second process of the same user finds the lock held.
	w.sys.sessionID = func() string { return "second-session" }
	follower := w.startSession(t)
	follower.initialize()
	follower.awaitStatus("Role: follower")
	first.awaitStatus("Sessions: 2")

	if code := follower.finish(); code != 0 {
		t.Errorf("follower exit code = %d, want 0", code)
	}
	if code := first.finish(); code != 0 {
		t.Errorf("host exit code = %d, want 0", code)
	}
}

func TestMCPReportsDiscordNotRunning(t *testing.T) {
	w := newWorld(t) // no Discord is listening

	s := w.startSession(t)
	s.initialize()
	s.awaitStatus("Role: host")
	s.awaitStatus("Discord: disconnected")
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestMCPEndsOnSignal(t *testing.T) {
	w := newWorld(t)
	w.discord(t)

	s := w.startSession(t)
	s.initialize()
	s.awaitStatus("Role: host")

	// The input is still open: the server is blocked reading it.
	w.raise()
	if code := s.exit(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(w.runtime, "control.sock")); err == nil {
		t.Error("the control socket was left behind")
	}
}

func TestMCPEndsWhenInputClosesBeforeInitialize(t *testing.T) {
	w := newWorld(t)
	srv := w.discord(t)
	s := w.startSession(t)
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	// Claude Desktop starts a copy at launch that it never initializes. It
	// must leave no trace: no lock asked for, no host dialled.
	w.noRuntimeDirectory(t)
	if events := srv.Events(); len(events) != 0 {
		t.Errorf("Discord was contacted: %v", events)
	}
}

func TestMCPStartupProblemsLeaveStandardOutputToTheProtocol(t *testing.T) {
	tests := []struct {
		name      string
		prepare   func(w *world)
		wantTools string
		wantRole  string
	}{
		{
			name:      "no runtime directory can be resolved",
			prepare:   func(w *world) { w.env = w.env.with("RICH_PRESENCE_RUNTIME_DIR", "relative") },
			wantTools: "presence_event,presence_status,presence_pause",
			wantRole:  "Role: off",
		},
		{
			name:      "this binary's version is not one the control protocol carries",
			prepare:   func(w *world) { w.sys.version = "not a version" },
			wantTools: "presence_event,presence_status,presence_pause",
			wantRole:  "Role: off",
		},
		{
			name:      "the session cannot be named",
			prepare:   func(w *world) { w.sys.sessionID = func() string { return "" } },
			wantTools: "",
			wantRole:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			tt.prepare(w)
			s := w.startSession(t)
			s.initialize()
			if got := strings.Join(s.toolNames(), ","); got != tt.wantTools {
				t.Errorf("tools = %q, want %q", got, tt.wantTools)
			}
			if tt.wantRole != "" {
				if got := s.status(); !strings.Contains(got, tt.wantRole) {
					t.Errorf("status = %q, want it to contain %q", got, tt.wantRole)
				}
			}
			if code := s.finish(); code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			if got := s.stderr.String(); got != "" {
				t.Errorf("standard error = %q, want nothing", got)
			}
		})
	}
}

// failingWriter is a standard output that cannot be written to.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("the pipe is closed") }

func TestMCPFailsWhenTheClientCannotBeAnswered(t *testing.T) {
	w := newWorld(t)
	w.env = w.env.with("RICH_PRESENCE_ENABLED", "false")
	var stderr strings.Builder
	code := w.sys.run([]string{"mcp"}, strings.NewReader(initializeLine+"\n"), failingWriter{}, &stderr, w.env.get)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if want := BinaryName + ": mcp: "; !strings.HasPrefix(stderr.String(), want) {
		t.Errorf("standard error = %q, want it to begin %q", stderr.String(), want)
	}
}

func TestMCPSurvivesAPanicWithoutWritingToStandardOutput(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(w *world)
		// ask is whether the client has something for the server to read.
		ask bool
	}{
		{"in the server", func(w *world) { w.sys.sessionID = func() string { panic("the hook said: secret prompt") } }, true},
		{"before the server", func(w *world) { w.sys.files = panickingFiles{} }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.env = w.env.with("RICH_PRESENCE_ENABLED", "false")
			tt.prepare(w)
			s := w.startSession(t)
			if tt.ask {
				s.send(initializeLine)
			}
			if code := s.exit(); code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			got := s.stderr.String()
			if !strings.Contains(got, "internal error") || strings.Contains(got, "secret") {
				t.Errorf("standard error = %q, want a fixed message that carries nothing of the panic", got)
			}
		})
	}
}

type panickingFiles struct{}

func (panickingFiles) ReadFile(string) ([]byte, error) { panic("secret") }

func TestMCPLogsConfigurationProblemsByCount(t *testing.T) {
	w := newWorld(t)
	w.env = w.env.with("RICH_PRESENCE_ENABLED", "false", "RICH_PRESENCE_PRIVACY", "bogus-value")
	s := w.startSession(t)
	s.initialize()
	if code := s.finish(); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	_, dirs, _ := config.Load(w.sys.goos, w.env.get, w.sys.files)
	log, err := os.ReadFile(diag.LogPath(dirs.Logs))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "run doctor") || !strings.Contains(string(log), "problems=1") {
		t.Errorf("log = %q, want the count of problems and the way to see them", log)
	}
	if strings.Contains(string(log), "bogus-value") {
		t.Errorf("the log holds a configured value: %q", log)
	}
}

func TestMCPAppliesTheProjectProfileOfAHooksDirectory(t *testing.T) {
	w := newWorld(t)
	project := t.TempDir()
	profile, err := json.Marshal(map[string]any{
		"enabled":  false,
		"privacy":  "minimal",
		"projects": []map[string]string{{"path": project, "privacy": "full", "name": "Chosen"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := config.ResolveDirs(w.sys.goos, w.env.get)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirs.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dirs.File, profile, 0o600); err != nil {
		t.Fatal(err)
	}

	s := w.startSession(t)
	s.initialize()
	if got := s.status(); !strings.Contains(got, "Privacy: minimal") {
		t.Errorf("status before any hook = %q, want the global level", got)
	}
	call, err := json.Marshal(map[string]any{"name": "presence_event", "arguments": map[string]string{"event": "UserPromptSubmit", "session_id": "abc", "cwd": project}})
	if err != nil {
		t.Fatal(err)
	}
	s.request("tools/call", string(call))
	if got := s.status(); !strings.Contains(got, "Privacy: full") {
		t.Errorf("status after a hook from the project = %q, want the profile's level", got)
	}
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestMCPRefusesArgumentsBeforeTouchingAnything(t *testing.T) {
	tests := [][]string{{"-bogus"}, {"extra"}}
	for _, extra := range tests {
		w := newWorld(t)
		code, stdout, stderr := w.run(append([]string{"mcp"}, extra...)...)
		if code != 2 || stdout != "" || stderr == "" {
			t.Errorf("mcp %v: got code %d, stdout %q, stderr %q; want 2, nothing, and a message", extra, code, stdout, stderr)
		}
		if _, err := os.Stat(w.runtime); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("mcp %v made the runtime directory", extra)
		}
	}
}
