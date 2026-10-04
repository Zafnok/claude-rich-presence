package diag

import "sync/atomic"

// Counters count things worth knowing without a log line for each. The zero
// value is ready, and is safe for use by several goroutines.
type Counters struct {
	eventsReceived atomic.Int64
	eventsDropped  atomic.Int64
	reconnects     atomic.Int64
	failovers      atomic.Int64
}

// EventReceived counts one event received.
func (c *Counters) EventReceived() { c.eventsReceived.Add(1) }

// EventDropped counts one event dropped: malformed, or the queue was full.
func (c *Counters) EventDropped() { c.eventsDropped.Add(1) }

// Reconnected counts one reconnection to Discord.
func (c *Counters) Reconnected() { c.reconnects.Add(1) }

// FailedOver counts one takeover of the host role.
func (c *Counters) FailedOver() { c.failovers.Add(1) }

// Counts is the value of every counter at one moment.
type Counts struct {
	EventsReceived int64
	EventsDropped  int64
	Reconnects     int64
	Failovers      int64
}

// Snapshot returns the current counts.
func (c *Counters) Snapshot() Counts {
	return Counts{
		EventsReceived: c.eventsReceived.Load(),
		EventsDropped:  c.eventsDropped.Load(),
		Reconnects:     c.reconnects.Load(),
		Failovers:      c.failovers.Load(),
	}
}

// Attrs returns the counts as log attributes.
func (c Counts) Attrs() []Attr {
	return []Attr{
		Count("events_received", c.EventsReceived),
		Count("events_dropped", c.EventsDropped),
		Count("reconnects", c.Reconnects),
		Count("failovers", c.Failovers),
	}
}
