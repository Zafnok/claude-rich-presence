package domain

import (
	"errors"
	"fmt"
	"time"
)

// Length limits, in bytes, on the strings an event or a session carries.
const (
	MaxIDLen      = 128
	MaxModelLen   = 64
	MaxProjectLen = 128
)

// ErrInvalidEvent is wrapped by every error Event.Validate returns.
var ErrInvalidEvent = errors.New("invalid event")

// Kind says what happened in a session.
type Kind string

// The event kinds.
const (
	KindSessionOpened      Kind = "session_opened"
	KindSessionRefreshed   Kind = "session_refreshed"
	KindTurnStarted        Kind = "turn_started"
	KindToolStarted        Kind = "tool_started"
	KindToolFinished       Kind = "tool_finished"
	KindAttentionNeeded    Kind = "attention_needed"
	KindIdle               Kind = "idle"
	KindTurnFinished       Kind = "turn_finished"
	KindCompactionStarted  Kind = "compaction_started"
	KindCompactionFinished Kind = "compaction_finished"
	KindModelChanged       Kind = "model_changed"
	KindSubagentStarted    Kind = "subagent_started"
	KindSubagentStopped    Kind = "subagent_stopped"
	KindSessionEnded       Kind = "session_ended"
)

// Kinds lists every event kind, in a fixed order.
func Kinds() []Kind {
	return []Kind{
		KindSessionOpened, KindSessionRefreshed, KindTurnStarted, KindToolStarted,
		KindToolFinished, KindAttentionNeeded, KindIdle, KindTurnFinished,
		KindCompactionStarted, KindCompactionFinished, KindModelChanged,
		KindSubagentStarted, KindSubagentStopped, KindSessionEnded,
	}
}

// Valid reports whether k is a known event kind.
func (k Kind) Valid() bool {
	_, ok := transitions[k]
	return ok
}

// Event is one thing that happened in one session. Every event carries the
// session id, surface and time, so that any event can create its session.
//
// The remaining fields are optional and belong to particular kinds:
//
//   - Tool: required on tool started.
//   - Model: required on model changed; optional on session opened and
//     session refreshed.
//   - Project and Privacy: optional on session opened and session refreshed.
//
// A field set on a kind that does not carry it is validated and then ignored.
type Event struct {
	SessionID string
	Surface   Surface
	At        time.Time
	Kind      Kind

	Tool    ToolKind
	Model   string
	Project string
	Privacy Privacy
}

// Validate reports whether the event may be applied. The error names the
// field at fault and never contains a field's value.
func (e Event) Validate() error {
	reason := ""
	switch {
	case !e.Kind.Valid():
		reason = "unknown kind"
	case e.At.IsZero():
		reason = "time is not set"
	case e.Tool != ToolNone && !e.Tool.Valid():
		reason = "unknown tool kind"
	case e.Privacy != "" && !e.Privacy.Valid():
		reason = "unknown privacy level"
	case e.Kind == KindToolStarted && e.Tool == ToolNone:
		reason = "tool started without a tool kind"
	case e.Kind == KindModelChanged && e.Model == "":
		reason = "model changed without a model"
	default:
		reason = checkCommon(e.SessionID, e.Surface, e.Model, e.Project)
	}
	if reason != "" {
		return fmt.Errorf("%w: %s", ErrInvalidEvent, reason)
	}
	return nil
}

// checkCommon checks the fields events and sessions share. It returns what is
// wrong, or the empty string.
//
// The model and the project are text that reaches a Discord text line, and
// they may come from another process, so each must already be as its adapter
// would have made it.
func checkCommon(id string, surface Surface, model, project string) string {
	switch {
	case id == "":
		return "session id is empty"
	case len(id) > MaxIDLen:
		return "session id is too long"
	case !surface.Valid():
		return "unknown surface"
	case len(model) > MaxModelLen:
		return "model is too long"
	case len(project) > MaxProjectLen:
		return "project is too long"
	case !modelLabel(model):
		return "model is not a model label"
	case CleanName(project) != project:
		return "project is not clean"
	}
	return ""
}
