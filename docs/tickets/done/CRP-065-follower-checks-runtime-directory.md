---
id: CRP-065
title: A follower checks the runtime directory before it dials
milestone: M6 Release
type: feature
status: done
priority: P1
blocked_by: [CRP-062]
blocks: []
model: claude-opus-5-5
effort: high
size: S
---

# CRP-065: A follower checks the runtime directory before it dials

## Goal

No copy of the program sends a session to a control socket that another local user could be listening on. Today only the host checks the directory; a follower trusts whatever is at the path.

## Context

Finding F1 of the [threat model](../../architecture/threat-model.md), severity medium.

`transport.Acquire` calls `transport.Prepare`, which refuses a runtime directory that the user does not own or that others can access. When that fails, the node cannot be host, so it follows: `host.Node.round` calls `follow`, and the dial in `internal/cli/hostwire.go` is `transport.Dial(ctx, paths.Socket, ...)` with no check at all. `status` and `doctor` dial the same way through `system.ask`.

On Linux without `XDG_RUNTIME_DIR`, on macOS without `TMPDIR`, and for the short hashed name on both, the directory is under `/tmp`, with a predictable name. Another user who creates it first, with a socket in it, is sent a `sync` and every `event` of every session: session id, status, tool kind, model family, timings, and the project name at the `full` level.

## Scope

- `transport.Dial` refuses, before connecting, a socket whose directory would not pass the checks `Prepare` makes: not a directory, a link, not owned by the user, or accessible to group or others. It must not create the directory. The error is distinct and is not `ErrNoHost`.
- The checks are shared with `Prepare`, not written twice.
- A node that meets the refusal logs it once as an error class, as it does for a lock that cannot be tried, and keeps retrying with its usual backoff.
- `status` reports the refusal as a failure with a fixed message. `doctor` already reports an unsafe directory and must not dial one.
- Windows keeps its rule: no ownership check, because permission bits mean nothing there.
- The row for F1 in the threat model: move it from the findings to the table of section 2, with the tests.

## Out of scope

- A second location to fall back to when the directory is refused. The threat model accepts that another user can deny presence.
- Checking the peer's credentials on the socket. The directory check is what ADR-0006 specifies.

## Acceptance criteria

- [x] On Unix, a test makes a runtime directory with mode 0755 that holds a listening socket, and `Dial` returns the new error without connecting: the listener accepts nothing.
- [x] On Unix, a test shows `Dial` refusing a directory that is a symbolic link to a safe one.
- [x] `checkDir` remains the one place the rules are written, and its ownership case is exercised for `Dial` with an injected owner, as `TestCheckDir` does for `Prepare`.
- [x] A node test shows that a node whose lock cannot be tried and whose dial is refused sends nothing and logs the refusal once.
- [x] `status` exits with a failure and a fixed message when the directory is refused.
- [x] The threat model names these tests against the threat "another local user being the host".

## Notes for the implementer

- Check the directory with `Lstat`, as `Prepare` does, so that a link is refused and not followed.
- There is a gap between the check and the connect. It is closed by the directory being owner-only: once it passes, only the user can change what is in it. Say so in a comment.
- The error must not carry the path into the log. Log a class.
- A real second user is not available in CI. Ownership is injected in the existing tests; keep to that.

## Why this model and effort

A trust-boundary fix in cross-platform file and socket code, where a wrong check fails open without anyone noticing.

## References

- [ADR-0006](../../architecture/adr/0006-control-channel.md), rule 1 and "Trust"
- [Threat model](../../architecture/threat-model.md), section 2 and finding F1
