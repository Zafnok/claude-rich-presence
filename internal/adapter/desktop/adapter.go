package desktop

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/mcp"
)

// Publisher takes presence events toward the host. Publish is called from
// Open, which runs while the client waits for its initialize to be answered,
// so it must return at once and must not perform I/O (ADR-0008).
type Publisher interface {
	Publish(domain.Event)
}

// Clock tells the time.
type Clock interface {
	Now() time.Time
}

// Options configures the adapter that reports Claude Desktop. Every field is
// required.
type Options struct {
	// Privacy is carried on the session so that it is rendered as the user's
	// other sessions are. The adapter publishes the same at every level: it
	// knows nothing that a level could withhold. At domain.PrivacyOff the
	// session is hidden, and the adapter publishes nothing.
	Privacy domain.Privacy
	// ID names the session. It must be unique to this process.
	ID        string
	Publisher Publisher
	Status    StatusSource
	Clock     Clock
}

// Adapter is the session of one open Claude Desktop, or the passive copy of
// the server, which has none.
//
// Make one with New or NewPassive, call Open when the client has initialized,
// and call Close when the server's input closes or the process is told to
// stop.
type Adapter struct {
	privacy domain.Privacy
	status  StatusSource
	id      string
	clock   Clock
	// pub is nil in the passive copy.
	pub Publisher

	mu     sync.Mutex
	opened bool
	closed bool
}

// New checks the options and builds the adapter that reports the app.
func New(opts Options) (*Adapter, error) {
	switch {
	case !opts.Privacy.Settable():
		return nil, errors.New("desktop: unknown privacy level")
	case opts.ID == "" || len(opts.ID) > domain.MaxIDLen:
		return nil, errors.New("desktop: session id must be 1 to 128 bytes")
	case opts.Publisher == nil:
		return nil, errors.New("desktop: publisher is nil")
	case opts.Status == nil:
		return nil, errors.New("desktop: status source is nil")
	case opts.Clock == nil:
		return nil, errors.New("desktop: clock is nil")
	}
	return &Adapter{privacy: opts.Privacy, status: opts.Status, id: opts.ID, clock: opts.Clock, pub: opts.Publisher}, nil
}

// NewPassive builds the adapter of the copy that reports nothing. It answers
// the status tool and publishes no event, whatever is called on it.
func NewPassive(privacy domain.Privacy, status StatusSource) (*Adapter, error) {
	switch {
	case !privacy.Settable():
		return nil, errors.New("desktop: unknown privacy level")
	case status == nil:
		return nil, errors.New("desktop: status source is nil")
	}
	return &Adapter{privacy: privacy, status: status}, nil
}

// Tools returns the status tool. There is no event tool: nothing in Claude
// Desktop would call it.
func (a *Adapter) Tools() []mcp.Tool {
	// The name and schema are constants that NewTool accepts, which the
	// tests show by listing the tools through a server.
	status, _ := mcp.NewTool(StatusToolName, statusDescription, json.RawMessage(`{"type":"object"}`), a.handleStatus)
	return []mcp.Tool{status}
}

// Open opens the session, begun now. Call it when MCP initialize completes:
// nothing may be published before (ADR-0007). A second call does nothing, and
// so does a call after Close.
func (a *Adapter) Open() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.opened || a.closed {
		return
	}
	a.opened = true
	a.publish(domain.Event{Kind: domain.KindSessionOpened, Privacy: a.privacy})
}

// Close ends the session, if it was opened. Calls after the first do nothing.
func (a *Adapter) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	a.closed = true
	if a.opened {
		a.publish(domain.Event{Kind: domain.KindSessionEnded})
	}
}

// publish stamps an event as this session's and gives it to the publisher. A
// publisher that panics loses the event and nothing else (ADR-0008). A hidden
// session, like the passive copy, publishes nothing.
func (a *Adapter) publish(e domain.Event) {
	if a.pub == nil || a.privacy == domain.PrivacyOff {
		return
	}
	defer func() { _ = recover() }()
	e.SessionID, e.Surface, e.At = a.id, domain.SurfaceDesktop, a.clock.Now()
	a.pub.Publish(e)
}
