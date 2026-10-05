// Package mcpclient is a scripted Model Context Protocol client for end-to-end
// tests.
//
// It is test code and is not in the measured set. Production packages must not
// import it, and it must not hold production logic.
//
// A [Client] starts a binary that serves the protocol on its standard streams,
// with an environment the test gives in full, performs initialize under a
// client name the test chooses, calls tools, and ends the process either by
// closing its input, as a client does, or by killing it. Each request waits
// for its response, and a line on standard output that is not a protocol
// message fails the test.
//
// The messages are written here from the specification and share nothing with
// internal/mcp, which this package must never import, so that the two check
// each other (ADR-0004).
package mcpclient
