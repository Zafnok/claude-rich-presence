package session

// The timeouts, for tests that advance the fake clock past them.
const (
	HandshakeTimeout = handshakeTimeout
	WriteTimeout     = writeTimeout
	ClearTimeout     = clearTimeout
)

var Jittered = jittered
