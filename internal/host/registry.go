package host

import (
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
	// opTick changes nothing. The idle period may have ended.
	opTick
)

// request is one thing for the goroutine that owns the registry to do.
type request struct {
	op      op
	source  uint64
	event   domain.Event
	session *domain.Session // for opSync; nil means the source has no session
}

// registry is the host's sessions and which source each belongs to. It is
// used from one goroutine and has no lock.
type registry struct {
	sessions *domain.Registry
	// owners maps a session id to the source that last synced it or sent an
	// event for it. A session belongs to one source: the latest to name it,
	// so that a follower that reconnects before its old connection is seen
	// to close does not lose its session when that happens.
	owners map[string]uint64
}

// apply carries out a request and reports whether what is shown may have
// changed. What the domain does not accept, such as a word a newer follower
// uses and this binary does not know, is dropped and counted, and changes
// nothing.
func (r *registry) apply(req request, counters *diag.Counters) (changed bool) {
	switch req.op {
	case opEvent:
		counters.EventReceived()
		if _, err := r.sessions.Apply(req.event); err != nil {
			counters.EventDropped()
			return false
		}
		if req.event.Kind == domain.KindSessionEnded {
			delete(r.owners, req.event.SessionID)
		} else {
			r.owners[req.event.SessionID] = req.source
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
