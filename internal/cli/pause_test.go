package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// writeConfig writes the world's configuration file.
func (w *world) writeConfig(t *testing.T, settings map[string]any) {
	t.Helper()
	raw, err := json.Marshal(settings)
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
	if err := os.WriteFile(dirs.File, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// logText is everything the world's processes have logged.
func (w *world) logText(t *testing.T) string {
	t.Helper()
	_, dirs, _ := config.Load(w.sys.goos, w.env.get, w.sys.files)
	log, err := os.ReadFile(diag.LogPath(dirs.Logs))
	if err != nil {
		t.Fatal(err)
	}
	return string(log)
}

// call calls a tool and returns its text.
func (s *client) call(name string, arguments map[string]any) string {
	s.t.Helper()
	params, err := json.Marshal(map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		s.t.Fatal(err)
	}
	resp := s.request("tools/call", string(params))
	content := resp["result"].(map[string]any)["content"].([]any)
	return content[0].(map[string]any)["text"].(string)
}

// hook calls the event tool as a hook does, from a directory.
func (s *client) hook(event, id, cwd string, fields ...string) {
	s.t.Helper()
	arguments := map[string]any{"event": event, "session_id": id, "cwd": cwd}
	for i := 0; i+1 < len(fields); i += 2 {
		arguments[fields[i]] = fields[i+1]
	}
	if got := s.call(ToolEvent, arguments); got != "{}" {
		s.t.Fatalf("the event tool answered %q", got)
	}
}

// activityCard is an activity as the fake Discord received it.
type activityCard struct {
	Details    string `json:"details"`
	State      string `json:"state"`
	Timestamps struct {
		Start int64 `json:"start"`
	} `json:"timestamps"`
	Assets struct {
		LargeImage string `json:"large_image"`
		LargeText  string `json:"large_text"`
		SmallImage string `json:"small_image"`
		SmallText  string `json:"small_text"`
	} `json:"assets"`
	Buttons []struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	} `json:"buttons"`
}

// told is what the fake Discord was last told to show: an activity, or
// nothing. sets is how many activities it has been told in all.
func told(t *testing.T, srv *fakediscord.Server) (last activityCard, shown bool, sets int) {
	t.Helper()
	for _, ev := range srv.Events() {
		switch ev.Kind {
		case fakediscord.KindClearActivity:
			last, shown = activityCard{}, false
		case fakediscord.KindSetActivity:
			last, shown = activityCard{}, true
			sets++
			if err := json.Unmarshal(ev.Activity, &last); err != nil {
				t.Fatalf("the activity %s is not what Discord documents: %v", ev.Activity, err)
			}
		}
	}
	return last, shown, sets
}

// awaitTold moves the clock on, a second at a time so that the rate limit
// lets updates through, until what Discord was last told satisfies want.
func (w *world) awaitTold(t *testing.T, srv *fakediscord.Server, what string, want func(c activityCard, shown bool) bool) activityCard {
	t.Helper()
	var last activityCard
	eventually(t, "Discord to be told "+what, func() bool {
		w.clock.Advance(time.Second)
		var shown bool
		last, shown, _ = told(t, srv)
		return want(last, shown)
	})
	return last
}

func cleared(_ activityCard, shown bool) bool { return !shown }

func stateBegins(prefix string) func(activityCard, bool) bool {
	return func(c activityCard, shown bool) bool { return shown && strings.HasPrefix(c.State, prefix) }
}

func TestMCPPausesForADurationAndComesBack(t *testing.T) {
	w := newWorld(t)
	srv := w.discord(t)
	s := w.startSession(t)
	s.initialize()
	if got := strings.Join(s.toolNames(), ","); got != "presence_event,presence_status,presence_pause" {
		t.Errorf("tools = %q, want the event, status and pause tools", got)
	}
	s.awaitStatus("Discord: connected")
	s.hook("UserPromptSubmit", "abc", "/work/project")
	w.awaitTold(t, srv, "the session thinking", stateBegins("Thinking"))

	// A week, so that moving the clock for the rate limit does not end it.
	if got := s.call(ToolPause, map[string]any{"minutes": 10080}); !strings.Contains(got, "paused for every session for 10080 minutes") {
		t.Fatalf("the pause tool answered %q", got)
	}
	until := w.clock.Now().Add(7 * 24 * time.Hour)
	if got := s.status(); !strings.Contains(got, "Paused: for another 168h0m0s") {
		t.Errorf("status = %q, want the pause and its length", got)
	}
	w.awaitTold(t, srv, "nothing", cleared)
	if got := s.call(ToolStatus, map[string]any{"preview": true}); !strings.Contains(got, "Presence: paused for another") || strings.Contains(got, "Line 1") {
		t.Errorf("the preview while paused is %q", got)
	}
	// The status command says so too, from what the host says of itself.
	if code, stdout, _ := w.run("status"); code != 0 || !strings.Contains(stdout, "Paused: until "+until.Format("2006-01-02 15:04:05 UTC")+"\n") {
		t.Errorf("the status command printed %q with code %d, want the end of the pause", stdout, code)
	}

	// While paused the session goes on, and Discord is told nothing.
	_, _, before := told(t, srv)
	s.hook("PreToolUse", "abc", "/work/project", "tool_name", "Edit")
	for range 60 {
		w.clock.Advance(time.Second)
	}
	s.awaitStatus("Sessions: 1")
	if _, shown, sets := told(t, srv); shown || sets != before {
		t.Errorf("during the pause Discord was told %d more activities, want none", sets-before)
	}

	// The pause ends by itself, with no event, and the present is shown.
	w.clock.Advance(until.Sub(w.clock.Now()))
	w.awaitTold(t, srv, "the session editing", stateBegins("Editing files"))
	if got := s.status(); !strings.Contains(got, "Paused: no") {
		t.Errorf("status after the pause = %q", got)
	}
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestMCPResumeRestoresTheActivity(t *testing.T) {
	w := newWorld(t)
	srv := w.discord(t)
	s := w.startSession(t)
	s.initialize()
	s.awaitStatus("Discord: connected")
	s.hook("UserPromptSubmit", "abc", "/work/project")
	w.awaitTold(t, srv, "the session thinking", stateBegins("Thinking"))

	if got := s.call(ToolPause, nil); !strings.Contains(got, "until it is resumed") {
		t.Fatalf("the pause tool answered %q", got)
	}
	w.awaitTold(t, srv, "nothing", cleared)
	if code, stdout, _ := w.run("status"); code != 0 || !strings.Contains(stdout, "Paused: until resumed\n") {
		t.Errorf("the status command printed %q with code %d", stdout, code)
	}
	if got := s.call(ToolPause, map[string]any{"resume": true}); !strings.Contains(got, "resumed") {
		t.Fatalf("the pause tool answered %q", got)
	}
	w.awaitTold(t, srv, "the session thinking again", stateBegins("Thinking"))
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestMCPPauseWithPresenceOff(t *testing.T) {
	w := newWorld(t)
	w.env = w.env.with("RICH_PRESENCE_ENABLED", "false")
	s := w.startSession(t)
	s.initialize()
	if got := s.call(ToolPause, nil); got != "Presence is off, so there is nothing to pause." {
		t.Errorf("the pause tool answered %q", got)
	}
	if got := s.call(ToolStatus, map[string]any{"preview": true}); !strings.Contains(got, "Presence: off. Nothing is shown.") {
		t.Errorf("the preview is %q", got)
	}
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// TestMCPPreviewMatchesWhatDiscordWasTold compares every slot of the preview
// with the activity the fake Discord received.
func TestMCPPreviewMatchesWhatDiscordWasTold(t *testing.T) {
	w := newWorld(t)
	// Everything is logged, so that the log would show the preview if it
	// were logged.
	w.env = w.env.with("RICH_PRESENCE_LOG_LEVEL", "debug")
	project := t.TempDir()
	w.writeConfig(t, map[string]any{
		"privacy": "minimal",
		"projects": []map[string]string{
			{"path": project, "privacy": "full", "name": "Chosen Project", "link": "https://github.com/owner/chosen"},
		},
	})
	srv := w.discord(t)
	s := w.startSession(t)
	s.initialize()
	s.awaitStatus("Discord: connected")
	s.hook("SessionStart", "abc", project, "model", "claude-opus-5-5")
	s.hook("PreToolUse", "abc", project, "tool_name", "Edit")
	c := w.awaitTold(t, srv, "the session editing", stateBegins("Editing files"))
	if len(c.Buttons) != 1 || c.Assets.SmallText == "" || c.Assets.LargeText == "" || !strings.Contains(c.Details, "Chosen Project") {
		t.Fatalf("the activity %+v has an empty slot, so the test shows little", c)
	}

	want := "PRIVATE PREVIEW of your Discord card. It can name your project, so do not paste it anywhere public.\n" +
		"This session: published at the privacy level full.\n" +
		"Presence: shown.\n" +
		"Line 1: " + c.Details + "\n" +
		"Line 2: " + c.State + "\n" +
		"Timer: counting up from " + time.Unix(c.Timestamps.Start, 0).UTC().Format("2006-01-02 15:04:05 UTC") + "\n" +
		"Large image: " + c.Assets.LargeImage + "\n" +
		"Large image hover text: " + c.Assets.LargeText + "\n" +
		"Small image: " + c.Assets.SmallImage + "\n" +
		"Small image hover text: " + c.Assets.SmallText + "\n" +
		"Button: " + c.Buttons[0].Label + "\n" +
		"Button link: " + c.Buttons[0].URL
	if got := s.call(ToolStatus, map[string]any{"preview": true}); got != want {
		t.Errorf("the preview is\n%s\nwant\n%s", got, want)
	}
	// The summary names nothing of it.
	if got := s.status(); strings.Contains(got, "Chosen") || strings.Contains(got, "github") {
		t.Errorf("the summary holds what only the preview may: %q", got)
	}
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	// The preview went to the tool result and nowhere else.
	log := w.logText(t)
	if !strings.Contains(log, "became the presence host") {
		t.Fatalf("the log is not at the debug level, so the test shows little: %q", log)
	}
	for _, private := range []string{"Chosen", "github.com", "PREVIEW", c.State, c.Assets.SmallText} {
		if strings.Contains(log, private) {
			t.Errorf("the log holds %q of the preview: %q", private, log)
		}
	}
}

// TestMCPHidesASessionInAProjectProfiledOff has one session in a hidden
// project and one elsewhere. The hidden one never reaches the host, so the
// host counts one session and shows the other as if it were alone.
func TestMCPHidesASessionInAProjectProfiledOff(t *testing.T) {
	w := newWorld(t)
	secret := t.TempDir()
	w.writeConfig(t, map[string]any{"projects": []map[string]string{{"path": secret, "privacy": "off"}}})
	srv := w.discord(t)

	hidden := w.startSession(t)
	hidden.initialize()
	hidden.awaitStatus("Discord: connected")
	// Before its directory is known, a session that may be hidden is.
	if got := hidden.status(); !strings.Contains(got, "Privacy: off") || !strings.Contains(got, "Sessions: 0") {
		t.Errorf("status before any hook = %q, want the session hidden and not counted", got)
	}
	hidden.hook("UserPromptSubmit", "hidden", secret)
	hidden.hook("PreToolUse", "hidden", secret, "tool_name", "Edit")

	w.sys.sessionID = func() string { return "second-session" }
	visible := w.startSession(t)
	visible.initialize()
	// The clock runs while waiting: a follower that dialled before the host
	// was listening tries again only after a wait.
	w.passUntilStatus(t, visible, "Role: follower")
	visible.hook("UserPromptSubmit", "visible", "/work/elsewhere")
	c := w.awaitTold(t, srv, "the visible session thinking", stateBegins("Thinking"))
	if strings.Contains(c.State, "sessions") {
		t.Errorf("the card counts sessions: %q, want the one visible session alone", c.State)
	}
	visible.awaitStatus("Sessions: 1")
	if got := hidden.status(); !strings.Contains(got, "Privacy: off") || !strings.Contains(got, "Sessions: 1") {
		t.Errorf("the hidden session's status = %q, want it hidden and the host counting one", got)
	}
	if got := hidden.call(ToolStatus, map[string]any{"preview": true}); !strings.Contains(got, "This session: hidden.") {
		t.Errorf("the hidden session's preview is %q", got)
	}

	// With the visible session gone nothing is left to show, although the
	// hidden one is still open and working.
	if code := visible.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	w.awaitTold(t, srv, "nothing", cleared)
	hidden.hook("PreToolUse", "hidden", secret, "tool_name", "Bash")
	hidden.awaitStatus("Sessions: 0")
	for range 60 {
		w.clock.Advance(time.Second)
	}
	if _, shown, _ := told(t, srv); shown {
		t.Error("the hidden session was shown")
	}
	if code := hidden.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestMCPDesktopIsHiddenWhenTheGlobalLevelIsOff(t *testing.T) {
	w := newWorld(t)
	w.env = w.env.with("RICH_PRESENCE_PRIVACY", "off")
	srv := w.discord(t)
	s := w.startSession(t)
	s.initializeAs("claude-ai")
	s.awaitStatus("Discord: connected")
	if got := s.status(); !strings.Contains(got, "Privacy: off") || !strings.Contains(got, "Sessions: 0") {
		t.Errorf("status = %q, want the session hidden and not counted", got)
	}
	for range 60 {
		w.clock.Advance(time.Second)
	}
	if _, shown, sets := told(t, srv); shown || sets != 0 {
		t.Errorf("Discord was told %d activities, want none", sets)
	}
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestPausedWord(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	at := now.UnixMilli()
	cases := []struct {
		name string
		p    *protocol.PauseState
		want string
	}{
		{"a host from before pausing", nil, "unavailable, the presence host is an older version"},
		{"never asked", &protocol.PauseState{}, "no"},
		{"resumed", &protocol.PauseState{At: at}, "no"},
		{"until resumed", &protocol.PauseState{Paused: true, At: at}, "until resumed"},
		{"until a time", &protocol.PauseState{Paused: true, Until: at + 90_000, At: at}, "until 2026-10-06 12:01:30 UTC"},
		{"a pause that has just ended", &protocol.PauseState{Paused: true, Until: at, At: at - 1}, "no"},
		{"a pause that ended long ago", &protocol.PauseState{Paused: true, Until: at - 90_000, At: at - 100_000}, "no"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pausedWord(c.p, now); got != c.want {
				t.Errorf("pausedWord = %q, want %q", got, c.want)
			}
		})
	}
}
