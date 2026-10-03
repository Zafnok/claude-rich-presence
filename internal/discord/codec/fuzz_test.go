package codec_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/discord/codec"
)

// FuzzReadFrame reads frames until the stream gives out. Whatever was read
// must write back as exactly the bytes consumed, and the stream must end with
// one of the frame layer's own errors.
func FuzzReadFrame(f *testing.F) {
	for _, seed := range []string{
		"",
		fixtureHandshake,
		fixtureSetActivity,
		fixtureClearActivity,
		fixtureHandshake + fixtureSetActivity,
		fixtureHandshake[:5],
		fixtureHandshake[:20],
		"\x01\x00\x00\x00" + "\x01\x00\x01\x00" + "x",
		"\x03\x00\x00\x00" + "\x0a\x00\x00\x00" + fixturePing,
		"\x02\x00\x00\x00" + "\x2b\x00\x00\x00" + fixtureClose,
		"\xff\xff\xff\xff" + "\xff\xff\xff\xff",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		var rewritten bytes.Buffer
		for {
			before := r.Len()
			frame, err := codec.ReadFrame(r)
			if err != nil {
				if err != io.EOF && !errors.Is(err, codec.ErrTooLarge) &&
					!errors.Is(err, codec.ErrTruncatedHeader) && !errors.Is(err, codec.ErrTruncatedPayload) {
					t.Fatalf("unexpected error %v", err)
				}
				if err == io.EOF && before != 0 {
					t.Fatalf("io.EOF with %d bytes left", before)
				}
				break
			}
			if len(frame.Payload) > codec.MaxPayload {
				t.Fatalf("payload of %d bytes", len(frame.Payload))
			}
			if err := codec.WriteFrame(&rewritten, frame); err != nil {
				t.Fatalf("WriteFrame of a frame that was read: %v", err)
			}
			if consumed := len(data) - r.Len(); !bytes.Equal(rewritten.Bytes(), data[:consumed]) {
				t.Fatalf("frames do not write back as the %d bytes consumed", consumed)
			}
		}
	})
}

// FuzzDecode covers every message decoder: the opcode chooses which one runs.
func FuzzDecode(f *testing.F) {
	seeds := []struct {
		op      codec.Opcode
		payload string
	}{
		{codec.OpHandshake, fixtureHandshake[8:]},
		{codec.OpFrame, fixtureSetActivity[8:]},
		{codec.OpFrame, fixtureClearActivity[8:]},
		{codec.OpFrame, fixtureReady},
		{codec.OpFrame, fixtureError},
		{codec.OpFrame, fixtureAck},
		{codec.OpFrame, `{"evt":"ERROR","data":"no"}`},
		{codec.OpFrame, ""},
		{codec.OpClose, fixtureClose},
		{codec.OpClose, `{"code":1e99}`},
		{codec.OpPing, fixturePing},
		{codec.OpPong, fixturePing},
		{99, "not json"},
	}
	for _, s := range seeds {
		f.Add(uint32(s.op), []byte(s.payload))
	}
	f.Fuzz(func(t *testing.T, op uint32, payload []byte) {
		m, err := codec.Decode(codec.Frame{Op: codec.Opcode(op), Payload: payload})
		if err != nil {
			if !errors.Is(err, codec.ErrMalformed) {
				t.Fatalf("unexpected error %v", err)
			}
			if m.Kind != codec.KindUnknown {
				t.Fatalf("kind %d returned with an error", m.Kind)
			}
			return
		}
		if m.Kind < codec.KindUnknown || m.Kind > codec.KindPing {
			t.Fatalf("kind %d is not defined", m.Kind)
		}
		if op > uint32(codec.OpPong) && m.Kind != codec.KindUnknown {
			t.Fatalf("unknown opcode %d decoded as kind %d", op, m.Kind)
		}
	})
}
