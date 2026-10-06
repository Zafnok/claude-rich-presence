package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/cli"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// preview is the private preview of the card, from the status tool.
func (c *session) preview() string {
	c.t.Helper()
	return c.Call(cli.ToolStatus, map[string]any{"preview": true}).Text
}

// awaitPreview polls the preview until it contains every one of want.
func (c *session) awaitPreview(want ...string) string {
	c.t.Helper()
	var got string
	eventually(c.t, "the preview of "+c.id+" to show "+strings.Join(want, ", "), func() bool {
		got = c.preview()
		for _, w := range want {
			if !strings.Contains(got, w) {
				return false
			}
		}
		return true
	}, func() string { return "last preview: " + got })
	return got
}

// hideProject writes a configuration in which the project of the session
// named name is hidden.
func (s *scene) hideProject(name string) {
	s.t.Helper()
	s.writeConfig(map[string]any{
		"privacy": "standard",
		"projects": []map[string]string{
			{"path": filepath.Join(s.home, "work", "project-of-"+name), "privacy": "off"},
		},
	})
}

// E-hide, on the control channel: a session in a project profiled off sends
// its host nothing beyond what keeps it connected, and a session elsewhere is
// unaffected (ADR-0012, CRP-073). The test is the host, so it reads every
// byte the two sessions send.
func TestAHiddenSessionSendsItsHostNothing(t *testing.T) {
	t.Parallel()
	s := newScene(t)
	s.hideProject("secret")
	h := hostControl(t, s)

	secret := s.start("secret")
	secret.awaitStatus("Role: follower", "Privacy: off")
	secret.hook("UserPromptSubmit")
	secret.hook("PreToolUse", "tool_name", "Edit")
	secret.hook("SubagentStart", "agent_id", "agent-1")
	secret.hook("PostToolUse")
	secret.hook("Stop")
	secret.awaitStatus("Privacy: off")

	open := s.start("open")
	open.hook("UserPromptSubmit")
	open.hook("PreToolUse", "tool_name", "Read")
	open.awaitStatus("Role: follower", "Privacy: standard")
	// It arrives in the session's sync or as an event, whichever the session
	// had to send when the hook came.
	eventually(t, "the visible session's work to reach the host", func() bool {
		return strings.Contains(h.received(), `"tool":"reading"`)
	})
	secret.finish()
	open.finish()

	keepsConnected := []string{`{"type":"hello",`, `{"type":"sync","session":null}`, `{"type":"status"}`, `{"type":"preview"}`}
	sessions := 0
	for _, line := range strings.Split(strings.TrimSpace(h.received()), "\n") {
		if strings.Contains(line, "secret") {
			t.Errorf("the hidden session is named on the control channel: %s", line)
		}
		connected := false
		for _, prefix := range keepsConnected {
			connected = connected || strings.HasPrefix(line, prefix)
		}
		if connected {
			continue
		}
		// Everything else is the visible session's, which the hooks named.
		sessions++
		if !strings.Contains(line, `"open"`) {
			t.Errorf("a line that is not needed to stay connected is not the visible session's: %s", line)
		}
	}
	if sessions == 0 {
		t.Error("the visible session sent nothing, so the test shows little")
	}
}

// E-hide, on the card: with one session hidden and one visible, the card
// shows the visible one alone, and counts one.
func TestAHiddenSessionIsNotCounted(t *testing.T) {
	t.Parallel()
	s := newScene(t)
	s.hideProject("secret")
	srv := s.discord(fakediscord.Behavior{})

	secret := s.start("secret")
	secret.awaitStatus("Role: host", "Discord: connected", "Privacy: off", "Sessions: 0")
	secret.hook("UserPromptSubmit")
	secret.hook("PreToolUse", "tool_name", "Edit")

	open := s.start("open")
	open.hook("UserPromptSubmit")
	open.hook("PreToolUse", "tool_name", "Bash")
	// Exactly these lines: no count of sessions after the status.
	awaitShown(t, srv, "the visible session alone", lines(surfaceLine, "Running commands"))
	open.awaitStatus("Role: follower", "Sessions: 1")
	secret.awaitStatus("Privacy: off", "Sessions: 1")
	if got := secret.awaitPreview("Line 2: Running commands"); !strings.Contains(got, "This session: hidden.") {
		t.Errorf("the hidden session's preview is %q, want it to say the session is hidden", got)
	}

	// The hidden session works on, and nothing of it is ever shown.
	secret.hook("PostToolUse")
	secret.hook("PreToolUse", "tool_name", "Read")
	open.hook("PreToolUse", "tool_name", "Grep")
	awaitShown(t, srv, "the visible session searching", lines(surfaceLine, "Searching"))
	for _, v := range showing(t, srv) {
		if strings.Contains(v.Raw, "sessions") || strings.Contains(v.Raw, "Editing") || strings.Contains(v.Raw, "Reading") || strings.Contains(v.Raw, "secret") {
			t.Errorf("Discord was shown something of the hidden session: %s", v.Raw)
		}
	}

	// When the visible session ends, the hidden one is not shown instead.
	open.finish()
	awaitShown(t, srv, "nothing", func(v shown) bool { return v.Clear })
	secret.awaitStatus("Sessions: 0")
	secret.finish()
}

// E-pause: a pause clears the card for every session, survives the host
// being killed, and a resume brings back the present without any new event
// (ADR-0012, CRP-073).
func TestAPauseOutlivesTheHost(t *testing.T) {
	t.Parallel()
	s := newScene(t)
	srv := s.discord(fakediscord.Behavior{})

	host := s.start("first")
	host.awaitStatus("Role: host", "Discord: connected")
	follower := s.start("second")
	follower.awaitStatus("Role: follower", "Sessions: 2")
	follower.hook("UserPromptSubmit")
	follower.hook("PreToolUse", "tool_name", "Bash")
	awaitShown(t, srv, "the follower's work", lines(surfaceLine, "Running commands · 2 sessions"))
	follower.awaitPreview("Presence: shown.", "Line 1: "+surfaceLine, "Line 2: Running commands · 2 sessions")

	// The host is asked to pause. The follower is told, without asking.
	if got := host.Call(cli.ToolPause, nil); got.IsError || !strings.Contains(got.Text, "paused for every session until it is resumed") {
		t.Fatalf("the pause tool answered %+v", got)
	}
	awaitShown(t, srv, "nothing", func(v shown) bool { return v.Clear })
	follower.awaitStatus("Paused: until resumed")
	follower.awaitPreview("Presence: paused until resumed. Nothing is shown.")

	host.Kill()
	follower.awaitStatus("Role: host", "Discord: connected", "Sessions: 1", "Paused: until resumed")
	follower.awaitPreview("Presence: paused until resumed. Nothing is shown.")

	// The follower resumes, with no hook in between: what returns is what the
	// session was doing all along.
	if got := follower.Call(cli.ToolPause, map[string]any{"resume": true}); got.IsError || !strings.Contains(got.Text, "resumed") {
		t.Fatalf("the pause tool answered %+v", got)
	}
	awaitShown(t, srv, "the follower's work, from the follower", func(v shown) bool {
		return v.Conn == 2 && lines(surfaceLine, "Running commands")(v)
	})
	follower.awaitStatus("Paused: no")
	// The new host showed nothing until then: its one activity is that one.
	sets := 0
	for _, v := range showing(t, srv) {
		if v.Conn == 2 && !v.Clear {
			sets++
		}
	}
	if sets != 1 {
		t.Errorf("the new host showed %d activities, want one, after the resume: %v", sets, showing(t, srv))
	}
	follower.finish()
}

// E-pause, with an older host: a host from before pausing ignores the
// messages, and the follower says that pausing is unavailable instead of
// claiming a pause that is not in force. The test is that host: it answers
// what a host answered before pausing existed, and passes over the rest.
func TestAnOlderHostCannotPause(t *testing.T) {
	t.Parallel()
	s := newScene(t)
	h := hostControl(t, s)

	one := s.start("one")
	one.awaitStatus("Role: follower", "Paused: unavailable, the presence host is an older version")
	for _, arguments := range []map[string]any{nil, {"minutes": 5}, {"resume": true}} {
		if got := one.Call(cli.ToolPause, arguments); !got.IsError || !strings.Contains(got.Text, "Pausing is unavailable") {
			t.Errorf("the pause tool answered %+v for %v, want that pausing is unavailable", got, arguments)
		}
	}
	if got := one.preview(); !strings.Contains(got, "the presence host is an older version") {
		t.Errorf("the preview is %q, want it to say why there is no card", got)
	}
	one.hook("UserPromptSubmit")
	one.hook("PreToolUse", "tool_name", "Read")
	eventually(t, "the session's work to reach the host", func() bool {
		return strings.Contains(h.received(), `"tool":"reading"`)
	})
	one.finish()
	if got := h.received(); strings.Contains(got, `"type":"pause"`) || strings.Contains(got, `"type":"resume"`) {
		t.Errorf("the follower sent a pause its host cannot take:\n%s", got)
	}
}
