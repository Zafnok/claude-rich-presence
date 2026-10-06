package transport

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
)

// Errors returned by Prepare, and by Dial inside an ErrUnsafeDir.
var (
	// ErrNotDirectory reports a runtime directory that is a file or a link.
	ErrNotDirectory = errors.New("is not a directory")
	// ErrNotOwned reports a runtime directory owned by another user.
	ErrNotOwned = errors.New("is owned by another user")
	// ErrAccessible reports a runtime directory that its group or everyone
	// can access.
	ErrAccessible = errors.New("is accessible to other users; it must be owner-only (mode 0700)")
)

// Prepare creates the runtime directory with owner-only permissions if it
// is missing, and refuses one that is unsafe.
//
// On Unix it refuses a directory that the current user does not own or that
// has any group or world permission. It never repairs one: a directory
// somebody else made is not trusted.
//
// Windows does not honour Unix permission bits, so there is no such check
// there. What protects the directory on Windows is the access control it
// inherits from the user's temporary directory, which by default admits the
// user, administrators and the system.
func Prepare(dir string) error {
	mkdirErr := os.MkdirAll(dir, 0o700)
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("runtime directory: %w", errors.Join(mkdirErr, err))
	}
	if err := checkInfo(info, os.Getuid(), runtime.GOOS != "windows"); err != nil {
		return fmt.Errorf("runtime directory %s %w", dir, err)
	}
	return nil
}

// checkInfo applies checkDir to what Lstat said of a directory. Prepare and
// Dial both come through here, so that a host and a follower refuse the
// same directories. info must come from Lstat and not Stat, so that a link
// is seen as a link.
func checkInfo(info fs.FileInfo, self int, unixRules bool) error {
	return checkDir(info.Mode(), dirOwner(info), self, unixRules)
}

// checkDir decides whether a directory with this mode and owner may hold
// the runtime files. unixRules turns on the ownership and permission checks.
func checkDir(mode fs.FileMode, owner, self int, unixRules bool) error {
	if !mode.IsDir() {
		return ErrNotDirectory
	}
	if !unixRules {
		return nil
	}
	if owner != self {
		return ErrNotOwned
	}
	if mode.Perm()&0o077 != 0 {
		return fmt.Errorf("%w, and has mode %04o", ErrAccessible, mode.Perm())
	}
	return nil
}
