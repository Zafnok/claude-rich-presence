package codec_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Zafnok/claude-rich-presence/internal/discord/codec"
)

var errStream = errors.New("stream failed")

// recorder keeps each Write as its own entry.
type recorder struct {
	writes [][]byte
	err    error
}

func (r *recorder) Write(p []byte) (int, error) {
	r.writes = append(r.writes, bytes.Clone(p))
	return len(p), r.err
}

func header(op, length uint32) []byte {
	return []byte{
		byte(op), byte(op >> 8), byte(op >> 16), byte(op >> 24),
		byte(length), byte(length >> 8), byte(length >> 16), byte(length >> 24),
	}
}

func TestWriteFrame(t *testing.T) {
	tests := []struct {
		name  string
		frame codec.Frame
		want  string
	}{
		{"handshake", codec.Frame{Op: codec.OpHandshake, Payload: []byte(fixtureHandshake[8:])}, fixtureHandshake},
		{"empty payload", codec.Frame{Op: codec.OpPong}, "\x04\x00\x00\x00\x00\x00\x00\x00"},
		{"wide opcode", codec.Frame{Op: 0x04030201, Payload: []byte("a")}, "\x01\x02\x03\x04\x01\x00\x00\x00a"},
		{
			"largest payload",
			codec.Frame{Op: codec.OpFrame, Payload: bytes.Repeat([]byte("x"), codec.MaxPayload)},
			"\x01\x00\x00\x00\x00\x00\x01\x00" + strings.Repeat("x", codec.MaxPayload),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w recorder
			if err := codec.WriteFrame(&w, tt.frame); err != nil {
				t.Fatalf("WriteFrame: %v", err)
			}
			if len(w.writes) != 1 {
				t.Fatalf("got %d writes, want 1", len(w.writes))
			}
			if got := string(w.writes[0]); got != tt.want {
				t.Errorf("wrote %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteFrameTooLarge(t *testing.T) {
	var w recorder
	err := codec.WriteFrame(&w, codec.Frame{Op: codec.OpFrame, Payload: make([]byte, codec.MaxPayload+1)})
	if !errors.Is(err, codec.ErrTooLarge) {
		t.Errorf("got %v, want ErrTooLarge", err)
	}
	if len(w.writes) != 0 {
		t.Errorf("got %d writes, want none", len(w.writes))
	}
}

func TestWriteFrameWriterError(t *testing.T) {
	w := recorder{err: errStream}
	if err := codec.WriteFrame(&w, codec.Frame{Op: codec.OpPong}); !errors.Is(err, errStream) {
		t.Errorf("got %v, want the writer's error", err)
	}
}

func TestReadFrameSplitAndJoined(t *testing.T) {
	stream := fixtureHandshake + fixtureSetActivity + "\x04\x00\x00\x00\x00\x00\x00\x00" + "\x63\x00\x00\x00\x02\x00\x00\x00{}"
	want := []codec.Frame{
		{Op: codec.OpHandshake, Payload: []byte(fixtureHandshake[8:])},
		{Op: codec.OpFrame, Payload: []byte(fixtureSetActivity[8:])},
		{Op: codec.OpPong, Payload: []byte{}},
		{Op: 99, Payload: []byte("{}")},
	}
	readers := map[string]io.Reader{
		"all frames in one read": strings.NewReader(stream),
		"one byte at a time":     iotest.OneByteReader(strings.NewReader(stream)),
		"data with the error":    iotest.DataErrReader(strings.NewReader(stream)),
	}
	for name, r := range readers {
		t.Run(name, func(t *testing.T) {
			for i, w := range want {
				got, err := codec.ReadFrame(r)
				if err != nil {
					t.Fatalf("frame %d: %v", i, err)
				}
				if got.Op != w.Op || !bytes.Equal(got.Payload, w.Payload) {
					t.Errorf("frame %d: got %d %q, want %d %q", i, got.Op, got.Payload, w.Op, w.Payload)
				}
			}
			if _, err := codec.ReadFrame(r); err != io.EOF {
				t.Errorf("after the last frame: got %v, want io.EOF", err)
			}
		})
	}
}

func TestReadFrameLargestPayload(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), codec.MaxPayload)
	got, err := codec.ReadFrame(io.MultiReader(bytes.NewReader(header(1, codec.MaxPayload)), bytes.NewReader(payload)))
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !bytes.Equal(got.Payload, payload) {
		t.Errorf("payload of %d bytes differs", len(got.Payload))
	}
}

func TestReadFrameTooLarge(t *testing.T) {
	for _, length := range []uint32{codec.MaxPayload + 1, 0xFFFFFFFF} {
		r := bytes.NewReader(append(header(1, length), "payload"...))
		if _, err := codec.ReadFrame(r); !errors.Is(err, codec.ErrTooLarge) {
			t.Errorf("length %d: got %v, want ErrTooLarge", length, err)
		}
		if r.Len() != len("payload") {
			t.Errorf("length %d: %d bytes left unread, want the whole payload", length, r.Len())
		}
	}
}

func TestReadFrameErrors(t *testing.T) {
	full := fixtureHandshake
	tests := []struct {
		name string
		r    io.Reader
		want error
	}{
		{"empty stream", strings.NewReader(""), io.EOF},
		{"one header byte", strings.NewReader(full[:1]), codec.ErrTruncatedHeader},
		{"seven header bytes", strings.NewReader(full[:7]), codec.ErrTruncatedHeader},
		{"header only", strings.NewReader(full[:8]), codec.ErrTruncatedPayload},
		{"part of the payload", strings.NewReader(full[:20]), codec.ErrTruncatedPayload},
		{"one byte short", strings.NewReader(full[:len(full)-1]), codec.ErrTruncatedPayload},
		{"failure before the header", iotest.ErrReader(errStream), errStream},
		{"failure inside the header", io.MultiReader(strings.NewReader(full[:3]), iotest.ErrReader(errStream)), errStream},
		{"failure inside the payload", io.MultiReader(strings.NewReader(full[:20]), iotest.ErrReader(errStream)), errStream},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := codec.ReadFrame(tt.r)
			if !errors.Is(err, tt.want) {
				t.Errorf("got %v, want %v", err, tt.want)
			}
			if f.Op != 0 || f.Payload != nil {
				t.Errorf("got frame %v with an error, want the zero frame", f)
			}
		})
	}
}

func TestFrameErrorsAreDistinct(t *testing.T) {
	errs := []error{io.EOF, codec.ErrTooLarge, codec.ErrTruncatedHeader, codec.ErrTruncatedPayload, codec.ErrMalformed}
	for i, a := range errs {
		for j, b := range errs {
			if i != j && errors.Is(a, b) {
				t.Errorf("%v matches %v", a, b)
			}
		}
	}
}
