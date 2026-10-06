package code

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/mcp"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
)

func TestNewChecksItsOptions(t *testing.T) {
	good := func() Options {
		return Options{
			Privacy:       domain.PrivacyStandard,
			ProvisionalID: provisional,
			Publisher:     &recorder{},
			Status:        fixedStatus{},
			Clock:         fakeclock.New(epoch),
		}
	}
	cases := []struct {
		name   string
		change func(*Options)
		ok     bool
	}{
		{"complete", func(*Options) {}, true},
		{"longest id", func(o *Options) { o.ProvisionalID = strings.Repeat("i", domain.MaxIDLen) }, true},
		{"no privacy level", func(o *Options) { o.Privacy = "" }, false},
		{"unknown privacy level", func(o *Options) { o.Privacy = "summary" }, false},
		{"empty id", func(o *Options) { o.ProvisionalID = "" }, false},
		{"id too long", func(o *Options) { o.ProvisionalID = strings.Repeat("i", domain.MaxIDLen+1) }, false},
		{"no publisher", func(o *Options) { o.Publisher = nil }, false},
		{"no status source", func(o *Options) { o.Status = nil }, false},
		{"no clock", func(o *Options) { o.Clock = nil }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opts := good()
			c.change(&opts)
			a, err := New(opts)
			if (err == nil) != c.ok {
				t.Fatalf("New error = %v, want ok %v", err, c.ok)
			}
			if a != nil {
				a.Close()
			}
		})
	}
}

// TestEventTable takes every row of the event table from tool input to the
// published presence event.
func TestEventTable(t *testing.T) {
	cases := []struct {
		event string
		calls []string
		want  []string
	}{
		{"SessionStart", []string{`{"event":"SessionStart","session_id":"s1","model":"claude-opus-5-5"}`},
			[]string{"session_refreshed s1 model=Opus 5.5"}},
		{"SessionStart", []string{`{"event":"SessionStart","session_id":"s1"}`},
			[]string{"session_refreshed s1"}},
		{"SessionStart", []string{`{"event":"SessionStart","session_id":"s1","model":"some-other-model"}`},
			[]string{"session_refreshed s1"}},
		{"UserPromptSubmit", []string{`{"event":"UserPromptSubmit","session_id":"s1"}`},
			[]string{"turn_started s1"}},
		{"PreToolUse", []string{`{"event":"PreToolUse","session_id":"s1","tool_name":"Bash"}`},
			[]string{"tool_started s1 tool=running"}},
		{"PreToolUse", []string{`{"event":"PreToolUse","session_id":"s1","tool_name":"SomethingNew"}`},
			[]string{"tool_started s1 tool=generic"}},
		{"PostToolUse", []string{`{"event":"PostToolUse","session_id":"s1"}`},
			[]string{"tool_finished s1"}},
		{"PostToolUseFailure", []string{`{"event":"PostToolUseFailure","session_id":"s1"}`},
			[]string{"tool_finished s1"}},
		{"Notification", []string{`{"event":"Notification","session_id":"s1","notification_type":"permission_prompt"}`},
			[]string{"attention_needed s1"}},
		{"Notification", []string{`{"event":"Notification","session_id":"s1","notification_type":"agent_needs_input"}`},
			[]string{"attention_needed s1"}},
		{"Notification", []string{`{"event":"Notification","session_id":"s1","notification_type":"elicitation_dialog"}`},
			[]string{"attention_needed s1"}},
		{"Notification", []string{`{"event":"Notification","session_id":"s1","notification_type":"idle_prompt"}`},
			[]string{"idle s1"}},
		{"Notification", []string{`{"event":"Notification","session_id":"s1","notification_type":"auth_success"}`},
			nil},
		{"Stop", []string{`{"event":"Stop","session_id":"s1"}`},
			[]string{"turn_finished s1"}},
		{"StopFailure", []string{`{"event":"StopFailure","session_id":"s1"}`},
			[]string{"turn_finished s1"}},
		{"PreCompact", []string{`{"event":"PreCompact","session_id":"s1"}`},
			[]string{"compaction_started s1"}},
		{"PostCompact", []string{`{"event":"PostCompact","session_id":"s1"}`},
			[]string{"compaction_finished s1"}},
		{"PostModelSwitch", []string{`{"event":"PostModelSwitch","session_id":"s1","to_model":"claude-sonnet-5-5"}`},
			[]string{"model_changed s1 model=Sonnet 5.5"}},
		{"PostModelSwitch", []string{`{"event":"PostModelSwitch","session_id":"s1","to_model":"mystery"}`},
			nil},
		{"SubagentStart", []string{`{"event":"SubagentStart","session_id":"s1","agent_id":"a1"}`},
			[]string{"subagent_started s1"}},
		{"SubagentStop", []string{
			`{"event":"SubagentStart","session_id":"s1","agent_id":"a1"}`,
			`{"event":"SubagentStop","session_id":"s1","agent_id":"a1"}`,
		}, []string{"subagent_started s1", "subagent_stopped s1"}},
	}
	covered := map[string]bool{}
	for _, c := range cases {
		covered[c.event] = true
		t.Run(c.event, func(t *testing.T) {
			wantEvents(t, afterBind(t, domain.PrivacyStandard, c.calls...), c.want...)
		})
	}
	for _, row := range HookEvents() {
		if !covered[row.Name] {
			t.Errorf("no case for %s", row.Name)
		}
	}
	if len(covered) != len(allowlist) {
		t.Errorf("cases name %d events, the allowlist has %d", len(covered), len(allowlist))
	}
}

func TestTheSessionOpensAndEnds(t *testing.T) {
	h := newHarness(t, domain.PrivacyStandard)
	h.clock.Advance(time.Minute)
	h.a.Open()
	h.a.Open()
	h.clock.Advance(time.Hour)
	wantEvents(t, h.finish(), "session_opened "+provisional+" privacy=standard", "session_ended "+provisional)
	if at := h.rec.events[0].At; !at.Equal(epoch.Add(time.Minute)) {
		t.Errorf("opened at %v", at)
	}
	if at := h.rec.events[1].At; !at.Equal(epoch.Add(time.Minute + time.Hour)) {
		t.Errorf("ended at %v", at)
	}
	h.a.Close()
	h.a.Open()
	h.call(bind)
	if got := len(h.rec.events); got != 2 {
		t.Errorf("published %d events, want nothing after Close", got)
	}
}

func TestNothingIsPublishedBeforeTheSessionOpens(t *testing.T) {
	h := newHarness(t, domain.PrivacyStandard)
	h.call(bind)
	wantEvents(t, h.finish())
}

// malformed are calls the adapter must ignore whole.
var malformed = []struct {
	name      string
	arguments string
}{
	{"no arguments", ``},
	{"an array", `[]`},
	{"a string", `"UserPromptSubmit"`},
	{"an empty object", `{}`},
	{"unknown event name", `{"event":"SessionEnd","session_id":"s2"}`},
	{"event name in the wrong case", `{"event":"stop","session_id":"s2"}`},
	{"event name of the wrong type", `{"event":7,"session_id":"s2"}`},
	{"event name too long", `{"event":"` + strings.Repeat("e", maxFieldLen+1) + `","session_id":"s2"}`},
	{"no event name", `{"session_id":"s2"}`},
	{"no session id", `{"event":"Stop"}`},
	{"empty session id", `{"event":"Stop","session_id":""}`},
	{"null session id", `{"event":"Stop","session_id":null}`},
	{"session id of the wrong type", `{"event":"Stop","session_id":12}`},
	{"session id with a line break", `{"event":"Stop","session_id":"s\n2"}`},
	{"session id with a byte that is not text", `{"event":"Stop","session_id":"s�2"}`},
	{"session id too long", `{"event":"Stop","session_id":"` + strings.Repeat("s", domain.MaxIDLen+1) + `"}`},
	{"tool started with no tool", `{"event":"PreToolUse","session_id":"s2"}`},
	{"tool started with an empty tool", `{"event":"PreToolUse","session_id":"s2","tool_name":""}`},
	{"tool name of the wrong type", `{"event":"PreToolUse","session_id":"s2","tool_name":{"name":"Bash"}}`},
	{"tool name too long", `{"event":"PreToolUse","session_id":"s2","tool_name":"` + strings.Repeat("t", maxFieldLen+1) + `"}`},
	{"notification with no type", `{"event":"Notification","session_id":"s2"}`},
	{"notification with an empty type", `{"event":"Notification","session_id":"s2","notification_type":""}`},
	{"notification type of the wrong type", `{"event":"Notification","session_id":"s2","notification_type":true}`},
	{"model switch with no model", `{"event":"PostModelSwitch","session_id":"s2"}`},
	{"model switch with an empty model", `{"event":"PostModelSwitch","session_id":"s2","to_model":""}`},
	{"model of the wrong type", `{"event":"SessionStart","session_id":"s2","model":["claude-opus-5-5"]}`},
	{"model too long", `{"event":"SessionStart","session_id":"s2","model":"claude-opus-5-5-` + strings.Repeat("m", maxFieldLen) + `"}`},
	{"subagent start with no agent", `{"event":"SubagentStart","session_id":"s2"}`},
	{"subagent start with an empty agent", `{"event":"SubagentStart","session_id":"s2","agent_id":""}`},
	{"subagent stop with no agent", `{"event":"SubagentStop","session_id":"s2"}`},
	{"working directory of the wrong type", `{"event":"Stop","session_id":"s2","cwd":5}`},
	{"working directory too long", `{"event":"Stop","session_id":"s2","cwd":"/` + strings.Repeat("d", maxCwdLen) + `"}`},
}

// TestMalformedCallsPublishNothing runs at the full level, where every field
// is read. Each call would move the session to s2 if it were accepted.
func TestMalformedCallsPublishNothing(t *testing.T) {
	for _, c := range malformed {
		t.Run(c.name, func(t *testing.T) {
			wantEvents(t, afterBind(t, domain.PrivacyFull, c.arguments))
		})
	}
}

func TestMalformedCallsAreCounted(t *testing.T) {
	h := newHarness(t, domain.PrivacyFull)
	h.a.Open()
	for _, c := range malformed {
		h.call(c.arguments)
	}
	h.finish()
	if got := h.a.ignored.Load(); got != int64(len(malformed)) {
		t.Errorf("ignored = %d, want %d", got, len(malformed))
	}
}

func TestACallMadeByTheModelIsIgnored(t *testing.T) {
	h := newHarness(t, domain.PrivacyStandard)
	h.a.Open()
	h.callWithMeta(`{"event":"UserPromptSubmit","session_id":"forged"}`, `{"claudecode/toolUseId":"toolu_1","progressToken":3}`)
	h.callWithMeta(`{"event":"UserPromptSubmit","session_id":"forged"}`, `{"claudecode/toolUseId":null}`)
	h.callWithMeta(bind, `{}`)
	h.callWithMeta(`{"event":"Stop","session_id":"s1"}`, `{"progressToken":3}`)
	got := h.finish()
	wantEvents(t, got, append(bound(domain.PrivacyStandard), "turn_finished s1", "session_ended s1")...)
	if ignored := h.a.ignored.Load(); ignored != 2 {
		t.Errorf("ignored = %d, want 2", ignored)
	}
}

// brokenClock is a clock that panics on demand.
type brokenClock struct {
	*fakeclock.Clock
	broken bool
}

func (c *brokenClock) Now() time.Time {
	if c.broken {
		panic("the clock broke while handling " + bind)
	}
	return c.Clock.Now()
}

func TestAnInternalErrorStillReturnsTheConstant(t *testing.T) {
	rec := &recorder{}
	clock := &brokenClock{Clock: fakeclock.New(epoch)}
	a, err := New(Options{Privacy: domain.PrivacyStandard, ProvisionalID: provisional, Publisher: rec, Status: fixedStatus{}, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	a.Open()
	clock.broken = true
	if got := a.handleEvent(json.RawMessage(bind), nil); got != constantResult {
		t.Errorf("result = %+v, want %+v", got, constantResult)
	}
	clock.broken = false
	// The lock was released, and the session is as it was.
	if got := a.handleEvent(json.RawMessage(bind), nil); got != constantResult {
		t.Errorf("result = %+v, want %+v", got, constantResult)
	}
	a.Close()
	if got := a.ignored.Load(); got != 1 {
		t.Errorf("ignored = %d, want 1", got)
	}
	if len(rec.events) != 5 || rec.events[3].Kind != domain.KindTurnStarted || rec.events[3].SessionID != "s1" {
		t.Errorf("published %+v", rec.events)
	}
}

func TestAnEmptyStringIsAnAbsentField(t *testing.T) {
	h := newHarness(t, domain.PrivacyFull)
	h.a.Open()
	h.call(`{"event":"UserPromptSubmit","session_id":"s1","cwd":""}`)
	h.call(`{"event":"SessionStart","session_id":"s1","cwd":"","model":""}`)
	h.call(`{"event":"PreToolUse","session_id":"s1","cwd":"","tool_name":""}`)
	h.call(`{"event":"SubagentStop","session_id":"s1","cwd":"","agent_id":""}`)
	h.call(`{"event":"Stop","session_id":"","cwd":""}`)
	wantEvents(t, h.finish(), append(bound(domain.PrivacyFull), "session_refreshed s1", "session_ended s1")...)
}

func TestSubagentStopsCountOnlyAfterTheirStart(t *testing.T) {
	start := func(id string) string {
		return `{"event":"SubagentStart","session_id":"s1","agent_id":"` + id + `"}`
	}
	stop := func(id string) string {
		return `{"event":"SubagentStop","session_id":"s1","agent_id":"` + id + `"}`
	}
	h := newHarness(t, domain.PrivacyStandard)
	h.a.Open()
	h.call(bind)
	count := func() int {
		t.Helper()
		sessions := replay(t, h.rec.snapshot())
		if len(sessions) != 1 {
			t.Fatalf("%d sessions, want 1", len(sessions))
		}
		return sessions[0].Subagents
	}
	steps := []struct {
		call      string
		published int
		subagents int
	}{
		{stop("compaction"), 0, 0},
		{start("a1"), 1, 1},
		{start("a1"), 0, 1},
		{start("a2"), 1, 2},
		{stop("a3"), 0, 2},
		{stop("a1"), 1, 1},
		{stop("a1"), 0, 1},
		{stop("a2"), 1, 0},
		{stop("a2"), 0, 0},
	}
	published := len(bound(domain.PrivacyStandard))
	for i, s := range steps {
		h.call(s.call)
		published += s.published
		waitFor(t, "the queue to drain", func() bool { return h.rec.count() >= published })
		if got := h.rec.count(); got != published {
			t.Fatalf("step %d: %d events published, want %d", i, got, published)
		}
		if got := count(); got != s.subagents {
			t.Errorf("step %d: %d subagents, want %d", i, got, s.subagents)
		}
	}
	if got := len(h.finish()); got != published+1 {
		t.Errorf("%d events in all, want %d", got, published+1)
	}
}

func (r *recorder) snapshot() []domain.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.Event(nil), r.events...)
}

func TestSubagentsAreTrackedUpToALimit(t *testing.T) {
	var calls []string
	for i := 0; i < maxAgents+3; i++ {
		calls = append(calls, `{"event":"SubagentStart","session_id":"s1","agent_id":"a`+strconv.Itoa(i)+`"}`)
	}
	// The stop of an agent that was not tracked is not counted either.
	calls = append(calls, `{"event":"SubagentStop","session_id":"s1","agent_id":"a`+strconv.Itoa(maxAgents)+`"}`)
	calls = append(calls, `{"event":"SubagentStop","session_id":"s1","agent_id":"a0"}`)
	got := afterBind(t, domain.PrivacyStandard, calls...)
	if len(got) != maxAgents+1 || got[maxAgents-1] != "subagent_started s1" || got[maxAgents] != "subagent_stopped s1" {
		t.Errorf("published %d events ending %q, want %d starts and one stop", len(got), got[len(got)-1], maxAgents)
	}
}

// TestSessionIdentity follows one process through a first hook and a clear.
func TestSessionIdentity(t *testing.T) {
	h := newHarness(t, domain.PrivacyFull)
	h.a.Open()
	opened := h.clock.Now()
	one := func(want string) domain.Session {
		t.Helper()
		waitFor(t, "the queue to drain", func() bool {
			events := h.rec.snapshot()
			return len(events) > 0 && events[len(events)-1].At.Equal(h.clock.Now())
		})
		sessions := replay(t, h.rec.snapshot())
		if len(sessions) != 1 || sessions[0].ID != want {
			t.Fatalf("sessions = %+v, want only %s", sessions, want)
		}
		if !sessions[0].Start.Equal(opened) {
			t.Errorf("session %s started %v, want %v", want, sessions[0].Start, opened)
		}
		return sessions[0]
	}
	h.clock.Advance(time.Second)
	h.call(`{"event":"Stop","session_id":"` + provisional + `"}`)
	one(provisional)

	// The first hook's id replaces the provisional one.
	h.clock.Advance(time.Minute)
	h.call(`{"event":"UserPromptSubmit","session_id":"s1","cwd":"/home/u/work/alpha"}`)
	if s := one("s1"); s.Status != domain.StatusWorking || s.Project != "alpha" || s.Privacy != domain.PrivacyFull {
		t.Errorf("session = %+v", s)
	}
	h.call(`{"event":"SessionStart","session_id":"s1","cwd":"/home/u/work/alpha","model":"claude-opus-5-5"}`)
	h.call(`{"event":"SubagentStart","session_id":"s1","cwd":"/home/u/work/alpha","agent_id":"a1"}`)
	if s := one("s1"); s.Model != "Opus 5.5" || s.Subagents != 1 {
		t.Errorf("session = %+v", s)
	}

	// A clear changes the id. The session moves and keeps what it knew.
	h.clock.Advance(time.Hour)
	h.call(`{"event":"SessionStart","session_id":"s2","cwd":"/home/u/work/alpha"}`)
	if s := one("s2"); s.Model != "Opus 5.5" || s.Project != "alpha" || s.Privacy != domain.PrivacyFull || s.Subagents != 0 {
		t.Errorf("session = %+v", s)
	}
	// The subagent belonged to the session that ended.
	h.call(`{"event":"SubagentStop","session_id":"s2","cwd":"/home/u/work/alpha","agent_id":"a1"}`)
	h.call(`{"event":"Stop","session_id":"s2","cwd":"/home/u/work/alpha"}`)
	one("s2")

	wantEvents(t, h.finish(),
		"session_opened "+provisional+" privacy=full",
		"turn_finished "+provisional,
		"session_ended "+provisional,
		"session_opened s1 project=alpha privacy=full",
		"turn_started s1",
		"session_refreshed s1 model=Opus 5.5",
		"subagent_started s1",
		"session_ended s1",
		"session_opened s2 model=Opus 5.5 project=alpha privacy=full",
		"session_refreshed s2",
		"turn_finished s2",
		"session_ended s2",
	)
	if sessions := replay(t, h.rec.events); len(sessions) != 0 {
		t.Errorf("sessions left after the end: %+v", sessions)
	}
}

func TestAModelThatCannotBeReadIsNotCarriedOver(t *testing.T) {
	got := afterBind(t, domain.PrivacyStandard,
		`{"event":"PostModelSwitch","session_id":"s1","to_model":"claude-opus-5-5"}`,
		`{"event":"PostModelSwitch","session_id":"s1","to_model":"mystery"}`,
		`{"event":"Stop","session_id":"s2"}`,
	)
	wantEvents(t, got, "model_changed s1 model=Opus 5.5", "session_ended s1", "session_opened s2 privacy=standard", "turn_finished s2")
}

func TestAMoveWithNoEventOfItsOwnStillCountsAsActivity(t *testing.T) {
	for _, call := range []string{
		`{"event":"SubagentStop","session_id":"s2","agent_id":"compaction"}`,
		`{"event":"Notification","session_id":"s2","notification_type":"auth_success"}`,
		`{"event":"PostModelSwitch","session_id":"s2","to_model":"mystery"}`,
	} {
		got := afterBind(t, domain.PrivacyStandard, call, `{"event":"SubagentStop","session_id":"s2","agent_id":"compaction"}`)
		wantEvents(t, got, "session_ended s1", "session_opened s2 privacy=standard", "session_refreshed s2")
	}
}

func TestTheProjectNameAtTheFullLevel(t *testing.T) {
	got := afterBind(t, domain.PrivacyFull,
		`{"event":"PreToolUse","session_id":"s1","tool_name":"Read","cwd":"C:\\Users\\u\\work\\alpha\\"}`,
		`{"event":"PostToolUse","session_id":"s1","cwd":"C:\\Users\\u\\work\\alpha"}`,
		`{"event":"PostToolUse","session_id":"s1","cwd":"C:\\"}`,
		`{"event":"PostToolUse","session_id":"s1"}`,
		`{"event":"Stop","session_id":"s1","cwd":"/home/u/work/beta/"}`,
		`{"event":"Notification","session_id":"s1","notification_type":"auth_success","cwd":"/home/u/work/gamma"}`,
	)
	wantEvents(t, got,
		"session_refreshed s1 project=alpha",
		"tool_started s1 tool=reading",
		"tool_finished s1",
		"tool_finished s1",
		"tool_finished s1",
		"session_refreshed s1 project=beta",
		"turn_finished s1",
		"session_refreshed s1 project=gamma",
	)
}

// everyEvent is one valid call per presence event, each with a working
// directory, a tool name or a model where the event carries one.
var everyEvent = []string{
	`{"event":"SessionStart","session_id":"s1","cwd":"/home/u/work/alpha","model":"claude-opus-5-5"}`,
	`{"event":"UserPromptSubmit","session_id":"s1","cwd":"/home/u/work/alpha"}`,
	`{"event":"PreToolUse","session_id":"s1","cwd":"/home/u/work/alpha","tool_name":"Edit"}`,
	`{"event":"PostToolUse","session_id":"s1","cwd":"/home/u/work/alpha"}`,
	`{"event":"PostToolUseFailure","session_id":"s1","cwd":"/home/u/work/alpha"}`,
	`{"event":"Notification","session_id":"s1","cwd":"/home/u/work/alpha","notification_type":"permission_prompt"}`,
	`{"event":"Notification","session_id":"s1","cwd":"/home/u/work/alpha","notification_type":"idle_prompt"}`,
	`{"event":"PreCompact","session_id":"s1","cwd":"/home/u/work/alpha"}`,
	`{"event":"PostCompact","session_id":"s1","cwd":"/home/u/work/alpha"}`,
	`{"event":"PostModelSwitch","session_id":"s1","cwd":"/home/u/work/alpha","to_model":"claude-haiku-4-5-20251001"}`,
	`{"event":"SubagentStart","session_id":"s1","cwd":"/home/u/work/alpha","agent_id":"a1"}`,
	`{"event":"SubagentStop","session_id":"s1","cwd":"/home/u/work/alpha","agent_id":"a1"}`,
	`{"event":"Stop","session_id":"s1","cwd":"/home/u/work/alpha"}`,
	`{"event":"StopFailure","session_id":"s1","cwd":"/home/u/work/alpha"}`,
	`{"event":"SessionStart","session_id":"s2","cwd":"/home/u/work/alpha"}`,
}

func TestPrivacyLevels(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		h := newHarness(t, domain.PrivacyMinimal)
		h.a.Open()
		for _, c := range everyEvent {
			h.call(c)
		}
		h.a.Close()
		// Only that the session exists, under which id, and when.
		refreshes := 0
		for _, e := range h.rec.events {
			bare := domain.Event{SessionID: e.SessionID, Surface: domain.SurfaceCode, At: e.At, Kind: e.Kind}
			switch e.Kind {
			case domain.KindSessionOpened:
				bare.Privacy = domain.PrivacyMinimal
			case domain.KindSessionRefreshed:
				refreshes++
			case domain.KindSessionEnded:
			default:
				t.Errorf("published %s", e.Kind)
			}
			if e != bare {
				t.Errorf("published %+v, want only %+v", e, bare)
			}
		}
		// Every call but the idle notice moves the time of last activity.
		// The last call also moves the session to s2.
		if want := len(everyEvent) - 1; refreshes != want {
			t.Errorf("%d refreshes, want %d", refreshes, want)
		}
		before := replay(t, h.rec.events[:len(h.rec.events)-1])
		if len(before) != 1 || before[0].ID != "s2" || before[0].Status != domain.StatusIdle || before[0].Subagents != 0 {
			t.Errorf("sessions = %+v", before)
		}
		if want := h.clock.Now(); !before[0].LastActivity.Equal(want) {
			t.Errorf("last activity %v, want %v", before[0].LastActivity, want)
		}
	})
	t.Run("standard", func(t *testing.T) {
		got := afterBind(t, domain.PrivacyStandard, everyEvent...)
		wantEvents(t, got,
			"session_refreshed s1 model=Opus 5.5",
			"turn_started s1",
			"tool_started s1 tool=editing",
			"tool_finished s1",
			"tool_finished s1",
			"attention_needed s1",
			"idle s1",
			"compaction_started s1",
			"compaction_finished s1",
			"model_changed s1 model=Haiku 4.5",
			"subagent_started s1",
			"subagent_stopped s1",
			"turn_finished s1",
			"turn_finished s1",
			"session_ended s1",
			"session_opened s2 model=Haiku 4.5 privacy=standard",
			"session_refreshed s2",
		)
	})
	t.Run("full", func(t *testing.T) {
		got := afterBind(t, domain.PrivacyFull, everyEvent...)
		wantEvents(t, got,
			"session_refreshed s1 project=alpha",
			"session_refreshed s1 model=Opus 5.5",
			"turn_started s1",
			"tool_started s1 tool=editing",
			"tool_finished s1",
			"tool_finished s1",
			"attention_needed s1",
			"idle s1",
			"compaction_started s1",
			"compaction_finished s1",
			"model_changed s1 model=Haiku 4.5",
			"subagent_started s1",
			"subagent_stopped s1",
			"turn_finished s1",
			"turn_finished s1",
			"session_ended s1",
			"session_opened s2 model=Haiku 4.5 project=alpha privacy=full",
			"session_refreshed s2",
		)
	})
}

// marker is seeded into every field that could carry user content.
const marker = "ZQXJ-SECRET"

// TestNothingLeaks is the leak test. Every field of every event that could
// carry user content holds the marker, and so does every allowlisted field
// but the event name and the session id. At every privacy level the marker
// reaches no published event, no tool result and no status report. Below the
// full level, no part of the working directory does either.
func TestNothingLeaks(t *testing.T) {
	content := map[string]any{
		"prompt":                 marker,
		"prompt_text":            marker,
		"tool_input":             map[string]any{"command": marker, "file_path": "/" + marker},
		"tool_response":          map[string]any{"stdout": marker},
		"tool_output":            marker,
		"error":                  marker,
		"message":                marker,
		"last_assistant_message": marker,
		"compact_summary":        marker,
		"custom_instructions":    marker,
		"transcript_path":        "/home/u/.claude/" + marker + ".jsonl",
		"agent_transcript_path":  "/home/u/.claude/" + marker + "-agent.jsonl",
		"scratchpad_dir":         "/tmp/" + marker,
		"session_title":          marker,
		"agent_type":             marker,
		"source":                 marker,
		"trigger":                marker,
		"reason":                 marker,
		"from_model":             marker,
		marker:                   marker,
	}
	// What an allowlisted field holds. Each keeps its shape, so that the call
	// is accepted and the value is read, and carries the marker.
	allowed := map[string][]string{
		fieldCwd:              {"/home/" + marker + "/parentdir/leafdir", `C:\Users\` + marker + `\parentdir\leafdir\`},
		fieldToolName:         {"mcp__" + marker + "__run", marker, "Bash"},
		fieldModel:            {"claude-opus-5-5-" + marker, "claude-" + strings.ToLower(marker) + "-5-5", marker},
		fieldToModel:          {"claude-opus-5-5-" + marker, "claude-" + strings.ToLower(marker) + "-5-5", marker},
		fieldNotificationType: {marker, "permission_prompt"},
		fieldAgentID:          {marker},
	}
	fragments := map[domain.Privacy][]string{
		domain.PrivacyMinimal:  {marker, "home", "Users", "parentdir", "leafdir"},
		domain.PrivacyStandard: {marker, "home", "Users", "parentdir", "leafdir"},
		domain.PrivacyFull:     {marker, "home", "Users", "parentdir"},
	}
	// With a resolver the directory is read at every level, to choose the level
	// and no more. It answers with the level the test is at, and no name.
	atLevel := func(level domain.Privacy) Resolver {
		return func(string) Settings { return Settings{Privacy: level} }
	}
	for _, resolve := range []bool{false, true} {
		for level, forbidden := range fragments {
			name := string(level)
			if resolve {
				name += " with a resolver"
			}
			t.Run(name, func(t *testing.T) {
				var h *harness
				if resolve {
					h = newProfileHarness(t, level, atLevel(level))
				} else {
					h = newHarness(t, level)
				}
				h.a.Open()
				calls := 0
				for round := 0; round < 3; round++ {
					for _, row := range allowlist {
						arguments := map[string]any{}
						for k, v := range content {
							arguments[k] = v
						}
						arguments[fieldEvent] = row.name
						for _, field := range row.fields {
							if values, ok := allowed[field]; ok {
								arguments[field] = values[round%len(values)]
							}
						}
						arguments[fieldSessionID] = "s" + strconv.Itoa(round)
						encoded, err := json.Marshal(arguments)
						if err != nil {
							t.Fatal(err)
						}
						h.call(string(encoded))
						calls++
					}
				}
				status := h.a.handleStatus(nil)
				h.a.Close()
				if got := h.a.ignored.Load(); got != 0 {
					t.Errorf("%d of %d calls were ignored, so their fields were not read", got, calls)
				}
				if len(h.rec.events) < calls/2 {
					t.Errorf("only %d events from %d calls", len(h.rec.events), calls)
				}
				sawProject := false
				for _, e := range h.rec.events {
					sawProject = sawProject || e.Project == "leafdir"
					checkClean(t, fmt.Sprintf("%+v", e), forbidden)
				}
				checkClean(t, fmt.Sprintf("%+v", status), forbidden)
				checkClean(t, fmt.Sprintf("%+v", status), []string{"leafdir", "/", `\`})
				if want := level == domain.PrivacyFull; sawProject != want {
					t.Errorf("project published: %v, want %v", sawProject, want)
				}
			})
		}
	}
}

func checkClean(t *testing.T, text string, forbidden []string) {
	t.Helper()
	for _, fragment := range forbidden {
		if strings.Contains(strings.ToLower(text), strings.ToLower(fragment)) {
			t.Errorf("%q appears in %q", fragment, text)
		}
	}
}

// stalled is a Publisher that blocks until it is released.
type stalled struct {
	recorder
	entered chan struct{}
	release chan struct{}
}

func (s *stalled) Publish(e domain.Event) {
	s.entered <- struct{}{}
	<-s.release
	s.recorder.Publish(e)
}

// TestAStalledPublisherNeverDelaysTheTool makes far more calls than the queue
// holds while the publisher is stuck. Every call returns. Had one waited for
// the publisher, the test would hang.
func TestAStalledPublisherNeverDelaysTheTool(t *testing.T) {
	pub := &stalled{entered: make(chan struct{}, 3*queueSize), release: make(chan struct{})}
	a, err := New(Options{Privacy: domain.PrivacyStandard, ProvisionalID: provisional, Publisher: pub, Status: fixedStatus{}, Clock: fakeclock.New(epoch)})
	if err != nil {
		t.Fatal(err)
	}
	a.Open()
	// The pump now holds the opening and is stuck, and the queue is empty.
	<-pub.entered
	const calls = 2*queueSize + 10
	for i := 0; i < calls; i++ {
		if got := a.handleEvent(json.RawMessage(bind), nil); got != constantResult {
			t.Fatalf("result = %+v", got)
		}
	}
	// The queue is full, so a move to another session is lost too, and the
	// adapter forgets it was asked.
	if got := a.handleEvent(json.RawMessage(`{"event":"Stop","session_id":"s2"}`), nil); got != constantResult {
		t.Fatalf("result = %+v", got)
	}
	dropped := a.dropped.Load()
	if dropped != calls+1-queueSize {
		t.Errorf("dropped = %d of %d calls with a queue of %d", dropped, calls+1, queueSize)
	}
	if !strings.Contains(a.handleStatus(nil).Text, "Events dropped: "+strconv.FormatInt(dropped, 10)) {
		t.Errorf("status = %q", a.handleStatus(nil).Text)
	}

	close(pub.release)
	// The opening, the two events of the move to s1, and one per call kept.
	want := 3 + queueSize
	waitFor(t, "the queue to drain", func() bool { return pub.count() == want })
	if got := a.handleEvent(json.RawMessage(`{"event":"Stop","session_id":"s2"}`), nil); got != constantResult {
		t.Fatalf("result = %+v", got)
	}
	a.Close()
	events := pub.events
	if sessions := replay(t, events[:len(events)-1]); len(sessions) != 1 || sessions[0].ID != "s2" {
		t.Errorf("sessions before the end = %+v, want only s2", sessions)
	}
	if sessions := replay(t, events); len(sessions) != 0 {
		t.Errorf("sessions after the end = %+v", sessions)
	}
}

// panicky is a Publisher that panics on one kind of event.
type panicky struct {
	recorder
	on domain.Kind
}

func (p *panicky) Publish(e domain.Event) {
	if e.Kind == p.on {
		panic("publisher failed")
	}
	p.recorder.Publish(e)
}

func TestAPublisherThatPanicsLosesOnlyThatEvent(t *testing.T) {
	pub := &panicky{on: domain.KindTurnStarted}
	a, err := New(Options{Privacy: domain.PrivacyStandard, ProvisionalID: provisional, Publisher: pub, Status: fixedStatus{}, Clock: fakeclock.New(epoch)})
	if err != nil {
		t.Fatal(err)
	}
	a.Open()
	a.handleEvent(json.RawMessage(bind), nil)
	a.handleEvent(json.RawMessage(`{"event":"Stop","session_id":"s1"}`), nil)
	a.Close()
	var got []string
	for _, e := range pub.events {
		got = append(got, show(e))
	}
	wantEvents(t, got,
		"session_opened "+provisional+" privacy=standard",
		"session_ended "+provisional,
		"session_opened s1 privacy=standard",
		"turn_finished s1",
		"session_ended s1",
	)
	if dropped := a.dropped.Load(); dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
}

func TestCloseBeforeOpenPublishesNothing(t *testing.T) {
	h := newHarness(t, domain.PrivacyStandard)
	wantEvents(t, h.finish())
}

// serveSession runs the adapter's tools behind a real server.
func serveSession(t *testing.T, a *Adapter, lines ...string) []string {
	t.Helper()
	const initialize = `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"claude-code","version":"2.1.284"}}}`
	input := initialize + "\n" + strings.Join(lines, "\n") + "\n"
	var out bytes.Buffer
	err := mcp.Serve(strings.NewReader(input), &out, mcp.Options{
		Name:    "presence",
		Version: "test",
		Initialize: func(mcp.ClientInfo) []mcp.Tool {
			a.Open()
			return a.Tools()
		},
		Shutdown: a.Close,
	})
	if err != nil {
		t.Fatal(err)
	}
	replies := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(replies) != len(lines)+1 {
		t.Fatalf("%d replies to %d requests", len(replies), len(lines)+1)
	}
	return replies[1:]
}

// TestTheResultIsByteIdentical sends every malformed call, a model's call and
// every kind of valid call through a real server. Each reply is the same
// bytes.
func TestTheResultIsByteIdentical(t *testing.T) {
	const want = `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{}"}]}}`
	request := func(arguments, meta string) string {
		params := `"name":"presence_event"`
		if arguments != "" {
			params += `,"arguments":` + arguments
		}
		if meta != "" {
			params += `,"_meta":` + meta
		}
		return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{` + params + `}}`
	}
	for _, level := range []domain.Privacy{domain.PrivacyMinimal, domain.PrivacyStandard, domain.PrivacyFull} {
		t.Run(string(level), func(t *testing.T) {
			var lines []string
			for _, c := range malformed {
				// A server refuses arguments that are not an object before
				// the tool sees them, with a protocol error.
				if c.arguments == "" || strings.HasPrefix(c.arguments, "{") {
					lines = append(lines, request(c.arguments, ""))
				}
			}
			lines = append(lines, request(bind, `{"claudecode/toolUseId":"toolu_1"}`))
			for _, c := range everyEvent {
				lines = append(lines, request(c, ""))
			}
			h := newHarness(t, level)
			for i, reply := range serveSession(t, h.a, lines...) {
				if reply != want {
					t.Errorf("reply to %s\n   = %s\nwant %s", lines[i], reply, want)
				}
			}
			// The server opened the session and closed it.
			if n := len(h.rec.events); n < 2 || h.rec.events[0].Kind != domain.KindSessionOpened || h.rec.events[n-1].Kind != domain.KindSessionEnded {
				t.Errorf("published %+v", h.rec.events)
			}
		})
	}
}

func TestTheToolsAsAClientSeesThem(t *testing.T) {
	h := newHarness(t, domain.PrivacyStandard)
	replies := serveSession(t, h.a, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var listed struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				InputSchema struct {
					Type       string `json:"type"`
					Properties map[string]struct {
						Type string   `json:"type"`
						Enum []string `json:"enum"`
					} `json:"properties"`
					Required             []string `json:"required"`
					AdditionalProperties *bool    `json:"additionalProperties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(replies[0]), &listed); err != nil {
		t.Fatal(err)
	}
	tools := listed.Result.Tools
	if len(tools) != 3 || tools[0].Name != EventToolName || tools[1].Name != StatusToolName || tools[2].Name != PauseToolName {
		t.Fatalf("tools = %+v", tools)
	}

	event := tools[0]
	if !strings.Contains(event.Description, "Hooks call this") || !strings.Contains(event.Description, "Do not call it yourself") {
		t.Errorf("event tool description = %q", event.Description)
	}
	// The schema declares the event name and the allowlisted fields, as
	// strings, and nothing else.
	wantFields := map[string]bool{fieldEvent: true}
	var wantNames []string
	for _, row := range HookEvents() {
		wantNames = append(wantNames, row.Name)
		for _, field := range row.Fields {
			wantFields[field] = true
		}
	}
	schema := event.InputSchema
	if schema.Type != "object" || len(schema.Properties) != len(wantFields) {
		t.Errorf("schema = %+v, want the %d fields %v", schema, len(wantFields), wantFields)
	}
	for name, property := range schema.Properties {
		if !wantFields[name] || property.Type != "string" {
			t.Errorf("schema declares %s as %+v", name, property)
		}
	}
	if got := schema.Properties[fieldEvent].Enum; !equal(got, wantNames) {
		t.Errorf("event names = %q, want %q", got, wantNames)
	}
	if !equal(schema.Required, []string{fieldEvent, fieldSessionID}) {
		t.Errorf("required = %q", schema.Required)
	}
	if schema.AdditionalProperties != nil {
		t.Error("the schema forbids other fields, which a client could act on")
	}

	status := tools[1]
	if !strings.Contains(status.Description, "Read-only") || status.InputSchema.Type != "object" ||
		len(status.InputSchema.Properties) != 1 || status.InputSchema.Properties["preview"].Type != "boolean" {
		t.Errorf("status tool = %+v", status)
	}
	// The preview is described as private, so that the model treats it so.
	if !strings.Contains(status.Description, "private preview") {
		t.Errorf("status tool description = %q", status.Description)
	}

	// The pause tool takes a duration and a flag, and nothing that could
	// change what is shown.
	pause := tools[2]
	properties := pause.InputSchema.Properties
	if pause.InputSchema.Type != "object" || len(properties) != 2 || properties["minutes"].Type != "integer" || properties["resume"].Type != "boolean" {
		t.Errorf("pause tool = %+v", pause)
	}
	if !strings.Contains(pause.Description, "only when the user asks") {
		t.Errorf("pause tool description = %q", pause.Description)
	}
}

func TestHookEvents(t *testing.T) {
	events := HookEvents()
	if len(events) != len(allowlist) {
		t.Fatalf("%d events, want %d", len(events), len(allowlist))
	}
	seen := map[string]bool{}
	for i, e := range events {
		row := allowlist[i]
		if e.Name != row.name || !equal(e.Fields, row.fields) {
			t.Errorf("event %d = %+v, want %+v", i, e, row)
		}
		if seen[e.Name] {
			t.Errorf("%s is listed twice", e.Name)
		}
		seen[e.Name] = true
		if e.Fields[0] != fieldSessionID || e.Fields[1] != fieldCwd {
			t.Errorf("%s reads %q, want the session id and working directory first", e.Name, e.Fields)
		}
		for _, required := range row.required {
			found := false
			for _, field := range row.fields {
				found = found || field == required
			}
			if !found {
				t.Errorf("%s requires %s and does not read it", e.Name, required)
			}
		}
		if (row.kind == "") != (e.Name == "Notification") {
			t.Errorf("%s has kind %q", e.Name, row.kind)
		}
	}
	// No hook is declared for the end of a session (ADR-0007, rule 7).
	if seen["SessionEnd"] {
		t.Error("SessionEnd is on the allowlist")
	}
	// The caller owns what it gets.
	events[0].Fields[0] = "changed"
	if allowlist[0].fields[0] != fieldSessionID {
		t.Error("HookEvents returned the allowlist's own slice")
	}
}

func TestStatusTool(t *testing.T) {
	report := func(t *testing.T, privacy domain.Privacy, s StatusSource) mcp.Result {
		t.Helper()
		rec := &recorder{}
		a, err := New(Options{Privacy: privacy, ProvisionalID: provisional, Publisher: rec, Status: s, Clock: fakeclock.New(epoch)})
		if err != nil {
			t.Fatal(err)
		}
		a.Open()
		a.handleEvent(json.RawMessage(`{"event":"UserPromptSubmit","session_id":"s1","cwd":"/home/u/work/alpha"}`), nil)
		a.handleEvent(json.RawMessage(`{"event":"Nonsense","session_id":"s1"}`), nil)
		result := a.handleStatus(json.RawMessage(`{"anything":"ignored"}`))
		a.Close()
		// Read-only: asking changed nothing that is published.
		if len(rec.events) < 4 || rec.events[len(rec.events)-1].Kind != domain.KindSessionEnded {
			t.Errorf("published %+v", rec.events)
		}
		return result
	}
	cases := []struct {
		name    string
		privacy domain.Privacy
		status  Status
		want    string
	}{
		{"host", domain.PrivacyFull, Status{Role: RoleHost, Discord: DiscordConnected, Sessions: 3},
			"Role: host\nDiscord: connected\nSessions: 3\nPrivacy: full\nPaused: no\nEvents ignored: 1\nEvents dropped: 0"},
		{"follower", domain.PrivacyStandard, Status{Role: RoleFollower, Discord: DiscordConnecting, Sessions: 1},
			"Role: follower\nDiscord: connecting\nSessions: 1\nPrivacy: standard\nPaused: no\nEvents ignored: 1\nEvents dropped: 0"},
		{"off", domain.PrivacyMinimal, Status{Role: RoleOff, Discord: DiscordDisconnected},
			"Role: off\nDiscord: disconnected\nSessions: 0\nPrivacy: minimal\nPaused: no\nEvents ignored: 1\nEvents dropped: 0"},
		{"nothing known", domain.PrivacyStandard, Status{},
			"Role: unknown\nDiscord: unknown\nSessions: 0\nPrivacy: standard\nPaused: no\nEvents ignored: 1\nEvents dropped: 0"},
		{"values outside the vocabulary", domain.PrivacyFull, Status{Role: "/home/u/work/alpha", Discord: "alpha", Sessions: 2},
			"Role: unknown\nDiscord: unknown\nSessions: 2\nPrivacy: full\nPaused: no\nEvents ignored: 1\nEvents dropped: 0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := report(t, c.privacy, fixedStatus(c.status))
			if got.IsError || got.Text != c.want {
				t.Errorf("status = %+v, want %q", got, c.want)
			}
			// No project name and no path.
			checkClean(t, got.Text, []string{"alpha", "/", `\`, "s1", provisional})
		})
	}
	t.Run("a source that panics", func(t *testing.T) {
		got := report(t, domain.PrivacyFull, panickyStatus{})
		if !got.IsError || got.Text != "Presence status is unavailable." {
			t.Errorf("status = %+v", got)
		}
	})
}

type panickyStatus struct{}

func (panickyStatus) Status() Status { panic("/home/u/work/alpha") }

// FuzzEventTool feeds arbitrary arguments and _meta to the event tool at each
// privacy level. The result is always the constant, and whatever is published
// is valid.
func FuzzEventTool(f *testing.F) {
	for _, c := range malformed {
		f.Add([]byte(c.arguments), []byte(nil))
	}
	for _, c := range everyEvent {
		f.Add([]byte(c), []byte(nil))
	}
	f.Add([]byte(bind), []byte(`{"claudecode/toolUseId":"toolu_1"}`))
	f.Add([]byte(`{"event":"Stop","session_id":"s1","cwd":"C:\\Users\\u\\work\\alpha\\"}`), []byte(`{}`))
	f.Add([]byte(`{"event":"Stop","session_id":"s1","cwd":"/\ufffd\u0000"}`), []byte(`[]`))
	f.Fuzz(func(t *testing.T, arguments, meta []byte) {
		for _, level := range []domain.Privacy{domain.PrivacyMinimal, domain.PrivacyStandard, domain.PrivacyFull} {
			rec := &recorder{}
			// Half the runs have a resolver, which reads the directory at every
			// level and answers with the level the run is at.
			var resolve Resolver
			if len(arguments)%2 == 1 {
				resolve = func(string) Settings { return Settings{Privacy: level} }
			}
			a, err := New(Options{Privacy: level, ProvisionalID: provisional, Publisher: rec, Status: fixedStatus{}, Clock: fakeclock.New(epoch), Resolve: resolve})
			if err != nil {
				t.Fatal(err)
			}
			a.Open()
			for i := 0; i < 2; i++ {
				if got := a.handleEvent(arguments, meta); got != constantResult {
					t.Errorf("result = %+v", got)
				}
			}
			a.Close()
			replay(t, rec.events)
			for _, e := range rec.events {
				if level != domain.PrivacyFull && e.Project != "" {
					t.Errorf("published a project at %s: %+v", level, e)
				}
				if level == domain.PrivacyMinimal && (e.Tool != "" || e.Model != "") {
					t.Errorf("published more than the session at minimal: %+v", e)
				}
			}
		}
	})
}
