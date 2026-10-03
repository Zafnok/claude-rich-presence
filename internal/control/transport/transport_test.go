package transport_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/transport"
)

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return err == nil
}

// roundTrip dials the socket, sends a line and reads it back. Something
// must be serving the socket by echoing.
func roundTrip(t *testing.T, socket string) {
	t.Helper()
	conn, err := transport.Dial(context.Background(), socket, patience)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(patience))
	if _, err := fmt.Fprintln(conn, "ping"); err != nil {
		t.Fatal(err)
	}
	got, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if got != "ping\n" {
		t.Errorf("read %q, want the line that was sent", got)
	}
}

// echo serves one connection on the listener by sending back what it reads.
// The returned channel yields the result of Accept.
func echo(listener *transport.Listener) <-chan error {
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err == nil {
			_, err = conn.Write([]byte(line))
		}
		done <- err
	}()
	return done
}

func TestPrepareCreatesTheDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if err := transport.Prepare(dir); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", dir)
	}
	if err := transport.Prepare(dir); err != nil {
		t.Errorf("Prepare() of an existing directory: %v", err)
	}
}

func TestPrepareRefusesAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := transport.Prepare(file); !errors.Is(err, transport.ErrNotDirectory) {
		t.Errorf("Prepare() error = %v, want ErrNotDirectory", err)
	}
}

func TestPrepareReportsADirectoryItCannotCreate(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := transport.Prepare(filepath.Join(file, "under-a-file"))
	if err == nil || !strings.Contains(err.Error(), "runtime directory") {
		t.Errorf("Prepare() error = %v, want one about the runtime directory", err)
	}
}

func TestCheckDir(t *testing.T) {
	cases := []struct {
		name      string
		mode      fs.FileMode
		owner     int
		self      int
		unixRules bool
		want      error
	}{
		{"owner-only directory", fs.ModeDir | 0o700, 1000, 1000, true, nil},
		{"owner read-only directory", fs.ModeDir | 0o500, 1000, 1000, true, nil},
		{"regular file", 0o600, 1000, 1000, true, transport.ErrNotDirectory},
		{"link", fs.ModeSymlink | 0o777, 1000, 1000, true, transport.ErrNotDirectory},
		{"owned by another user", fs.ModeDir | 0o700, 0, 1000, true, transport.ErrNotOwned},
		{"another user's open directory", fs.ModeDir | 0o777, 0, 1000, true, transport.ErrNotOwned},
		{"group can read", fs.ModeDir | 0o740, 1000, 1000, true, transport.ErrAccessible},
		{"group can enter", fs.ModeDir | 0o710, 1000, 1000, true, transport.ErrAccessible},
		{"everyone can read", fs.ModeDir | 0o704, 1000, 1000, true, transport.ErrAccessible},
		{"everyone can write", fs.ModeDir | 0o702, 1000, 1000, true, transport.ErrAccessible},
		{"the usual 0755", fs.ModeDir | 0o755, 1000, 1000, true, transport.ErrAccessible},
		{"sticky world-writable", fs.ModeDir | fs.ModeSticky | 0o777, 1000, 1000, true, transport.ErrAccessible},
		{"windows directory, bits not honoured", fs.ModeDir | 0o777, -1, -1, false, nil},
		{"windows file", 0o666, -1, -1, false, transport.ErrNotDirectory},
		{"windows link", fs.ModeSymlink | 0o777, -1, -1, false, transport.ErrNotDirectory},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := transport.CheckDir(c.mode, c.owner, c.self, c.unixRules)
			if !errors.Is(err, c.want) {
				t.Errorf("checkDir() = %v, want %v", err, c.want)
			}
			if c.want == nil && err != nil {
				t.Errorf("checkDir() = %v, want no error", err)
			}
		})
	}
}

func TestCheckDirNamesTheMode(t *testing.T) {
	err := transport.CheckDir(fs.ModeDir|0o755, 1000, 1000, true)
	if err == nil || !strings.Contains(err.Error(), "0755") || !strings.Contains(err.Error(), "0700") {
		t.Errorf("checkDir() = %v, want the mode found and the mode required", err)
	}
}

func TestLockIsExclusive(t *testing.T) {
	paths := testPaths(t)
	first := acquire(t, paths)

	if second, err := transport.Acquire(paths); !errors.Is(err, transport.ErrLocked) || second != nil {
		t.Fatalf("second Acquire() = %v, %v, want nil and ErrLocked", second, err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if err := first.Release(); err != nil {
		t.Errorf("second Release() error = %v", err)
	}
	third, err := transport.Acquire(paths)
	if err != nil {
		t.Fatalf("Acquire() after Release() error = %v", err)
	}
	if err := third.Release(); err != nil {
		t.Errorf("Release() error = %v", err)
	}
}

func TestAcquirePreparesTheDirectory(t *testing.T) {
	paths := testPaths(t)
	if exists(t, paths.Dir) {
		t.Fatal("the test directory already exists")
	}
	acquire(t, paths)
	if !exists(t, paths.Lock) {
		t.Errorf("no lock file at %s", paths.Lock)
	}
}

func TestAcquireRefusesAnUnsafeDirectory(t *testing.T) {
	paths := testPaths(t)
	if err := os.WriteFile(paths.Dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if lock, err := transport.Acquire(paths); !errors.Is(err, transport.ErrNotDirectory) || lock != nil {
		t.Errorf("Acquire() = %v, %v, want nil and ErrNotDirectory", lock, err)
	}
}

func TestAcquireReportsALockFileItCannotOpen(t *testing.T) {
	for name, lockPath := range map[string]string{
		"missing directory": filepath.Join("missing", "host.lock"),
		"zero byte in name": "host\x00.lock",
	} {
		t.Run(name, func(t *testing.T) {
			paths := testPaths(t)
			paths.Lock = filepath.Join(paths.Dir, lockPath)
			lock, err := transport.Acquire(paths)
			if err == nil || errors.Is(err, transport.ErrLocked) || lock != nil {
				t.Fatalf("Acquire() = %v, %v, want an error that is not ErrLocked", lock, err)
			}
			if !strings.Contains(err.Error(), "host lock") {
				t.Errorf("Acquire() error = %v, want it to name the host lock", err)
			}
		})
	}
}

func TestListenerAndDialerExchangeData(t *testing.T) {
	paths := testPaths(t)
	listener, err := acquire(t, paths).Listen()
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	served := echo(listener)
	roundTrip(t, paths.Socket)
	if err := <-served; err != nil {
		t.Errorf("server: %v", err)
	}
}

func TestClosingTheListenerRemovesTheSocket(t *testing.T) {
	paths := testPaths(t)
	lock := acquire(t, paths)
	listener, err := lock.Listen()
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	if !exists(t, paths.Socket) {
		t.Fatalf("no socket file at %s while listening", paths.Socket)
	}
	if got := listener.Addr().String(); got != paths.Socket {
		t.Errorf("Addr() = %q, want %q", got, paths.Socket)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if exists(t, paths.Socket) {
		t.Error("the socket file is still there after Close()")
	}
	if err := listener.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
	if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Errorf("Accept() after Close() error = %v, want net.ErrClosed", err)
	}

	again, err := lock.Listen()
	if err != nil {
		t.Fatalf("Listen() after Close() error = %v", err)
	}
	defer again.Close()
	served := echo(again)
	roundTrip(t, paths.Socket)
	if err := <-served; err != nil {
		t.Errorf("server: %v", err)
	}
}

// The socket file goes first and the lock second, so that nobody can take
// the lock while a socket of the old holder is still on disk.
func TestReleaseClosesTheListenerBeforeTheLock(t *testing.T) {
	paths := testPaths(t)
	lock := acquire(t, paths)
	listener, err := lock.Listen()
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if exists(t, paths.Socket) {
		t.Error("the socket file is still there after Release()")
	}
	if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Errorf("Accept() after Release() error = %v, want net.ErrClosed", err)
	}
	if err := listener.Close(); err != nil {
		t.Errorf("Close() after Release() error = %v", err)
	}
	acquire(t, paths)
}

func TestReleaseAfterTheListenerClosed(t *testing.T) {
	paths := testPaths(t)
	lock := acquire(t, paths)
	listener, err := lock.Listen()
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Errorf("Release() error = %v", err)
	}
}

// A second Listen would remove the socket file under the first listener,
// which leaves a host nobody can reach.
func TestListenTwiceIsRefused(t *testing.T) {
	paths := testPaths(t)
	lock := acquire(t, paths)
	listener, err := lock.Listen()
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	if second, err := lock.Listen(); !errors.Is(err, transport.ErrListening) || second != nil {
		t.Fatalf("second Listen() = %v, %v, want nil and ErrListening", second, err)
	}
	served := echo(listener)
	roundTrip(t, paths.Socket)
	if err := <-served; err != nil {
		t.Errorf("server: %v", err)
	}
}

// Without the lock there is no way to the socket file: Acquire gives no
// lock to listen with, and a released lock refuses.
func TestOnlyTheLockHolderCanRemoveTheSocket(t *testing.T) {
	paths := testPaths(t)
	former := acquire(t, paths)
	if err := former.Release(); err != nil {
		t.Fatal(err)
	}

	holder := startHelper(t, helperListen, paths)
	holder.expect("listening")

	if lock, err := transport.Acquire(paths); !errors.Is(err, transport.ErrLocked) || lock != nil {
		t.Fatalf("Acquire() = %v, %v, want nil and ErrLocked", lock, err)
	}
	if listener, err := former.Listen(); !errors.Is(err, transport.ErrLockReleased) || listener != nil {
		t.Fatalf("Listen() on a released lock = %v, %v, want nil and ErrLockReleased", listener, err)
	}
	if !exists(t, paths.Socket) {
		t.Fatal("the holder's socket file is gone")
	}
	roundTrip(t, paths.Socket)
}

func TestListenReportsAStaleSocketItCannotRemove(t *testing.T) {
	paths := testPaths(t)
	lock := acquire(t, paths)
	if err := os.MkdirAll(filepath.Join(paths.Socket, "occupied"), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := lock.Listen()
	if err == nil || listener != nil || !strings.Contains(err.Error(), "stale control socket") {
		t.Errorf("Listen() = %v, %v, want an error about the stale socket", listener, err)
	}
}

func TestListenReportsASocketItCannotBind(t *testing.T) {
	paths := testPaths(t)
	paths.Socket = filepath.Join(paths.Dir, "missing", "control.sock")
	listener, err := acquire(t, paths).Listen()
	if err == nil || listener != nil || !strings.Contains(err.Error(), "control socket") {
		t.Errorf("Listen() = %v, %v, want an error about the control socket", listener, err)
	}
}

func TestListenReplacesALeftoverFile(t *testing.T) {
	paths := testPaths(t)
	lock := acquire(t, paths)
	if err := os.WriteFile(paths.Socket, []byte("leftover"), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := lock.Listen()
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()
	served := echo(listener)
	roundTrip(t, paths.Socket)
	if err := <-served; err != nil {
		t.Errorf("server: %v", err)
	}
}

// The resolver's limit is the platform's: a socket path at the limit binds
// and connects, and one byte more does neither.
func TestSocketPathLimitIsThePlatformLimit(t *testing.T) {
	limit := socketPathLimit()

	at := testPaths(t)
	at.Socket = padded(at.Dir+string(os.PathSeparator), limit)
	listener, err := acquire(t, at).Listen()
	if err != nil {
		t.Fatalf("Listen() on a path of %d bytes: %v", limit, err)
	}
	defer listener.Close()
	served := echo(listener)
	roundTrip(t, at.Socket)
	if err := <-served; err != nil {
		t.Errorf("server: %v", err)
	}

	over := testPaths(t)
	over.Socket = padded(over.Dir+string(os.PathSeparator), limit+1)
	if listener, err := acquire(t, over).Listen(); err == nil {
		listener.Close()
		t.Errorf("Listen() on a path of %d bytes succeeded, want it over the limit", limit+1)
	}
	_, err = transport.Dial(context.Background(), over.Socket, patience)
	if err == nil || errors.Is(err, transport.ErrNoHost) {
		t.Errorf("Dial() on a path over the limit: %v, want an error that is not ErrNoHost", err)
	}
}

func TestDialWithNoRuntimeDirectoryIsNoHost(t *testing.T) {
	paths := testPaths(t)
	if _, err := transport.Dial(context.Background(), paths.Socket, patience); !errors.Is(err, transport.ErrNoHost) {
		t.Errorf("Dial() error = %v, want ErrNoHost", err)
	}
}

func TestDialWithNoSocketFileIsNoHost(t *testing.T) {
	paths := testPaths(t)
	if err := transport.Prepare(paths.Dir); err != nil {
		t.Fatal(err)
	}
	const timeout = 2 * time.Second
	start := time.Now()
	conn, err := transport.Dial(context.Background(), paths.Socket, timeout)
	if !errors.Is(err, transport.ErrNoHost) || conn != nil {
		t.Fatalf("Dial() = %v, %v, want nil and ErrNoHost", conn, err)
	}
	if elapsed := time.Since(start); elapsed >= timeout {
		t.Errorf("Dial() took %v, want it within the timeout of %v", elapsed, timeout)
	}
}

func TestDialRefusesToWaitWithoutLimit(t *testing.T) {
	paths := testPaths(t)
	listener, err := acquire(t, paths).Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	for _, timeout := range []time.Duration{0, -time.Second} {
		conn, err := transport.Dial(context.Background(), paths.Socket, timeout)
		if !errors.Is(err, transport.ErrNoTimeout) || conn != nil {
			t.Errorf("Dial() with timeout %v = %v, %v, want nil and ErrNoTimeout", timeout, conn, err)
		}
	}
}

func TestDialAfterTheListenerClosedIsNoHost(t *testing.T) {
	paths := testPaths(t)
	listener, err := acquire(t, paths).Listen()
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Dial(context.Background(), paths.Socket, patience); !errors.Is(err, transport.ErrNoHost) {
		t.Errorf("Dial() error = %v, want ErrNoHost", err)
	}
}

func TestDialStopsWhenTheContextEnds(t *testing.T) {
	paths := testPaths(t)
	listener, err := acquire(t, paths).Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn, err := transport.Dial(ctx, paths.Socket, patience)
	if err == nil || errors.Is(err, transport.ErrNoHost) || conn != nil {
		t.Errorf("Dial() = %v, %v, want an error that is not ErrNoHost", conn, err)
	}
}

func TestClassifyDialError(t *testing.T) {
	timeout := &net.OpError{Op: "dial", Net: "unix", Err: context.DeadlineExceeded}
	cases := []struct {
		name   string
		err    error
		noHost bool
	}{
		{"socket file missing", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", fs.ErrNotExist)}, true},
		{"connection refused", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", transport.ErrRefused)}, true},
		{"directory missing", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", transport.ErrNoDir)}, true},
		{"timed out", timeout, false},
		{"cancelled", context.Canceled, false},
		{"permission denied", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", fs.ErrPermission)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := transport.ClassifyDialError(c.err)
			if errors.Is(got, transport.ErrNoHost) != c.noHost {
				t.Errorf("classifyDialError(%v) = %v, want ErrNoHost: %v", c.err, got, c.noHost)
			}
			if !errors.Is(got, c.err) {
				t.Errorf("classifyDialError(%v) = %v, which lost the cause", c.err, got)
			}
		})
	}
}

// Two processes start at once and both go for the lock. Exactly one may
// have it.
func TestTwoProcessesContendForTheLock(t *testing.T) {
	paths := testPaths(t)
	helpers := []*helper{
		startHelper(t, helperHold, paths),
		startHelper(t, helperHold, paths),
	}
	counts := map[string]int{}
	for _, h := range helpers {
		counts[h.line()]++
	}
	if counts["locked"] != 1 || counts["busy"] != 1 {
		t.Fatalf("helpers reported %v, want one locked and one busy", counts)
	}
	if lock, err := transport.Acquire(paths); !errors.Is(err, transport.ErrLocked) || lock != nil {
		t.Errorf("Acquire() while a process holds the lock = %v, %v, want nil and ErrLocked", lock, err)
	}
	for _, h := range helpers {
		h.finish()
	}
	acquire(t, paths)
}

func TestKillingTheHolderFreesTheLockForAWaitingProcess(t *testing.T) {
	paths := testPaths(t)
	holder := startHelper(t, helperHold, paths)
	holder.expect("locked")

	waiter := startHelper(t, helperWait, paths)
	waiter.expect("waiting")
	if lock, err := transport.Acquire(paths); !errors.Is(err, transport.ErrLocked) || lock != nil {
		t.Fatalf("Acquire() while the holder lives = %v, %v, want nil and ErrLocked", lock, err)
	}

	holder.kill()
	waiter.expect("locked")
	waiter.finish()
}

// A host that is killed leaves its socket file behind. The next holder
// listens all the same.
func TestSocketLeftByADeadProcessDoesNotPreventListening(t *testing.T) {
	paths := testPaths(t)
	dead := startHelper(t, helperListen, paths)
	dead.expect("listening")
	roundTrip(t, paths.Socket)
	dead.kill()

	if !exists(t, paths.Socket) {
		t.Fatal("the killed process left no socket file, so this test shows nothing")
	}
	if _, err := transport.Dial(context.Background(), paths.Socket, patience); !errors.Is(err, transport.ErrNoHost) {
		t.Errorf("Dial() to a dead host's socket: %v, want ErrNoHost", err)
	}

	listener, err := acquireEventually(t, paths).Listen()
	if err != nil {
		t.Fatalf("Listen() over a dead host's socket: %v", err)
	}
	defer listener.Close()
	served := echo(listener)
	roundTrip(t, paths.Socket)
	if err := <-served; err != nil {
		t.Errorf("server: %v", err)
	}
}

func TestHelperReportsWhatItCannotDo(t *testing.T) {
	paths := testPaths(t)
	if err := os.WriteFile(paths.Dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{helperHold, helperListen, helperWait, "no-such-mode"} {
		h := startHelper(t, mode, paths)
		line := h.line()
		if mode == helperWait && line == "waiting" {
			line = h.line()
		}
		if !strings.HasPrefix(line, "error:") {
			t.Errorf("helper %q printed %q, want an error", mode, line)
		}
	}

	h := startHelper(t, helperHold, transport.Paths{Dir: "relative"})
	if line := h.line(); !strings.HasPrefix(line, "error:") {
		t.Errorf("helper with a relative directory printed %q, want an error", line)
	}
}
