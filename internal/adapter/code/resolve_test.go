package code

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
)

// Directories the tests resolve. Anything else has no profile.
const (
	dirNamed    = "/home/u/work/shuyi"        // full, with a display name
	dirNamedSub = "/home/u/work/shuyi/engine" // inside the named project
	dirFull     = "/home/u/work/open"         // full, no name
	dirStandard = "/home/u/work/careful"      // standard, with a name that is not shown
	dirMinimal  = "/home/u/work/private"      // minimal
	dirOther    = "/home/u/work/other"        // no profile
)

// profiles is a Resolver over a table, with the global level for what it does
// not know.
func profiles(global domain.Privacy) Resolver {
	table := map[string]Settings{
		dirNamed:    {Privacy: domain.PrivacyFull, Name: "Visions of Shuyi"},
		dirNamedSub: {Privacy: domain.PrivacyFull, Name: "Visions of Shuyi"},
		dirFull:     {Privacy: domain.PrivacyFull},
		dirStandard: {Privacy: domain.PrivacyStandard, Name: "Careful Work"},
		dirMinimal:  {Privacy: domain.PrivacyMinimal, Name: "Private Work"},
	}
	return func(cwd string) Settings {
		if s, ok := table[cwd]; ok {
			return s
		}
		return Settings{Privacy: global}
	}
}

func newProfileHarness(t *testing.T, global domain.Privacy, resolve Resolver) *harness {
	t.Helper()
	h := &harness{t: t, rec: &recorder{}, clock: fakeclock.New(epoch)}
	a, err := New(Options{
		Privacy:       global,
		ProvisionalID: provisional,
		Publisher:     h.rec,
		Status:        fixedStatus{Role: RoleHost, Discord: DiscordConnected, Sessions: 1},
		Clock:         h.clock,
		Resolve:       resolve,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.a = a
	return h
}

// hookAt is a hook of the given event, in session s1, in a directory.
func hookAt(event, dir string, extra ...string) string {
	arguments := `{"event":"` + event + `","session_id":"s1","cwd":"` + dir + `"`
	for _, e := range extra {
		arguments += "," + e
	}
	return arguments + "}"
}

// finalSession is the one session left in a registry that replays events
// up to, but not including, the session's end.
func finalSession(t *testing.T, events []domain.Event) domain.Session {
	t.Helper()
	sessions := replay(t, events[:len(events)-1])
	if len(sessions) != 1 {
		t.Fatalf("sessions = %+v, want one", sessions)
	}
	return sessions[0]
}

func TestAProfileRaisesTheLevelForOneProject(t *testing.T) {
	t.Run("in the profiled project", func(t *testing.T) {
		h := newProfileHarness(t, domain.PrivacyMinimal, profiles(domain.PrivacyMinimal))
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", dirNamed))
		h.call(hookAt("PreToolUse", dirNamed, `"tool_name":"Edit"`))
		wantEvents(t, h.finish(),
			"session_opened "+provisional+" privacy=minimal",
			"session_ended "+provisional,
			"session_opened s1 project=Visions of Shuyi privacy=full",
			"turn_started s1",
			"tool_started s1 tool=editing",
			"session_ended s1",
		)
	})
	t.Run("elsewhere", func(t *testing.T) {
		h := newProfileHarness(t, domain.PrivacyMinimal, profiles(domain.PrivacyMinimal))
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", dirOther))
		h.call(hookAt("PreToolUse", dirOther, `"tool_name":"Edit"`))
		wantEvents(t, h.finish(),
			"session_opened "+provisional+" privacy=minimal",
			"session_ended "+provisional,
			"session_opened s1 privacy=minimal",
			"session_refreshed s1",
			"session_refreshed s1",
			"session_ended s1",
		)
	})
}

func TestAProfileLowersTheLevelForOneProject(t *testing.T) {
	h := newProfileHarness(t, domain.PrivacyFull, profiles(domain.PrivacyFull))
	h.a.Open()
	h.call(hookAt("SessionStart", dirMinimal, `"model":"claude-opus-5-5"`))
	h.call(hookAt("PreToolUse", dirMinimal, `"tool_name":"Edit"`))
	h.call(hookAt("PostModelSwitch", dirMinimal, `"to_model":"claude-haiku-4-5-20251001"`))
	h.call(hookAt("Notification", dirMinimal, `"notification_type":"permission_prompt"`))
	got := h.finish()
	wantEvents(t, got,
		// The session opens at the global level, before any directory is known.
		"session_opened "+provisional+" privacy=full",
		"session_ended "+provisional,
		"session_opened s1 privacy=minimal",
		"session_refreshed s1",
		"session_refreshed s1",
		"session_refreshed s1",
		"session_refreshed s1",
		"session_ended s1",
	)
	s := finalSession(t, h.rec.events)
	if s.Project != "" || s.Tool != domain.ToolNone || s.Model != "" || s.Status != domain.StatusIdle || s.Privacy != domain.PrivacyMinimal {
		t.Errorf("session at the host = %+v, want nothing past what minimal allows", s)
	}
}

func TestAnOpeningAtAHigherLevelCarriesNothingMore(t *testing.T) {
	h := newProfileHarness(t, domain.PrivacyFull, profiles(domain.PrivacyFull))
	h.a.Open()
	h.a.Close()
	if len(h.rec.events) != 2 {
		t.Fatalf("published %+v", h.rec.events)
	}
	opened := h.rec.events[0]
	want := domain.Event{SessionID: provisional, Surface: domain.SurfaceCode, At: epoch, Kind: domain.KindSessionOpened, Privacy: domain.PrivacyFull}
	if opened != want {
		t.Errorf("opening = %+v, want %+v", opened, want)
	}
}

func TestADisplayNameReplacesTheDirectoryNameAtFullOnly(t *testing.T) {
	const dir = "/home/u/work/alpha"
	cases := []struct {
		name     string
		settings Settings
		want     []string
	}{
		{"full with a name", Settings{Privacy: domain.PrivacyFull, Name: "Visions of Shuyi"},
			[]string{"session_opened s1 project=Visions of Shuyi privacy=full", "turn_started s1"}},
		{"full without a name", Settings{Privacy: domain.PrivacyFull},
			[]string{"session_opened s1 project=alpha privacy=full", "turn_started s1"}},
		{"standard with a name", Settings{Privacy: domain.PrivacyStandard, Name: "Visions of Shuyi"},
			[]string{"session_opened s1 privacy=standard", "turn_started s1"}},
		{"minimal with a name", Settings{Privacy: domain.PrivacyMinimal, Name: "Visions of Shuyi"},
			[]string{"session_opened s1 privacy=minimal", "session_refreshed s1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newProfileHarness(t, domain.PrivacyStandard, func(string) Settings { return c.settings })
			h.a.Open()
			h.call(hookAt("UserPromptSubmit", dir))
			want := append([]string{"session_opened " + provisional + " privacy=standard", "session_ended " + provisional}, c.want...)
			wantEvents(t, h.finish(), append(want, "session_ended s1")...)
		})
	}
}

func TestMovingToAHigherLevelRefreshesTheSession(t *testing.T) {
	t.Run("minimal to full", func(t *testing.T) {
		h := newProfileHarness(t, domain.PrivacyMinimal, profiles(domain.PrivacyMinimal))
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", dirOther))
		h.call(hookAt("PostToolUse", dirNamed))
		h.call(hookAt("PostToolUse", dirNamed))
		wantEvents(t, h.finish(),
			"session_opened "+provisional+" privacy=minimal",
			"session_ended "+provisional,
			"session_opened s1 privacy=minimal",
			"session_refreshed s1",
			"session_refreshed s1 project=Visions of Shuyi privacy=full",
			"tool_finished s1",
			"tool_finished s1",
			"session_ended s1",
		)
		s := finalSession(t, h.rec.events)
		if s.Project != "Visions of Shuyi" || s.Privacy != domain.PrivacyFull {
			t.Errorf("session at the host = %+v", s)
		}
	})
	t.Run("minimal to standard", func(t *testing.T) {
		h := newProfileHarness(t, domain.PrivacyMinimal, profiles(domain.PrivacyMinimal))
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", dirOther))
		h.call(hookAt("Stop", dirStandard))
		wantEvents(t, h.finish(),
			"session_opened "+provisional+" privacy=minimal",
			"session_ended "+provisional,
			"session_opened s1 privacy=minimal",
			"session_refreshed s1",
			"session_refreshed s1 privacy=standard",
			"turn_finished s1",
			"session_ended s1",
		)
	})
	t.Run("standard to full without a name", func(t *testing.T) {
		h := newProfileHarness(t, domain.PrivacyStandard, profiles(domain.PrivacyStandard))
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", dirOther))
		h.call(hookAt("Stop", dirFull))
		wantEvents(t, h.finish(),
			"session_opened "+provisional+" privacy=standard",
			"session_ended "+provisional,
			"session_opened s1 privacy=standard",
			"turn_started s1",
			"session_refreshed s1 project=open privacy=full",
			"turn_finished s1",
			"session_ended s1",
		)
	})
}

func TestMovingToALowerLevelEndsAndReopensTheSession(t *testing.T) {
	h := newProfileHarness(t, domain.PrivacyFull, profiles(domain.PrivacyFull))
	h.a.Open()
	h.call(hookAt("SessionStart", dirNamed, `"model":"claude-opus-5-5"`))
	h.call(hookAt("SubagentStart", dirNamed, `"agent_id":"a1"`))
	h.call(hookAt("PreToolUse", dirNamed, `"tool_name":"Edit"`))
	h.call(hookAt("PostToolUse", dirStandard))
	h.call(hookAt("SubagentStop", dirStandard, `"agent_id":"a1"`))
	h.call(hookAt("PreToolUse", dirStandard, `"tool_name":"Read"`))
	// An event that publishes nothing of its own still shows the session alive.
	h.call(hookAt("Notification", dirMinimal, `"notification_type":"auth_success"`))
	got := h.finish()
	wantEvents(t, got,
		"session_opened "+provisional+" privacy=full",
		"session_ended "+provisional,
		"session_opened s1 project=Visions of Shuyi privacy=full",
		"session_refreshed s1 model=Opus 5.5",
		"subagent_started s1",
		"tool_started s1 tool=editing",
		// Leaving full: what was published at full is ended with the session.
		"session_ended s1",
		"session_opened s1 model=Opus 5.5 privacy=standard",
		"tool_finished s1",
		// The subagent's start was forgotten with the old session at the host.
		"tool_started s1 tool=reading",
		"session_ended s1",
		"session_opened s1 privacy=minimal",
		"session_refreshed s1",
		"session_ended s1",
	)
	// Applying the events, the project is gone as soon as the level drops, the
	// session keeps its start time, and the host counts no subagent.
	registry := domain.NewRegistry()
	for i, e := range h.rec.events {
		if _, err := registry.Apply(e); err != nil {
			t.Fatal(err)
		}
		if i == 8 {
			s := registry.Snapshot()[0]
			if s.Project != "" || s.Subagents != 0 || s.Model != "Opus 5.5" || s.Privacy != domain.PrivacyStandard || !s.Start.Equal(epoch) {
				t.Errorf("session after leaving full = %+v", s)
			}
		}
	}
}

func TestAHookWithoutADirectoryKeepsTheSettingsLastResolved(t *testing.T) {
	t.Run("at full with a name", func(t *testing.T) {
		h := newProfileHarness(t, domain.PrivacyMinimal, profiles(domain.PrivacyMinimal))
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", dirNamed))
		h.call(`{"event":"PreToolUse","session_id":"s1","tool_name":"Edit"}`)
		h.call(`{"event":"PostToolUse","session_id":"s1"}`)
		wantEvents(t, h.finish(),
			"session_opened "+provisional+" privacy=minimal",
			"session_ended "+provisional,
			"session_opened s1 project=Visions of Shuyi privacy=full",
			"turn_started s1",
			"tool_started s1 tool=editing",
			"tool_finished s1",
			"session_ended s1",
		)
	})
	t.Run("at minimal below a higher global level", func(t *testing.T) {
		h := newProfileHarness(t, domain.PrivacyFull, profiles(domain.PrivacyFull))
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", dirMinimal))
		h.call(`{"event":"PreToolUse","session_id":"s1","tool_name":"Edit"}`)
		h.call(`{"event":"SessionStart","session_id":"s1","model":"claude-opus-5-5"}`)
		wantEvents(t, h.finish(),
			"session_opened "+provisional+" privacy=full",
			"session_ended "+provisional,
			"session_opened s1 privacy=minimal",
			"session_refreshed s1",
			"session_refreshed s1",
			"session_refreshed s1",
			"session_ended s1",
		)
	})
	t.Run("before any directory", func(t *testing.T) {
		h := newProfileHarness(t, domain.PrivacyStandard, profiles(domain.PrivacyStandard))
		h.a.Open()
		h.call(`{"event":"UserPromptSubmit","session_id":"s1"}`)
		wantEvents(t, h.finish(), append(bound(domain.PrivacyStandard), "session_ended s1")...)
	})
}

func TestAMovedDirectoryInsideAProjectChangesNothing(t *testing.T) {
	h := newProfileHarness(t, domain.PrivacyMinimal, profiles(domain.PrivacyMinimal))
	h.a.Open()
	h.call(hookAt("UserPromptSubmit", dirNamed))
	h.call(hookAt("PostToolUse", dirNamedSub))
	wantEvents(t, h.finish(),
		"session_opened "+provisional+" privacy=minimal",
		"session_ended "+provisional,
		"session_opened s1 project=Visions of Shuyi privacy=full",
		"turn_started s1",
		"tool_finished s1",
		"session_ended s1",
	)
}

func TestMovingBetweenProjectsAtFullRenamesTheSession(t *testing.T) {
	h := newProfileHarness(t, domain.PrivacyFull, profiles(domain.PrivacyFull))
	h.a.Open()
	h.call(hookAt("UserPromptSubmit", dirNamed))
	h.call(hookAt("PostToolUse", dirFull))
	wantEvents(t, h.finish(),
		"session_opened "+provisional+" privacy=full",
		"session_ended "+provisional,
		"session_opened s1 project=Visions of Shuyi privacy=full",
		"turn_started s1",
		"session_refreshed s1 project=open",
		"tool_finished s1",
		"session_ended s1",
	)
}

// A resolver that fails, or that answers with a level that does not exist,
// leaves the session at minimal, without a name, and the call still returns
// its constant.
func TestAFailingResolverIsTreatedAsMinimal(t *testing.T) {
	cases := []struct {
		name    string
		resolve Resolver
	}{
		{"panics", func(string) Settings { panic("/home/u/work/shuyi") }},
		{"unknown level", func(string) Settings { return Settings{Privacy: "summary", Name: "Visions of Shuyi"} }},
		{"no level", func(string) Settings { return Settings{Name: "Visions of Shuyi"} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newProfileHarness(t, domain.PrivacyFull, c.resolve)
			h.a.Open()
			h.call(hookAt("UserPromptSubmit", dirNamed))
			h.call(hookAt("PreToolUse", dirNamed, `"tool_name":"Edit"`))
			h.call(`{"event":"PostToolUse","session_id":"s1"}`)
			status := h.a.handleStatus(nil)
			wantEvents(t, h.finish(),
				"session_opened "+provisional+" privacy=full",
				"session_ended "+provisional,
				"session_opened s1 privacy=minimal",
				"session_refreshed s1",
				"session_refreshed s1",
				"session_refreshed s1",
				"session_ended s1",
			)
			if !strings.Contains(status.Text, "Privacy: minimal") {
				t.Errorf("status = %q", status.Text)
			}
			if got := h.a.ignored.Load(); got != 0 {
				t.Errorf("%d calls were counted as ignored", got)
			}
			checkClean(t, status.Text, []string{"shuyi", "/"})
		})
	}
}

func TestTheStatusToolReportsTheEffectiveLevel(t *testing.T) {
	h := newProfileHarness(t, domain.PrivacyMinimal, profiles(domain.PrivacyMinimal))
	level := func() string {
		t.Helper()
		text := h.a.handleStatus(nil).Text
		for _, line := range strings.Split(text, "\n") {
			if rest, ok := strings.CutPrefix(line, "Privacy: "); ok {
				return rest
			}
		}
		t.Fatalf("no level in %q", text)
		return ""
	}
	h.a.Open()
	if got := level(); got != "minimal" {
		t.Errorf("before any hook, level = %s", got)
	}
	h.call(hookAt("UserPromptSubmit", dirNamed))
	if got := level(); got != "full" {
		t.Errorf("in a full project, level = %s", got)
	}
	h.call(hookAt("UserPromptSubmit", dirStandard))
	if got := level(); got != "standard" {
		t.Errorf("in a standard project, level = %s", got)
	}
	h.finish()
}

func TestADroppedCallLeavesTheResolvedSettingsAsTheyWere(t *testing.T) {
	pub := &stalled{entered: make(chan struct{}, 3*queueSize), release: make(chan struct{})}
	a, err := New(Options{
		Privacy: domain.PrivacyFull, ProvisionalID: provisional, Publisher: pub,
		Status: fixedStatus{}, Clock: fakeclock.New(epoch), Resolve: profiles(domain.PrivacyFull),
	})
	if err != nil {
		t.Fatal(err)
	}
	a.Open()
	<-pub.entered
	call := func(arguments string) { a.handleEvent(json.RawMessage(arguments), nil) }
	// Fill the queue with calls that change nothing.
	call(hookAt("UserPromptSubmit", dirNamed))
	for i := 0; i < queueSize+5; i++ {
		call(hookAt("PostToolUse", dirNamed))
	}
	dropped := a.dropped.Load()
	if dropped == 0 {
		t.Fatal("nothing was dropped")
	}
	// A call that would lower the level is lost whole. The level stays, and
	// so the status tool does not claim the lower one.
	call(hookAt("PostToolUse", dirMinimal))
	if got := a.dropped.Load(); got != dropped+1 {
		t.Errorf("dropped = %d, want %d", got, dropped+1)
	}
	if text := a.handleStatus(nil).Text; !strings.Contains(text, "Privacy: full") {
		t.Errorf("status = %q", text)
	}
	close(pub.release)
	waitFor(t, "the queue to drain", func() bool { return pub.count() == 3+queueSize })
	// The next call makes the same moves again.
	call(hookAt("PostToolUse", dirMinimal))
	a.Close()
	got := make([]string, 0, len(pub.events))
	for _, e := range pub.events[len(pub.events)-4:] {
		got = append(got, show(e))
	}
	wantEvents(t, got,
		"session_ended s1",
		"session_opened s1 privacy=minimal",
		"session_refreshed s1",
		"session_ended s1",
	)
}

func TestWithoutAResolverTheWorkingDirectoryIsStillDiscardedBelowFull(t *testing.T) {
	h := newHarness(t, domain.PrivacyStandard)
	h.a.Open()
	// Too long a directory would make a read call malformed. It is not read.
	h.call(hookAt("UserPromptSubmit", "/"+strings.Repeat("d", maxCwdLen)))
	wantEvents(t, h.finish(), append(bound(domain.PrivacyStandard), "session_ended s1")...)
}

func TestWithAResolverTheWorkingDirectoryIsReadAtEveryLevel(t *testing.T) {
	for _, global := range []domain.Privacy{domain.PrivacyMinimal, domain.PrivacyStandard, domain.PrivacyFull} {
		t.Run(string(global), func(t *testing.T) {
			var seen []string
			h := newProfileHarness(t, global, func(cwd string) Settings {
				seen = append(seen, cwd)
				return Settings{Privacy: global}
			})
			h.a.Open()
			h.call(hookAt("UserPromptSubmit", dirOther))
			h.finish()
			if len(seen) != 1 || seen[0] != dirOther {
				t.Errorf("resolver saw %q", seen)
			}
		})
	}
}
