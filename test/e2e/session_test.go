package e2e

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/cli"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// The first text line of every activity of a Claude Code session.
const surfaceLine = "Claude Code"

// tolerance is how much closer than the interval two updates may be recorded
// by the fake Discord. The product measures the interval where it writes, and
// the fake stamps an update where it reads it, on another goroutine of a
// machine that is also running other tests. An update that was read late
// makes the next one look early by as much. The product's own arithmetic is
// tested exactly, against a fake clock, in internal/schedule and
// internal/discord/session; what this suite adds is that the built binary is
// configured with the interval and keeps to it.
const tolerance = 250 * time.Millisecond

// E1: one session, from start to end.
func TestOneSession(t *testing.T) {
	t.Parallel()
	s := newScene(t)
	srv := s.discord(fakediscord.Behavior{})

	c := s.start("one")
	c.awaitStatus("Role: host", "Discord: connected", "Sessions: 1")
	// Until a hook says where the session is, it is shown at the lowest
	// level: in use, and for how long.
	opened := awaitShown(t, srv, "the session", lines(surfaceLine, ""))

	c.hook("UserPromptSubmit")
	awaitShown(t, srv, "a prompt being worked on", lines(surfaceLine, "Thinking"))
	c.hook("PreToolUse", "tool_name", "Bash")
	awaitShown(t, srv, "a command running", lines(surfaceLine, "Running commands"))
	c.hook("Stop")
	awaitShown(t, srv, "an idle session", lines(surfaceLine, "Idle"))
	c.finish()

	var got []string
	all := showing(t, srv)
	for _, v := range all {
		if v.Clear {
			got = append(got, "clear")
			continue
		}
		got = append(got, v.Details+" / "+v.State)
		if v.Start != opened.Start {
			t.Errorf("the elapsed timer moved: %v, want the start %d throughout", v, opened.Start)
		}
	}
	want := []string{surfaceLine + " / ", surfaceLine + " / Thinking", surfaceLine + " / Running commands", surfaceLine + " / Idle", "clear"}
	// The session's end and the host's shutdown may each clear.
	if got = slices.Compact(got); !slices.Equal(got, want) {
		t.Errorf("Discord was told to show %q, want %q", got, want)
	}
	if handshakes := count(srv, fakediscord.KindHandshake); handshakes != 1 {
		t.Errorf("%d handshakes, want one connection for the whole session", handshakes)
	}
	// The host gave up the socket with its role.
	if _, err := os.Stat(s.paths().Socket); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the control socket is still there after the host ended, or cannot be read: %v", err)
	}
}

// E2: events arriving much faster than Discord may be updated.
func TestRapidEvents(t *testing.T) {
	t.Parallel()
	s := newScene(t)
	srv := s.discord(fakediscord.Behavior{})
	c := s.start("rapid")
	awaitShown(t, srv, "the session", lines(surfaceLine, ""))

	tools := []string{"Bash", "Read", "Grep", "WebFetch", "Agent"}
	burst := func() {
		c.hook("UserPromptSubmit")
		for i := range 40 {
			c.hook("PreToolUse", "tool_name", tools[i%len(tools)])
			c.hook("PostToolUse")
		}
	}
	// Two bursts, the second while the first is still held back, and then
	// the state that must be the one left showing.
	burst()
	eventually(t, "the first burst to reach Discord", func() bool { return len(showing(t, srv)) >= 2 })
	burst()
	c.hook("PreToolUse", "tool_name", "Edit")
	awaitShown(t, srv, "the last state", lines(surfaceLine, "Editing files"))

	all := showing(t, srv)
	for i := 1; i < len(all); i++ {
		if gap := all[i].At.Sub(all[i-1].At); gap < interval-tolerance {
			t.Errorf("updates %d and %d are %v apart, want at least %v: %v then %v", i, i+1, gap, interval, all[i-1], all[i])
		}
	}
	// 162 events in a few moments may not become more updates than the time
	// they took allows.
	span := all[len(all)-1].At.Sub(all[0].At)
	if most := int((span+tolerance)/interval) + 1; len(all) > most {
		t.Errorf("%d updates in %v, want at most %d", len(all), span, most)
	}
	c.finish()
}

// E11: presence switched off, by the configuration and by running remotely.
func TestPresenceOff(t *testing.T) {
	t.Parallel()
	for name, setting := range map[string]string{
		"disabled":           "RICH_PRESENCE_ENABLED=false",
		"remote environment": cli.EnvRemote + "=true",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newScene(t)
			s.extra = []string{setting}
			srv := s.discord(fakediscord.Behavior{})

			c := s.start("off")
			if got, want := c.Tools(), []string{cli.ToolEvent, cli.ToolStatus, cli.ToolPause}; !slices.Equal(got, want) {
				t.Errorf("tools = %q, want %q", got, want)
			}
			c.hook("UserPromptSubmit")
			c.hook("PreToolUse", "tool_name", "Bash")
			if got := c.status(); !strings.Contains(got, "Role: off") || !strings.Contains(got, "Sessions: 0") {
				t.Errorf("status = %q, want role off and no sessions", got)
			}
			c.finish()

			// The process has ended, so what it did not do, it never will.
			if _, err := os.Stat(s.runtime); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the runtime directory, where the lock and the socket go, was made, or cannot be read: %v", err)
			}
			if events := srv.Events(); len(events) != 0 {
				t.Errorf("Discord was contacted: %v", events)
			}
		})
	}
}

// E14: every command, once, as a user would run it.
func TestCommands(t *testing.T) {
	t.Parallel()
	const usage = "usage: rich-presence <command>"

	t.Run("without a host or a Discord", func(t *testing.T) {
		t.Parallel()
		s := newScene(t)
		tests := []struct {
			args       []string
			code       int
			stdout     string
			stderr     string
			restEmpty  bool
			wholeMatch bool
		}{
			{args: []string{"version"}, code: 0, stdout: "rich-presence " + versionCurrent + "\n", wholeMatch: true},
			{args: []string{}, code: 0, stdout: usage},
			{args: []string{"help"}, code: 0, stdout: usage},
			{args: []string{"frobnicate"}, code: 2, stderr: usage},
			{args: []string{"version", "extra"}, code: 2, stderr: "version takes no arguments"},
			{args: []string{"mcp", "extra"}, code: 2, stderr: "mcp takes no arguments"},
			{args: []string{"status"}, code: 3, stdout: "No presence host is running."},
			{args: []string{"doctor"}, code: 1, stdout: "Discord"},
		}
		for _, tt := range tests {
			code, stdout, stderr := s.run(built.current, tt.args...)
			what := fmt.Sprintf("%q: exit %d, standard output %q, standard error %q", tt.args, code, stdout, stderr)
			if code != tt.code || !strings.Contains(stdout, tt.stdout) || !strings.Contains(stderr, tt.stderr) {
				t.Errorf("%s; want exit %d, %q on standard output and %q on standard error", what, tt.code, tt.stdout, tt.stderr)
			}
			if tt.wholeMatch && stdout != tt.stdout {
				t.Errorf("%s; want standard output to be exactly %q", what, tt.stdout)
			}
			if tt.stderr == "" && stderr != "" {
				t.Errorf("%s; want nothing on standard error", what)
			}
		}
		// None of these is a host, so none of them made the host's files.
		if _, err := os.Stat(s.paths().Lock); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a command that is not a host made the lock file, or it cannot be read: %v", err)
		}
	})

	t.Run("with a host and a Discord", func(t *testing.T) {
		t.Parallel()
		s := newScene(t)
		srv := s.discord(fakediscord.Behavior{})
		c := s.start("commands")
		c.awaitStatus("Role: host", "Discord: connected", "Sessions: 1")
		awaitShown(t, srv, "the session", lines(surfaceLine, ""))

		code, stdout, stderr := s.run(built.current, "status")
		if code != 0 || stderr != "" {
			t.Errorf("status: exit %d, standard error %q, want 0 and nothing", code, stderr)
		}
		for _, want := range []string{"Role: host\n", "Discord: connected\n", "Sessions: 1\n", "Version: " + versionCurrent + "\n", "Uptime: "} {
			if !strings.Contains(stdout, want) {
				t.Errorf("status printed %q, want it to contain %q", stdout, want)
			}
		}

		code, stdout, stderr = s.run(built.current, "doctor")
		if code != 0 || stderr != "" {
			t.Errorf("doctor: exit %d, standard error %q, want 0 and nothing; it printed:\n%s", code, stderr, stdout)
		}
		if strings.Contains(stdout, s.home) {
			t.Errorf("doctor printed the home directory, which is not safe to paste in public:\n%s", stdout)
		}

		// Neither command is a session, and neither disturbed the one there is.
		c.awaitStatus("Role: host", "Discord: connected", "Sessions: 1")
		c.finish()
	})
}
