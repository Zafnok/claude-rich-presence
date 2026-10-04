package session

import (
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/diag"
)

// State is where the manager is in the life of the Discord connection.
type State int

// The states. A manager starts disconnected and ends stopped.
const (
	// Disconnected is waiting to try again.
	Disconnected State = iota
	// Connecting is dialling, or waiting for Discord to answer the handshake.
	Connecting
	// Ready is connected: activities are sent.
	Ready
	// Stopped is after Run has returned.
	Stopped
)

var stateNames = [...]string{
	Disconnected: "disconnected",
	Connecting:   "connecting",
	Ready:        "ready",
	Stopped:      "stopped",
}

var stateAttrs = [...]diag.Attr{
	Disconnected: diag.State("state", "disconnected"),
	Connecting:   diag.State("state", "connecting"),
	Ready:        diag.State("state", "ready"),
	Stopped:      diag.State("state", "stopped"),
}

func (s State) String() string { return stateNames[s] }

// The classes of error a Status reports. They are names, never an error's own
// text.
const (
	// ErrorNotRunning is no Discord client listening.
	ErrorNotRunning = "discord_not_running"
	// ErrorDial is any other failure to open the connection.
	ErrorDial = "dial_failed"
	// ErrorHandshakeTimeout is a handshake Discord did not answer in time.
	ErrorHandshakeTimeout = "handshake_timeout"
	// ErrorInvalidApplicationID is Discord refusing the application id.
	ErrorInvalidApplicationID = "invalid_application_id"
	// ErrorClosedByDiscord is a close frame for any other reason.
	ErrorClosedByDiscord = "closed_by_discord"
	// ErrorConnectionLost is the stream ending or failing to read.
	ErrorConnectionLost = "connection_lost"
	// ErrorWrite is a write that failed or did not finish in time.
	ErrorWrite = "write_failed"
	// ErrorProtocol is a frame from Discord that could not be understood.
	ErrorProtocol = "protocol_error"
	// ErrorActivityRejected is Discord answering an activity with an error.
	// The connection stays up.
	ErrorActivityRejected = "activity_rejected"
)

// failure is why a connection attempt or a connection ended: the class as
// Status reports it and as the log does.
type failure struct {
	class string
	attr  diag.Attr
}

var (
	failNotRunning       = failure{ErrorNotRunning, diag.ErrorClass(ErrorNotRunning)}
	failDial             = failure{ErrorDial, diag.ErrorClass(ErrorDial)}
	failHandshakeTimeout = failure{ErrorHandshakeTimeout, diag.ErrorClass(ErrorHandshakeTimeout)}
	failInvalidID        = failure{ErrorInvalidApplicationID, diag.ErrorClass(ErrorInvalidApplicationID)}
	failClosed           = failure{ErrorClosedByDiscord, diag.ErrorClass(ErrorClosedByDiscord)}
	failLost             = failure{ErrorConnectionLost, diag.ErrorClass(ErrorConnectionLost)}
	failWrite            = failure{ErrorWrite, diag.ErrorClass(ErrorWrite)}
	failProtocol         = failure{ErrorProtocol, diag.ErrorClass(ErrorProtocol)}
	failActivity         = failure{ErrorActivityRejected, diag.ErrorClass(ErrorActivityRejected)}
)

// Status is a snapshot for diagnostics.
type Status struct {
	State State
	// LastError is the class of the most recent error, one of the Error
	// constants, or empty if there has been none. It is not cleared by a
	// later success.
	LastError string
	// LastUpdate is when Discord last acknowledged an activity or a clear.
	// It is zero if it never has.
	LastUpdate time.Time
}
