// Package transport dials the Discord IPC endpoint: a named pipe on Windows, a
// Unix socket elsewhere, with endpoint discovery. Platform code lives in files
// suffixed _windows.go and _unix.go behind an interface declared in a
// platform-neutral file.
//
// It must not import internal/discord/session, internal/host, internal/cli or
// any adapter, and must hold no policy.
package transport
