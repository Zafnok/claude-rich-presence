package transport

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
)

// Errors returned by Acquire and by HostLock.Listen.
var (
	// ErrLocked reports that another process holds the host lock.
	ErrLocked = errors.New("host lock is held by another process")
	// ErrLockReleased reports a call on a lock that was released.
	ErrLockReleased = errors.New("host lock was released")
	// ErrListening reports a second Listen while the first listener is open.
	ErrListening = errors.New("already listening on the control socket")
)

// HostLock is the exclusive per-user lock that elects the host (ADR-0005).
// The operating system releases it when the holding process exits for any
// reason. The platform call behind it is openExclusive.
//
// Only a HostLock can listen on the control socket, and so only the lock
// holder can remove the socket file through this package.
type HostLock struct {
	mu       sync.Mutex
	file     *os.File // nil once released
	socket   string
	listener *Listener
}

// Acquire prepares the runtime directory and tries once, without blocking,
// to take the host lock. It returns ErrLocked if another process holds it.
//
// On Windows any other open handle on the lock file also reads as ErrLocked,
// for as long as it is open.
func Acquire(paths Paths) (*HostLock, error) {
	if err := Prepare(paths.Dir); err != nil {
		return nil, err
	}
	file, err := openExclusive(paths.Lock)
	if errors.Is(err, errHeld) {
		return nil, ErrLocked
	}
	if err != nil {
		return nil, fmt.Errorf("host lock: %w", err)
	}
	return &HostLock{file: file, socket: paths.Socket}, nil
}

// Listen removes a socket file left by a dead host and listens on the
// control socket. The lock makes the removal safe: while it is held, no
// other process can be listening there.
func (l *HostLock) Listen() (*Listener, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil, ErrLockReleased
	}
	if l.listener != nil && !l.listener.isClosed() {
		return nil, ErrListening
	}
	if err := removeIfExists(l.socket); err != nil {
		return nil, fmt.Errorf("stale control socket: %w", err)
	}
	inner, err := net.ListenUnix("unix", &net.UnixAddr{Name: l.socket, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("control socket: %w", err)
	}
	// Close removes the file itself, on every operating system alike.
	inner.SetUnlinkOnClose(false)
	l.listener = &Listener{Listener: inner, path: l.socket}
	return l.listener, nil
}

// Release closes the listener if one is open, which removes the socket
// file, and only then gives up the lock. Releasing twice is harmless.
func (l *HostLock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	var err error
	if l.listener != nil {
		err = l.listener.Close()
	}
	err = errors.Join(err, l.file.Close())
	l.file = nil
	return err
}
