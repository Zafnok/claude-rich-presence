package diag_test

import (
	"errors"
	"strings"
	"sync"
	"time"
)

var errInjected = errors.New("injected")

// start is the time every test logger stamps its lines with.
var start = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func now() time.Time { return start }

// memFS is a file system in memory. An error set for an operation is
// returned by it, and the operation does nothing.
type memFS struct {
	mu    sync.Mutex
	dirs  []string
	files map[string][]byte
	calls int

	mkdirErr, sizeErr, appendErr, renameErr error
}

func newMemFS() *memFS { return &memFS{files: map[string][]byte{}} }

func (m *memFS) MkdirAll(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.mkdirErr != nil {
		return m.mkdirErr
	}
	m.dirs = append(m.dirs, dir)
	return nil
}

func (m *memFS) Size(name string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return int64(len(m.files[name])), m.sizeErr
}

func (m *memFS) Append(name string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.appendErr != nil {
		return m.appendErr
	}
	m.files[name] = append(m.files[name], data...)
	return nil
}

func (m *memFS) Rename(from, to string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.renameErr != nil {
		return m.renameErr
	}
	m.files[to] = m.files[from]
	delete(m.files, from)
	return nil
}

func (m *memFS) read(name string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return string(m.files[name])
}

func (m *memFS) names() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var names []string
	for name := range m.files {
		names = append(names, name)
	}
	return names
}

// lines is a writer that keeps each Write as one line.
type lines struct {
	mu  sync.Mutex
	got []string
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.got = append(l.got, string(p))
	return len(p), nil
}

func (l *lines) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.got, "")
}
