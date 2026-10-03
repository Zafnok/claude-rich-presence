package domain

import (
	"cmp"
	"slices"
)

// Registry is the set of open sessions, reduced from events.
//
// It is not safe for concurrent use. The host owns it from a single
// goroutine, so it has no lock.
//
// Events are upserts and are applied in the order they arrive. The registry
// does not reorder or drop an event by its timestamp: a wall clock can step
// backwards, and a session that then ignored every event would be stuck.
type Registry struct {
	sessions map[string]Session
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{sessions: map[string]Session{}}
}

// Apply reduces one event into the registry and returns the new snapshot.
//
// An event for an unknown session creates it, except session ended, which
// removes the session and does nothing if it is not there. An invalid event
// returns an error wrapping ErrInvalidEvent and changes nothing.
func (r *Registry) Apply(e Event) ([]Session, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	if e.Kind == KindSessionEnded {
		r.Remove(e.SessionID)
		return r.Snapshot(), nil
	}
	s, ok := r.sessions[e.SessionID]
	if !ok {
		s = newSession(e)
	}
	r.sessions[e.SessionID] = s.apply(e)
	return r.Snapshot(), nil
}

// Sync replaces the session with the same id wholesale, or adds it. It is
// how a follower resends its state to a new host. An invalid session returns
// an error wrapping ErrInvalidSession and changes nothing.
func (r *Registry) Sync(s Session) error {
	if err := s.Validate(); err != nil {
		return err
	}
	r.sessions[s.ID] = s
	return nil
}

// Remove drops a session, as when its adapter's connection closes. Removing
// an unknown id does nothing.
func (r *Registry) Remove(id string) {
	delete(r.sessions, id)
}

// Snapshot returns a copy of the open sessions, ordered by id. The caller
// owns it: changing it does not change the registry.
func (r *Registry) Snapshot() []Session {
	out := make([]Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Session) int { return cmp.Compare(a.ID, b.ID) })
	return out
}
