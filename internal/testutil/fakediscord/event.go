package fakediscord

import (
	"encoding/json"
	"time"
)

// Kind says what a recorded event was.
type Kind int

const (
	// KindHandshake is a handshake frame that parsed, whether or not it was
	// accepted.
	KindHandshake Kind = iota + 1
	// KindSetActivity is a SET_ACTIVITY command with an activity.
	KindSetActivity
	// KindClearActivity is a SET_ACTIVITY command with a null or absent
	// activity.
	KindClearActivity
	// KindCommand is a frame carrying any other command.
	KindCommand
	// KindPing is a ping frame.
	KindPing
	// KindPong is a pong frame.
	KindPong
	// KindClose is a close frame.
	KindClose
	// KindInvalid is a frame the server could not understand: an unknown
	// opcode, a payload that is not the expected JSON, or a declared length
	// over the limit.
	KindInvalid
	// KindDisconnect is the end of a connection, whichever side ended it. It
	// has no opcode and no payload.
	KindDisconnect
)

var kindNames = map[Kind]string{
	KindHandshake:     "handshake",
	KindSetActivity:   "set-activity",
	KindClearActivity: "clear-activity",
	KindCommand:       "command",
	KindPing:          "ping",
	KindPong:          "pong",
	KindClose:         "close",
	KindInvalid:       "invalid",
	KindDisconnect:    "disconnect",
}

func (k Kind) String() string {
	if name, ok := kindNames[k]; ok {
		return name
	}
	return "unknown"
}

// Event is one thing the server received, in the order it was received.
type Event struct {
	Kind Kind
	// Time is what the test's time function returned when the event was
	// recorded.
	Time time.Time
	// Conn numbers the connection the event arrived on, from 1, in the order
	// the server accepted them.
	Conn int
	// Opcode and Payload are the frame as it arrived. Payload is empty for an
	// oversized frame, which is never read.
	Opcode  uint32
	Payload []byte

	// Version and ClientID are set for a handshake.
	Version  int
	ClientID string

	// Command, Nonce, PID and Activity are set for a frame carrying a command.
	// Nonce is empty if the nonce was not a string. Activity is the raw JSON,
	// and is nil for a clear.
	Command  string
	Nonce    string
	PID      int64
	Activity json.RawMessage
}
