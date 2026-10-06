package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Encode writes m to w as one line, in a single write. A message that would
// not decode is not written: the error wraps ErrMissingField or
// ErrInvalidValue. Any other error is the writer's.
func Encode(w io.Writer, m Message) error {
	v, err := m.wire()
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(v)
}

// Decode reads one line, with or without its newline.
//
// A type this binary does not know decodes to Unknown, and fields it does not
// know are ignored. The errors wrap ErrLineTooLong, ErrMalformed,
// ErrMissingField or ErrInvalidValue.
func Decode(line []byte) (Message, error) {
	line = bytes.TrimSpace(line)
	if len(line) > MaxLineBytes {
		return nil, ErrLineTooLong
	}
	var head typed
	if json.Unmarshal(line, &head) != nil {
		return nil, ErrMalformed
	}
	switch head.Type {
	case "":
		return nil, missing("type")
	case TypeHello:
		return decodeAs[Hello](line)
	case TypeWelcome:
		return decodeAs[Welcome](line)
	case TypeRefuse:
		return decodeAs[Refuse](line)
	case TypeSync:
		return decodeAs[Sync](line)
	case TypeEvent:
		return decodeAs[Event](line)
	case TypeStandDown:
		return decodeAs[StandDown](line)
	case TypeStatus:
		return decodeAs[Status](line)
	case TypeStatusResult:
		return decodeAs[StatusResult](line)
	case TypePause:
		return decodeAs[Pause](line)
	case TypeResume:
		return decodeAs[Resume](line)
	case TypePreview:
		return decodeAs[Preview](line)
	case TypePreviewResult:
		return decodeAs[PreviewResult](line)
	}
	return Unknown{}, nil
}

// checked is a message that can say whether it is complete.
type checked interface {
	Message
	check() error
}

// decodeAs reads a line whose type is already known. The JSON error is not
// wrapped, because it can quote the input.
func decodeAs[T checked](line []byte) (Message, error) {
	var m T
	if json.Unmarshal(line, &m) != nil {
		return nil, ErrMalformed
	}
	if err := m.check(); err != nil {
		return nil, err
	}
	return m, nil
}

// Decoder reads messages from a stream, one per line.
type Decoder struct {
	lines *bufio.Scanner
}

// NewDecoder returns a decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	lines := bufio.NewScanner(r)
	// The buffer must hold the longest line and its newline.
	lines.Buffer(make([]byte, 0, 4096), MaxLineBytes+1)
	return &Decoder{lines: lines}
}

// Next returns the next message. Blank lines are skipped.
//
// At the end of the stream it returns io.EOF. A line over the limit returns
// ErrLineTooLong, and so does every later call: the connection is to be
// closed. A reader error is returned as it is, and is also final. Any other
// error concerns one line only, and the next call reads the line after it.
func (d *Decoder) Next() (Message, error) {
	for d.lines.Scan() {
		if len(bytes.TrimSpace(d.lines.Bytes())) == 0 {
			continue
		}
		return Decode(d.lines.Bytes())
	}
	err := d.lines.Err()
	switch {
	case err == nil:
		return nil, io.EOF
	case errors.Is(err, bufio.ErrTooLong):
		return nil, ErrLineTooLong
	}
	return nil, err
}
