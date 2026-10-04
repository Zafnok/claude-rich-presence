// Package transport dials the Discord IPC endpoint: a named pipe on Windows, a
// Unix socket elsewhere, with endpoint discovery. Platform code lives in files
// suffixed _windows.go and _unix.go behind an interface declared in a
// platform-neutral file.
//
// It must not import internal/discord/session, internal/host, internal/cli or
// any adapter, and must hold no policy.
//
// # Discovery
//
// [Prefixes] lists where a Discord client may be listening, and [Candidates]
// adds the indices 0 to 9 to each. A [Dialer] tries them in order and returns
// the first that connects. When several Discord builds are running, the first
// found wins.
//
// # Windows
//
// The pipe is opened and served by github.com/Microsoft/go-winio, the one
// module outside the standard library and golang.org/x that this project
// links, and only on Windows (ADR-0017). The standard library can open the
// pipe for overlapped I/O from Go 1.26, and deadlines, a write during a
// pending read and a close during a pending read all work. But its file type
// updates one offset from both the reading and the writing goroutine without
// synchronisation, which the race detector reports.
//
// A pipe that exists but has no free instance answers ERROR_PIPE_BUSY. The
// dialer gives such a pipe the attempt timeout to free an instance and then
// moves on to the next candidate, as it does for a pipe that does not exist.
// If no candidate connects, the caller is told Discord is not running and
// tries again on its own schedule.
package transport
