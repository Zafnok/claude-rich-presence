package fakediscord

import (
	"crypto/rand"
	"errors"
	"io"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// This file is the only one that uses golang.org/x/sys and go-winio. The
// standard library can open a named pipe as a client but cannot create one.

// Named pipes share one namespace per machine, so the location is a random
// name prefix rather than a directory.
func newLocation() (string, error) { return `\\.\pipe\fakediscord-` + rand.Text() + `-`, nil }

// removeLocation has nothing to remove: a pipe name is gone when its last
// handle is closed.
func removeLocation(string) error { return nil }

func locationPrefix(location string) string { return location + "discord-ipc-" }

func locationEnv(string) []string { return nil }

const (
	pipeBufferSize = 64 * 1024
	// backlog is how many clients can connect before the server gets round
	// to accepting one. A named pipe needs one instance per client, and a
	// client that finds none free is told the pipe is busy.
	backlog = 8
)

// instance is one instance of the pipe, waiting for a client. The kernel
// writes to overlapped while the connect is pending, so an instance is always
// on the heap and is never copied.
type instance struct {
	handle     windows.Handle
	overlapped windows.Overlapped
	// immediate records that a client was there before the connect was
	// asked for, so there is no pending operation to collect.
	immediate bool
}

// pipeListener serves one pipe name.
type pipeListener struct {
	name      string
	path      *uint16
	stop      windows.Handle // manual-reset event, set by Interrupt
	instances [backlog]*instance
	done      bool
}

func listen(addr string) (listener, error) {
	path, err := windows.UTF16PtrFromString(addr)
	if err != nil {
		return nil, err
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	l := &pipeListener{name: addr, path: path, stop: stop}
	for i := range l.instances {
		// Only the first refuses to share a name that already exists.
		if l.instances[i], err = newInstance(path, i == 0); err != nil {
			_ = l.Close()
			return nil, err
		}
	}
	return l, nil
}

// newInstance creates an instance of the pipe and starts waiting for a client
// on it. The pipe is in byte-stream mode and is opened for overlapped I/O, so
// that a read and a write can be in flight together.
func newInstance(path *uint16, first bool) (*instance, error) {
	flags := uint32(windows.PIPE_ACCESS_DUPLEX | windows.FILE_FLAG_OVERLAPPED)
	if first {
		flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	mode := uint32(windows.PIPE_TYPE_BYTE | windows.PIPE_READMODE_BYTE | windows.PIPE_WAIT | windows.PIPE_REJECT_REMOTE_CLIENTS)
	handle, err := windows.CreateNamedPipe(path, flags, mode, windows.PIPE_UNLIMITED_INSTANCES, pipeBufferSize, pipeBufferSize, 0, nil)
	if err != nil {
		return nil, err
	}
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	in := &instance{handle: handle, overlapped: windows.Overlapped{HEvent: event}}
	err = windows.ConnectNamedPipe(handle, &in.overlapped)
	switch {
	case errors.Is(err, windows.ERROR_IO_PENDING):
	case err == nil, errors.Is(err, windows.ERROR_PIPE_CONNECTED), errors.Is(err, windows.ERROR_NO_DATA):
		// A client connected between the two calls, and may even have gone
		// again. Either way it is a connection to serve.
		in.immediate = true
		_ = windows.SetEvent(event)
	default:
		in.close()
		return nil, err
	}
	return in, nil
}

// close abandons an instance that no client was handed to.
func (in *instance) close() {
	if !in.immediate {
		// Cancel the connect and wait for the kernel to let go of the
		// overlapped structure.
		var n uint32
		_ = windows.CancelIoEx(in.handle, &in.overlapped)
		_ = windows.GetOverlappedResult(in.handle, &in.overlapped, &n, true)
	}
	_ = windows.CloseHandle(in.handle)
	_ = windows.CloseHandle(in.overlapped.HEvent)
}

func (l *pipeListener) Accept() (io.ReadWriteCloser, error) {
	if l.done {
		return nil, net.ErrClosed
	}
	handles := []windows.Handle{l.stop}
	for _, in := range l.instances {
		handles = append(handles, in.overlapped.HEvent)
	}
	signalled, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
	if err != nil {
		l.done = true
		return nil, err
	}
	if signalled == windows.WAIT_OBJECT_0 {
		l.done = true
		return nil, net.ErrClosed
	}

	slot := int(signalled-windows.WAIT_OBJECT_0) - 1
	in := l.instances[slot]
	if !in.immediate {
		var n uint32
		err = windows.GetOverlappedResult(in.handle, &in.overlapped, &n, false)
		in.immediate = true // nothing is pending any more
	}
	var next *instance
	if err == nil {
		next, err = newInstance(l.path, false)
	}
	if err != nil {
		l.done = true
		return nil, err
	}
	l.instances[slot] = next
	_ = windows.CloseHandle(in.overlapped.HEvent)
	// The standard library's file type races with itself when a read and a
	// write overlap on one pipe handle (ADR-0017), which a server-initiated
	// send during a pending read is. go-winio's does not, and closing it
	// unblocks a pending read.
	return winio.NewOpenFile(in.handle)
}

func (l *pipeListener) Interrupt() { _ = windows.SetEvent(l.stop) }

func (l *pipeListener) Close() error {
	for i, in := range l.instances {
		if in != nil {
			in.close()
			l.instances[i] = nil
		}
	}
	return windows.CloseHandle(l.stop)
}
