package host

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/presence"
)

const (
	// retryBase is the one constant the timing of the election derives from.
	// It is the longest first wait of a node that has just lost its host, so
	// it is also most of the gap in presence at a failover.
	retryBase = 100 * time.Millisecond
	// retryCap bounds the delay between rounds that reach no host.
	retryCap = 64 * retryBase
	// standDownDelay is how long a host that stood down waits before it
	// tries the lock again. A follower's first three waits are at most one,
	// two and four times retryBase, so by then every follower has tried the
	// lock three times, and the one that asked has had its chance to win.
	standDownDelay = 8 * retryBase
	// hasteWait and hasteTries are how a node that is newer than the host
	// it has just lost gets to the lock first: it tries hasteTries times,
	// hasteWait apart, all within the half of retryBase that is the least
	// any other follower waits.
	hasteWait  = retryBase / 16
	hasteTries = 6
	// greetTimeout is how long a connection has to be welcomed. One that
	// says nothing, or is refused, is closed when it has passed.
	greetTimeout = 10 * retryBase
	// queueSize is how many events may wait for the host. One more replaces
	// them all with a sync.
	queueSize = 64
)

// Role is the part a node plays in presence.
type Role int

// The roles.
const (
	// RoleNone is neither of the others: the node has not found its role
	// yet, is between two, or has stopped.
	RoleNone Role = iota
	// RoleFollower forwards the node's session to a host.
	RoleFollower
	// RoleHost holds the lock and the Discord connection.
	RoleHost
)

// Status is the diagnostic summary of presence, as this node knows it. It
// says nothing about any session.
type Status struct {
	Role Role
	// Discord is the host's connection to Discord, or unknown when the node
	// has no host or has not heard from it yet.
	Discord protocol.DiscordState
	// Sessions is how many sessions the host holds.
	Sessions int
	// Version is the host's binary version.
	Version string
	// Uptime is how long the host has been host.
	Uptime time.Duration
	// Paused says presence is switched off for now, by the latest pause the
	// node knows of. PausedUntil is when that ends by itself, or the zero
	// time for a pause that lasts until it is resumed.
	Paused      bool
	PausedUntil time.Time
	// NoPause says the host is from before pausing, and cannot pause.
	NoPause bool
}

// outcome is how one round of the role loop ended.
type outcome int

const (
	// missed is a round that reached no host and did not become one.
	missed outcome = iota
	// lost is a round that had a host, or was one, and no longer does.
	lost
	// yielded is a round as host that ended by standing down.
	yielded
)

// Node is what an adapter holds: the node's own session, and behind Publish
// either the presence host or a connection to one. See the package
// documentation.
type Node struct {
	version  string
	acquire  func() (Lock, error)
	dial     func(ctx context.Context) (io.ReadWriteCloser, error)
	discord  func() Discord
	render   func([]domain.Session, time.Time, presence.Settings) (domain.Activity, bool)
	settings presence.Settings
	link     func(string) (string, bool)
	clock    Clock
	jitter   func() float64
	counters *diag.Counters
	log      *diag.Logger

	// wake tells the goroutine in Run that there is something to send.
	wake chan struct{}

	// mu is held only to read and write the fields below, never across a
	// call that could wait.
	mu sync.Mutex
	// own holds the node's session, or nothing.
	own *domain.Registry
	// queue holds the events published since the last sync was taken. It is
	// empty whenever resync is set.
	queue []domain.Event
	// resync says the next thing to send is the session as a whole.
	resync bool
	// asked says the host's status is wanted.
	asked bool
	role  Role
	// term is the term as host, while role is RoleHost.
	term *term
	// heard is what the host last said of itself, while role is
	// RoleFollower.
	heard Status
	// pause is the latest pause or resume the node knows of, and offer says
	// the host is still to be given it.
	pause protocol.PauseState
	offer bool
	// old says the host has said of itself and cannot pause, and seen is the
	// card it last said it shows. Both are of the host that is followed now.
	old  bool
	seen *protocol.PreviewResult

	// The rest belongs to the goroutine in Run.
	followed bool // a host has been followed since this node was last host
	// yielded says this node stood down and has not followed a newer host
	// since: so far, it stood down for nothing.
	yielded bool
	// haste is how many rounds are still to be tried in a hurry.
	haste      int
	lockWarned bool // a lock that cannot be tried has been reported
	dialWarned bool // a dial that was refused as unsafe has been reported
}

// New checks the configuration and returns a node. The caller must call Run,
// once.
func New(cfg Config) (*Node, error) {
	switch {
	case protocol.Encode(io.Discard, protocol.Hello{Protocol: protocol.Version, Version: cfg.Version}) != nil:
		return nil, errors.New("host: version is not a binary version")
	case cfg.Acquire == nil:
		return nil, errors.New("host: acquire is nil")
	case cfg.Dial == nil:
		return nil, errors.New("host: dial is nil")
	case cfg.Discord == nil:
		return nil, errors.New("host: discord is nil")
	case cfg.Render == nil:
		return nil, errors.New("host: render is nil")
	case cfg.Clock == nil:
		return nil, errors.New("host: clock is nil")
	case cfg.Jitter == nil:
		return nil, errors.New("host: jitter is nil")
	case cfg.Counters == nil:
		return nil, errors.New("host: counters is nil")
	}
	return &Node{
		version:  cfg.Version,
		acquire:  cfg.Acquire,
		dial:     cfg.Dial,
		discord:  cfg.Discord,
		render:   cfg.Render,
		settings: cfg.Settings,
		link:     cfg.Link,
		clock:    cfg.Clock,
		jitter:   cfg.Jitter,
		counters: cfg.Counters,
		log:      cfg.Logger,
		wake:     make(chan struct{}, 1),
		own:      domain.NewRegistry(),
		queue:    make([]domain.Event, 0, queueSize),
	}, nil
}

// Publish takes one event of the node's session. It updates the node's own
// state and leaves the event for the host. It never blocks and performs no
// I/O, in every role and between roles. An invalid event is dropped and
// counted.
//
// A node has one session: an event for another session id replaces it.
func (n *Node) Publish(e domain.Event) {
	n.mu.Lock()
	sessions, err := n.own.Apply(e)
	if err != nil {
		n.mu.Unlock()
		n.counters.EventDropped()
		return
	}
	moved := false
	if e.Kind != domain.KindSessionEnded {
		for _, s := range sessions {
			if s.ID != e.SessionID {
				n.own.Remove(s.ID)
				moved = true
			}
		}
	}
	switch {
	case n.resync:
		// The sync that is due carries this event's effect.
	case moved, len(n.queue) == queueSize:
		// The host is told the whole state instead: it holds a session this
		// node has left, or the events are more than it has taken.
		n.queue = n.queue[:0]
		n.resync = true
	default:
		n.queue = append(n.queue, e)
	}
	n.mu.Unlock()
	n.signal()
}

// Status returns the diagnostic summary. It returns at once from memory: a
// host answers for itself, and a follower with what its host last said. The
// follower also asks the host again, so that a later call is up to date.
func (n *Node) Status() Status {
	n.mu.Lock()
	role, t, heard, pause, old := n.role, n.term, n.heard, n.pause, n.old
	if role == RoleFollower {
		n.asked = true
	}
	n.mu.Unlock()
	st := Status{Discord: protocol.DiscordUnknown}
	switch role {
	case RoleHost:
		st = statusOf(RoleHost, t.status())
	case RoleFollower:
		n.signal()
		st = heard
	}
	// The pause is the latest the node knows of, which a request of its own
	// that the host has yet to take may be. A host that cannot pause shows
	// presence whatever the node knows.
	st.NoPause = role == RoleFollower && old
	if !st.NoPause && pausedAt(pause, n.clock.Now()) {
		st.Paused = true
		if pause.Until != 0 {
			st.PausedUntil = time.UnixMilli(pause.Until).UTC()
		}
	}
	return st
}

// statusOf is a host's summary of itself, as a node reports it.
func statusOf(role Role, r protocol.StatusResult) Status {
	return Status{
		Role:     role,
		Discord:  r.Discord,
		Sessions: r.Sessions,
		Version:  r.Version,
		Uptime:   time.Duration(r.UptimeSeconds) * time.Second,
	}
}

// Run is the role loop. It returns when ctx ends, after the node has given
// up whatever role it had, and no goroutine of the node outlives it.
func (n *Node) Run(ctx context.Context) {
	delay := retryBase
	for ctx.Err() == nil {
		// A panic costs the round, as a host that could not be reached does.
		out := missed
		n.protect(func() { out = n.round(ctx) })
		if out != missed {
			delay = retryBase
		}
		wait := standDownDelay
		switch {
		case out == yielded:
		case n.haste > 0:
			n.haste--
			wait = hasteWait
		default:
			wait = jittered(delay, n.jitter())
			delay = min(2*delay, retryCap)
		}
		if !n.sleep(ctx.Done(), wait) {
			return
		}
	}
}

// round tries the lock, and is host or follows one until that ends.
func (n *Node) round(ctx context.Context) outcome {
	lock, err := n.acquire()
	if err == nil {
		n.lockWarned = false
		return n.host(ctx, lock)
	}
	held := errors.Is(err, ErrLocked)
	if !held && !n.lockWarned {
		n.lockWarned = true
		n.log.Warn("the host lock cannot be tried, so this process can only follow a host", diag.ErrorClass("lock_failed"))
	}
	// A node that cannot try the lock could not take over from a host, so
	// it does not ask one to stand down.
	return n.follow(ctx, held)
}

// attach gives the node a host: itself, with its term, or the one that
// welcomed it, with that host's version. What the new host is sent first is
// the session as a whole.
func (n *Node) attach(role Role, t *term, version string) {
	n.mu.Lock()
	n.role, n.term = role, t
	n.heard = Status{Role: role, Discord: protocol.DiscordUnknown, Version: version}
	n.queue = n.queue[:0]
	n.resync, n.asked = true, true
	// A pause outlives a host because each node offers the next host the
	// latest it knows of.
	n.offer = n.pause.At != 0
	n.old, n.seen = false, nil
	n.mu.Unlock()
	n.signal()
}

// detach leaves the node without a host.
func (n *Node) detach() {
	n.mu.Lock()
	n.role, n.term = RoleNone, nil
	n.mu.Unlock()
}

// take removes and returns everything that is waiting for the host, in the
// order it is to be sent: a pause or a resume, so that a host that is to be
// paused shows nothing first, then the session as a whole if that is due, or
// else the queued events. A follower also asks for the host's status and its
// card whenever it sends something or either has been asked for.
func (n *Node) take() []protocol.Message {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []protocol.Message
	if n.offer {
		n.offer = false
		out = append(out, pauseRequest(n.pause))
	}
	if n.resync {
		n.resync = false
		out = append(out, protocol.Sync{Session: n.session()})
	}
	for _, e := range n.queue {
		out = append(out, protocol.Event{Event: protocol.EventFromDomain(e)})
	}
	n.queue = n.queue[:0]
	if n.role == RoleFollower && (n.asked || len(out) > 0) {
		n.asked = false
		out = append(out, protocol.Status{}, protocol.Preview{})
	}
	return out
}

// session is the node's own session as a sync carries it, or nil if it has
// none. The caller holds mu.
func (n *Node) session() *protocol.SessionState {
	own := n.own.Snapshot()
	if len(own) == 0 {
		return nil
	}
	s := protocol.SessionFromDomain(own[0])
	return &s
}

// signal asks the goroutine in Run to look at what is waiting. One request
// is enough however many are made before it gets to it.
func (n *Node) signal() {
	select {
	case n.wake <- struct{}{}:
	default:
	}
}

// protect runs f and reports whether it returned. A panic in f is recovered
// and logged (ADR-0008). What it carried is not logged: it could hold what a
// session sent.
func (n *Node) protect(f func()) (returned bool) {
	defer func() {
		if recover() != nil {
			n.log.Error("recovered from a panic in the presence host", diag.ErrorClass("panic"))
		}
	}()
	f()
	return true
}

// sleep waits for d on the clock. It reports false if cancel was closed
// first.
func (n *Node) sleep(cancel <-chan struct{}, d time.Duration) bool {
	passed := make(chan struct{})
	stop := n.clock.AfterFunc(d, func() { close(passed) })
	defer stop()
	select {
	case <-cancel:
		return false
	case <-passed:
		return true
	}
}

// jittered places a wait between half of delay and all of it.
func jittered(delay time.Duration, jitter float64) time.Duration {
	half := delay / 2
	return half + time.Duration(jitter*float64(half))
}

// skippable reports whether a codec error concerns one message only, so that
// the connection stays open (docs/protocol/control.md, "Errors").
func skippable(err error) bool {
	return errors.Is(err, protocol.ErrMalformed) ||
		errors.Is(err, protocol.ErrMissingField) ||
		errors.Is(err, protocol.ErrInvalidValue)
}
