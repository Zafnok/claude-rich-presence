package code

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/mcp"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
)

// dirHidden has a profile whose level is off.
const dirHidden = "/home/u/work/hidden"

// hiding is a Resolver in which one project is hidden, over the others of
// profiles.
func hiding(global domain.Privacy) Resolver {
	rest := profiles(global)
	return func(cwd string) Settings {
		if cwd == dirHidden {
			return Settings{Privacy: domain.PrivacyOff, Name: "Hidden Work", Link: "https://github.com/owner/hidden"}
		}
		return rest(cwd)
	}
}

// newHidingHarness is an adapter whose configuration hides a project.
func newHidingHarness(t *testing.T, global domain.Privacy) *harness {
	t.Helper()
	h := &harness{t: t, rec: &recorder{}, clock: fakeclock.New(epoch)}
	a, err := New(Options{
		Privacy:       global,
		MayHide:       true,
		ProvisionalID: provisional,
		Publisher:     h.rec,
		Status:        fixedStatus{Role: RoleHost, Discord: DiscordConnected, Sessions: 1},
		Clock:         h.clock,
		Resolve:       hiding(global),
	})
	if err != nil {
		t.Fatal(err)
	}
	h.a = a
	return h
}

func TestASessionInAHiddenProjectPublishesNothing(t *testing.T) {
	h := newHidingHarness(t, domain.PrivacyFull)
	h.a.Open()
	for _, call := range []string{
		hookAt("UserPromptSubmit", dirHidden),
		hookAt("SessionStart", dirHidden, `"model":"claude-opus-5-5"`),
		hookAt("PreToolUse", dirHidden, `"tool_name":"Edit"`),
		hookAt("SubagentStart", dirHidden, `"agent_id":"a1"`),
		hookAt("SubagentStop", dirHidden, `"agent_id":"a1"`),
		hookAt("PostModelSwitch", dirHidden, `"to_model":"claude-haiku-4-5-20251001"`),
		hookAt("Notification", dirHidden, `"notification_type":"permission_prompt"`),
		// A hook without a directory keeps the session hidden.
		`{"event":"Stop","session_id":"s1"}`,
		// So does a new session id, as after a clear.
		`{"event":"SessionStart","session_id":"s2","cwd":"` + dirHidden + `"}`,
	} {
		h.call(call)
	}
	status := h.a.handleStatus(nil)
	wantEvents(t, h.finish())
	if !strings.Contains(status.Text, "Privacy: off") {
		t.Errorf("status = %q, want the level off", status.Text)
	}
	if got := h.a.ignored.Load() + h.a.dropped.Load(); got != 0 {
		t.Errorf("%d calls were counted as ignored or dropped", got)
	}
}

func TestASessionThatMayBeHiddenOpensOnlyWhenItsDirectoryIsKnown(t *testing.T) {
	h := newHidingHarness(t, domain.PrivacyStandard)
	h.a.Open()
	// No directory yet: the session could be in the hidden project.
	h.call(bind)
	if got := h.rec.count(); got != 0 {
		t.Fatalf("published %d events before the directory was known", got)
	}
	h.clock.Advance(time.Minute)
	h.call(hookAt("PreToolUse", dirOther, `"tool_name":"Edit"`))
	wantEvents(t, h.finish(),
		"session_opened s1 privacy=standard",
		"tool_started s1 tool=editing",
		"session_ended s1",
	)
	// The session began when the process did, not when it was first shown.
	if opened := h.rec.events[0]; !opened.At.Equal(epoch) {
		t.Errorf("the session opened at %v, want %v", opened.At, epoch)
	}
}

func TestMovingIntoAHiddenProjectEndsTheSessionAndMovingOutOpensIt(t *testing.T) {
	h := newHidingHarness(t, domain.PrivacyStandard)
	h.a.Open()
	h.call(hookAt("SessionStart", dirNamed, `"model":"claude-opus-5-5"`))
	h.call(hookAt("SubagentStart", dirNamed, `"agent_id":"a1"`))
	h.call(hookAt("PreToolUse", dirHidden, `"tool_name":"Edit"`))
	h.call(hookAt("PostToolUse", dirHidden))
	// The subagent's stop is not published either, and is forgotten.
	h.call(hookAt("SubagentStop", dirHidden, `"agent_id":"a1"`))
	h.call(hookAt("PreToolUse", dirNamed, `"tool_name":"Read"`))
	h.call(hookAt("SubagentStop", dirNamed, `"agent_id":"a1"`))
	got := h.finish()
	wantEvents(t, got,
		"session_opened s1 project=Visions of Shuyi privacy=full",
		"session_refreshed s1 model=Opus 5.5",
		"subagent_started s1",
		// Into the hidden project: the session ends, and nothing follows.
		"session_ended s1",
		// Out of it: a new session, with what the adapter knows of it.
		"session_opened s1 model=Opus 5.5 project=Visions of Shuyi privacy=full",
		"tool_started s1 tool=reading",
		"session_ended s1",
	)
	for _, e := range h.rec.events {
		if strings.Contains(e.Project, "Hidden") || e.Link != "" {
			t.Errorf("published something of the hidden project: %+v", e)
		}
	}
	// The session that reopened has no subagent left over from before.
	if s := finalSession(t, h.rec.events); s.Subagents != 0 {
		t.Errorf("the reopened session has %d subagents, want none", s.Subagents)
	}
}

func TestAGlobalLevelOfOffHidesEverySessionButAProfiledOne(t *testing.T) {
	t.Run("without a resolver nothing is ever published", func(t *testing.T) {
		h := newHarness(t, domain.PrivacyOff)
		h.a.Open()
		h.call(bind)
		h.call(hookAt("PreToolUse", dirNamed, `"tool_name":"Edit"`))
		wantEvents(t, h.finish())
	})
	t.Run("a profile shows its project", func(t *testing.T) {
		// MayHide is not set: a global level of off is enough.
		h := newProfileHarness(t, domain.PrivacyOff, profiles(domain.PrivacyOff))
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", dirOther))
		h.call(hookAt("UserPromptSubmit", dirStandard))
		h.call(hookAt("UserPromptSubmit", dirOther))
		wantEvents(t, h.finish(),
			"session_opened s1 privacy=standard",
			"turn_started s1",
			"session_ended s1",
		)
	})
}

func TestAFailingResolverHidesTheSessionWhenAProjectMayBeHidden(t *testing.T) {
	// What a session has before its directory is known is what it gets when
	// the directory cannot be resolved.
	h := &harness{t: t, rec: &recorder{}, clock: fakeclock.New(epoch)}
	a, err := New(Options{
		Privacy: domain.PrivacyFull, MayHide: true, ProvisionalID: provisional, Publisher: h.rec,
		Status: fixedStatus{}, Clock: h.clock,
		Resolve: func(string) Settings { panic(dirHidden) },
	})
	if err != nil {
		t.Fatal(err)
	}
	h.a = a
	a.Open()
	h.call(hookAt("UserPromptSubmit", dirHidden))
	wantEvents(t, h.finish())
}

func TestADroppedCallLeavesAHiddenSessionAsItWas(t *testing.T) {
	// The queue is full when the session moves into the hidden project, so
	// the end of the session is lost. The next call ends it.
	pub := &stalled{entered: make(chan struct{}, 3*queueSize), release: make(chan struct{})}
	a, err := New(Options{
		Privacy: domain.PrivacyStandard, MayHide: true, ProvisionalID: provisional, Publisher: pub,
		Status: fixedStatus{}, Clock: fakeclock.New(epoch), Resolve: hiding(domain.PrivacyStandard),
	})
	if err != nil {
		t.Fatal(err)
	}
	a.Open()
	call := func(arguments string) { a.handleEvent(json.RawMessage(arguments), nil) }
	call(hookAt("UserPromptSubmit", dirOther))
	// The pump now holds the opening and is stuck, and the queue is empty.
	<-pub.entered
	for i := 0; i < queueSize+5; i++ {
		call(hookAt("PostToolUse", dirOther))
	}
	dropped := a.dropped.Load()
	if dropped == 0 {
		t.Fatal("nothing was dropped")
	}
	call(hookAt("PostToolUse", dirHidden))
	if got := a.dropped.Load(); got != dropped+1 {
		t.Errorf("dropped = %d, want %d", got, dropped+1)
	}
	if text := a.handleStatus(nil).Text; !strings.Contains(text, "Privacy: standard") {
		t.Errorf("after the dropped call the status is %q, want the level as it was", text)
	}
	close(pub.release)
	waitFor(t, "the queue to drain", func() bool { return pub.count() == 2+queueSize })
	call(hookAt("PostToolUse", dirHidden))
	a.Close()
	if last := pub.events[len(pub.events)-1]; last.Kind != domain.KindSessionEnded {
		t.Errorf("the last event is %s, want the end of the session", show(last))
	}
	if sessions := replay(t, pub.events); len(sessions) != 0 {
		t.Errorf("sessions after the session was hidden = %+v", sessions)
	}
	ended := 0
	for _, e := range pub.events {
		if e.Kind == domain.KindSessionEnded {
			ended++
		}
	}
	if ended != 1 {
		t.Errorf("the session ended %d times, want once: closing a hidden session publishes nothing", ended)
	}
}

// pauser records what the pause tool asks of it.
type pauser struct {
	refuse bool
	calls  []string
}

func (p *pauser) Pause(until time.Time) bool {
	if until.IsZero() {
		p.calls = append(p.calls, "pause")
	} else {
		p.calls = append(p.calls, "pause until "+until.UTC().Format(time.RFC3339))
	}
	return !p.refuse
}

func (p *pauser) Resume() bool {
	p.calls = append(p.calls, "resume")
	return !p.refuse
}

// newPausingHarness is an adapter with a pauser, bound to a session.
func newPausingHarness(t *testing.T, p Pauser) *harness {
	t.Helper()
	h := &harness{t: t, rec: &recorder{}, clock: fakeclock.New(epoch)}
	a, err := New(Options{
		Privacy: domain.PrivacyFull, ProvisionalID: provisional, Publisher: h.rec,
		Status: fixedStatus{Role: RoleHost}, Pauser: p, Clock: h.clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.a = a
	a.Open()
	h.call(hookAt("UserPromptSubmit", dirNamed))
	return h
}

func TestPauseTool(t *testing.T) {
	// The clock is one second past the epoch when the tool is called.
	cases := []struct {
		name      string
		arguments string
		want      string
		wantCalls []string
	}{
		{"no arguments", ``, pauseIndefinite, []string{"pause"}},
		{"an empty object", `{}`, pauseIndefinite, []string{"pause"}},
		{"null", `null`, pauseIndefinite, []string{"pause"}},
		{"zero minutes", `{"minutes":0}`, pauseIndefinite, []string{"pause"}},
		{"a duration", `{"minutes":30}`, "Presence is paused for every session for 30 minutes. It returns by itself.", []string{"pause until 2026-10-03T12:30:01Z"}},
		{"the longest duration", `{"minutes":10080}`, "Presence is paused for every session for 10080 minutes. It returns by itself.", []string{"pause until 2026-10-10T12:00:01Z"}},
		{"resume", `{"resume":true}`, pauseResumed, []string{"resume"}},
		{"resume wins over a duration", `{"resume":true,"minutes":5}`, pauseResumed, []string{"resume"}},
		{"resume set to false pauses", `{"resume":false}`, pauseIndefinite, []string{"pause"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &pauser{}
			h := newPausingHarness(t, p)
			got := h.a.handlePause(json.RawMessage(c.arguments))
			if got.IsError || got.Text != c.want {
				t.Errorf("result = %+v, want %q", got, c.want)
			}
			if !equal(p.calls, c.wantCalls) {
				t.Errorf("the pauser was asked %q, want %q", p.calls, c.wantCalls)
			}
		})
	}
}

func TestPauseToolRefusesWhatItCannotRead(t *testing.T) {
	for name, arguments := range map[string]string{
		"negative minutes":      `{"minutes":-1}`,
		"too many minutes":      `{"minutes":10081}`,
		"minutes as text":       `{"minutes":"ten"}`,
		"a fraction of minutes": `{"minutes":1.5}`,
		"resume as text":        `{"resume":"yes"}`,
		"not an object":         `[30]`,
		"not JSON":              `minutes=30`,
	} {
		t.Run(name, func(t *testing.T) {
			p := &pauser{}
			h := newPausingHarness(t, p)
			got := h.a.handlePause(json.RawMessage(arguments))
			if !got.IsError || got.Text != pauseBadMinutes {
				t.Errorf("result = %+v, want the refusal", got)
			}
			if len(p.calls) != 0 {
				t.Errorf("the pauser was asked %q, want nothing", p.calls)
			}
		})
	}
}

func TestPauseToolSaysWhenPausingIsNotPossible(t *testing.T) {
	t.Run("an older host", func(t *testing.T) {
		for _, arguments := range []string{``, `{"minutes":5}`, `{"resume":true}`} {
			h := newPausingHarness(t, &pauser{refuse: true})
			got := h.a.handlePause(json.RawMessage(arguments))
			if !got.IsError || got.Text != pauseOldHost {
				t.Errorf("result for %q = %+v, want that pausing is unavailable", arguments, got)
			}
		}
	})
	t.Run("presence is off", func(t *testing.T) {
		h := newPausingHarness(t, nil)
		got := h.a.handlePause(json.RawMessage(`{"minutes":5}`))
		if got.IsError || got.Text != pauseOff {
			t.Errorf("result = %+v, want that presence is off", got)
		}
	})
	t.Run("a pauser that panics", func(t *testing.T) {
		h := newPausingHarness(t, panickyPauser{})
		got := h.a.handlePause(nil)
		if !got.IsError || got.Text != pauseUnavailable {
			t.Errorf("result = %+v", got)
		}
		checkClean(t, got.Text, []string{"shuyi", "/"})
	})
}

type panickyPauser struct{}

func (panickyPauser) Pause(time.Time) bool { panic(dirNamed) }
func (panickyPauser) Resume() bool         { panic(dirNamed) }

// TestThePauseToolCanOnlyPauseAndResume gives the tool inputs that try to
// change what is shown, a privacy level and a profile. Whatever it is given,
// all it does is call the pauser, and the session is published as it was.
func TestThePauseToolCanOnlyPauseAndResume(t *testing.T) {
	attempts := []string{
		`{"privacy":"full"}`,
		`{"minutes":5,"privacy":"full","level":"full","enabled":true}`,
		`{"resume":true,"project":"Injected","name":"Injected","link":"https://github.com/evil/repo"}`,
		`{"projects":[{"path":"/","privacy":"full","link":"https://github.com/evil/repo"}]}`,
		`{"details":"Injected","state":"Injected","summary":"Injected","cwd":"/home/u/work/open"}`,
		`{"event":"UserPromptSubmit","session_id":"s9","cwd":"/home/u/work/open"}`,
		`{"minutes":5,"until":"9999-01-01T00:00:00Z","at":1}`,
	}
	p := &pauser{}
	h := &harness{t: t, rec: &recorder{}, clock: fakeclock.New(epoch)}
	a, err := New(Options{
		Privacy: domain.PrivacyStandard, ProvisionalID: provisional, Publisher: h.rec,
		Status: fixedStatus{Role: RoleHost}, Pauser: p, Clock: h.clock, Resolve: profiles(domain.PrivacyStandard),
	})
	if err != nil {
		t.Fatal(err)
	}
	h.a = a
	a.Open()
	h.call(hookAt("UserPromptSubmit", dirMinimal))
	before := a.session
	for _, arguments := range attempts {
		a.handlePause(json.RawMessage(arguments))
	}
	if a.session != before {
		t.Errorf("the session is now %+v, was %+v", a.session, before)
	}
	wantEvents(t, h.finish(),
		"session_opened "+provisional+" privacy=minimal",
		"session_ended "+provisional,
		"session_opened s1 privacy=minimal",
		"session_refreshed s1",
		"session_ended s1",
	)
	for _, call := range p.calls {
		if call != "resume" && !strings.HasPrefix(call, "pause") {
			t.Errorf("the pauser was asked %q", call)
		}
	}
	if len(p.calls) != len(attempts) {
		t.Errorf("the pauser was asked %d times for %d calls", len(p.calls), len(attempts))
	}
}

// FuzzPauseTool feeds arbitrary arguments to the pause tool. It never
// publishes anything, never changes the session, and asks the pauser at most
// once.
func FuzzPauseTool(f *testing.F) {
	for _, s := range []string{``, `{}`, `null`, `{"minutes":30}`, `{"resume":true}`, `{"minutes":-1}`, `{"minutes":1e99}`, `[`, `{"privacy":"full"}`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, arguments []byte) {
		p := &pauser{}
		rec := &recorder{}
		a, err := New(Options{Privacy: domain.PrivacyMinimal, ProvisionalID: provisional, Publisher: rec, Status: fixedStatus{}, Pauser: p, Clock: fakeclock.New(epoch)})
		if err != nil {
			t.Fatal(err)
		}
		a.Open()
		before := a.session
		got := a.handlePause(arguments)
		if a.session != before {
			t.Errorf("the session changed to %+v", a.session)
		}
		a.Close()
		if len(rec.events) != 2 {
			t.Errorf("published %+v, want the opening and the end", rec.events)
		}
		if len(p.calls) > 1 || got.IsError != (len(p.calls) == 0) {
			t.Errorf("result %+v with the pauser asked %q", got, p.calls)
		}
	})
}

// fixedPreview is a PreviewSource that returns itself, as known.
type fixedPreview Preview

func (p fixedPreview) Preview() (Preview, bool) { return Preview(p), true }

// unknownPreview is a PreviewSource that knows nothing.
type unknownPreview struct{}

func (unknownPreview) Preview() (Preview, bool) { return Preview{}, false }

// previewOf asks the status tool for the preview, in an adapter with the
// given status and card.
func previewOf(t *testing.T, privacy domain.Privacy, s Status, p PreviewSource) mcp.Result {
	t.Helper()
	rec := &recorder{}
	a, err := New(Options{Privacy: privacy, ProvisionalID: provisional, Publisher: rec, Status: fixedStatus(s), Preview: p, Clock: fakeclock.New(epoch)})
	if err != nil {
		t.Fatal(err)
	}
	a.Open()
	result := a.handleStatus(json.RawMessage(`{"preview":true}`))
	a.Close()
	// Read-only: asking published nothing.
	if privacy != domain.PrivacyOff && len(rec.events) != 2 {
		t.Errorf("published %+v", rec.events)
	}
	return result
}

func TestThePreviewListsEverySlotOfTheCard(t *testing.T) {
	card := domain.Activity{
		Details: "Claude Code · Visions of Shuyi", State: "Editing files · Opus · 2 sessions",
		Start:      time.Date(2026, 10, 3, 9, 30, 0, 0, time.FixedZone("west", -5*3600)),
		LargeImage: "logo", LargeText: "Claude Code", SmallImage: "working", SmallText: "Editing files",
		Button: domain.Button{Label: "View on GitHub", URL: "https://github.com/owner/visions"},
	}
	got := previewOf(t, domain.PrivacyFull, Status{Role: RoleFollower, Discord: DiscordConnected, Sessions: 2}, fixedPreview{Shown: true, Activity: card})
	want := previewHeading +
		"This session: published at the privacy level full.\n" +
		"Presence: shown.\n" +
		"Line 1: Claude Code · Visions of Shuyi\n" +
		"Line 2: Editing files · Opus · 2 sessions\n" +
		"Timer: counting up from 2026-10-03 14:30:00 UTC\n" +
		"Large image: logo\n" +
		"Large image hover text: Claude Code\n" +
		"Small image: working\n" +
		"Small image hover text: Editing files\n" +
		"Button: View on GitHub\n" +
		"Button link: https://github.com/owner/visions"
	if got.IsError || got.Text != want {
		t.Errorf("preview =\n%s\nwant\n%s", got.Text, want)
	}
	if !strings.HasPrefix(got.Text, "PRIVATE PREVIEW") {
		t.Error("the preview is not labelled as private")
	}

	t.Run("an empty slot is named", func(t *testing.T) {
		got := previewOf(t, domain.PrivacyMinimal, Status{Role: RoleHost}, fixedPreview{Shown: true, Activity: domain.Activity{Details: "Claude Code", LargeImage: "logo"}})
		want := previewHeading +
			"This session: published at the privacy level minimal.\n" +
			"Presence: shown.\n" +
			"Line 1: Claude Code\n" +
			"Line 2: (empty)\n" +
			"Timer: (empty)\n" +
			"Large image: logo\n" +
			"Large image hover text: (empty)\n" +
			"Small image: (empty)\n" +
			"Small image hover text: (empty)\n" +
			"Button: (empty)\n" +
			"Button link: (empty)"
		if got.Text != want {
			t.Errorf("preview =\n%s\nwant\n%s", got.Text, want)
		}
	})
}

func TestThePreviewSaysWhyNothingIsShown(t *testing.T) {
	card := fixedPreview{Shown: true, Activity: domain.Activity{Details: "Claude Code · Visions of Shuyi"}}
	published := "This session: published at the privacy level standard.\n"
	cases := []struct {
		name    string
		privacy domain.Privacy
		status  Status
		source  PreviewSource
		want    string
	}{
		{"paused until resumed", domain.PrivacyStandard, Status{Role: RoleHost, Paused: true}, card,
			published + "Presence: paused until resumed. Nothing is shown."},
		{"paused for a while", domain.PrivacyStandard, Status{Role: RoleFollower, Paused: true, PausedUntil: epoch.Add(90 * time.Second)}, card,
			published + "Presence: paused for another 1m30s. Nothing is shown."},
		{"presence off", domain.PrivacyStandard, Status{Role: RoleOff}, card,
			published + "Presence: off. Nothing is shown."},
		{"no source of the card", domain.PrivacyStandard, Status{Role: RoleHost}, nil,
			published + "Presence: off. Nothing is shown."},
		{"an older host", domain.PrivacyStandard, Status{Role: RoleFollower, NoPause: true}, unknownPreview{},
			published + "Presence: the card cannot be read, because the presence host is an older version. Restart your other sessions so that they update."},
		{"not known yet", domain.PrivacyStandard, Status{Role: RoleFollower}, unknownPreview{},
			published + "Presence: the card is not known yet. Ask again in a moment."},
		{"nothing shown", domain.PrivacyStandard, Status{Role: RoleHost}, fixedPreview{},
			published + "Presence: nothing is shown. No session is published, or every session has been idle for long enough to clear the card."},
		{"this session hidden, nothing shown", domain.PrivacyOff, Status{Role: RoleHost}, fixedPreview{},
			"This session: hidden. Its privacy level is off, so it is not published and not counted.\n" +
				"Presence: nothing is shown. No session is published, or every session has been idle for long enough to clear the card."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := previewOf(t, c.privacy, c.status, c.source)
			if got.IsError || got.Text != previewHeading+c.want {
				t.Errorf("preview =\n%s\nwant\n%s", got.Text, previewHeading+c.want)
			}
		})
	}
	t.Run("this session hidden, another shown", func(t *testing.T) {
		got := previewOf(t, domain.PrivacyOff, Status{Role: RoleFollower, Sessions: 1}, card)
		if !strings.Contains(got.Text, "This session: hidden.") || !strings.Contains(got.Text, "Line 1: Claude Code · Visions of Shuyi") {
			t.Errorf("preview =\n%s", got.Text)
		}
	})
	t.Run("a source that panics", func(t *testing.T) {
		got := previewOf(t, domain.PrivacyFull, Status{Role: RoleHost}, panickyPreview{})
		if !got.IsError || got.Text != statusUnavailable {
			t.Errorf("preview = %+v", got)
		}
	})
}

type panickyPreview struct{}

func (panickyPreview) Preview() (Preview, bool) { panic(dirNamed) }

func TestTheSummaryIsNotThePreview(t *testing.T) {
	// The summary is safe to paste anywhere, so it holds nothing of the card
	// whatever its arguments are, short of asking for the preview.
	card := fixedPreview{Shown: true, Activity: domain.Activity{Details: "Claude Code · Visions of Shuyi"}}
	for _, arguments := range []string{``, `{}`, `{"preview":false}`, `{"preview":"true"}`, `{"preview":1}`, `[true]`, `preview`} {
		a, err := New(Options{Privacy: domain.PrivacyFull, ProvisionalID: provisional, Publisher: &recorder{}, Status: fixedStatus{Role: RoleHost}, Preview: card, Clock: fakeclock.New(epoch)})
		if err != nil {
			t.Fatal(err)
		}
		got := a.handleStatus(json.RawMessage(arguments))
		a.Close()
		if got.IsError || !strings.HasPrefix(got.Text, "Role: host\n") {
			t.Errorf("status for %q = %+v, want the summary", arguments, got)
		}
		checkClean(t, got.Text, []string{"shuyi", "private", "preview"})
	}
}

func TestTheSummarySaysWhetherPresenceIsPaused(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		want   string
	}{
		{"not paused", Status{}, "Paused: no\n"},
		{"until resumed", Status{Paused: true}, "Paused: until resumed\n"},
		{"for whole seconds", Status{Paused: true, PausedUntil: epoch.Add(10 * time.Minute)}, "Paused: for another 10m0s\n"},
		{"rounded up", Status{Paused: true, PausedUntil: epoch.Add(1500 * time.Millisecond)}, "Paused: for another 2s\n"},
		{"never less than a second", Status{Paused: true, PausedUntil: epoch.Add(-time.Second)}, "Paused: for another 1s\n"},
		{"an older host", Status{Paused: true, NoPause: true}, "Paused: unavailable, the presence host is an older version\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := New(Options{Privacy: domain.PrivacyFull, ProvisionalID: provisional, Publisher: &recorder{}, Status: fixedStatus(c.status), Clock: fakeclock.New(epoch)})
			if err != nil {
				t.Fatal(err)
			}
			got := a.handleStatus(nil)
			a.Close()
			if !strings.Contains(got.Text, "\n"+c.want) {
				t.Errorf("status = %q, want it to hold %q", got.Text, c.want)
			}
		})
	}
}
