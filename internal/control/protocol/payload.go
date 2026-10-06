package protocol

import (
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// EventData is a presence event on the wire. It is kept apart from
// domain.Event, with explicit conversion, so that a change to the domain
// cannot silently change the wire format.
//
// The closed vocabularies (surface, kind, tool, privacy) travel as their
// words and are not judged here: the host validates the converted event, so a
// newer follower's new word is dropped by an older host rather than closing
// the connection.
type EventData struct {
	SessionID string `json:"session_id"`
	Surface   string `json:"surface"`
	// At is in Unix milliseconds.
	At      int64  `json:"at"`
	Kind    string `json:"kind"`
	Tool    string `json:"tool,omitempty"`
	Model   string `json:"model,omitempty"`
	Project string `json:"project,omitempty"`
	Privacy string `json:"privacy,omitempty"`
	// Link is a repository link, on a session opened event. It is not judged
	// here beyond the limit on a line: the host validates it and drops it
	// alone when it is not valid (ADR-0012).
	Link string `json:"link,omitempty"`
}

// SessionState is a session on the wire, as a sync carries it. See EventData
// for why it is separate from domain.Session.
type SessionState struct {
	ID      string `json:"id"`
	Surface string `json:"surface"`
	Status  string `json:"status"`
	Tool    string `json:"tool,omitempty"`
	Model   string `json:"model,omitempty"`
	Project string `json:"project,omitempty"`
	Privacy string `json:"privacy"`
	// Start and LastActivity are in Unix milliseconds.
	Start        int64 `json:"start"`
	LastActivity int64 `json:"last_activity"`
	Subagents    int   `json:"subagents"`
	// Link is a repository link. See EventData.
	Link string `json:"link,omitempty"`
}

// EventFromDomain converts an event for the wire. Times are cut to the
// millisecond.
func EventFromDomain(e domain.Event) EventData {
	return EventData{
		SessionID: e.SessionID,
		Surface:   string(e.Surface),
		At:        toMillis(e.At),
		Kind:      string(e.Kind),
		Tool:      string(e.Tool),
		Model:     e.Model,
		Project:   e.Project,
		Privacy:   string(e.Privacy),
		Link:      e.Link,
	}
}

// Domain converts an event from the wire. It does not validate: the caller
// passes the result to the domain, which does.
func (e EventData) Domain() domain.Event {
	return domain.Event{
		SessionID: e.SessionID,
		Surface:   domain.Surface(e.Surface),
		At:        fromMillis(e.At),
		Kind:      domain.Kind(e.Kind),
		Tool:      domain.ToolKind(e.Tool),
		Model:     e.Model,
		Project:   e.Project,
		Privacy:   domain.Privacy(e.Privacy),
		Link:      e.Link,
	}
}

// SessionFromDomain converts a session for the wire. Times are cut to the
// millisecond.
func SessionFromDomain(s domain.Session) SessionState {
	return SessionState{
		ID:           s.ID,
		Surface:      string(s.Surface),
		Status:       string(s.Status),
		Tool:         string(s.Tool),
		Model:        s.Model,
		Project:      s.Project,
		Privacy:      string(s.Privacy),
		Start:        toMillis(s.Start),
		LastActivity: toMillis(s.LastActivity),
		Subagents:    s.Subagents,
		Link:         s.Link,
	}
}

// Domain converts a session from the wire. It does not validate: the caller
// passes the result to the domain, which does.
func (s SessionState) Domain() domain.Session {
	return domain.Session{
		ID:           s.ID,
		Surface:      domain.Surface(s.Surface),
		Status:       domain.Status(s.Status),
		Tool:         domain.ToolKind(s.Tool),
		Model:        s.Model,
		Project:      s.Project,
		Privacy:      domain.Privacy(s.Privacy),
		Start:        fromMillis(s.Start),
		LastActivity: fromMillis(s.LastActivity),
		Subagents:    s.Subagents,
		Link:         s.Link,
	}
}

// PreviewOf is what a host shows, for the wire: an activity, or nothing when
// shown is false. The start is cut to the millisecond.
func PreviewOf(a domain.Activity, shown bool) PreviewResult {
	if !shown {
		return PreviewResult{}
	}
	return PreviewResult{
		Shown:       true,
		Details:     a.Details,
		State:       a.State,
		Start:       toMillis(a.Start),
		LargeImage:  a.LargeImage,
		LargeText:   a.LargeText,
		SmallImage:  a.SmallImage,
		SmallText:   a.SmallText,
		ButtonLabel: a.Button.Label,
		ButtonURL:   a.Button.URL,
	}
}

// Activity undoes PreviewOf. It reports false when nothing is shown.
func (m PreviewResult) Activity() (domain.Activity, bool) {
	if !m.Shown {
		return domain.Activity{}, false
	}
	return domain.Activity{
		Details:    m.Details,
		State:      m.State,
		Start:      fromMillis(m.Start),
		LargeImage: m.LargeImage,
		LargeText:  m.LargeText,
		SmallImage: m.SmallImage,
		SmallText:  m.SmallText,
		Button:     domain.Button{Label: m.ButtonLabel, URL: m.ButtonURL},
	}, true
}

func (e EventData) check() error {
	return firstError(
		checkText("event.session_id", e.SessionID, true, domain.MaxIDLen),
		checkText("event.surface", e.Surface, true, MaxWordLen),
		checkTime("event.at", e.At),
		checkText("event.kind", e.Kind, true, MaxWordLen),
		checkText("event.tool", e.Tool, false, MaxWordLen),
		checkText("event.model", e.Model, false, domain.MaxModelLen),
		checkText("event.project", e.Project, false, domain.MaxProjectLen),
		checkText("event.privacy", e.Privacy, false, MaxWordLen),
	)
}

func (s SessionState) check() error {
	subagents := error(nil)
	if s.Subagents < 0 {
		subagents = invalid("session.subagents")
	}
	return firstError(
		checkText("session.id", s.ID, true, domain.MaxIDLen),
		checkText("session.surface", s.Surface, true, MaxWordLen),
		checkText("session.status", s.Status, true, MaxWordLen),
		checkText("session.tool", s.Tool, false, MaxWordLen),
		checkText("session.model", s.Model, false, domain.MaxModelLen),
		checkText("session.project", s.Project, false, domain.MaxProjectLen),
		checkText("session.privacy", s.Privacy, true, MaxWordLen),
		checkTime("session.start", s.Start),
		checkTime("session.last_activity", s.LastActivity),
		subagents,
	)
}

// firstError returns the first error that is not nil.
func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// toMillis is a time in Unix milliseconds. The zero time, which the domain
// reads as "not set", becomes 0.
func toMillis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// fromMillis undoes toMillis.
func fromMillis(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
