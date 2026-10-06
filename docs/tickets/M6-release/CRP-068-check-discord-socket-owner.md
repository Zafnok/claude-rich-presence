---
id: CRP-068
title: Check who owns the Discord socket on Unix
milestone: M6 Release
type: feature
status: todo
priority: P2
blocked_by: [CRP-062]
blocks: []
model: claude-sonnet-5-5
effort: high
size: S
---

# CRP-068: Check who owns the Discord socket on Unix

## Goal

On Linux and macOS the program does not send the activity to a `discord-ipc` socket that belongs to another user.

## Context

Finding F4 of the [threat model](../../architecture/threat-model.md), severity low.

`internal/discord/transport` tries each candidate endpoint in order and uses the first that connects. On Unix the directory is the first of `XDG_RUNTIME_DIR`, `TMPDIR`, `TMP` and `TEMP` that is set, and `/tmp` when none is. Where that is a shared directory, another local user can create `discord-ipc-0` and receive what would have gone to Discord: at the `full` level that includes the project name.

Discord's protocol has no authentication, and on Windows there is no equivalent check to make on a named pipe, so the threat model accepts impersonation in general. On Unix the owner of the socket file is one system call away.

## Scope

- On Unix, a candidate whose socket file is not owned by the current user is passed over as if it were absent, and the next candidate is tried.
- It is passed over without comment in the log, like any absent endpoint, and `doctor` counts only endpoints that would be used.
- Windows is unchanged.
- The row for F4 in the threat model: move it from the findings to the table of section 3, with the tests, and keep the accepted risk for Windows.

## Out of scope

- Authenticating Discord in any other way.
- Any change to the order of candidates, which is Discord's.

## Acceptance criteria

- [ ] A Unix test, with the owner injected as the control transport's tests inject it, shows a candidate owned by another user id passed over and the next one used.
- [ ] A Unix test shows that when the only candidate is owned by another user the result is `ErrNotRunning`.
- [ ] The existing dial tests pass unchanged on every operating system.
- [ ] `doctor` does not count a socket owned by another user as a Discord endpoint.
- [ ] The threat model names these tests against the threat "a process impersonating Discord at a lower index", and says what remains accepted on Windows.

## Notes for the implementer

- Load the `discord-ipc` skill first.
- Check with `Lstat` before connecting. The gap between the check and the connect matters only in a shared directory, where the sticky bit stops anyone but the owner replacing the file; say so in a comment, and do not claim more.
- A Flatpak or Snap Discord runs as the same user, so its socket has the same owner. Confirm that on a real installation if one is at hand and record it; if not, list it as unverified.
- 100% coverage applies: put the ownership lookup behind the same kind of seam `dirOwner` uses in `internal/control/transport`.

## Why this model and effort

Cross-platform socket code with a build-tagged seam, where the coverage rule makes the seam the hard part.

## References

- [Threat model](../../architecture/threat-model.md), section 3 and finding F4
- The `discord-ipc` skill
