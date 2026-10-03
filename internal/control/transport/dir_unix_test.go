//go:build unix

package transport_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/control/transport"
)

func TestPrepareCreatesAnOwnerOnlyDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	if err := transport.Prepare(dir); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("mode = %04o, want 0700", perm)
	}
}

func TestPrepareRefusesGroupOrWorldAccess(t *testing.T) {
	for _, perm := range []os.FileMode{0o750, 0o710, 0o705, 0o701, 0o770, 0o777, 0o755} {
		t.Run(fmt.Sprintf("%04o", perm), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "run")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			// Chmod, because the mode given to Mkdir is cut by the umask.
			if err := os.Chmod(dir, perm); err != nil {
				t.Fatal(err)
			}
			err := transport.Prepare(dir)
			if !errors.Is(err, transport.ErrAccessible) {
				t.Fatalf("Prepare() error = %v, want ErrAccessible", err)
			}
			for _, part := range []string{dir, fmt.Sprintf("%04o", perm), "0700"} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("Prepare() error = %q, want it to mention %q", err, part)
				}
			}
			info, err := os.Lstat(dir)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != perm {
				t.Errorf("Prepare() changed the mode to %04o", info.Mode().Perm())
			}

			paths := transport.Paths{Dir: dir, Socket: filepath.Join(dir, "s"), Lock: filepath.Join(dir, "l")}
			if lock, err := transport.Acquire(paths); !errors.Is(err, transport.ErrAccessible) || lock != nil {
				t.Errorf("Acquire() = %v, %v, want nil and ErrAccessible", lock, err)
			}
		})
	}
}

func TestPrepareRefusesALink(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := transport.Prepare(link); !errors.Is(err, transport.ErrNotDirectory) {
		t.Errorf("Prepare() error = %v, want ErrNotDirectory", err)
	}
}

// The root directory belongs to root, so to anyone else it is a directory
// owned by another user.
func TestPrepareRefusesAnotherUsersDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, which owns every directory this test could use")
	}
	if err := transport.Prepare("/"); !errors.Is(err, transport.ErrNotOwned) {
		t.Errorf("Prepare(\"/\") error = %v, want ErrNotOwned", err)
	}
}
