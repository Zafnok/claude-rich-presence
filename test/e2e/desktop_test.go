package e2e

import (
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/cli"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/mcpclient"
)

// The first text line of every activity of the Claude Desktop session.
const desktopLine = "Claude Desktop"

// E13: the binary as Claude Desktop runs it (CRP-002). A copy is started at
// launch and has its input closed before it is initialized. Then two copies
// run until the app quits, told apart by their client names, and only one of
// them reports the app.
func TestClaudeDesktop(t *testing.T) {
	t.Parallel()
	s := newScene(t)
	srv := s.discord(fakediscord.Behavior{})
	onlyStatus := []string{cli.ToolStatus}

	// The copy that is never initialized leaves no trace.
	early := s.launch(built.current, "early")
	early.finish()
	if _, err := os.Stat(s.runtime); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a copy that was never initialized made the runtime directory, where the lock and the socket go, or it cannot be read: %v", err)
	}
	if events := srv.Events(); len(events) != 0 {
		t.Errorf("a copy that was never initialized contacted Discord: %v", events)
	}

	before := time.Now().Unix()
	app := s.startAs(mcpclient.ClaudeDesktop, "app")
	after := time.Now().Unix()
	second := s.startAs(mcpclient.ClaudeDesktopSecond, "second")
	for _, c := range []*session{app, second} {
		if got := c.Tools(); !slices.Equal(got, onlyStatus) {
			t.Errorf("%s: tools = %q, want %q", c.id, got, onlyStatus)
		}
	}

	// Two copies, one session: the app is shown, idle, and for how long.
	app.awaitStatus("Role: host", "Discord: connected", "Sessions: 1")
	second.awaitStatus("Role: passive", "Discord: connected", "Sessions: 1")
	alone := awaitShown(t, srv, "the app", lines(desktopLine, "Idle"))
	if alone.Start < before || alone.Start > after {
		t.Errorf("the elapsed timer starts at %d, want when the app opened, between %d and %d", alone.Start, before, after)
	}

	// A Claude Code session at work takes the focus, and the app is counted.
	// The timer stays the app's, which opened first, a second or more before.
	eventually(t, "the next second", func() bool { return time.Now().Unix() > alone.Start })
	coding := s.start("coding")
	coding.awaitStatus("Role: follower", "Sessions: 2")
	coding.hook("UserPromptSubmit")
	both := awaitShown(t, srv, "the Code session and the count", lines(surfaceLine, "Thinking · 2 sessions"))
	if both.Start != alone.Start {
		t.Errorf("the elapsed timer starts at %d, want the app's %d, the earliest start of the two", both.Start, alone.Start)
	}
	second.awaitStatus("Role: passive", "Sessions: 2")
	coding.finish()
	back := awaitShown(t, srv, "the app again", lines(desktopLine, "Idle"))
	if back.Start != alone.Start {
		t.Errorf("the elapsed timer of the app moved from %d to %d", alone.Start, back.Start)
	}

	// The app quits: both inputs close. The session goes, and it was the
	// last, so nothing is shown.
	app.finish()
	awaitShown(t, srv, "nothing", func(v shown) bool { return v.Clear })
	// The second copy took nothing over while it was alone.
	second.awaitStatus("Role: passive", "Discord: unknown", "Sessions: 0")
	second.finish()
	if handshakes := count(srv, fakediscord.KindHandshake); handshakes != 1 {
		t.Errorf("%d handshakes, want one: only the reporting copy ever connects to Discord", handshakes)
	}
	for _, v := range showing(t, srv) {
		if !v.Clear && v.Details != desktopLine && v.Details != surfaceLine {
			t.Errorf("Discord was told to show %v, which is neither surface", v)
		}
	}
}
