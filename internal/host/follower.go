package host

import (
	"context"
	"io"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
)

// follow is one attempt to follow a host: dial, hello, and on a welcome the
// node's session and then its events, for as long as the connection lasts.
// mayAsk says whether an older host may be asked to stand down.
func (n *Node) follow(ctx context.Context, mayAsk bool) outcome {
	conn, err := n.dial(ctx)
	if err != nil {
		n.log.Debug("no presence host could be reached")
		return missed
	}
	// Nothing below has a deadline. The connection is closed when ctx ends,
	// which fails whatever read or write is pending on it.
	defer closeWith(ctx, conn)()

	lines := protocol.NewDecoder(conn)
	welcome, ok := n.greet(conn, lines)
	if !ok {
		n.log.Debug("the presence host did not welcome this process")
		return missed
	}
	asked := mayAsk && newer(n.version, welcome.Version)
	if asked {
		// Asked once. A host that does not agree it is older carries on,
		// and so does this node, as its follower.
		n.log.Info("asked an older presence host to stand down", diag.Version("host_version", welcome.Version))
		_ = protocol.Encode(conn, protocol.StandDown{})
	}
	if newer(welcome.Version, n.version) {
		// If this node stood down, it was not for nothing.
		n.yielded = false
	}
	n.log.Info("following the presence host", diag.Version("host_version", welcome.Version))
	n.followed = true
	n.haste = 0
	n.attach(RoleFollower, nil, welcome.Version)

	// The reader hears what the host says. It ends when the connection does.
	heard := make(chan struct{})
	go func() {
		defer close(heard)
		n.protect(func() { n.hear(lines) })
	}()
	defer func() {
		_ = conn.Close()
		<-heard
		n.detach()
		n.log.Info("stopped following the presence host")
		if asked {
			// Whether the host stood down or is gone for another reason,
			// the newer node is the one that should take over.
			n.haste = hasteTries
		}
	}()
	for {
		select {
		case <-heard:
			// The connection has ended: the host has gone, or ctx has ended.
			return lost
		case <-n.wake:
			if !n.forward(conn) {
				return lost
			}
		}
	}
}

// greet says hello and reads the host's answer. It reports false when there
// is no welcome: the host refused, or the connection ended first. Lines it
// does not understand are passed over, as everywhere on the channel.
func (n *Node) greet(conn io.Writer, lines *protocol.Decoder) (protocol.Welcome, bool) {
	if protocol.Encode(conn, protocol.Hello{Protocol: protocol.Version, Version: n.version}) != nil {
		return protocol.Welcome{}, false
	}
	for {
		m, err := lines.Next()
		if err != nil && !skippable(err) {
			return protocol.Welcome{}, false
		}
		switch m := m.(type) {
		case protocol.Welcome:
			return m, true
		case protocol.Refuse:
			if m.Reason == protocol.ReasonUnsupportedProtocol {
				// A host that cannot speak this protocol is taken to be the
				// older binary and is asked to stand down. A host does so
				// only if the hello it refused carried a newer protocol than
				// its own, so a newer host that has dropped this protocol
				// ignores the request.
				_ = protocol.Encode(conn, protocol.StandDown{})
			}
			return protocol.Welcome{}, false
		}
	}
}

// hear reads what the host sends until the connection ends, and remembers
// the latest summary of itself, the pause it holds and the card it shows.
func (n *Node) hear(lines *protocol.Decoder) {
	for {
		m, err := lines.Next()
		if err != nil && !skippable(err) {
			return
		}
		switch m := m.(type) {
		case protocol.StatusResult:
			n.mu.Lock()
			n.heard = statusOf(RoleFollower, m)
			n.old = m.Pause == nil
			n.mu.Unlock()
			if m.Pause != nil {
				n.learn(*m.Pause)
			}
		case protocol.PreviewResult:
			n.mu.Lock()
			n.seen = &m
			n.mu.Unlock()
		}
	}
}

// forward sends everything that is waiting. It reports false when the
// connection has failed. A message the codec will not carry is dropped and
// counted, and the connection stays.
func (n *Node) forward(conn io.Writer) bool {
	for _, m := range n.take() {
		err := protocol.Encode(conn, m)
		switch {
		case err == nil:
		case skippable(err):
			n.counters.EventDropped()
		default:
			return false
		}
	}
	return true
}
