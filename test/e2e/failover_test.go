package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// E3: two sessions, one working and one idle. The working one is the
// follower, so that what is shown cannot be the host showing itself. The
// elapsed timer is the idle one's, which started first.
func TestTwoSessions(t *testing.T) {
	t.Parallel()
	s := newScene(t)
	srv := s.discord(fakediscord.Behavior{})

	idle := s.start("idle")
	idle.awaitStatus("Role: host", "Discord: connected")
	alone := awaitShown(t, srv, "the first session", lines(surfaceLine, ""))

	// The elapsed timer counts in seconds. The second session starts in a
	// later second than the first, so that the two starts can be told apart.
	eventually(t, "the next second", func() bool { return time.Now().Unix() > alone.Start })
	working := s.start("working")
	working.awaitStatus("Role: follower", "Sessions: 2")

	working.hook("UserPromptSubmit")
	working.hook("PreToolUse", "tool_name", "Read")
	both := awaitShown(t, srv, "the working session and the count", lines(surfaceLine, "Reading files · 2 sessions"))
	if both.Start != alone.Start {
		t.Errorf("the elapsed timer starts at %d, want the idle session's %d, the earliest start of the two", both.Start, alone.Start)
	}

	working.finish()
	idle.finish()
}

// E4 and E5: the host goes away while a follower lives, cleanly and by being
// killed. The host's session started first, so the elapsed timer is its start
// until it goes. Then the timer moves forward once, to the follower's own
// start, which the follower carried with it: not to the moment it took over.
func TestHostLeaves(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		leave func(host *session)
		// cleared says whether the host tells Discord to show nothing before
		// it goes.
		cleared bool
	}{
		{"exits cleanly", func(host *session) { host.finish() }, true},
		{"is killed", func(host *session) { host.Kill() }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newScene(t)
			srv := s.discord(fakediscord.Behavior{})

			host := s.start("first")
			host.awaitStatus("Role: host", "Discord: connected")
			first := awaitShown(t, srv, "the first session", lines(surfaceLine, ""))
			eventually(t, "the next second", func() bool { return time.Now().Unix() > first.Start })

			opening := time.Now().Unix()
			follower := s.start("second")
			opened := time.Now().Unix()
			follower.awaitStatus("Role: follower", "Sessions: 2")
			follower.hook("UserPromptSubmit")
			follower.hook("PreToolUse", "tool_name", "Bash")
			before := awaitShown(t, srv, "the follower's work", lines(surfaceLine, "Running commands · 2 sessions"))
			if before.Conn != 1 || before.Start != first.Start {
				t.Fatalf("before the host left, Discord showed %v; want the host's start %d, the earliest of the two, on the first connection", before, first.Start)
			}

			// The takeover happens in a later second than the follower's
			// start, so that the two can be told apart.
			eventually(t, "the second after the follower started", func() bool { return time.Now().Unix() > opened })
			tt.leave(host)

			follower.awaitStatus("Role: host", "Discord: connected", "Sessions: 1")
			restored := awaitShown(t, srv, "the follower's work, from the follower", func(v shown) bool {
				return v.Conn == 2 && lines(surfaceLine, "Running commands")(v)
			})
			if restored.Start < opening || restored.Start > opened {
				t.Errorf("the elapsed timer begins at %d, want the follower's own start, between %d and %d; a later time means it restarted at the takeover",
					restored.Start, opening, opened)
			}
			if got := lastOn(t, srv, 1); got.Clear != tt.cleared {
				t.Errorf("the last thing on the old host's connection was %v; want a clear: %t", got, tt.cleared)
			}
			if handshakes := count(srv, fakediscord.KindHandshake); handshakes != 2 {
				t.Errorf("%d handshakes, want two: the old host's and the new one's", handshakes)
			}
			follower.finish()
		})
	}
}

// lastOn is the last thing Discord was told to show on one connection.
func lastOn(t *testing.T, srv *fakediscord.Server, conn int) shown {
	t.Helper()
	var last shown
	for _, v := range showing(t, srv) {
		if v.Conn == conn {
			last = v
		}
	}
	return last
}

// E6: a follower is killed.
func TestFollowerIsKilled(t *testing.T) {
	t.Parallel()
	s := newScene(t)
	srv := s.discord(fakediscord.Behavior{})

	host := s.start("host")
	host.awaitStatus("Role: host", "Discord: connected")
	follower := s.start("follower")
	follower.awaitStatus("Role: follower", "Sessions: 2")
	host.hook("UserPromptSubmit")
	both := awaitShown(t, srv, "both sessions", lines(surfaceLine, "Thinking · 2 sessions"))

	follower.Kill()

	host.awaitStatus("Role: host", "Sessions: 1")
	left := awaitShown(t, srv, "the host's session alone", lines(surfaceLine, "Thinking"))
	if left.Start != both.Start || left.Conn != 1 {
		t.Errorf("after the follower died Discord showed %v; want the host's session as before, %v, without the count", left, both)
	}
	host.finish()
}

// E7: Discord is not running when the session starts, and is started later.
func TestDiscordAppearsLater(t *testing.T) {
	t.Parallel()
	s := newScene(t)

	c := s.start("early")
	c.awaitStatus("Role: host", "Discord: disconnected", "Sessions: 1")
	c.hook("UserPromptSubmit")
	c.hook("PreToolUse", "tool_name", "Grep")

	srv := s.discord(fakediscord.Behavior{})
	c.awaitStatus("Discord: connected")
	// What happened while Discord was away is not replayed: it is shown the
	// present.
	awaitShown(t, srv, "the session", func(shown) bool { return true })
	if got := showing(t, srv)[0]; !lines(surfaceLine, "Searching")(got) {
		t.Errorf("the first thing Discord was told to show was %v, want the session as it is now", got)
	}
	c.finish()
}

// E8: Discord goes away and comes back.
func TestDiscordRestarts(t *testing.T) {
	t.Parallel()
	s := newScene(t)
	first := s.discord(fakediscord.Behavior{})

	c := s.start("steady")
	c.awaitStatus("Role: host", "Discord: connected")
	c.hook("UserPromptSubmit")
	before := awaitShown(t, first, "the session at work", lines(surfaceLine, "Thinking"))

	first.Close()
	eventually(t, "the host to notice that Discord is gone", func() bool {
		return !strings.Contains(c.status(), "Discord: connected")
	})

	second := s.discord(fakediscord.Behavior{})
	c.awaitStatus("Discord: connected")
	restored := awaitShown(t, second, "the session again", lines(surfaceLine, "Thinking"))
	if restored.Start != before.Start {
		t.Errorf("the elapsed timer restarted: it began at %d and now begins at %d", before.Start, restored.Start)
	}
	c.finish()
}

// E12: a newer binary starts while an older one is the host.
func TestNewerBinaryTakesOver(t *testing.T) {
	t.Parallel()
	newer, err := newerBinary()
	if err != nil {
		t.Fatal(err)
	}
	s := newScene(t)
	srv := s.discord(fakediscord.Behavior{})

	old := s.start("old")
	old.awaitStatus("Role: host", "Discord: connected")
	old.hook("UserPromptSubmit")
	before := awaitShown(t, srv, "the old binary's session", lines(surfaceLine, "Thinking"))
	if _, stdout, _ := s.run(built.current, "status"); !strings.Contains(stdout, "Version: "+versionCurrent+"\n") {
		t.Fatalf("status printed %q, want the old binary as the host", stdout)
	}

	upgraded := s.startBinary(newer, "new")

	upgraded.awaitStatus("Role: host", "Discord: connected", "Sessions: 2")
	old.awaitStatus("Role: follower", "Sessions: 2")
	restored := awaitShown(t, srv, "both sessions, from the newer binary", func(v shown) bool {
		return v.Conn == 2 && lines(surfaceLine, "Thinking · 2 sessions")(v)
	})
	if restored.Start != before.Start {
		t.Errorf("the elapsed timer restarted: it began at %d and now begins at %d", before.Start, restored.Start)
	}
	// The status command of either version finds the newer host.
	for _, binary := range []string{built.current, newer} {
		code, stdout, stderr := s.run(binary, "status")
		if code != 0 || !strings.Contains(stdout, "Version: "+versionNewer+"\n") || !strings.Contains(stdout, "Sessions: 2\n") {
			t.Errorf("status: exit %d, printed %q and %q; want the newer binary as the host of two sessions", code, stdout, stderr)
		}
	}
	// The old binary does not take the role back.
	old.hook("PreToolUse", "tool_name", "Bash")
	awaitShown(t, srv, "the old binary's work, through the newer host", func(v shown) bool {
		return v.Conn == 2 && lines(surfaceLine, "Running commands · 2 sessions")(v)
	})
	upgraded.awaitStatus("Role: host")
	if handshakes := count(srv, fakediscord.KindHandshake); handshakes != 2 {
		t.Errorf("%d handshakes, want two: the role changed hands once", handshakes)
	}

	old.finish()
	upgraded.finish()
}
