package transport

import (
	"io/fs"
	"os"
	"syscall"
)

const (
	// errHeld is what openExclusive returns when another process has the
	// lock file open: ERROR_SHARING_VIOLATION, which the syscall package
	// does not name.
	errHeld = syscall.Errno(32)
	// errRefused is the connect error for a socket file nobody listens on:
	// WSAECONNREFUSED, which the syscall package does not name.
	errRefused = syscall.Errno(10061)
	// errNoDir is the connect error when the directory of the socket does
	// not exist: WSAENETDOWN, observed on Windows 11. A missing file in an
	// existing directory gives errRefused.
	errNoDir = syscall.Errno(10050)
)

// openExclusive opens the lock file with no sharing, so that a second open
// fails while the first handle exists. The system closes the handle when
// the process exits.
func openExclusive(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil,
		syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

// dirOwner has no meaning on Windows, where ownership is not checked. It
// returns what os.Getuid returns there.
func dirOwner(fs.FileInfo) int {
	return -1
}
