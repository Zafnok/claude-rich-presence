// Package cli dispatches commands and is the composition root: the only
// package that constructs real operating-system implementations and wires them
// to the rest of the program.
//
// It may import any package under internal except internal/testutil. Nothing
// under internal may import it.
//
// # Commands
//
//	mcp      serve the presence tools to Claude over standard input and output
//	status   print the running host's summary
//	doctor   run the checks of internal/diag and print the report
//	version  print the version
//	help     print the usage
//
// Run is the entry point, and main only calls it. The exit codes are 0 for
// success, 1 for a failure, 2 for a usage error, and 3 from status when no
// host is running. doctor exits 1 unless every check passes. A command that
// takes no arguments refuses one with a usage error.
//
// # The mcp command
//
// It loads the configuration, starts a host node unless presence is off,
// serves until the input closes or an interrupt or termination signal
// arrives, and exits 0. In that order it stops the adapter, which publishes
// the session's end, and then the node, which clears the presence it holds
// and gives up the lock and the socket.
//
// Presence is off when the configuration has enabled false, or when
// CLAUDE_CODE_REMOTE is true. The server then offers the same tools and
// answers them, and no lock file or socket is made. Presence also goes off,
// with the server unchanged, when the node cannot be started: no runtime
// directory can be found, or this binary's version is not one the control
// protocol carries. Presence never stops the server (ADR-0008).
//
// Standard output carries protocol messages and nothing else, including when
// the server fails. A failure of the server is a line on standard error with
// exit code 1. A panic is recovered, logged by class, and reported as a fixed
// line on standard error, because what it carried may be work content.
//
// The adapter is Claude Code's whatever client initializes: the surface is
// chosen from the client's name once there is a second adapter (CRP-050).
//
// # The status command
//
// It is not a node. It dials the control socket, says hello, asks for the
// status, reads one answer and closes, and holds no session. It prints what
// the host says of itself: the Discord connection, the number of sessions,
// the host's version and how long it has been host.
//
// # The real system
//
// The system type holds every real thing the commands reach for as plain
// values, and realSystem is where they are made. A test replaces the clock,
// the Discord dialer and the handful of others it must, and runs the rest for
// real, in a runtime directory of its own. hostConfig supplies the ports of
// the host node; the lock and the listener it returns are small wrappers,
// because the transport returns concrete types and the node asks for
// interfaces.
//
// The wiring is covered by the unit tests and by the end-to-end tests in
// test/e2e, which run every command of the built binary as a real process.
package cli
