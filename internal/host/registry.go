package host

import (
	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// op is what a request asks of the goroutine that owns the registry.
type op int

const (
	// opEvent applies one event from a source.
	opEvent op = iota
	// opSync replaces what a source holds with one session, or with none.
	opSync
	// opClosed removes what a source holds: its connection has ended.
	opClosed
	// opTick changes nothing. The idle period, or a pause, may have ended.
	opTick
	// opPause offers a pause or a resume.
	opPause
	// opPeek does nothing. Whoever sent it knows, once it is taken, that
	// everything sent before it has been shown.
	opPeek
)

// request is one thing for the goroutine that owns the registry to do.
type request struct {
	op      op
	source  uint64
	event   domain.Event
	session *domain.Session // for opSync; nil means the source has no session
	pause   protocol.PauseState
}

// registry is the host's sessions and which source each belongs to. It is
// used from one goroutine and has no lock.
type registry struct {
	sessions *domain.Registry
	// owners maps a session id to the source it belongs to: the one whose
	// event created it, or the latest to sync it. Only a sync moves a
	// session to another source. So a follower that reconnects before its
	// old connection is seen to close keeps its session, and what is still
	// to be read on the old connection can neither take the session back
	// nor end it.
	owners map[string]uint64
	// pause is the latest pause or resume the host was offered.
	pause protocol.PauseState
}

// apply carries out a request and reports whether what is shown may have
// changed. What the domain does not accept, such as a word a newer follower
// uses and this binary does not know, is dropped and counted, and changes
// nothing.
func (r *registry) apply(req request, counters *diag.Counters) (changed bool) {
	switch req.op {
	case opEvent:
		counters.EventReceived()
		id := req.event.SessionID
		if owner, owned := r.owners[id]; owned && owner != req.source {
			counters.EventDropped()
			return false
		}
		if _, err := r.sessions.Apply(req.event); err != nil {
			counters.EventDropped()
			return false
		}
		if req.event.Kind == domain.KindSessionEnded {
			delete(r.owners, id)
		} else {
			r.owners[id] = req.source
		}
		return true
	case opSync:
		keep := ""
		if req.session != nil {
			if r.sessions.Sync(*req.session) != nil {
				counters.EventDropped()
				return false
			}
			keep = req.session.ID
			r.owners[keep] = req.source
		}
		return r.release(req.source, keep) || keep != ""
	case opClosed:
		return r.release(req.source, "")
	case opPause:
		if !supersedes(req.pause, r.pause) {
			return false
		}
		r.pause = req.pause
		return true
	case opPeek:
		return false
	}
	return true
}

// release removes every session of a source but the one named keep, and
// reports whether there was any.
func (r *registry) release(source uint64, keep string) (removed bool) {
	for id, owner := range r.owners {
		if owner == source && id != keep {
			delete(r.owners, id)
			r.sessions.Remove(id)
			removed = true
		}
	}
	return removed
}
