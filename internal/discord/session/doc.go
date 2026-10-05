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
// The update scheduler sits between Set and the connection, and enforces the
// interval between two updates written to Discord. The manager keeps the
// desired activity itself and gives it to the scheduler only while the
// connection is ready, so nothing is held back for a connection that is not
// there. If the connection is lost in the moment between the scheduler's
// decision and the hand-over, the manager refuses the update, and a refused
// update is as if it had not been made: it does not count towards the
// interval.
//
// When a connection becomes ready, Discord has forgotten what it was showing,
// so the current activity is sent at once. The one exception is the limit: if
// an activity was written to Discord, on this connection or an earlier one,
// less than an interval ago, the current activity is sent when that interval
// has passed. An activity set before the first connection, or while Discord is
// absent, is therefore shown as soon as the handshake is done. The interval is
// the one in [Config].
//
// # Time
//
// Nothing here uses a deadline on the connection. A write that does not
// finish, a handshake that is not answered and a clear that is not
// acknowledged are each ended by a timer on the injected clock that closes the
// connection, which fails the pending read and write on every operating
// system.
package session
