//go:build unix

package transport

import (
	"io/fs"
	"os"
	"syscall"
)

const (
	// errHeld is what openExclusive returns when another process holds the
	// lock.
	errHeld = syscall.EWOULDBLOCK
	// errRefused is the connect error for a socket file nobody listens on.
	errRefused = syscall.ECONNREFUSED
	// errNoDir is the connect error when the directory of the socket does
	// not exist, which on Unix is the same as for a missing file.
	errNoDir = syscall.ENOENT
)

// openExclusive opens the lock file and takes an advisory lock on it without
// blocking. The lock belongs to the open file, so the system drops it when
// the process exits.
func openExclusive(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

// dirOwner returns the user id that owns the directory.
func dirOwner(info fs.FileInfo) int {
	return int(info.Sys().(*syscall.Stat_t).Uid)
}
