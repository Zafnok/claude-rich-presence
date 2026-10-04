package diag

import (
	"path/filepath"
	"sync"
)

const (
	// LogName is the name of the log file inside the log directory.
	LogName = "rich-presence.log"
	// rotatedSuffix is added to LogName to name the one predecessor kept.
	rotatedSuffix = ".1"
	// MaxLogSize is the cap on the log file, in bytes.
	MaxLogSize = 1 << 20
)

// LogPath returns the log file inside dir.
func LogPath(dir string) string {
	return filepath.Join(dir, LogName)
}

// FS is the part of the file system the log file needs. OSFS is the real
// one.
type FS interface {
	// MkdirAll creates dir and its parents, for the user alone.
	MkdirAll(dir string) error
	// Size returns the size of the file, and zero if there is none.
	Size(name string) (int64, error)
	// Append adds data to the end of the file, creating it for the user
	// alone if there is none, and leaves it closed.
	Append(name string, data []byte) error
	// Rename moves a file, replacing any file at the new name.
	Rename(from, to string) error
}

// File is the capped log file. Each Write is one message.
//
// A message is written whenever the file is below the cap, so the file never
// exceeds the cap plus one message. A message that finds the file at or over
// the cap first renames it to its predecessor, replacing the predecessor
// there was.
//
// The file is not held open: the host and every adapter write to the same
// one, and on Windows an open file cannot be renamed. The size is read
// before each message for the same reason.
type File struct {
	mu   sync.Mutex
	fsys FS
	name string
	max  int64
	// off is set when there is no log directory. Messages are discarded.
	off bool
}

// NewFile returns the log file in dir, capped at max bytes. If dir is empty
// or cannot be created, the file discards what is written to it.
func NewFile(fsys FS, dir string, max int64) *File {
	return &File{
		fsys: fsys,
		name: LogPath(dir),
		max:  max,
		off:  dir == "" || fsys.MkdirAll(dir) != nil,
	}
}

// Write appends one message. On any failure the message is lost and the
// error returned; the next message is tried afresh.
func (f *File) Write(p []byte) (int, error) {
	if f.off {
		return len(p), nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	size, err := f.fsys.Size(f.name)
	if err == nil && size >= f.max {
		err = f.fsys.Rename(f.name, f.name+rotatedSuffix)
	}
	if err == nil {
		err = f.fsys.Append(f.name, p)
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
