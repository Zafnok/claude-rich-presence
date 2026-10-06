package host

import (
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// A pause switches the whole presence off without changing any setting
// (ADR-0012). The host holds it, and it must outlive the host. So every node
// remembers the latest pause or resume it knows of, which is its own request
// or what a host said, and offers it to each host it gains. A host keeps the
// latest of what it is offered, so the nodes agree whatever order they arrive
// in. See "Pausing" in docs/protocol/control.md.

// Preview is the card as the host last handed it to Discord. It can name a
// project, so it is for the user's own eyes and must not be logged.
type Preview struct {
	// Shown is false when the host shows nothing.
	Shown    bool
	Activity domain.Activity
}

// supersedes reports whether a host that holds old is to keep next instead:
// the request the user made later. Of two made at the same time a pause wins
// over a resume, and the pause that ends later over the other.
func supersedes(next, old protocol.PauseState) bool {
	switch {
	case next.At != old.At:
		return next.At > old.At
	case next.Paused != old.Paused:
		return next.Paused
	case !next.Paused || old.Until == 0:
		return false
	}
	return next.Until == 0 || next.Until > old.Until
}

// pausedAt reports whether p keeps presence off at the time now.
func pausedAt(p protocol.PauseState, now time.Time) bool {
	return p.Paused && (p.Until == 0 || now.UnixMilli() < p.Until)
}

// request is p as a node offers it to a host.
func pauseRequest(p protocol.PauseState) protocol.Message {
	if p.Paused {
		return protocol.Pause{Until: p.Until, At: p.At}
	}
	return protocol.Resume{At: p.At}
}

// Pause switches presence off for every session: until the time until, or
// until Resume when until is the zero time. It returns at once and performs
// no I/O: the request is left for the host, and for the next one if there is
// none now. It reports false, and does nothing, when the host is from before
// pausing and would ignore the request.
func (n *Node) Pause(until time.Time) bool {
	p := protocol.PauseState{Paused: true}
	if !until.IsZero() {
		p.Until = until.UnixMilli()
	}
	return n.ask(p)
}

// Resume ends a pause, with the rules of Pause.
func (n *Node) Resume() bool {
	return n.ask(protocol.PauseState{})
}

// ask records the user's request as the latest and leaves it for the host.
func (n *Node) ask(p protocol.PauseState) bool {
	now := n.clock.Now().UnixMilli()
	n.mu.Lock()
	if n.role == RoleFollower && n.old {
		n.mu.Unlock()
		return false
	}
	// The user's latest request is the latest of all, even when the clock
	// has not moved since the one before.
	p.At = max(now, n.pause.At+1)
	n.pause, n.offer = p, true
	n.mu.Unlock()
	n.signal()
	return true
}

// learn takes a pause a host holds, if it is later than the one the node
// knows.
func (n *Node) learn(p protocol.PauseState) {
	n.mu.Lock()
	if supersedes(p, n.pause) {
		n.pause = p
	}
	n.mu.Unlock()
}

// known is the latest pause the node knows of.
func (n *Node) known() protocol.PauseState {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.pause
}

// Preview returns the card as the host last showed it, and false when that
// is not known: the node has no host, or its host has not said yet, or is
// from before previews. It returns at once from memory. A follower also asks
// its host again, so that a later call is up to date.
func (n *Node) Preview() (Preview, bool) {
	n.mu.Lock()
	role, t, seen := n.role, n.term, n.seen
	if role == RoleFollower {
		n.asked = true
	}
	n.mu.Unlock()
	switch {
	case role == RoleHost:
		seen = t.card.Load()
	case role == RoleFollower:
		n.signal()
	}
	if seen == nil {
		return Preview{}, false
	}
	activity, shown := seen.Activity()
	return Preview{Shown: shown, Activity: activity}, true
}
