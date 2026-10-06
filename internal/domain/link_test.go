package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

const repoLink = "https://github.com/me/visions"

// only is the one session of a registry.
func only(t *testing.T, r *domain.Registry) domain.Session {
	t.Helper()
	sessions := r.Snapshot()
	if len(sessions) != 1 {
		t.Fatalf("sessions = %+v, want one", sessions)
	}
	return sessions[0]
}

func TestASessionTakesItsLinkWhenItOpens(t *testing.T) {
	r := domain.NewRegistry()
	opened := event(domain.KindSessionOpened, t0)
	opened.Link = repoLink
	if _, err := r.Apply(opened); err != nil {
		t.Fatal(err)
	}
	if got := only(t, r).Link; got != repoLink {
		t.Errorf("link = %q, want %q", got, repoLink)
	}
}

// TestNoLaterEventChangesTheLink passes a link on every kind of event to a
// session that has one, and to one that has none.
func TestNoLaterEventChangesTheLink(t *testing.T) {
	for _, had := range []string{"", repoLink} {
		for _, kind := range domain.Kinds() {
			if kind == domain.KindSessionEnded {
				continue
			}
			r := domain.NewRegistry()
			opened := event(domain.KindSessionOpened, t0)
			opened.Link = had
			if _, err := r.Apply(opened); err != nil {
				t.Fatal(err)
			}
			e := event(kind, t0.Add(time.Second))
			e.Tool, e.Model = domain.ToolEditing, "Opus"
			e.Link = "https://github.com/other/place"
			if _, err := r.Apply(e); err != nil {
				t.Fatalf("%s: %v", kind, err)
			}
			if got := only(t, r).Link; got != had {
				t.Errorf("after %s the link is %q, want %q", kind, got, had)
			}
		}
	}
}

func TestAnEventThatIsNotAnOpeningCreatesASessionWithoutALink(t *testing.T) {
	for _, kind := range domain.Kinds() {
		if kind == domain.KindSessionOpened || kind == domain.KindSessionEnded {
			continue
		}
		r := domain.NewRegistry()
		e := event(kind, t0)
		e.Tool, e.Model = domain.ToolEditing, "Opus"
		e.Link = repoLink
		if _, err := r.Apply(e); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if got := only(t, r).Link; got != "" {
			t.Errorf("a session created by %s has the link %q, want none", kind, got)
		}
	}
}

func TestALinkHasALengthLimit(t *testing.T) {
	atLimit := strings.Repeat("a", domain.MaxLinkLen)

	e := event(domain.KindSessionOpened, t0)
	e.Link = atLimit
	if err := e.Validate(); err != nil {
		t.Errorf("an event with a link at the limit: %v", err)
	}
	e.Link += "a"
	if err := e.Validate(); !errors.Is(err, domain.ErrInvalidEvent) || strings.Contains(err.Error(), "aaa") {
		t.Errorf("an event with a link over the limit: %v, want an invalid event that does not repeat it", err)
	}

	s := session(domain.StatusIdle)
	s.Link = atLimit
	if err := s.Validate(); err != nil {
		t.Errorf("a session with a link at the limit: %v", err)
	}
	s.Link += "a"
	if err := s.Validate(); !errors.Is(err, domain.ErrInvalidSession) || strings.Contains(err.Error(), "aaa") {
		t.Errorf("a session with a link over the limit: %v, want an invalid session that does not repeat it", err)
	}
}

func TestASyncCarriesTheLink(t *testing.T) {
	r := domain.NewRegistry()
	s := session(domain.StatusIdle)
	s.Link = repoLink
	if err := r.Sync(s); err != nil {
		t.Fatal(err)
	}
	if got := only(t, r).Link; got != repoLink {
		t.Errorf("link = %q, want %q", got, repoLink)
	}
}

func TestActivitiesWithDifferentButtonsAreNotEqual(t *testing.T) {
	a := domain.Activity{Details: "Coding", Button: domain.Button{Label: "View on GitHub", URL: repoLink}}
	for name, b := range map[string]domain.Activity{
		"no button":     {Details: "Coding"},
		"another label": {Details: "Coding", Button: domain.Button{Label: "View repository", URL: repoLink}},
		"another link":  {Details: "Coding", Button: domain.Button{Label: "View on GitHub", URL: repoLink + "2"}},
	} {
		if a.Equal(b) || b.Equal(a) {
			t.Errorf("%s: equal, want different", name)
		}
	}
	if !a.Equal(a) {
		t.Error("an activity with a button is not equal to itself")
	}
}
