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
type Listener struct {
	net.Listener
	path string

	mu     sync.Mutex
	closed bool
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
	return errors.Join(l.Listener.Close(), removeIfExists(l.path))
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
