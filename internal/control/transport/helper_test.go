package transport_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/transport"
)

// envHelper selects a helper mode. When it is set, the test binary does not
// run tests: it plays a second process in the election, so that locks are
// contended and holders are killed for real.
const envHelper = "RICH_PRESENCE_TRANSPORT_HELPER"

// Helper modes.
const (
	// helperHold tries the lock once, reports "locked" or "busy", and holds
	// it until its standard input closes.
	helperHold = "hold"
	// helperListen takes the lock and listens, reports "listening", and
	// echoes what each connection sends. It never cleans up.
	helperListen = "listen"
	// helperWait reports "waiting", retries the lock until it gets it, and
	// reports "locked".
	helperWait = "wait"
)

// patience bounds every wait on another process.
const patience = 30 * time.Second

func TestMain(m *testing.M) {
	if mode := os.Getenv(envHelper); mode != "" {
		os.Exit(runHelper(mode))
	}
	os.Exit(m.Run())
}

func runHelper(mode string) int {
	paths, err := transport.Locate(os.Getenv)
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	switch mode {
	case helperHold:
		lock, err := transport.Acquire(paths)
		if errors.Is(err, transport.ErrLocked) {
			fmt.Println("busy")
			return 0
		}
		if err != nil {
			fmt.Println("error:", err)
			return 1
		}
		fmt.Println("locked")
		io.Copy(io.Discard, os.Stdin)
		lock.Release()
	case helperListen:
		lock, err := transport.Acquire(paths)
		if err != nil {
			fmt.Println("error:", err)
			return 1
		}
		listener, err := lock.Listen()
		if err != nil {
			fmt.Println("error:", err)
			return 1
		}
		fmt.Println("listening")
		for {
			conn, err := listener.Accept()
			if err != nil {
				return 1
			}
			go io.Copy(conn, conn)
		}
	case helperWait:
		fmt.Println("waiting")
		deadline := time.Now().Add(patience)
		for {
			lock, err := transport.Acquire(paths)
			if err == nil {
				fmt.Println("locked")
				io.Copy(io.Discard, os.Stdin)
				lock.Release()
				return 0
			}
			if !errors.Is(err, transport.ErrLocked) || time.Now().After(deadline) {
				fmt.Println("error:", err)
				return 1
			}
			time.Sleep(5 * time.Millisecond)
		}
	default:
		fmt.Println("error: unknown helper mode", mode)
		return 1
	}
	return 0
}

// helper is a second process running this test binary in a helper mode.
type helper struct {
	t     *testing.T
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
}

// startHelper starts a helper whose runtime directory is paths.Dir.
func startHelper(t *testing.T, mode string, paths transport.Paths) *helper {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	// A test binary built for coverage warns on exit unless it has somewhere
	// to write its counters. The helper's are not needed.
	cmd.Env = append(os.Environ(), envHelper+"="+mode, transport.EnvRuntimeDir+"="+paths.Dir, "GOCOVERDIR="+t.TempDir())
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	h := &helper{t: t, cmd: cmd, stdin: stdin, lines: make(chan string, 16)}
	go func() {
		defer close(h.lines)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			h.lines <- scanner.Text()
		}
	}()
	t.Cleanup(h.kill)
	return h
}

// line returns the next line the helper prints.
func (h *helper) line() string {
	h.t.Helper()
	select {
	case line, ok := <-h.lines:
		if !ok {
			h.t.Fatal("helper exited without printing the expected line")
		}
		return line
	case <-time.After(patience):
		h.t.Fatal("helper printed nothing in time")
		return ""
	}
}

// expect fails the test unless the helper's next line is want.
func (h *helper) expect(want string) {
	h.t.Helper()
	if got := h.line(); got != want {
		h.t.Fatalf("helper printed %q, want %q", got, want)
	}
}

// finish closes the helper's input, which lets it release what it holds and
// exit, and waits for it.
func (h *helper) finish() {
	h.t.Helper()
	h.stdin.Close()
	if err := h.cmd.Wait(); err != nil {
		h.t.Errorf("helper exited with %v", err)
	}
}

// kill ends the helper with no chance to clean up, and waits until it is
// gone. Killing a helper that already ended is harmless.
func (h *helper) kill() {
	h.cmd.Process.Kill()
	h.cmd.Wait()
}

// testPaths returns runtime files in a fresh directory short enough for a
// socket on every operating system. The directory does not exist yet.
func testPaths(t *testing.T) transport.Paths {
	t.Helper()
	root := "/tmp"
	if runtime.GOOS == "windows" {
		root = os.TempDir()
	}
	base, err := os.MkdirTemp(root, "rp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	dir := base + string(os.PathSeparator) + "run"
	paths, err := transport.Resolve(runtime.GOOS, env(transport.EnvRuntimeDir, dir), os.Getuid())
	if err != nil {
		t.Fatal(err)
	}
	if paths.Dir != dir {
		t.Fatalf("test directory %q is too long for a socket: resolved to %q", dir, paths.Dir)
	}
	return paths
}

// acquire takes the lock or fails the test, and releases it when the test
// ends.
func acquire(t *testing.T, paths transport.Paths) *transport.HostLock {
	t.Helper()
	lock, err := transport.Acquire(paths)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	t.Cleanup(func() { lock.Release() })
	return lock
}

// acquireEventually retries the lock until the operating system has let go
// of a dead holder's.
func acquireEventually(t *testing.T, paths transport.Paths) *transport.HostLock {
	t.Helper()
	deadline := time.Now().Add(patience)
	for {
		lock, err := transport.Acquire(paths)
		if err == nil {
			t.Cleanup(func() { lock.Release() })
			return lock
		}
		if !errors.Is(err, transport.ErrLocked) || time.Now().After(deadline) {
			t.Fatalf("Acquire() error = %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
