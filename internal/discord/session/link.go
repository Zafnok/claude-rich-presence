package session

import (
	"io"

	"github.com/Zafnok/claude-rich-presence/internal/discord/codec"
)

// closeInvalidClientID is the code of the close frame Discord sends for an
// application id it does not know.
const closeInvalidClientID = 4000

// incoming is one result of reading the stream: a frame, or the error that
// ended it.
type incoming struct {
	frame codec.Frame
	err   error
}

// link is one open connection and the goroutine that reads it. The manager's
// goroutine does everything else: it writes, and it closes.
type link struct {
	conn   io.ReadWriteCloser
	frames chan incoming
	quit   chan struct{}
	done   chan struct{}
}

func open(conn io.ReadWriteCloser) *link {
	l := &link{
		conn:   conn,
		frames: make(chan incoming),
		quit:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go l.read()
	return l
}

// read hands over frames until the stream fails, and the failure last.
func (l *link) read() {
	defer close(l.done)
	for {
		f, err := codec.ReadFrame(l.conn)
		select {
		case l.frames <- incoming{f, err}:
		case <-l.quit:
			return
		}
		if err != nil {
			return
		}
	}
}

// hangUp closes the stream, which fails a read or a write that is pending on
// it. It may be called from any goroutine, any number of times.
func (l *link) hangUp() { _ = l.conn.Close() }

// close ends the link and waits for its reader.
func (l *link) close() {
	close(l.quit)
	l.hangUp()
	<-l.done
}

// receive decodes what the reader handed over. It reports false, and why,
// when the connection is over: the stream failed, the frame could not be
// understood, or Discord sent a close frame.
func receive(in incoming) (codec.Message, failure, bool) {
	if in.err != nil {
		return codec.Message{}, failLost, false
	}
	msg, err := codec.Decode(in.frame)
	if err != nil {
		return codec.Message{}, failProtocol, false
	}
	if msg.Kind == codec.KindClose {
		if msg.Code == closeInvalidClientID {
			return codec.Message{}, failInvalidID, false
		}
		return codec.Message{}, failClosed, false
	}
	return msg, failure{}, true
}
