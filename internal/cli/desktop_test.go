package cli

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/adapter/desktop"
	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/mcp"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// The client names Claude Desktop gives its two copies of the server
// (CRP-002).
const (
	clientDesktop = "claude-ai"
	clientPassive = "local-agent-mode-Rich Presence"
)

// pass polls cond as eventually does, and lets the fake clock run meanwhile,
// so that the waits of the host and of the Discord connection end.
func (w *world) pass(t *testing.T, what string, cond func() bool) {
	t.Helper()
	eventually(t, what, func() bool {
		w.clock.Advance(100 * time.Millisecond)
		return cond()
	})
}

// passUntilStatus lets the clock run until the status tool says every one of
// want.
func (w *world) passUntilStatus(t *testing.T, s *client, want ...string) {
	t.Helper()
	w.pass(t, "status "+strings.Join(want, ", "), func() bool {
		got := s.status()
		for _, text := range want {
			if !strings.Contains(got, text) {
				return false
			}
		}
		return true
	})
}

// card is what the fake Discord was last told to show.
type card struct {
	Details    string `json:"details"`
	State      string `json:"state"`
	Timestamps struct {
		Start int64 `json:"start"`
	} `json:"timestamps"`
	// clear is set when it was told to show nothing.
	clear bool
}

// showing is the last thing the fake Discord was told, or false if it was
// told nothing yet.
func showing(t *testing.T, srv *fakediscord.Server) (card, bool) {
	t.Helper()
	var last card
	told := false
	for _, ev := range srv.Events() {
		switch ev.Kind {
		case fakediscord.KindClearActivity:
			last, told = card{clear: true}, true
		case fakediscord.KindSetActivity:
			last = card{}
			if err := json.Unmarshal(ev.Activity, &last); err != nil {
				t.Fatalf("the activity %s cannot be read: %v", ev.Activity, err)
			}
			told = true
		}
	}
	return last, told
}

// passUntilShown lets the clock run until the fake Discord shows what want
// accepts.
func (w *world) passUntilShown(t *testing.T, srv *fakediscord.Server, what string, want func(card) bool) card {
	t.Helper()
	var last card
	w.pass(t, "Discord to show "+what, func() bool {
		var told bool
		last, told = showing(t, srv)
		return told && want(last)
	})
	return last
}

func (w *world) noRuntimeDirectory(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(w.runtime); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the runtime directory, where the lock and the socket go, was made, or could not be read: %v", err)
	}
}

func TestMCPDesktopReportsTheApp(t *testing.T) {
	tests := []struct {
		level string
		state string
	}{
		{"minimal", ""},
		{"standard", "Idle"},
		{"full", "Idle"},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			w := newWorld(t)
			w.env = w.env.with("RICH_PRESENCE_PRIVACY", tt.level)
			srv := w.discord(t)
			opened := w.clock.Now().Unix()

			s := w.startSession(t)
			s.initializeAs(clientDesktop)
			if got := strings.Join(s.toolNames(), ","); got != "presence_status" {
				t.Errorf("tools = %q, want the status tool alone", got)
			}
			w.passUntilStatus(t, s, "Role: host", "Discord: connected", "Sessions: 1", "Privacy: "+tt.level)
			got := w.passUntilShown(t, srv, "the app", func(c card) bool { return !c.clear })
			if got.Details != "Claude Desktop" || got.State != tt.state || got.Timestamps.Start != opened {
				t.Errorf("Discord shows %+v, want Claude Desktop, %q and a timer from %d", got, tt.state, opened)
			}

			if code := s.finish(); code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			if last, _ := showing(t, srv); !last.clear {
				t.Errorf("after the input closed Discord shows %+v, want nothing", last)
			}
		})
	}
}

func TestMCPDesktopBesideAWorkingCodeSession(t *testing.T) {
	w := newWorld(t)
	srv := w.discord(t)

	app := w.startSession(t)
	app.initializeAs(clientDesktop)
	w.passUntilStatus(t, app, "Role: host", "Discord: connected")

	w.clock.Advance(time.Minute)
	started := w.clock.Now().Unix()
	w.sys.sessionID = func() string { return "code-session" }
	coding := w.startSession(t)
	coding.initialize()
	w.passUntilStatus(t, coding, "Role: follower", "Sessions: 2")
	// The hook says where the session is, which is what lifts it from the
	// lowest level to the configured one.
	call, err := json.Marshal(map[string]any{"name": "presence_event", "arguments": map[string]string{"event": "UserPromptSubmit", "session_id": "abc", "cwd": t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	coding.request("tools/call", string(call))

	got := w.passUntilShown(t, srv, "the Code session", func(c card) bool { return c.Details == "Claude Code" && c.State != "" })
	if got.State != "Thinking · 2 sessions" || got.Timestamps.Start < started {
		t.Errorf("Discord shows %+v, want the Code session thinking, a count of two, and its own timer from %d", got, started)
	}

	// The Code session ends, and the app is what is left.
	if code := coding.finish(); code != 0 {
		t.Errorf("Code exit code = %d, want 0", code)
	}
	w.passUntilShown(t, srv, "the app alone", func(c card) bool { return c.Details == "Claude Desktop" && c.State == "Idle" })
	if code := app.finish(); code != 0 {
		t.Errorf("Desktop exit code = %d, want 0", code)
	}
}

func TestMCPPassiveCopyReportsNoSession(t *testing.T) {
	w := newWorld(t)
	w.discord(t)

	// As Claude Desktop starts them: one copy of each client name.
	app := w.startSession(t)
	app.initializeAs(clientDesktop)
	w.passUntilStatus(t, app, "Role: host", "Discord: connected", "Sessions: 1")

	w.sys.sessionID = func() string { return "second-copy" }
	passive := w.startSession(t)
	passive.initializeAs(clientPassive)
	if got := strings.Join(passive.toolNames(), ","); got != "presence_status" {
		t.Errorf("tools = %q, want the status tool alone", got)
	}
	// It reports what the host says of itself, and the host knows one session.
	w.passUntilStatus(t, passive, "Role: passive", "Discord: connected", "Sessions: 1")
	if got := app.status(); !strings.Contains(got, "Sessions: 1") {
		t.Errorf("the status of the host = %q, want one session for the two copies", got)
	}

	// The host goes, and the passive copy stays and takes nothing over.
	if code := app.finish(); code != 0 {
		t.Errorf("Desktop exit code = %d, want 0", code)
	}
	w.passUntilStatus(t, passive, "Role: passive", "Discord: unknown", "Sessions: 0")
	if got := strings.Join(passive.toolNames(), ","); got != "presence_status" {
		t.Errorf("after the host left, tools = %q, want the status tool still", got)
	}
	if code := passive.finish(); code != 0 {
		t.Errorf("passive exit code = %d, want 0", code)
	}
}

func TestMCPPassiveCopyNeverAsksForTheLock(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want string
	}{
		{"with no host running", nil, "Role: passive\nDiscord: unknown\nSessions: 0\n"},
		{"with presence off", []string{"RICH_PRESENCE_ENABLED", "false"}, "Role: off\nDiscord: disconnected\nSessions: 0\n"},
		{"with no runtime directory", []string{"RICH_PRESENCE_RUNTIME_DIR", "relative"}, "Role: off\nDiscord: disconnected\nSessions: 0\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.env = w.env.with(tt.env...)
			srv := w.discord(t)

			s := w.startSession(t)
			s.initializeAs(clientPassive)
			for range 3 {
				if got := s.status(); !strings.HasPrefix(got, tt.want) {
					t.Errorf("status = %q, want it to begin %q", got, tt.want)
				}
			}
			if code := s.finish(); code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			// The process has ended, so what it did not do, it never will.
			w.noRuntimeDirectory(t)
			if events := srv.Events(); len(events) != 0 {
				t.Errorf("Discord was contacted: %v", events)
			}
		})
	}
}

func TestMCPDesktopStaysWhileItsInputIsOpen(t *testing.T) {
	// alive shows the process still serves its client.
	alive := func(t *testing.T, s *client) {
		t.Helper()
		if got := strings.Join(s.toolNames(), ","); got != "presence_status" {
			t.Errorf("tools = %q, want the status tool still", got)
		}
		select {
		case code := <-s.done:
			t.Fatalf("the process ended with code %d while its input was open", code)
		default:
		}
	}

	t.Run("the host is lost", func(t *testing.T) {
		w := newWorld(t)
		w.discord(t)
		first := w.startSession(t)
		first.initialize()
		w.passUntilStatus(t, first, "Role: host")

		w.sys.sessionID = func() string { return "the-app" }
		app := w.startSession(t)
		app.initializeAs(clientDesktop)
		w.passUntilStatus(t, app, "Role: follower", "Sessions: 2")

		if code := first.finish(); code != 0 {
			t.Errorf("host exit code = %d, want 0", code)
		}
		w.passUntilStatus(t, app, "Role: host", "Sessions: 1")
		alive(t, app)
		if code := app.finish(); code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	t.Run("Discord is lost", func(t *testing.T) {
		w := newWorld(t)
		srv := w.discord(t)
		app := w.startSession(t)
		app.initializeAs(clientDesktop)
		w.passUntilStatus(t, app, "Role: host", "Discord: connected")

		srv.Close()
		w.passUntilStatus(t, app, "Role: host", "Discord: disconnected", "Sessions: 1")
		alive(t, app)
		if code := app.finish(); code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	t.Run("it is asked to stand down", func(t *testing.T) {
		w := newWorld(t)
		w.discord(t)
		app := w.startSession(t)
		app.initializeAs(clientDesktop)
		w.passUntilStatus(t, app, "Role: host", "Discord: connected")

		// A newer binary starts, and the host gives the role up to it.
		w.sys.version = "1.3.0"
		w.sys.sessionID = func() string { return "newer" }
		newer := w.startSession(t)
		newer.initialize()
		w.passUntilStatus(t, newer, "Role: host", "Sessions: 2")
		w.passUntilStatus(t, app, "Role: follower", "Sessions: 2")
		alive(t, app)

		if code := app.finish(); code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		if code := newer.finish(); code != 0 {
			t.Errorf("newer exit code = %d, want 0", code)
		}
	})
}

func TestMCPDesktopWithPresenceOff(t *testing.T) {
	w := newWorld(t)
	w.env = w.env.with("RICH_PRESENCE_ENABLED", "false")
	srv := w.discord(t)

	s := w.startSession(t)
	s.initializeAs(clientDesktop)
	if got := s.status(); !strings.HasPrefix(got, "Role: off\nDiscord: disconnected\nSessions: 0\n") {
		t.Errorf("status = %q, want role off and no sessions", got)
	}
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	w.noRuntimeDirectory(t)
	if events := srv.Events(); len(events) != 0 {
		t.Errorf("Discord was contacted: %v", events)
	}
}

func TestMCPDesktopSessionThatCannotBeNamedOffersNoTools(t *testing.T) {
	w := newWorld(t)
	w.sys.sessionID = func() string { return "" }
	s := w.startSession(t)
	s.initializeAs(clientDesktop)
	if got := s.toolNames(); len(got) != 0 {
		t.Errorf("tools = %q, want none", got)
	}
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestInitializeAfterShutdownStartsNothing(t *testing.T) {
	w := newWorld(t)
	m := &mcpSession{sys: w.sys, getenv: w.env.get}
	m.shutdown()
	if tools := m.initialize(mcp.ClientInfo{Name: clientDesktop, Version: "0.1.0"}); tools != nil {
		t.Errorf("initialize after shutdown offered %d tools, want none", len(tools))
	}
	w.noRuntimeDirectory(t)
}

func TestHostAsker(t *testing.T) {
	passive := desktop.Status{Role: desktop.RolePassive}
	answered := desktop.Status{Role: desktop.RolePassive, Discord: desktop.DiscordConnected, Sessions: 3}

	t.Run("answers from memory and asks once at a time", func(t *testing.T) {
		asked := make(chan struct{})
		answer := make(chan protocol.StatusResult)
		h := &hostAsker{ask: func() (protocol.StatusResult, error) {
			asked <- struct{}{}
			return <-answer, nil
		}}
		// Neither call waits for the host, and the second starts no question.
		for range 2 {
			if got := h.Status(); got != passive {
				t.Errorf("before any answer, Status() = %+v, want %+v", got, passive)
			}
		}
		<-asked
		answer <- protocol.StatusResult{Discord: protocol.DiscordConnected, Sessions: 3}
		h.stop()
		select {
		case <-asked:
			t.Error("a second question was asked while the first was on its way")
		default:
		}
		if got := h.Status(); got != answered {
			t.Errorf("after the answer, Status() = %+v, want %+v", got, answered)
		}
	})

	t.Run("asks nothing once stopped", func(t *testing.T) {
		h := &hostAsker{ask: func() (protocol.StatusResult, error) {
			t.Error("the host was asked after stop")
			return protocol.StatusResult{}, nil
		}}
		h.stop()
		if got := h.Status(); got != passive {
			t.Errorf("Status() = %+v, want %+v", got, passive)
		}
		h.stop()
	})

	failures := map[string]func() (protocol.StatusResult, error){
		"a host that cannot be reached": func() (protocol.StatusResult, error) {
			return protocol.StatusResult{Discord: protocol.DiscordConnected, Sessions: 9}, errors.New("no host")
		},
		"a question that panics": func() (protocol.StatusResult, error) { panic("secret") },
	}
	for name, fail := range failures {
		t.Run(name+" forgets the last answer", func(t *testing.T) {
			results := make(chan func() (protocol.StatusResult, error), 2)
			results <- func() (protocol.StatusResult, error) {
				return protocol.StatusResult{Discord: protocol.DiscordConnected, Sessions: 3}, nil
			}
			results <- fail
			h := &hostAsker{ask: func() (protocol.StatusResult, error) { return (<-results)() }}
			h.Status()
			eventually(t, "the first answer", func() bool {
				h.mu.Lock()
				defer h.mu.Unlock()
				return h.last.Sessions == 3
			})
			// This call reports the first answer and asks the question that fails.
			if got := h.Status(); got != answered {
				t.Errorf("Status() = %+v, want %+v", got, answered)
			}
			eventually(t, "the failure", func() bool {
				h.mu.Lock()
				defer h.mu.Unlock()
				return !h.asking && h.last == desktop.Status{}
			})
			h.stop()
			if got := h.Status(); got != passive {
				t.Errorf("after the failure, Status() = %+v, want %+v", got, passive)
			}
		})
	}
}
