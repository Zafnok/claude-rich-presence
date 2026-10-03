// Package transport provides the socket path, listener, dialer and host lock
// of the control channel. Platform code lives in files suffixed _windows.go
// and _unix.go behind an interface declared in a platform-neutral file.
//
// It must not import internal/host, internal/cli or any adapter, and must
// hold no policy.
package transport
