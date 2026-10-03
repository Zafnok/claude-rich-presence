// Package fakediscord is a scripted Discord IPC server for tests.
//
// It is test code and is not in the measured set. Production packages must not
// import it, and it must not hold production logic.
//
// It listens where a Discord client would, under a name unique to the test: a
// Unix socket on Linux and macOS, a named pipe on Windows. By default it
// behaves like Discord for the two things this project uses: it answers a
// valid handshake with a ready event, and acknowledges each set-activity
// command with the same nonce. A [Behavior] and the Send methods make it
// deviate. Everything it receives is recorded as an [Event].
//
// The framing here is written from Discord's documentation and is deliberately
// independent of internal/discord/codec, which it must never import, so that
// the two implementations check each other (ADR-0004).
//
// # What is emulated
//
//   - Handshake, opcode 0: version 1 and a non-empty client id get a READY
//     dispatch. Anything else gets a close frame, code 4004 or 4000, and the
//     connection is closed.
//   - SET_ACTIVITY in a frame, opcode 1: answered with the same command, the
//     same nonce and the activity as data.
//   - Any other command: answered with an ERROR event, code 4002.
//   - Ping, opcode 3: answered with a pong carrying the same payload.
//   - Close, opcode 2: the connection is closed.
//   - Anything that cannot be understood, a frame over 64 KiB, or a command
//     before the handshake: a close frame, code 1003, and the connection is
//     closed. Discord does not document this case; the code is the one the
//     arrpc reimplementation uses.
//
// # Windows
//
// A named pipe needs one instance per client. The server keeps eight waiting,
// so up to eight clients can connect before it has accepted any. A ninth is
// told the pipe is busy, as a client of a real Discord can be.
package fakediscord
