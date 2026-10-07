package host_test

import (
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/host"
)

// withLinks gives a node the validation the binary has: the rules of
// ADR-0012 and the default host.
func withLinks(cfg *host.Config) {
	cfg.Link = func(raw string) (string, bool) {
		return config.ValidateLink(raw, []string{config.DefaultLinkHost})
	}
}

// linkOf is the link the host holds for a session, and whether it holds the
// session at all.
func linkOf(a *proc, id string) (string, bool) {
	for _, s := range a.held() {
		if s.ID == id {
			return s.Link, true
		}
	}
	return "", false
}

// badLinks are links a host must not publish. Each carries a word that must
// not reach the log.
var badLinks = map[string]string{
	"credentials":      "https://user:zqsecret@github.com/me/visions",
	"another host":     "https://zqsecret.example.org/me/visions",
	"not https":        "http://github.com/me/zqsecret",
	"a query":          "https://github.com/me/visions?zqsecret=1",
	"a longer path":    "https://github.com/me/visions/zqsecret",
	"not a link":       "zqsecret",
	"over the limit":   "https://github.com/me/zqsecret" + strings.Repeat("a", 600),
	"script":           "javascript:zqsecret",
	"a look-alike":     "https://github.com.zqsecret.example/me/visions",
	"a port":           "https://github.com:8443/me/zqsecret",
	"a fragment":       "https://github.com/me/visions#zqsecret",
	"an escaped slash": "https://github.com/me/visions%2Fzqsecret",
}

func TestAFollowersValidLinkIsHeldInItsPublishedForm(t *testing.T) {
	w, a := hosting(t, withLinks)
	q := w.join("one")
	q.welcomed("1.0.0")

	s := sessionAt("session-one", w.clock.Now())
	s.Link = "https://GitHub.com/me/visions.git"
	q.say(protocol.Sync{Session: s})
	w.eventually("the session to be held with its link", func() bool {
		link, held := linkOf(a, "session-one")
		return held && link == "https://github.com/me/visions"
	})

	e := protocol.EventData{SessionID: "session-two", Surface: "code", At: w.clock.Now().UnixMilli(), Kind: "session_opened", Link: "https://github.com/me/other"}
	q.say(protocol.Event{Event: e})
	w.eventually("the opened session to be held with its link", func() bool {
		link, held := linkOf(a, "session-two")
		return held && link == "https://github.com/me/other"
	})
}

// TestAnInvalidLinkIsDroppedAndTheSessionIsKept sends each bad link in a
// sync and in an opening event. The host keeps the session, without a link,
// keeps the connection, and warns without repeating the link.
func TestAnInvalidLinkIsDroppedAndTheSessionIsKept(t *testing.T) {
	for name, link := range badLinks {
		t.Run(name, func(t *testing.T) {
			w, a := hosting(t, withLinks)
			q := w.join("one")
			q.welcomed("1.0.0")
			before := a.counters.Snapshot().EventsDropped

			s := sessionAt("session-one", w.clock.Now())
			s.Link = link
			q.say(protocol.Sync{Session: s})
			mark(w, a, q, a.rendersSinceLock())
			if got, held := linkOf(a, "session-one"); !held || got != "" {
				t.Errorf("the synced session: held %v with the link %q, want held with none", held, got)
			}
			// The opening event is for another session, which replaces the
			// first on this connection.
			q.say(protocol.Event{Event: protocol.EventData{SessionID: "session-two", Surface: "code", At: w.clock.Now().UnixMilli(), Kind: "session_opened", Link: link}})
			mark(w, a, q, a.rendersSinceLock())
			if got, held := linkOf(a, "session-two"); !held || got != "" {
				t.Errorf("the opened session: held %v with the link %q, want held with none", held, got)
			}
			if got := a.counters.Snapshot().EventsDropped - before; got != 0 {
				t.Errorf("%d messages dropped, want none: only the link is dropped", got)
			}
			if got := q.status().Sessions; got != 2 {
				t.Errorf("the host reports %d sessions, want 2", got)
			}
			log := a.logs.String()
			if !strings.Contains(log, "link_invalid") {
				t.Errorf("the log has no warning of the link:\n%s", log)
			}
			if strings.Contains(log, "zqsecret") {
				t.Errorf("the log repeats the link:\n%s", log)
			}
		})
	}
}

// TestAHostWithNoValidationPublishesNoLink: a node built without the
// function drops every link, a valid one too.
func TestAHostWithNoValidationPublishesNoLink(t *testing.T) {
	w, a := hosting(t)
	q := w.join("one")
	q.welcomed("1.0.0")
	s := sessionAt("session-one", w.clock.Now())
	s.Link = "https://github.com/me/visions"
	q.say(protocol.Sync{Session: s})
	mark(w, a, q, a.rendersSinceLock())
	if got, held := linkOf(a, "session-one"); !held || got != "" {
		t.Errorf("held %v with the link %q, want held with none", held, got)
	}
}

// TestAFollowerThatSendsNoLinkIsAcceptedWithoutAWarning is a follower from
// before the field existed.
func TestAFollowerThatSendsNoLinkIsAcceptedWithoutAWarning(t *testing.T) {
	w, a := hosting(t, withLinks)
	q := w.join("one")
	q.welcomed("1.0.0")
	q.write(`{"type":"sync","session":{"id":"session-one","surface":"code","status":"idle","privacy":"standard","start":1,"last_activity":1,"subagents":0}}` + "\n")
	mark(w, a, q, a.rendersSinceLock())
	if got, held := linkOf(a, "session-one"); !held || got != "" {
		t.Errorf("held %v with the link %q, want held with none", held, got)
	}
	if log := a.logs.String(); strings.Contains(log, "link_invalid") {
		t.Errorf("the log warns of a link that was not sent:\n%s", log)
	}
}
