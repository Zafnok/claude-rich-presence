package transport

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"sync"
)

// Listener accepts follower connections on the control socket. It is made
// by HostLock.Listen.
//
// The network listener inside is not exported: closing it directly would
// leave the socket file behind.
type Listener struct {
	inner net.Listener
	path  string

	mu     sync.Mutex
	closed bool
}

// Accept waits for the next follower connection. After Close it returns an
// error wrapping net.ErrClosed.
func (l *Listener) Accept() (net.Conn, error) {
	return l.inner.Accept()
}

// Addr returns the address of the control socket.
func (l *Listener) Addr() net.Addr {
	return l.inner.Addr()
}

// Close stops listening and removes the socket file. Closing twice is
// harmless.
func (l *Listener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	return errors.Join(l.inner.Close(), removeIfExists(l.path))
}

func (l *Listener) isClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
