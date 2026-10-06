package code

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/mcp"
)

// maxPauseMinutes is the longest pause with a duration: one week. A longer
// one is a pause until resumed.
const maxPauseMinutes = 7 * 24 * 60

const pauseDescription = "Pauses or resumes Discord Rich Presence for every open session, without changing any setting. Call it only when the user asks. With no arguments it pauses until resumed. With minutes it pauses for that long and presence returns by itself. With resume set to true it ends a pause. It cannot change what is shown, a privacy level or a project profile."

const pauseSchema = `{"type":"object","properties":{"minutes":{"type":"integer","minimum":1,"maximum":10080,"description":"How long to pause for. Leave out to pause until resumed."},"resume":{"type":"boolean","description":"End the pause."}}}`

// The pause tool's answers that do not depend on its arguments.
const (
	pauseOff         = "Presence is off, so there is nothing to pause."
	pauseOldHost     = "Pausing is unavailable: the session that holds the Discord connection runs an older version. Restart your other sessions so that they update, then try again."
	pauseBadMinutes  = "Nothing was changed: minutes must be a whole number from 1 to 10080."
	pauseUnavailable = "Pausing is unavailable."
	pauseResumed     = "Presence is resumed for every session."
	pauseIndefinite  = "Presence is paused for every session until it is resumed."
)

// Pauser switches the whole presence off and on again. It can do nothing
// else: it has no way to change what is shown, a privacy level or a profile
// (ADR-0012). Both methods are called from a tool call that Claude waits on,
// so they must return at once and must not perform I/O (ADR-0008). Each
// reports false when the presence host cannot pause.
type Pauser interface {
	// Pause switches presence off until the time until, or until Resume when
	// until is the zero time.
	Pause(until time.Time) bool
	Resume() bool
}

// handlePause is the pause tool, which the model calls when the user asks.
// It reads a number of minutes and a flag, and nothing else of its
// arguments. All it can do with them is call the Pauser.
func (a *Adapter) handlePause(arguments json.RawMessage) (result mcp.Result) {
	defer func() {
		if recover() != nil {
			result = mcp.Result{Text: pauseUnavailable, IsError: true}
		}
	}()
	if a.pauser == nil {
		return mcp.Result{Text: pauseOff}
	}
	// The minutes are read as any JSON number, because a client may write a
	// whole number as 30.0.
	var asked struct {
		Minutes float64 `json:"minutes"`
		Resume  bool    `json:"resume"`
	}
	// No arguments at all ask for a pause until resumed.
	if len(arguments) > 0 && json.Unmarshal(arguments, &asked) != nil {
		return mcp.Result{Text: pauseBadMinutes, IsError: true}
	}
	// The comparison is written so that it also refuses what is not a number.
	minutes := int64(0)
	if asked.Minutes >= 0 && asked.Minutes <= maxPauseMinutes {
		minutes = int64(asked.Minutes)
	}
	if float64(minutes) != asked.Minutes {
		return mcp.Result{Text: pauseBadMinutes, IsError: true}
	}
	var done bool
	text := pauseIndefinite
	switch {
	case asked.Resume:
		done, text = a.pauser.Resume(), pauseResumed
	case minutes == 0:
		done = a.pauser.Pause(time.Time{})
	default:
		done = a.pauser.Pause(a.clock.Now().Add(time.Duration(minutes) * time.Minute))
		text = "Presence is paused for every session for " + strconv.FormatInt(minutes, 10) + " minutes. It returns by itself."
	}
	if !done {
		return mcp.Result{Text: pauseOldHost, IsError: true}
	}
	return mcp.Result{Text: text}
}
