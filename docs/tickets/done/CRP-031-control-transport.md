---
id: CRP-031
title: Control transport and the host lock
milestone: M3 Host
type: feature
status: done
priority: P0
blocked_by: [CRP-004, CRP-005]
blocks: [CRP-032]
model: claude-opus-5-5
effort: high
size: M
---

# CRP-031: Control transport and the host lock

## Goal

The operating-system pieces under the control channel: where the socket and lock live, an exclusive lock that the system releases when its holder dies, and a listener and dialer for the socket.

## Context

[ADR-0005](../../architecture/adr/0005-presence-host-election.md) elects the host with a lock, and [ADR-0006](../../architecture/adr/0006-control-channel.md) fixes the transport and the directory rules. The lock is what makes stale-socket cleanup safe.

CRP-002 found that the first Windows location, `%LOCALAPPDATA%\rich-presence`, does not work: Claude Desktop redirects it for every process it starts, and a Unix socket cannot be bound or connected there. ADR-0006 now puts the Windows runtime directory under `%TEMP%`. The evidence is in the [findings](../../research/crp-002-desktop-extension.md).

## Scope

Package `internal/control/transport`:

- **Runtime directory resolver**, a pure function of operating system and environment, implementing the table in ADR-0006: the preferred directory, the fallback, the override variable, and the short hashed fallback when the socket path would exceed the platform limit.
- **Directory preparation**: create with owner-only permissions; on Unix, refuse a directory not owned by the current user or accessible to others.
- **Host lock** behind an interface, with two implementations:
  - Unix: an advisory lock on the lock file, non-blocking.
  - Windows: opening the lock file with no sharing.
  
  Both are released by the operating system when the process exits for any reason.
- **Listener**: only callable by the lock holder. Removes a stale socket file, then listens.
- **Dialer**: connect with a timeout; distinguish "no host" from other errors.
- Closing the listener removes the socket file. Releasing the lock happens after that.

## Out of scope

- Messages, which are CRP-030.
- The election loop and what happens over connections, which is CRP-032.

## Acceptance criteria

- [x] The resolver is tested for all three operating systems on every operating system, including the override, each fallback, and a path at, one under and one over the length limit.
- [x] For a Windows environment, the resolver returns `%TEMP%\rich-presence`, and never a folder directly under `%LOCALAPPDATA%` or `%APPDATA%`.
- [x] Two processes contend for the lock and exactly one gets it. Tested with a real second process on each operating system.
- [x] When the lock holder is killed, a waiting process can take the lock. Tested with a real process on each operating system.
- [x] A leftover socket file from a dead process does not prevent a new holder from listening.
- [x] A process that does not hold the lock cannot remove the socket file through this package's API.
- [x] On Unix, a runtime directory with group or world access is refused with a clear error.
- [x] A dial with no listener returns the "no host" error within the timeout.
- [x] Listener and dialer exchange data on Linux, macOS and Windows in CI.
- [x] Only the standard library is used. If the Windows lock or socket cannot be done with it, stop and record why before reaching for `golang.org/x/sys`.
- [x] Platform files contain only the system call and its error mapping.

## Notes for the implementer

- The second-process tests can re-execute the test binary with an environment variable that selects a helper mode. Keep that helper covered.
- Unix sockets on Windows live in the file system like elsewhere but do not honour Unix permission bits. State what protects the directory on Windows, which is the default access control on the user's temporary directory, and test what can be tested.
- On Windows, resolve the temporary directory from the environment passed in, not from a system call, so the resolver stays a pure function. CRP-002 saw the same `TEMP` value in a terminal, in a Code-tab session and in a server started by Claude Desktop.
- A socket file removed while its listener is still running leaves a host nobody can reach: followers are refused the lock and refused the connection. CRP-002 did this by accident. Nothing but the lock holder may remove the file, which is already a criterion above.
- Check the macOS socket path limit against the temporary directory macOS actually assigns, which is long. The hashed fallback is likely to be the common path there.
- On Windows, deleting a socket file that another process still has open behaves differently than on Unix. Test the stale-file case for real.

## Why this model and effort

Three operating systems, real multi-process behaviour, and security-relevant permission checks.

## References

- [ADR-0005](../../architecture/adr/0005-presence-host-election.md), [ADR-0006](../../architecture/adr/0006-control-channel.md)
- [Risk register](../../architecture/risks.md), R4 and R10
