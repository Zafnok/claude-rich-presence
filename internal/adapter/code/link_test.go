package code

import (
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/presence"
)

// Directories with a link in their profile, and their links.
const (
	dirShared      = "/home/u/work/visions"        // full, with a link
	dirSharedSub   = "/home/u/work/visions/engine" // inside it
	dirSharedOther = "/home/u/work/atlas"          // full, with another link
	dirSharedQuiet = "/home/u/work/quiet"          // minimal, with a link
	dirSharedMid   = "/home/u/work/mid"            // standard, with a link

	linkVisions = "https://github.com/me/visions"
	linkAtlas   = "https://github.com/me/atlas"
	linkQuiet   = "https://github.com/me/quiet"
	linkMid     = "https://github.com/me/mid"

	// plantedLink is a link that tool input carries and nothing may publish.
	plantedLink = "https://github.com/planted/link"
)

// linked is the profiles of the other tests, and more with links.
func linked(global domain.Privacy) Resolver {
	rest := profiles(global)
	table := map[string]Settings{
		dirShared:      {Privacy: domain.PrivacyFull, Link: linkVisions},
		dirSharedSub:   {Privacy: domain.PrivacyFull, Link: linkVisions},
		dirSharedOther: {Privacy: domain.PrivacyFull, Link: linkAtlas},
		dirSharedQuiet: {Privacy: domain.PrivacyMinimal, Link: linkQuiet},
		dirSharedMid:   {Privacy: domain.PrivacyStandard, Link: linkMid},
	}
	return func(cwd string) Settings {
		if s, ok := table[cwd]; ok {
			return s
		}
		return rest(cwd)
	}
}

// buttonOf is the button Discord would be given for the one session that
// the events leave open.
func buttonOf(t *testing.T, events []domain.Event) domain.Button {
	t.Helper()
	sessions := replay(t, events)
	if len(sessions) != 1 {
		t.Fatalf("sessions = %+v, want one", sessions)
	}
	a, ok := presence.Render(sessions, epoch, presence.Settings{})
	if !ok {
		t.Fatal("nothing is shown")
	}
	return a.Button
}

func TestASessionInAProjectWithALinkOpensWithIt(t *testing.T) {
	tests := []struct {
		dir  string
		want string
	}{
		{dirShared, "session_opened s1 project=visions privacy=full link=" + linkVisions},
		// The link does not depend on the level: it is its own opt-in.
		{dirSharedMid, "session_opened s1 privacy=standard link=" + linkMid},
		{dirSharedQuiet, "session_opened s1 privacy=minimal link=" + linkQuiet},
		{dirNamed, "session_opened s1 project=Visions of Shuyi privacy=full"},
		{dirOther, "session_opened s1 privacy=standard"},
	}
	for _, tt := range tests {
		h := newProfileHarness(t, domain.PrivacyStandard, linked(domain.PrivacyStandard))
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", tt.dir))
		got := h.finish()
		// Before the directory is known the session has no link.
		wantEvents(t, []string{got[0], got[2]}, "session_opened "+provisional+" privacy=minimal", tt.want)
		// Only an opening carries a link.
		for _, e := range h.rec.events {
			if e.Link != "" && e.Kind != domain.KindSessionOpened {
				t.Errorf("%s carries a link", show(e))
			}
		}
	}
}

func TestTheButtonIsThatOfTheSessionsProject(t *testing.T) {
	for dir, want := range map[string]domain.Button{
		dirShared:      {Label: "View on GitHub", URL: linkVisions},
		dirSharedQuiet: {Label: "View on GitHub", URL: linkQuiet},
		dirNamed:       {},
		dirOther:       {},
	} {
		h := newProfileHarness(t, domain.PrivacyStandard, linked(domain.PrivacyStandard))
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", dir))
		h.finish()
		if got := buttonOf(t, h.rec.events[:len(h.rec.events)-1]); got != want {
			t.Errorf("in %s the button is %+v, want %+v", dir, got, want)
		}
	}
}

func TestMovingToAProjectWithAnotherLinkReopensTheSession(t *testing.T) {
	h := newProfileHarness(t, domain.PrivacyFull, linked(domain.PrivacyFull))
	h.a.Open()
	h.call(hookAt("SessionStart", dirShared, `"model":"claude-opus-5-5"`))
	// Inside the same project, and with no directory: the link is as it was
	// and the session is not reopened. Without a display name the project is
	// named for the directory, so that follows.
	h.call(hookAt("PreToolUse", dirSharedSub, `"tool_name":"Edit"`))
	h.call(`{"event":"PostToolUse","session_id":"s1"}`)
	// Another project with a link, then one with none, then back.
	h.call(hookAt("PreToolUse", dirSharedOther, `"tool_name":"Read"`))
	h.call(hookAt("PostToolUse", dirFull))
	h.call(hookAt("Stop", dirShared))
	got := h.finish()
	wantEvents(t, got,
		"session_opened "+provisional+" privacy=minimal",
		"session_ended "+provisional,
		"session_opened s1 project=visions privacy=full link="+linkVisions,
		"session_refreshed s1 model=Opus 5.5",
		"session_refreshed s1 project=engine",
		"tool_started s1 tool=editing",
		"tool_finished s1",
		"session_ended s1",
		"session_opened s1 model=Opus 5.5 project=atlas privacy=full link="+linkAtlas,
		"tool_started s1 tool=reading",
		"session_ended s1",
		"session_opened s1 model=Opus 5.5 project=open privacy=full",
		"tool_finished s1",
		"session_ended s1",
		"session_opened s1 model=Opus 5.5 project=visions privacy=full link="+linkVisions,
		"turn_finished s1",
		"session_ended s1",
	)
	// The session keeps its start time through every reopening.
	for _, e := range h.rec.events {
		if e.Kind == domain.KindSessionOpened && !e.At.Equal(epoch) {
			t.Errorf("%s opened at %v, want the start %v", show(e), e.At, epoch)
		}
	}
}

// TestNoToolInputSetsALink passes a link in every field the event tool reads
// and in fields it does not, for every hook event, with and without profiles.
// Nothing published carries a link and no button results (ADR-0012).
func TestNoToolInputSetsALink(t *testing.T) {
	names := []string{
		fieldSessionID, fieldCwd, fieldToolName, fieldModel, fieldToModel, fieldNotificationType, fieldAgentID,
		"link", "url", "repository", "button", "buttons", "project", "name",
	}
	harnesses := map[string]func() *harness{
		"profiles":               func() *harness { return newProfileHarness(t, domain.PrivacyFull, linked(domain.PrivacyFull)) },
		"no profiles at full":    func() *harness { return newHarness(t, domain.PrivacyFull) },
		"no profiles at minimal": func() *harness { return newHarness(t, domain.PrivacyMinimal) },
	}
	for name, build := range harnesses {
		for _, row := range allowlist {
			h := build()
			h.a.Open()
			// Every field at once, and then each alone beside valid values.
			all := `{"event":"` + row.name + `"`
			for _, field := range names {
				all += `,"` + field + `":"` + plantedLink + `"`
			}
			h.call(all + "}")
			for _, field := range names {
				valid := map[string]string{
					fieldSessionID: "s1", fieldToolName: "Edit", fieldToModel: "claude-opus-5-5",
					fieldNotificationType: "permission_prompt", fieldAgentID: "a1",
				}
				valid[field] = plantedLink
				arguments := `{"event":"` + row.name + `"`
				for key, value := range valid {
					arguments += `,"` + key + `":"` + value + `"`
				}
				h.call(arguments + "}")
			}
			h.a.Close()
			if len(h.rec.events) < 4 {
				t.Fatalf("%s, %s: published %d events, want the calls to have been taken", name, row.name, len(h.rec.events))
			}
			for i, e := range h.rec.events {
				if e.Link != "" {
					t.Errorf("%s, %s: %s carries a link", name, row.name, show(e))
				}
				for _, s := range replay(t, h.rec.events[:i+1]) {
					if a, _ := presence.Render([]domain.Session{s}, epoch, presence.Settings{}); a.Button != (domain.Button{}) {
						t.Errorf("%s, %s: a button results: %+v", name, row.name, a.Button)
					}
				}
			}
		}
	}
}

// TestAToolInputCannotChangeTheLinkOfAProject plants a link in the fields of
// calls made inside a project that has one. The link stays the profile's.
func TestAToolInputCannotChangeTheLinkOfAProject(t *testing.T) {
	planted := `"link":"` + plantedLink + `","url":"` + plantedLink + `","model":"` + plantedLink + `"`
	h := newProfileHarness(t, domain.PrivacyFull, linked(domain.PrivacyFull))
	h.a.Open()
	h.call(hookAt("UserPromptSubmit", dirShared))
	h.call(hookAt("PreToolUse", dirShared, `"tool_name":"`+plantedLink+`"`, planted))
	h.call(hookAt("SessionStart", dirShared, planted))
	h.finish()
	for _, e := range h.rec.events {
		if e.Link != "" && e.Link != linkVisions {
			t.Errorf("%s carries a link that is not the profile's", show(e))
		}
	}
	if got, want := buttonOf(t, h.rec.events[:len(h.rec.events)-1]), (domain.Button{Label: "View on GitHub", URL: linkVisions}); got != want {
		t.Errorf("button = %+v, want %+v", got, want)
	}
}

// TestAFailingResolverGivesNoLink: what a resolver that panics, or answers
// with a level that does not exist, said of a link is not used.
func TestAFailingResolverGivesNoLink(t *testing.T) {
	resolvers := map[string]Resolver{
		"panics":        func(string) Settings { panic("injected") },
		"unknown level": func(string) Settings { return Settings{Privacy: "everything", Link: linkVisions} },
	}
	for name, resolve := range resolvers {
		h := newProfileHarness(t, domain.PrivacyFull, resolve)
		h.a.Open()
		h.call(hookAt("UserPromptSubmit", dirShared))
		h.finish()
		for _, e := range h.rec.events {
			if e.Link != "" {
				t.Errorf("%s: %s carries a link", name, show(e))
			}
		}
	}
}
