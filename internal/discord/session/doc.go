// Package session manages the Discord connection: connect, reconnect, apply an
// activity, clear it. It reaches the pipe through a port.
//
// It may import internal/domain, internal/discord/codec, internal/schedule and
// internal/diag. It must not import internal/discord/transport, internal/host,
// internal/cli or any adapter; the real dialer is handed to it by
// internal/cli.
//
// # Shape
//
// A [Manager] is given the activity to show with Set and Clear, which never
// block, and keeps Discord showing it from Run. Run is one goroutine that owns
// the connection: it dials, sends the handshake, writes every frame and closes
// the stream. A second goroutine per connection reads frames and hands them to
// the first. Nothing else touches the stream.
//
// The update scheduler sits between Set and the connection. What it emits
// while Discord is not connected is dropped, and the scheduler is reset when
// the connection is ready, so the current activity is sent again. The dropped
// emission still counts towards the rate limit: an activity set less than the
// scheduler's interval before Discord became reachable is shown when that
// interval has passed, not at once. That errs on the side of the limit, across
// a reconnect as well.
//
// # Time
//
// Nothing here uses a deadline on the connection. A write that does not
// finish, a handshake that is not answered and a clear that is not
// acknowledged are each ended by a timer on the injected clock that closes the
// connection, which fails the pending read and write on every operating
// system.
package session
