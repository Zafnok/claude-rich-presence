// Package session manages the Discord connection: connect, reconnect, apply an
// activity, clear it. It reaches the pipe through a port.
//
// It may import internal/domain and internal/discord/codec. It must not import
// internal/discord/transport, internal/host, internal/cli or any adapter; the
// real dialer is handed to it by internal/cli.
package session
