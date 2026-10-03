package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidSession is wrapped by every error Session.Validate returns.
var ErrInvalidSession = errors.New("invalid session")

// Session is the state of one open session, reduced from its events.
type Session struct {
	ID      string
	Surface Surface
	Status  Status
	// Tool is the kind of tool in use. It is ToolNone unless Status is
	// StatusWorking.
	Tool ToolKind
	// Model is a short family label chosen by the adapter, or empty when not
	// known. The domain does not interpret it.
	Model string
	// Project is the project name, or empty when the adapter did not send one.
	Project string
	Privacy Privacy
	// Start is when the session began. It feeds the elapsed timer and travels
	// with the session through a failover.
	Start        time.Time
	LastActivity time.Time
	Subagents    int
}

// transitions is the state machine as data: for each event kind, the statuses
// it leaves and the status it leads to. A status that is not listed is left
// unchanged. Session ended lists none because it removes the session.
//
// It matches the table under "Session state" in docs/architecture/README.md
// cell for cell.
var transitions = map[Kind]map[Status]Status{
	KindSessionOpened:    {},
	KindSessionRefreshed: {},
	KindTurnStarted: {
		StatusIdle:       StatusWorking,
		StatusWaiting:    StatusWorking,
		StatusCompacting: StatusWorking,
	},
	KindToolStarted: {
		StatusIdle:       StatusWorking,
		StatusWaiting:    StatusWorking,
		StatusCompacting: StatusWorking,
	},
	// A tool can finish just after its turn does, so this does not leave idle.
	KindToolFinished: {
		StatusWaiting: StatusWorking,
	},
	KindAttentionNeeded: {
		StatusIdle:    StatusWaiting,
		StatusWorking: StatusWaiting,
	},
	KindIdle: {
		StatusWorking:    StatusIdle,
		StatusWaiting:    StatusIdle,
		StatusCompacting: StatusIdle,
	},
	KindTurnFinished: {
		StatusWorking:    StatusIdle,
		StatusWaiting:    StatusIdle,
		StatusCompacting: StatusIdle,
	},
	// Only a working session compacts and returns, so compacting an idle
	// session by hand does not leave it shown as working afterwards.
	KindCompactionStarted: {
		StatusWorking: StatusCompacting,
	},
	KindCompactionFinished: {
		StatusCompacting: StatusWorking,
	},
	KindModelChanged:    {},
	KindSubagentStarted: {},
	KindSubagentStopped: {},
	KindSessionEnded:    {},
}

// newSession is the session an event creates when its id is not known: idle,
// at the most private level, begun at the time of the event.
func newSession(e Event) Session {
	return Session{
		ID:           e.SessionID,
		Surface:      e.Surface,
		Status:       StatusIdle,
		Privacy:      PrivacyMinimal,
		Start:        e.At,
		LastActivity: e.At,
	}
}

// apply returns the session after a valid event of any kind but session ended.
func (s Session) apply(e Event) Session {
	if next, ok := transitions[e.Kind][s.Status]; ok {
		s.Status = next
	}

	switch e.Kind {
	case KindSessionOpened, KindSessionRefreshed:
		if e.Model != "" {
			s.Model = e.Model
		}
		if e.Project != "" {
			s.Project = e.Project
		}
		if e.Privacy != "" {
			s.Privacy = e.Privacy
		}
	case KindTurnStarted:
		s.Tool = ToolNone
	case KindToolStarted:
		s.Tool = e.Tool
	case KindModelChanged:
		s.Model = e.Model
	case KindSubagentStarted:
		s.Subagents++
	case KindSubagentStopped:
		// A stop whose start was not seen is ignored.
		if s.Subagents > 0 {
			s.Subagents--
		}
	}
	if s.Status != StatusWorking {
		s.Tool = ToolNone
	}

	// Events may arrive out of order after a failover, so the times only ever
	// widen. The idle notice reports inactivity and is not itself activity.
	if e.At.Before(s.Start) {
		s.Start = e.At
	}
	if e.Kind != KindIdle && e.At.After(s.LastActivity) {
		s.LastActivity = e.At
	}
	return s
}

// Validate reports whether the session is well formed, as a session received
// in a sync must be. The error never contains a field's value.
func (s Session) Validate() error {
	reason := ""
	switch {
	case !s.Status.Valid():
		reason = "unknown status"
	case s.Tool != ToolNone && !s.Tool.Valid():
		reason = "unknown tool kind"
	case s.Tool != ToolNone && s.Status != StatusWorking:
		reason = "tool kind on a session that is not working"
	case !s.Privacy.Valid():
		reason = "unknown privacy level"
	case s.Start.IsZero():
		reason = "start time is not set"
	case s.LastActivity.Before(s.Start):
		reason = "last activity is before the start"
	case s.Subagents < 0:
		reason = "negative subagent count"
	default:
		reason = checkCommon(s.ID, s.Surface, s.Model, s.Project)
	}
	if reason != "" {
		return fmt.Errorf("%w: %s", ErrInvalidSession, reason)
	}
	return nil
}
