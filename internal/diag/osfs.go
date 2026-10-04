package diag

import (
	"errors"
	"io/fs"
	"os"
)

// OSFS is the real file system.
type OSFS struct{}

// MkdirAll implements FS.
func (OSFS) MkdirAll(dir string) error {
	return os.MkdirAll(dir, 0o700)
}

// Size implements FS.
func (OSFS) Size(name string) (int64, error) {
	info, err := os.Stat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Append implements FS.
func (OSFS) Append(name string, data []byte) error {
	file, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return errors.Join(err, file.Close())
}

// Rename implements FS.
func (OSFS) Rename(from, to string) error {
	return os.Rename(from, to)
}
