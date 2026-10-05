package e2e

import (
	"bufio"
	"bytes"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/cli"
	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	ctransport "github.com/Zafnok/claude-rich-presence/internal/control/transport"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// marker is a string that stands for one piece of the user's work. It is
// unlike anything the product writes of its own accord, so finding it
// anywhere means that piece got there.
func marker(of string) string { return "zqmark-" + of + "-7f3a9c" }

// contains reports whether text holds the marker, in any letter case: a
// marker that was only lower-cased or capitalised on its way has leaked all
// the same.
func contains(text, mark string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(mark))
}

// controlHost is the test standing in for a presence host: it holds the lock,
// listens on the control socket, welcomes whoever connects and keeps every
// byte it is sent. It is how the control channel is observed without a tap.
type controlHost struct {
	t    *testing.T
	lock *ctransport.HostLock
	wg   sync.WaitGroup

	mu      sync.Mutex
	traffic bytes.Buffer
	conns   []net.Conn
}

// hostControl takes the scene's lock and serves its control socket, before
// any process of the scene starts.
func hostControl(t *testing.T, s *scene) *controlHost {
	t.Helper()
	lock, err := ctransport.Acquire(s.paths())
	if err != nil {
		t.Fatalf("taking the host lock: %v", err)
	}
	h := &controlHost{t: t, lock: lock}
	t.Cleanup(h.release)
	listener, err := lock.Listen()
	if err != nil {
		t.Fatalf("listening on the control socket: %v", err)
	}
	h.wg.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			h.mu.Lock()
			h.conns = append(h.conns, conn)
			h.mu.Unlock()
			h.wg.Go(func() { h.serve(conn) })
		}
	})
	return h
}

// serve answers one connection as a host does, as far as a follower needs:
// a welcome for its hello, and a result for each status request.
func (h *controlHost) serve(conn net.Conn) {
	lines := bufio.NewReader(conn)
	for {
		line, err := lines.ReadBytes('\n')
		h.mu.Lock()
		h.traffic.Write(line)
		h.mu.Unlock()
		if err != nil {
			return
		}
		var answer protocol.Message
		switch msg, _ := protocol.Decode(line); msg := msg.(type) {
		case protocol.Hello:
			answer = protocol.Answer(msg, versionCurrent)
		case protocol.Status:
			answer = protocol.StatusResult{Discord: protocol.DiscordConnected, Sessions: 1, Version: versionCurrent}
		default:
			continue
		}
		if protocol.Encode(conn, answer) != nil {
			return
		}
	}
}

// received is every byte the followers have sent so far.
func (h *controlHost) received() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.traffic.String()
}

// release gives the role up as a host that ends does: the socket closes, the
// connections close and the lock is free for a follower to take. A second
// call does nothing.
func (h *controlHost) release() {
	if err := h.lock.Release(); err != nil {
		h.t.Errorf("releasing the host lock: %v", err)
	}
	h.mu.Lock()
	for _, conn := range h.conns {
		_ = conn.Close()
	}
	h.mu.Unlock()
	h.wg.Wait()
}

// work is one session's worth of hooks, carrying a marker in every field
// that holds the user's work, and in every field the adapter reads but must
// not pass on as it is.
type work struct {
	// forbidden are the markers that may appear nowhere, at any level.
	forbidden []string
	// project is the last element of the working directory: the project's
	// name, published at the full level and at no other.
	project string
	cwd     string
}

func newWork() *work {
	w := &work{project: marker("project")}
	parent := w.mark("cwd-parent")
	w.cwd = "/" + parent + "/" + w.project
	if runtime.GOOS == "windows" {
		w.cwd = `C:\` + parent + `\` + w.project
	}
	return w
}

// mark makes a marker that may appear nowhere.
func (w *work) mark(of string) string {
	m := marker(of)
	w.forbidden = append(w.forbidden, m)
	return m
}

// send plays the hooks into a session. Every call carries what every hook of
// Claude Code carries, and then what its own event adds, as recorded in
// docs/research/crp-001-claude-code-adapter.md.
func (w *work) send(c *session) {
	c.t.Helper()
	hook := func(event string, fields map[string]any) {
		c.t.Helper()
		arguments := map[string]any{
			"event":           event,
			"session_id":      c.id,
			"cwd":             w.cwd,
			"hook_event_name": event,
			"transcript_path": "/" + w.mark("transcript-path") + "/transcript.jsonl",
			"scratchpad_dir":  "/" + w.mark("scratchpad-dir"),
			"prompt_id":       w.mark("prompt-id"),
			"permission_mode": w.mark("permission-mode"),
			"session_title":   w.mark("session-title"),
		}
		for k, v := range fields {
			arguments[k] = v
		}
		c.event(arguments)
	}
	toolInput := map[string]any{
		"command":     "echo " + w.mark("tool-input-command"),
		"file_path":   "/" + w.mark("tool-input-file-path") + "/main.go",
		"description": w.mark("tool-input-description"),
		"content":     w.mark("tool-input-content"),
	}

	hook("SessionStart", map[string]any{"source": "compact", "model": "claude-opus-5-5"})
	hook("SessionStart", map[string]any{"source": w.mark("source"), "model": "claude-" + w.mark("model") + "-9-9"})
	hook("UserPromptSubmit", map[string]any{"prompt": w.mark("prompt")})
	hook("PreToolUse", map[string]any{"tool_name": "Bash", "tool_input": toolInput, "tool_use_id": w.mark("tool-use-id")})
	hook("PostToolUse", map[string]any{"tool_name": "Bash", "tool_input": toolInput,
		"tool_response": map[string]any{"stdout": w.mark("tool-response-stdout"), "stderr": w.mark("tool-response-stderr")}})
	// A tool whose name is not in the vocabulary: its name says what the
	// user has installed and is doing.
	hook("PreToolUse", map[string]any{"tool_name": "mcp__" + w.mark("tool-server") + "__" + w.mark("tool-name"), "tool_input": toolInput})
	hook("PostToolUseFailure", map[string]any{"tool_name": w.mark("failed-tool-name"), "error": w.mark("tool-error"), "tool_input": toolInput})
	hook("Notification", map[string]any{"notification_type": "permission_prompt", "message": w.mark("notification-message")})
	hook("Notification", map[string]any{"notification_type": w.mark("notification-type"), "message": w.mark("notification-message-2")})
	hook("PreToolUse", map[string]any{"tool_name": "Agent", "tool_input": map[string]any{"prompt": w.mark("agent-prompt")}})
	hook("SubagentStart", map[string]any{"agent_id": w.mark("agent-id"), "agent_type": w.mark("agent-type")})
	hook("SubagentStop", map[string]any{"agent_id": marker("agent-id"), "agent_type": marker("agent-type"),
		"agent_transcript_path": "/" + w.mark("agent-transcript-path"), "last_assistant_message": w.mark("agent-last-message")})
	hook("PreCompact", map[string]any{"trigger": "manual", "custom_instructions": w.mark("custom-instructions")})
	hook("PostCompact", map[string]any{"trigger": "manual", "compact_summary": w.mark("compact-summary")})
	hook("PostModelSwitch", map[string]any{"to_model": "claude-sonnet-5-5", "from_model": w.mark("from-model"), "requested_model": w.mark("requested-model")})
	hook("PostModelSwitch", map[string]any{"to_model": w.mark("to-model")})
	hook("Stop", map[string]any{"last_assistant_message": w.mark("last-assistant-message")})
	hook("StopFailure", map[string]any{"error": w.mark("stop-error"), "last_assistant_message": w.mark("failed-last-message")})

	// Calls the adapter refuses whole: an event that does not exist, a field
	// of the wrong type, and a call made by the model, not by a hook.
	hook(w.mark("event-name"), map[string]any{"prompt": w.mark("unknown-event-prompt")})
	hook("PreToolUse", map[string]any{"tool_name": []string{w.mark("tool-name-list")}})
	if _, rpcErr := c.Request("tools/call", map[string]any{
		"name":      cli.ToolEvent,
		"arguments": map[string]any{"event": "UserPromptSubmit", "session_id": c.id, "cwd": w.cwd, "prompt": w.mark("model-call-prompt")},
		"_meta":     map[string]any{"claudecode/toolUseId": w.mark("model-call-meta")},
	}); rpcErr != nil {
		c.t.Fatalf("a call made by the model failed: %+v", rpcErr)
	}
}

// E9: at each privacy level, nothing of the user's work leaves the adapter
// but what the level allows.
func TestPrivacy(t *testing.T) {
	t.Parallel()
	for _, level := range []string{"minimal", "standard", "full"} {
		t.Run(level, func(t *testing.T) {
			t.Parallel()
			s := newScene(t)
			s.extra = []string{config.EnvPrefix + "PRIVACY=" + level}
			srv := s.discord(fakediscord.Behavior{})
			w := newWork()
			sessionMark := marker("session-id")

			// First the binary follows the test, which keeps what it is sent.
			h := hostControl(t, s)
			c := s.start(sessionMark + "-a")
			c.cwd = w.cwd
			c.awaitStatus("Role: follower")
			w.send(c)
			// A hook under a new session id opens that session at every
			// level, so its arrival shows that all before it has arrived.
			c.id = sessionMark + "-b"
			c.hook("UserPromptSubmit", "prompt", w.mark("prompt-b"))
			eventually(t, "the last hook to reach the control socket", func() bool {
				return contains(h.received(), c.id)
			})

			// Then the test gives up the role, and the binary takes it and
			// talks to Discord.
			h.release()
			c.awaitStatus("Role: host", "Discord: connected")
			awaitShown(t, srv, "the session", func(v shown) bool { return !v.Clear })
			w.cwd += "-2"
			w.project += "-2"
			c.cwd = w.cwd
			w.send(c)
			c.hook("PreToolUse", "tool_name", "Edit")
			var last shown
			if level != "minimal" {
				last = awaitShown(t, srv, "the last hook", func(v shown) bool { return v.State == "Editing files · Sonnet 5.5" })
			}
			// What a user would paste into a public issue, taken while the
			// session is there to be reported on.
			status := c.status()
			_, doctorOut, doctorErr := s.run(built.current, "doctor")
			_, statusOut, statusErr := s.run(built.current, "status")
			c.finish()

			// The process has ended: what it wrote is all it will write.
			control := h.received()
			var discord strings.Builder
			for _, ev := range srv.Events() {
				discord.Write(ev.Payload)
				discord.WriteByte('\n')
			}
			places := map[string]string{
				"the fake Discord":            discord.String(),
				"the control socket":          control,
				"the log":                     s.logText(),
				"standard error":              c.Stderr(),
				"the status tool":             status,
				"the doctor's report":         doctorOut + doctorErr,
				"the status command's output": statusOut + statusErr,
			}

			// The observations are real: each place did carry the session.
			if !contains(control, protocol.TypeSync) || !contains(control, protocol.TypeEvent) {
				t.Errorf("the control socket carried no session: %q", control)
			}
			if count(srv, fakediscord.KindSetActivity) == 0 {
				t.Error("the fake Discord was told to show nothing at all")
			}
			if !contains(places["the log"], "became the presence host") {
				t.Errorf("the log does not record the session: %q", places["the log"])
			}

			for place, text := range places {
				for _, mark := range w.forbidden {
					if contains(text, mark) {
						t.Errorf("%s carried %s, which may never leave the adapter:\n%s", place, mark, text)
					}
				}
				// The session id is how the host tells sessions apart, so the
				// control socket carries it. Nothing else may.
				if place != "the control socket" && contains(text, sessionMark) {
					t.Errorf("%s carried the session id:\n%s", place, text)
				}
			}

			// The project name is published at full, and only there.
			published := level == "full"
			for _, place := range []string{"the fake Discord", "the control socket"} {
				if got := contains(places[place], marker("project")); got != published {
					t.Errorf("at %s, %s carried the project name: %t, want %t:\n%s", level, place, got, published, places[place])
				}
			}
			for _, place := range []string{"the log", "standard error", "the status tool", "the doctor's report", "the status command's output"} {
				if contains(places[place], marker("project")) {
					t.Errorf("%s carried the project name:\n%s", place, places[place])
				}
			}
			switch level {
			case "minimal":
				// In use, and for how long: no status, no model, no count.
				for _, v := range showing(t, srv) {
					if !v.Clear && (v.Details != surfaceLine || v.State != "") {
						t.Errorf("at minimal Discord was told to show %v, want the surface alone", v)
					}
				}
				for _, word := range []string{`"turn_started"`, `"tool_started"`, `"working"`, `"model":`, `"tool":`, `"project":`, "Sonnet", "Opus"} {
					if contains(control, word) {
						t.Errorf("at minimal the control socket carried %q:\n%s", word, control)
					}
				}
			case "standard":
				if last.Details != surfaceLine {
					t.Errorf("at standard Discord was told to show %v, want the surface alone on the first line", last)
				}
			case "full":
				if want := surfaceLine + " · " + w.project; last.Details != want {
					t.Errorf("at full Discord was told to show %v, want %q on the first line", last, want)
				}
			}
		})
	}
}
