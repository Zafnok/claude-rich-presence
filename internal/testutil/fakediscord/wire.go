package fakediscord

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
)

// The wire format, from Discord's documentation: two little-endian unsigned
// 32-bit integers, the opcode and the payload length, then the payload.
const (
	opHandshake uint32 = 0
	opFrame     uint32 = 1
	opClose     uint32 = 2
	opPing      uint32 = 3
	opPong      uint32 = 4

	headerSize = 8

	// MaxFrameSize is the largest frame, header included, that the server
	// reads. It is the limit in Discord's own library.
	MaxFrameSize = 64 * 1024
)

// Close codes the server sends on its own initiative.
const (
	// CloseInvalidClientID and CloseInvalidVersion are from Discord's table of
	// RPC close event codes.
	CloseInvalidClientID = 4000
	CloseInvalidVersion  = 4004
	// CloseUnsupported is what the arrpc reimplementation sends for anything
	// it cannot handle. Discord does not document a code for that.
	CloseUnsupported = 1003
	// ErrorInvalidCommand is from Discord's table of RPC error codes.
	ErrorInvalidCommand = 4002
)

var errOversize = errors.New("fakediscord: frame over the size limit")

// frame builds a whole frame as one slice, so that it is written in one call.
func frame(opcode uint32, payload []byte) []byte {
	b := make([]byte, headerSize, headerSize+len(payload))
	binary.LittleEndian.PutUint32(b[0:4], opcode)
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(payload)))
	return append(b, payload...)
}

// readFrame reads one frame. For a declared length over the limit it returns
// the opcode and errOversize without reading the payload.
func readFrame(r io.Reader) (opcode uint32, payload []byte, err error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	opcode = binary.LittleEndian.Uint32(header[0:4])
	length := binary.LittleEndian.Uint32(header[4:8])
	if length > MaxFrameSize-headerSize {
		return opcode, nil, errOversize
	}
	payload = make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return opcode, payload, nil
}

type handshakePayload struct {
	V        int    `json:"v"`
	ClientID string `json:"client_id"`
}

type commandPayload struct {
	Cmd   string          `json:"cmd"`
	Nonce json.RawMessage `json:"nonce"`
	Args  struct {
		PID      int64           `json:"pid"`
		Activity json.RawMessage `json:"activity"`
	} `json:"args"`
}

// classify turns a frame into an event. The nonce is returned as it arrived,
// to be echoed byte for byte.
func classify(opcode uint32, payload []byte) (ev Event, nonce json.RawMessage) {
	ev = Event{Kind: KindInvalid, Opcode: opcode, Payload: payload}
	switch opcode {
	case opHandshake:
		var h handshakePayload
		if json.Unmarshal(payload, &h) != nil {
			return ev, nil
		}
		ev.Kind, ev.Version, ev.ClientID = KindHandshake, h.V, h.ClientID
	case opFrame:
		var c commandPayload
		if json.Unmarshal(payload, &c) != nil {
			return ev, nil
		}
		nonce = c.Nonce
		// A nonce that is not a string is still echoed; it just has no
		// string form to record.
		_ = json.Unmarshal(c.Nonce, &ev.Nonce)
		ev.Command, ev.PID = c.Cmd, c.Args.PID
		switch {
		case ev.Command != "SET_ACTIVITY":
			ev.Kind = KindCommand
		case len(c.Args.Activity) == 0 || bytes.Equal(c.Args.Activity, []byte("null")):
			ev.Kind = KindClearActivity
		default:
			ev.Kind, ev.Activity = KindSetActivity, c.Args.Activity
		}
	case opClose:
		ev.Kind = KindClose
	case opPing:
		ev.Kind = KindPing
	case opPong:
		ev.Kind = KindPong
	}
	return ev, nonce
}

// mustJSON encodes a value built from this package's own types, which always
// encode.
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func closePayload(code int, message string) []byte {
	return mustJSON(map[string]any{"code": code, "message": message})
}

// readyPayload is the dispatch Discord sends after a good handshake. The
// shape is from Discord's documentation; the user is plainly not a real one.
func readyPayload() []byte {
	return mustJSON(map[string]any{
		"cmd": "DISPATCH",
		"evt": "READY",
		"data": map[string]any{
			"v": 1,
			"config": map[string]any{
				"cdn_host":     "cdn.fakediscord.invalid",
				"api_endpoint": "//fakediscord.invalid/api",
				"environment":  "production",
			},
			"user": map[string]any{
				"id":            "0",
				"username":      "fakediscord",
				"discriminator": "0",
				"avatar":        nil,
			},
		},
		"nonce": nil,
	})
}

func nonceOrNull(nonce json.RawMessage) json.RawMessage {
	if len(nonce) == 0 {
		return json.RawMessage("null")
	}
	return nonce
}

func ackPayload(ev Event, nonce json.RawMessage) []byte {
	data := json.RawMessage("null")
	if ev.Activity != nil {
		data = ev.Activity
	}
	return mustJSON(map[string]any{
		"cmd":   ev.Command,
		"data":  data,
		"evt":   nil,
		"nonce": nonceOrNull(nonce),
	})
}

func errorPayload(command string, nonce json.RawMessage, code int, message string) []byte {
	return mustJSON(map[string]any{
		"cmd":   command,
		"data":  map[string]any{"code": code, "message": message},
		"evt":   "ERROR",
		"nonce": nonceOrNull(nonce),
	})
}
