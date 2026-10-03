package fakediscord

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

// The server's decoder reads bytes from outside the process, so it is fuzzed:
// whatever arrives, it returns a frame within the limit or an error, and
// classifying the frame never panics.
func FuzzReadFrame(f *testing.F) {
	f.Add(frame(opHandshake, []byte(`{"v":1,"client_id":"1234567890"}`)))
	f.Add(frame(opFrame, []byte(`{"cmd":"SET_ACTIVITY","args":{"pid":1,"activity":{"details":"ab"}},"nonce":"n"}`)))
	f.Add(frame(opFrame, []byte(`{"cmd":"SET_ACTIVITY","args":{"pid":1,"activity":null},"nonce":7}`)))
	f.Add(frame(opClose, []byte(`{"code":1000,"message":"bye"}`)))
	f.Add(frame(opPing, nil))
	f.Add(frame(opPong, []byte(`[]`)))
	f.Add(frame(99, []byte(`{}`)))
	f.Add([]byte{1, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF})
	f.Add([]byte{1, 0, 0})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		opcode, payload, err := readFrame(bytes.NewReader(data))
		switch {
		case errors.Is(err, errOversize):
			if payload != nil {
				t.Fatalf("an oversized frame came with %d bytes of payload", len(payload))
			}
			return
		case err != nil:
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("unexpected error %v", err)
			}
			return
		}
		if len(payload) > MaxFrameSize-headerSize {
			t.Fatalf("read a payload of %d bytes, over the limit", len(payload))
		}
		if !bytes.Equal(frame(opcode, payload), data[:headerSize+len(payload)]) {
			t.Fatalf("frame does not encode back to the bytes it was read from")
		}

		ev, nonce := classify(opcode, payload)
		if ev.Kind < KindHandshake || ev.Kind > KindInvalid {
			t.Fatalf("classified as %v", ev.Kind)
		}
		if ev.Opcode != opcode || !bytes.Equal(ev.Payload, payload) {
			t.Fatalf("event does not carry the frame it came from")
		}
		if (ev.Activity != nil) != (ev.Kind == KindSetActivity) {
			t.Fatalf("%v event with activity %q", ev.Kind, ev.Activity)
		}
		// Every answer the server can give to it must itself be a frame: a
		// header whose length is that of the JSON that follows.
		for _, handshaken := range []bool{false, true} {
			for _, b := range []Behavior{{}, {RejectHandshake: 4000, ActivityError: 4000}} {
				answer, _ := reply(&conn{handshaken: handshaken}, ev, nonce, b)
				if answer == nil {
					continue
				}
				if len(answer) < headerSize || binary.LittleEndian.Uint32(answer[4:8]) != uint32(len(answer)-headerSize) {
					t.Fatalf("answer % x has the wrong length in its header", answer)
				}
				// A pong carries whatever the ping did.
				if ev.Kind != KindPing && !json.Valid(answer[headerSize:]) {
					t.Fatalf("answer payload %q is not JSON", answer[headerSize:])
				}
			}
		}
	})
}
