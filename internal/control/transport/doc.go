// Package transport provides the socket path, listener, dialer and host lock
// of the control channel (ADR-0005, ADR-0006).
//
// Locate finds the runtime files. Acquire tries the host lock. The process
// that gets it calls Listen on the lock; every other process calls Dial.
// Listen is a method of the held lock, so that only the lock holder can
// remove the socket file.
//
// Platform code lives in platform_windows.go and platform_unix.go, which hold
// only the system call and the errors it returns. Every decision is in the
// platform-neutral files.
//
// It must not import internal/host, internal/cli or any adapter, and must
// hold no policy.
package transport
