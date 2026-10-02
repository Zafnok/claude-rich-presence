---
id: CRP-062
title: Security review and threat model
milestone: M6 Release
type: chore
status: todo
priority: P1
blocked_by: [CRP-043]
blocks: []
model: claude-opus-5-5
effort: high
size: S
---

# CRP-062: Security review and threat model

## Goal

An independent pass over the finished code for the ways a local, unprivileged, always-running helper can hurt its user, before anyone outside installs it.

## Context

The program runs in every Claude session, listens on a local socket, parses input from three sources, and publishes to a social platform. Its privilege is low, but its reach is every session the user opens.

## Scope

- `docs/architecture/threat-model.md`: assets, trust boundaries, and for each threat below, the mitigation and the test that demonstrates it.

  | Boundary | Threats to consider |
  |---|---|
  | Hook events into the adapter | Oversized or malformed input; content leaking past the allowlist |
  | Control socket | Another local user or process connecting; a hostile follower; path and symlink tricks in the runtime directory; a planted lock or socket file |
  | Discord pipe | A hostile process impersonating Discord at a lower index; malformed frames; an oversized frame |
  | Configuration file and environment | Hostile values; paths |
  | Log file | Content leakage; unbounded growth; symlink at the log path |
  | The bundle and plugin | Tampered release assets; a changed bundle URL |
  | Claude itself | The model calling `presence_event` with invented input; tool output influencing the model |

- A review of the code against that model, using `/security-review` and a manual read of the transport, protocol and adapter packages.
- `SECURITY.md`: supported versions and how to report a vulnerability privately.
- A check that the binary links no network client. A test over the build's package list that fails if an HTTP or DNS package is present.

## Out of scope

- Fixing findings beyond trivial ones. Each finding becomes a ticket with a severity.
- Code signing, which is CRP-063.

## Acceptance criteria

- [ ] The threat model exists and every listed threat has a mitigation and a named test, or an accepted-risk note with a reason.
- [ ] `SECURITY.md` exists and GitHub private vulnerability reporting is enabled. Enabling it is an owner action.
- [ ] The no-network test exists and passes.
- [ ] Every finding is a ticket with a severity. No finding of high severity is open at the first stable release.
- [ ] The review states what it did not cover.

## Notes for the implementer

- A process that pretends to be Discord can only receive what we would have sent to Discord, which is already public by intent. Say so, and consider whether anything else flows that way.
- The model calling `presence_event` directly can at worst make presence wrong. Confirm no input can do more than that.
- Review with a fresh session that has not seen the implementation discussions, so it is not anchored on the author's reasoning.

## Why this model and effort

Adversarial reasoning over the whole system, where a miss is costly.

## References

- [ADR-0006](../../architecture/adr/0006-control-channel.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- [Risk register](../../architecture/risks.md)
