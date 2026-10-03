package codec

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// ErrMalformed is a payload that is not the JSON its opcode calls for.
var ErrMalformed = errors.New("codec: malformed payload")

const (
	protocolVersion = 1
	cmdSetActivity  = "SET_ACTIVITY"
	cmdDispatch     = "DISPATCH"
	evtReady        = "READY"
	evtError        = "ERROR"
)

// Discord's activity type numbers, indexed by domain.ActivityType. Only these
// four are accepted over this interface.
var activityTypes = [...]int{
	domain.ActivityPlaying:   0,
	domain.ActivityListening: 2,
	domain.ActivityWatching:  3,
	domain.ActivityCompeting: 5,
}

type handshake struct {
	V        int    `json:"v"`
	ClientID string `json:"client_id"`
}

type command struct {
	Cmd   string `json:"cmd"`
	Args  args   `json:"args"`
	Nonce string `json:"nonce"`
}

type args struct {
	PID      int       `json:"pid"`
	Activity *activity `json:"activity"`
}

type activity struct {
	State      string      `json:"state,omitempty"`
	Details    string      `json:"details,omitempty"`
	Timestamps *timestamps `json:"timestamps,omitempty"`
	Assets     *assets     `json:"assets,omitempty"`
	Type       int         `json:"type,omitempty"`
}

type timestamps struct {
	Start int64 `json:"start"`
}

type assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

// marshal encodes one of the types above. Encoding them cannot fail: they hold
// only strings and integers. Text is not escaped for HTML.
func marshal(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// Handshake is the first frame a client sends.
func Handshake(applicationID string) Frame {
	return Frame{Op: OpHandshake, Payload: marshal(handshake{V: protocolVersion, ClientID: applicationID})}
}

// SetActivity is the command that shows a, or clears the activity when a is
// nil. It takes one nonce from the generator and returns it, so that the
// caller can match the acknowledgement.
func SetActivity(pid int, nonce func() string, a *domain.Activity) (Frame, string) {
	c := command{Cmd: cmdSetActivity, Args: args{PID: pid, Activity: wireActivity(a)}, Nonce: nonce()}
	return Frame{Op: OpFrame, Payload: marshal(c)}, c.Nonce
}

// Pong answers a ping, carrying the ping's payload back.
func Pong(pingPayload []byte) Frame {
	return Frame{Op: OpPong, Payload: pingPayload}
}

func wireActivity(a *domain.Activity) *activity {
	if a == nil {
		return nil
	}
	w := &activity{State: a.State, Details: a.Details}
	if !a.Start.IsZero() {
		w.Timestamps = &timestamps{Start: a.Start.Unix()}
	}
	if as := (assets{a.LargeImage, a.LargeText, a.SmallImage, a.SmallText}); as != (assets{}) {
		w.Assets = &as
	}
	if a.Type >= 0 && int(a.Type) < len(activityTypes) {
		w.Type = activityTypes[a.Type]
	}
	return w
}

// Kind says what a decoded frame is.
type Kind int

// The kinds of message. KindUnknown is anything well formed that this package
// does not use: an unknown opcode, another event, the reply to another
// command.
const (
	KindUnknown Kind = iota
	KindReady
	KindError
	KindAck
	KindClose
	KindPing
)

// Message is a decoded frame.
type Message struct {
	Kind Kind
	// Nonce is that of the command an acknowledgement or an error answers.
	Nonce string
	// Code and Text are Discord's code and message, in an error and a close.
	Code int
	Text string
	// Payload is the body of a ping, to send back with Pong.
	Payload []byte
}

type envelope struct {
	Cmd   string          `json:"cmd"`
	Evt   string          `json:"evt"`
	Nonce string          `json:"nonce"`
	Data  json.RawMessage `json:"data"`
}

type status struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Decode classifies f. Unknown opcodes, events, commands and JSON fields are
// tolerated. A payload that is not the JSON its opcode calls for returns
// ErrMalformed.
func Decode(f Frame) (Message, error) {
	switch f.Op {
	case OpPing:
		return Message{Kind: KindPing, Payload: f.Payload}, nil
	case OpClose:
		var s status
		if json.Unmarshal(f.Payload, &s) != nil {
			return Message{}, ErrMalformed
		}
		return Message{Kind: KindClose, Code: s.Code, Text: s.Message}, nil
	case OpFrame:
		return decodeEnvelope(f.Payload)
	}
	return Message{}, nil
}

func decodeEnvelope(payload []byte) (Message, error) {
	var e envelope
	if json.Unmarshal(payload, &e) != nil {
		return Message{}, ErrMalformed
	}
	switch {
	case e.Evt == evtError:
		var s status
		if len(e.Data) > 0 && json.Unmarshal(e.Data, &s) != nil {
			return Message{}, ErrMalformed
		}
		return Message{Kind: KindError, Nonce: e.Nonce, Code: s.Code, Text: s.Message}, nil
	case e.Cmd == cmdDispatch && e.Evt == evtReady:
		return Message{Kind: KindReady}, nil
	case e.Cmd == cmdSetActivity && e.Evt == "":
		return Message{Kind: KindAck, Nonce: e.Nonce}, nil
	}
	return Message{}, nil
}
