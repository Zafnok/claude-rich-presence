package presence_test

import (
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/presence"
)

func TestASessionWithALinkGetsOneButtonNamedForItsHost(t *testing.T) {
	tests := []struct {
		name string
		link string
		want domain.Button
	}{
		{"no link", "", domain.Button{}},
		{"github", "https://github.com/me/visions", domain.Button{Label: "View on GitHub", URL: "https://github.com/me/visions"}},
		{"gitlab", "https://gitlab.com/me/visions", domain.Button{Label: "View on GitLab", URL: "https://gitlab.com/me/visions"}},
		{"bitbucket", "https://bitbucket.org/me/visions", domain.Button{Label: "View on Bitbucket", URL: "https://bitbucket.org/me/visions"}},
		{"codeberg", "https://codeberg.org/me/visions", domain.Button{Label: "View on Codeberg", URL: "https://codeberg.org/me/visions"}},
		{"a host that is not in the table", "https://git.example.org/me/visions", domain.Button{Label: "View repository", URL: "https://git.example.org/me/visions"}},
		{"a host that only ends like one in the table", "https://notgithub.com/me/visions", domain.Button{Label: "View repository", URL: "https://notgithub.com/me/visions"}},
		{"a link that is not https", "http://github.com/me/visions", domain.Button{}},
		{"text that is not a link", "github.com/me/visions", domain.Button{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := mustRender(t, []domain.Session{session("a", func(s *domain.Session) { s.Link = tt.link })}, presence.Settings{})
			if a.Button != tt.want {
				t.Errorf("button = %+v, want %+v", a.Button, tt.want)
			}
			if n := utf8.RuneCountInString(a.Button.Label); n > 32 {
				t.Errorf("the label has %d characters, and Discord allows 32", n)
			}
		})
	}
}

// TestTheButtonIsShownAtEveryLevel: a link is its own opt-in, made in the
// project's profile, and does not depend on the privacy level.
func TestTheButtonIsShownAtEveryLevel(t *testing.T) {
	const link = "https://github.com/me/visions"
	for _, privacy := range []domain.Privacy{domain.PrivacyMinimal, domain.PrivacyStandard, domain.PrivacyFull} {
		a := mustRender(t, []domain.Session{session("a", func(s *domain.Session) { s.Privacy, s.Link = privacy, link })}, presence.Settings{})
		if a.Button.URL != link {
			t.Errorf("at %s the button is %+v, want one with the link", privacy, a.Button)
		}
	}
}

// TestTheButtonIsTheFocusSessionsAndFollowsFocus has two sessions. The
// button is that of the one in focus, and changes when focus does.
func TestTheButtonIsTheFocusSessionsAndFollowsFocus(t *testing.T) {
	const linkA, linkB = "https://github.com/me/a", "https://gitlab.com/me/b"
	sessions := func(working string) []domain.Session {
		status := func(id string) domain.Status {
			if id == working {
				return domain.StatusWorking
			}
			return domain.StatusIdle
		}
		return []domain.Session{
			session("a", func(s *domain.Session) { s.Link, s.Status = linkA, status("a") }),
			session("b", func(s *domain.Session) { s.Link, s.Status = linkB, status("b") }),
			session("c", func(s *domain.Session) { s.Status = status("c") }),
		}
	}
	for working, want := range map[string]domain.Button{
		"a": {Label: "View on GitHub", URL: linkA},
		"b": {Label: "View on GitLab", URL: linkB},
		// The session in focus has no link, so no other session's is shown.
		"c": {},
	} {
		if got := mustRender(t, sessions(working), presence.Settings{}).Button; got != want {
			t.Errorf("with %s in focus the button is %+v, want %+v", working, got, want)
		}
	}
}

func TestNoButtonWhenNothingIsShown(t *testing.T) {
	idle := session("a", func(s *domain.Session) {
		s.Status, s.Link = domain.StatusIdle, "https://github.com/me/a"
	})
	if a, ok := presence.Render([]domain.Session{idle}, now, presence.Settings{IdleClear: time.Minute}); ok || a != (domain.Activity{}) {
		t.Errorf("Render = %+v, %v, want nothing", a, ok)
	}
}
