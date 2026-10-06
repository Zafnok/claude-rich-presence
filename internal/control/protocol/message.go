package protocol

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// The protocol versions this binary speaks. A change an older host cannot
// ignore increments Version.
const (
	MinVersion = 1
	Version    = 1
)

// Limits on what a line and its fields may hold, in bytes.
const (
	// MaxLineBytes is the longest line, not counting its newline.
	MaxLineBytes = 64 * 1024
	// MaxVersionLen bounds a binary version.
	MaxVersionLen = 64
	// MaxWordLen bounds a word from a closed vocabulary, such as an event
	// kind. The words themselves are judged by the domain, not here.
	MaxWordLen = 32
	// MaxCardTextLen bounds each piece of text in a preview. It is Discord's
	// longest limit on any of them, that of a button's URL.
	MaxCardTextLen = domain.MaxLinkLen
)

// The errors a message can fail with. Each is wrapped with the name of the
// field at fault and never with a field's value, so they are safe to log.
var (
	ErrLineTooLong  = errors.New("control line is too long")
	ErrMalformed    = errors.New("control line is not a JSON object of the expected shape")
	ErrMissingField = errors.New("control message is missing a required field")
	ErrInvalidValue = errors.New("control message has an invalid value")
)

// The message types, as they appear in the "type" field.
const (
	TypeHello         = "hello"
	TypeWelcome       = "welcome"
	TypeRefuse        = "refuse"
	TypeSync          = "sync"
	TypeEvent         = "event"
	TypeStandDown     = "stand_down"
	TypeStatus        = "status"
	TypeStatusResult  = "status_result"
	TypePause         = "pause"
	TypeResume        = "resume"
	TypePreview       = "preview"
	TypePreviewResult = "preview_result"
)

// Message is one line on the control channel. The set is closed: the types
// in this file are the only implementations.
type Message interface {
	// Type is the message's "type" on the wire. It is empty for Unknown.
	Type() string
	// wire validates the message and returns the value to encode.
	wire() (any, error)
}

// Hello is the first message from a follower.
type Hello struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
}

// Welcome is the host's answer to a hello it accepts. Protocol is the highest
// version the host speaks; the connection speaks the version of the hello.
type Welcome struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
}

// Refuse is the host's answer to a hello it does not accept. Whatever the
// reason, the follower retries the election later.
type Refuse struct {
	Reason Reason `json:"reason"`
}

// Sync carries the follower's complete current session. A nil Session means
// the follower has none. It is idempotent.
type Sync struct {
	Session *SessionState `json:"session"`
}

// Event carries one presence event.
type Event struct {
	Event EventData `json:"event"`
}

// StandDown asks the host to give up the lock.
type StandDown struct{}

// Status asks the host for a StatusResult.
type Status struct{}

// StatusResult is the host's summary of itself. It holds nothing about any
// session, so it is safe to paste in a public issue.
type StatusResult struct {
	Discord       DiscordState `json:"discord"`
	Sessions      int          `json:"sessions"`
	Version       string       `json:"version"`
	UptimeSeconds int64        `json:"uptime_seconds"`
	// Pause is the pause the host holds. A host that can pause always sends
	// it, so nil means a host from before pausing.
	Pause *PauseState `json:"pause,omitempty"`
}

// Pause asks the host to show nothing: until the time Until, or until a
// resume when Until is zero. It can only take presence away.
type Pause struct {
	// Until and At are in Unix milliseconds.
	Until int64 `json:"until,omitempty"`
	// At is when the user asked. Of two requests the host keeps the later.
	At int64 `json:"at"`
}

// Resume ends a pause.
type Resume struct {
	// At is when the user asked, as in Pause.
	At int64 `json:"at"`
}

// PauseState is the latest pause or resume the host was given.
type PauseState struct {
	// Paused says the latest request was a pause. One with an Until that has
	// passed has ended.
	Paused bool `json:"paused"`
	// Until is when the pause ends, or zero for a pause until resumed.
	Until int64 `json:"until,omitempty"`
	// At is when the user asked, or zero if nothing was ever asked.
	At int64 `json:"at,omitempty"`
}

// Preview asks the host for a PreviewResult.
type Preview struct{}

// PreviewResult is the card as the host last handed it to Discord. Unlike a
// StatusResult it can name a project, so it is for the user's own eyes and
// is never logged.
type PreviewResult struct {
	// Shown is false when the host shows nothing. The rest is then empty.
	Shown   bool   `json:"shown"`
	Details string `json:"details,omitempty"`
	State   string `json:"state,omitempty"`
	// Start is the beginning of the elapsed timer, in Unix milliseconds.
	Start       int64  `json:"start,omitempty"`
	LargeImage  string `json:"large_image,omitempty"`
	LargeText   string `json:"large_text,omitempty"`
	SmallImage  string `json:"small_image,omitempty"`
	SmallText   string `json:"small_text,omitempty"`
	ButtonLabel string `json:"button_label,omitempty"`
	ButtonURL   string `json:"button_url,omitempty"`
}

// Unknown is a message whose type this binary does not know. It is to be
// ignored. It keeps nothing of the line it came from and cannot be encoded.
type Unknown struct{}

// Reason says why a host refused a hello. The vocabulary is closed: a reason
// this binary does not know decodes to ReasonOther.
type Reason string

// The refusal reasons.
const (
	ReasonUnsupportedProtocol Reason = "unsupported_protocol"
	ReasonStandingDown        Reason = "standing_down"
	ReasonOther               Reason = "other"
)

// Valid reports whether r is a known reason.
func (r Reason) Valid() bool {
	return r == ReasonUnsupportedProtocol || r == ReasonStandingDown || r == ReasonOther
}

// UnmarshalJSON reads a reason, turning one it does not know into ReasonOther.
func (r *Reason) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*r = Reason(s)
	if s != "" && !r.Valid() {
		*r = ReasonOther
	}
	return nil
}

// DiscordState is the state of the host's connection to Discord. The
// vocabulary is closed: a state this binary does not know decodes to
// DiscordUnknown.
type DiscordState string

// The Discord connection states.
const (
	DiscordConnected    DiscordState = "connected"
	DiscordConnecting   DiscordState = "connecting"
	DiscordDisconnected DiscordState = "disconnected"
	DiscordUnknown      DiscordState = "unknown"
)

// Valid reports whether d is a known state.
func (d DiscordState) Valid() bool {
	return d == DiscordConnected || d == DiscordConnecting || d == DiscordDisconnected || d == DiscordUnknown
}

// UnmarshalJSON reads a state, turning one it does not know into
// DiscordUnknown.
func (d *DiscordState) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*d = DiscordState(s)
	if s != "" && !d.Valid() {
		*d = DiscordUnknown
	}
	return nil
}

// Type implements Message.
func (Hello) Type() string { return TypeHello }

// Type implements Message.
func (Welcome) Type() string { return TypeWelcome }

// Type implements Message.
func (Refuse) Type() string { return TypeRefuse }

// Type implements Message.
func (Sync) Type() string { return TypeSync }

// Type implements Message.
func (Event) Type() string { return TypeEvent }

// Type implements Message.
func (StandDown) Type() string { return TypeStandDown }

// Type implements Message.
func (Status) Type() string { return TypeStatus }

// Type implements Message.
func (StatusResult) Type() string { return TypeStatusResult }

// Type implements Message.
func (Pause) Type() string { return TypePause }

// Type implements Message.
func (Resume) Type() string { return TypeResume }

// Type implements Message.
func (Preview) Type() string { return TypePreview }

// Type implements Message.
func (PreviewResult) Type() string { return TypePreviewResult }

// Type implements Message.
func (Unknown) Type() string { return "" }

// typed is the "type" field every encoded message starts with.
type typed struct {
	Type string `json:"type"`
}

func (m Hello) wire() (any, error) {
	return struct {
		typed
		Hello
	}{typed{TypeHello}, m}, m.check()
}

func (m Welcome) wire() (any, error) {
	return struct {
		typed
		Welcome
	}{typed{TypeWelcome}, m}, m.check()
}

func (m Refuse) wire() (any, error) {
	return struct {
		typed
		Refuse
	}{typed{TypeRefuse}, m}, m.check()
}

func (m Sync) wire() (any, error) {
	return struct {
		typed
		Sync
	}{typed{TypeSync}, m}, m.check()
}

func (m Event) wire() (any, error) {
	return struct {
		typed
		Event
	}{typed{TypeEvent}, m}, m.check()
}

func (m StandDown) wire() (any, error) { return typed{TypeStandDown}, m.check() }

func (m Status) wire() (any, error) { return typed{TypeStatus}, m.check() }

func (m StatusResult) wire() (any, error) {
	return struct {
		typed
		StatusResult
	}{typed{TypeStatusResult}, m}, m.check()
}

func (m Pause) wire() (any, error) {
	return struct {
		typed
		Pause
	}{typed{TypePause}, m}, m.check()
}

func (m Resume) wire() (any, error) {
	return struct {
		typed
		Resume
	}{typed{TypeResume}, m}, m.check()
}

func (m Preview) wire() (any, error) { return typed{TypePreview}, m.check() }

func (m PreviewResult) wire() (any, error) {
	return struct {
		typed
		PreviewResult
	}{typed{TypePreviewResult}, m}, m.check()
}

func (Unknown) wire() (any, error) {
	return nil, invalid("type")
}

func (m Hello) check() error   { return checkVersions("hello", m.Protocol, m.Version) }
func (m Welcome) check() error { return checkVersions("welcome", m.Protocol, m.Version) }

func (m Refuse) check() error {
	switch {
	case m.Reason == "":
		return missing("refuse.reason")
	case !m.Reason.Valid():
		return invalid("refuse.reason")
	}
	return nil
}

func (m Sync) check() error {
	if m.Session == nil {
		return nil
	}
	return m.Session.check()
}

func (m Event) check() error { return m.Event.check() }

func (StandDown) check() error { return nil }

func (Status) check() error { return nil }

func (m StatusResult) check() error {
	switch {
	case m.Discord == "":
		return missing("status_result.discord")
	case !m.Discord.Valid():
		return invalid("status_result.discord")
	case m.Sessions < 0:
		return invalid("status_result.sessions")
	case m.UptimeSeconds < 0:
		return invalid("status_result.uptime_seconds")
	case m.Pause != nil && m.Pause.Until < 0:
		return invalid("status_result.pause.until")
	case m.Pause != nil && m.Pause.At < 0:
		return invalid("status_result.pause.at")
	}
	return checkBinaryVersion("status_result.version", m.Version)
}

func (m Pause) check() error {
	if m.Until < 0 {
		return invalid("pause.until")
	}
	return checkTime("pause.at", m.At)
}

func (m Resume) check() error { return checkTime("resume.at", m.At) }

func (Preview) check() error { return nil }

func (m PreviewResult) check() error {
	start := error(nil)
	if m.Start < 0 {
		start = invalid("preview_result.start")
	}
	return firstError(
		checkText("preview_result.details", m.Details, false, MaxCardTextLen),
		checkText("preview_result.state", m.State, false, MaxCardTextLen),
		start,
		checkText("preview_result.large_image", m.LargeImage, false, MaxCardTextLen),
		checkText("preview_result.large_text", m.LargeText, false, MaxCardTextLen),
		checkText("preview_result.small_image", m.SmallImage, false, MaxCardTextLen),
		checkText("preview_result.small_text", m.SmallText, false, MaxCardTextLen),
		checkText("preview_result.button_label", m.ButtonLabel, false, MaxCardTextLen),
		checkText("preview_result.button_url", m.ButtonURL, false, MaxCardTextLen),
	)
}

// checkVersions checks the two fields hello and welcome share.
func checkVersions(message string, protocol int, version string) error {
	switch {
	case protocol == 0:
		return missing(message + ".protocol")
	case protocol < 0:
		return invalid(message + ".protocol")
	}
	return checkBinaryVersion(message+".version", version)
}

// checkBinaryVersion checks a binary version. The alphabet has no path
// separator and no space, so a version cannot smuggle a path or a sentence.
func checkBinaryVersion(field, v string) error {
	switch {
	case v == "":
		return missing(field)
	case len(v) > MaxVersionLen:
		return invalid(field)
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		if !letter && !digit && c != '.' && c != '-' && c != '+' && c != '_' && c != '(' && c != ')' {
			return invalid(field)
		}
	}
	return nil
}

// checkText checks a string field: present if required, and within its limit.
func checkText(field, v string, required bool, limit int) error {
	switch {
	case required && v == "":
		return missing(field)
	case len(v) > limit:
		return invalid(field)
	}
	return nil
}

// checkTime checks a required time in Unix milliseconds.
func checkTime(field string, ms int64) error {
	switch {
	case ms == 0:
		return missing(field)
	case ms < 0:
		return invalid(field)
	}
	return nil
}

func missing(field string) error { return fmt.Errorf("%w: %s", ErrMissingField, field) }

func invalid(field string) error { return fmt.Errorf("%w: %s", ErrInvalidValue, field) }

// Supported reports whether this binary speaks protocol version v.
func Supported(v int) bool {
	return v >= MinVersion && v <= Version
}

// Answer is the host's reply to a hello: a welcome if it speaks the
// follower's protocol version, otherwise a refusal. binaryVersion is the
// host's own.
func Answer(h Hello, binaryVersion string) Message {
	if !Supported(h.Protocol) {
		return Refuse{Reason: ReasonUnsupportedProtocol}
	}
	return Welcome{Protocol: Version, Version: binaryVersion}
}
