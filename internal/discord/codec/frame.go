package codec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Opcode is the first field of a frame header.
type Opcode uint32

// The opcodes Discord defines. A frame read from the stream may carry any
// other value; it is returned as it is and decodes as KindUnknown.
const (
	OpHandshake Opcode = 0
	OpFrame     Opcode = 1
	OpClose     Opcode = 2
	OpPing      Opcode = 3
	OpPong      Opcode = 4
)

// MaxPayload is the largest payload, in bytes, that is read or written.
const MaxPayload = 64 * 1024

const headerSize = 8

// The errors of the frame layer.
var (
	// ErrTooLarge is a payload, declared or given, longer than MaxPayload.
	ErrTooLarge = errors.New("codec: payload exceeds the maximum size")
	// ErrTruncatedHeader is a stream that ended inside a frame header.
	ErrTruncatedHeader = errors.New("codec: stream ended inside a frame header")
	// ErrTruncatedPayload is a stream that ended before the declared payload
	// length was read.
	ErrTruncatedPayload = errors.New("codec: stream ended inside a frame payload")
)

// Frame is one unit on the wire.
type Frame struct {
	Op      Opcode
	Payload []byte
}

// WriteFrame writes f to w in a single call to Write, because Discord expects
// the header and the payload to arrive together.
func WriteFrame(w io.Writer, f Frame) error {
	if len(f.Payload) > MaxPayload {
		return ErrTooLarge
	}
	buf := make([]byte, headerSize, headerSize+len(f.Payload))
	binary.LittleEndian.PutUint32(buf[0:4], uint32(f.Op))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(len(f.Payload)))
	_, err := w.Write(append(buf, f.Payload...))
	return err
}

// ReadFrame reads exactly one frame from r, however the bytes are split across
// reads, and consumes nothing past it. A stream that ends before the first
// byte of a header returns io.EOF. A declared length above MaxPayload returns
// ErrTooLarge before any of the payload is read.
func ReadFrame(r io.Reader) (Frame, error) {
	var header [headerSize]byte
	switch _, err := io.ReadFull(r, header[:]); err {
	case nil:
	case io.EOF:
		return Frame{}, io.EOF
	case io.ErrUnexpectedEOF:
		return Frame{}, ErrTruncatedHeader
	default:
		return Frame{}, fmt.Errorf("codec: read header: %w", err)
	}
	f := Frame{Op: Opcode(binary.LittleEndian.Uint32(header[0:4]))}
	length := binary.LittleEndian.Uint32(header[4:8])
	if length > MaxPayload {
		return Frame{}, ErrTooLarge
	}
	f.Payload = make([]byte, length)
	switch _, err := io.ReadFull(r, f.Payload); err {
	case nil:
	case io.EOF, io.ErrUnexpectedEOF:
		return Frame{}, ErrTruncatedPayload
	default:
		return Frame{}, fmt.Errorf("codec: read payload: %w", err)
	}
	return f, nil
}
