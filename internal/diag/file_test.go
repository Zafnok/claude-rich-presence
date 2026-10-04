package diag_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
)

func TestLogPath(t *testing.T) {
	if got, want := diag.LogPath("logs"), filepath.Join("logs", "rich-presence.log"); got != want {
		t.Errorf("LogPath = %q, want %q", got, want)
	}
}

// messages are of uneven sizes, some larger than the cap used with them.
func messages() []string {
	var out []string
	for i := 0; i < 400; i++ {
		out = append(out, strings.Repeat("x", (i*37)%150)+"\n")
	}
	return out
}

func TestFileStaysWithinCapAndKeepsOnePredecessor(t *testing.T) {
	const limit = 100
	fsys := newMemFS()
	file := diag.NewFile(fsys, "logs", limit)
	name := diag.LogPath("logs")
	rotated := false
	for _, msg := range messages() {
		before := fsys.read(name)
		if n, err := file.Write([]byte(msg)); n != len(msg) || err != nil {
			t.Fatalf("Write = %d, %v", n, err)
		}
		after := fsys.read(name)
		if len(after) > limit+len(msg) {
			t.Fatalf("log is %d bytes after a message of %d, cap %d", len(after), len(msg), limit)
		}
		switch {
		case len(before) < limit:
			if after != before+msg {
				t.Fatalf("a message below the cap was not appended")
			}
		default:
			rotated = true
			if after != msg {
				t.Fatalf("a message at the cap did not start a new file")
			}
			if got := fsys.read(name + ".1"); got != before {
				t.Fatalf("the predecessor is not the file that was full")
			}
		}
		names := fsys.names()
		sort.Strings(names)
		if len(names) > 2 || names[0] != name || (len(names) == 2 && names[1] != name+".1") {
			t.Fatalf("files are %v", names)
		}
	}
	if !rotated {
		t.Fatal("the file never rotated")
	}
	if !reflect.DeepEqual(fsys.dirs, []string{"logs"}) {
		t.Errorf("created %v, want the log directory once", fsys.dirs)
	}
}

func TestFileOnTheRealFileSystem(t *testing.T) {
	const limit = 100
	dir := filepath.Join(t.TempDir(), "a", "logs")
	file := diag.NewFile(diag.OSFS{}, dir, limit)
	for _, msg := range messages() {
		if _, err := file.Write([]byte(msg)); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(diag.LogPath(dir))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > int64(limit+len(msg)) {
			t.Fatalf("log is %d bytes after a message of %d, cap %d", info.Size(), len(msg), limit)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if want := []string{diag.LogName, diag.LogName + ".1"}; !reflect.DeepEqual(names, want) {
		t.Errorf("log directory holds %v, want %v", names, want)
	}
}

func TestNoLogDirectoryDisablesLogging(t *testing.T) {
	tests := []struct {
		name  string
		dir   string
		err   error
		calls int
	}{
		{"cannot be created", "logs", errInjected, 1},
		{"is not known", "", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := newMemFS()
			fsys.mkdirErr = tt.err
			log := diag.Open(fsys, tt.dir, config.LogDebug, now)
			log.Error("m")
			log.Warn("m")
			if fsys.calls != tt.calls || len(fsys.names()) != 0 {
				t.Errorf("file system saw %d calls and holds %v", fsys.calls, fsys.names())
			}
			file := diag.NewFile(fsys, tt.dir, 10)
			if n, err := file.Write([]byte("abc")); n != 3 || err != nil {
				t.Errorf("Write = %d, %v, want the message discarded without error", n, err)
			}
		})
	}
}

func TestWriteFailureLosesOnlyThatMessage(t *testing.T) {
	tests := []struct {
		name string
		set  func(*memFS, error)
	}{
		{"size", func(m *memFS, err error) { m.sizeErr = err }},
		{"rename", func(m *memFS, err error) { m.renameErr = err }},
		{"append", func(m *memFS, err error) { m.appendErr = err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := newMemFS()
			name := diag.LogPath("logs")
			file := diag.NewFile(fsys, "logs", 4)
			if _, err := file.Write([]byte("full")); err != nil {
				t.Fatal(err)
			}
			tt.set(fsys, errInjected)
			if n, err := file.Write([]byte("lost")); n != 0 || !errors.Is(err, errInjected) {
				t.Errorf("Write = %d, %v, want the injected error", n, err)
			}
			// The full file is either still in place or already the
			// predecessor, and the lost message is in neither.
			if got := fsys.read(name) + fsys.read(name+".1"); got != "full" {
				t.Errorf("log and predecessor hold %q after a failed write", got)
			}

			// The logger carries on through the same failure.
			log := diag.NewLogger(file, config.LogDebug, now)
			log.Error("m")

			tt.set(fsys, nil)
			if _, err := file.Write([]byte("next")); err != nil {
				t.Fatal(err)
			}
			if got := fsys.read(name); got != "next" {
				t.Errorf("log holds %q, want the message after the failure", got)
			}
			if got := fsys.read(name + ".1"); got != "full" {
				t.Errorf("predecessor holds %q", got)
			}
		})
	}
}

func TestOSFS(t *testing.T) {
	fsys := diag.OSFS{}
	dir := t.TempDir()
	name := filepath.Join(dir, "f")

	if size, err := fsys.Size(name); size != 0 || err != nil {
		t.Errorf("Size of a missing file = %d, %v", size, err)
	}
	if _, err := fsys.Size(name + "\x00"); err == nil {
		t.Error("Size of an invalid name gave no error")
	}
	if err := fsys.Append(filepath.Join(dir, "missing", "f"), []byte("x")); err == nil {
		t.Error("Append in a missing directory gave no error")
	}
	for _, part := range []string{"ab", "c"} {
		if err := fsys.Append(name, []byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if size, err := fsys.Size(name); size != 3 || err != nil {
		t.Errorf("Size = %d, %v, want 3", size, err)
	}
	other := filepath.Join(dir, "g")
	if err := fsys.Append(other, []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Rename(name, other); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(other); string(data) != "abc" || err != nil {
		t.Errorf("after Rename the target holds %q, %v", data, err)
	}
	if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after Rename the source: %v", err)
	}
}

func TestLogFilesAreForTheUserAlone(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows does not have permission bits")
	}
	dir := filepath.Join(t.TempDir(), "logs")
	file := diag.NewFile(diag.OSFS{}, dir, 10)
	if _, err := file.Write([]byte("m")); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, diag.LogPath(dir): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got&^want != 0 {
			t.Errorf("%s has mode %04o, want no more than %04o", path, got, want)
		}
	}
}
