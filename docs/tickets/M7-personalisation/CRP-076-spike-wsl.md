---
id: CRP-076
title: "Spike: Claude Code in WSL with Discord on Windows"
milestone: M7 Personalisation
type: spike
status: todo
priority: P2
blocked_by: [CRP-033]
blocks: []
model: claude-opus-5-5
effort: high
size: M
---

# CRP-076: Spike: Claude Code in WSL with Discord on Windows

## Goal

Find out whether presence can work when Claude Code runs inside the Windows Subsystem for Linux and Discord runs on Windows, and at what cost.

## Context

Discord's local interface on Windows is a named pipe. A Linux process inside WSL cannot open it, so our Linux binary there finds no Discord. Today this is listed as unsupported. Many Windows developers run Claude Code in WSL, so the audience may be large.

One other project supports this by running its background process on the Windows side, started from inside WSL. Another offers it as an opt-in. A third states it will not work.

This is a spike. Prototype code is thrown away.

## Scope

On a Windows machine with WSL 2, Discord for Windows, and Claude Code installed inside WSL, evaluate:

| # | Approach | Questions |
|---|---|---|
| W1 | The Linux adapter starts the Windows build of our binary through WSL's ability to run Windows programs, and relays to it | Is that ability always on? What does it cost per start? Does the Windows process keep running, and does it stop when the session ends? How do the two halves talk: standard streams, or the control socket? |
| W2 | The control socket shared across the boundary: a Windows-side presence host, with WSL adapters as followers | Can a Unix socket file on the Windows file system be opened from WSL 2, or the reverse? If not, is there another channel that needs no extra software? |
| W3 | Reaching the Discord pipe directly from Linux | Is there any way without a helper program on the Windows side? |
| W4 | Detection | How does the binary know it is inside WSL, and how does it find a Windows-side copy of itself? The bundle carries only the Linux binary there |
| W5 | Mixed use | With sessions in WSL and sessions on Windows at once, is there one presence host or two? |

Also record: whether a console window flashes on the Windows side, behaviour under WSL 1, and what a user would have to install or configure.

## Out of scope

- Remote development over SSH and containers on other machines.
- Production code.

## Acceptance criteria

- [ ] `docs/research/crp-076-wsl.md` answers W1 to W5 with what was run, the Windows and WSL versions, and what was observed.
- [ ] It ends with a recommendation: support it and how, support it as an opt-in, or keep it unsupported, with the reasons and the estimated size of the work.
- [ ] If the recommendation is to build, an ADR is proposed with the `record-decision` skill and tickets are written with the `write-ticket` skill.
- [ ] If not, the limitation in [risks.md](../../architecture/risks.md) is updated with what was learned.
- [ ] No prototype code is merged.

## Notes for the implementer

- The owner's machine is Windows. Whether WSL is installed there is not known; ask. The steps need a person at that machine.
- Any approach that needs a second binary on the Windows side has a delivery problem: the plugin's bundle supplies the Linux binary inside WSL. Say how the Windows binary would get there.
- Keep [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md) in view: no network listeners reachable from outside the machine.
- Time box: one working day.

## Why this model and effort

Cross-boundary operating-system behaviour with several candidate designs and little documentation.

## References

- [Viability: what each surface can show](../../architecture/viability.md#what-each-surface-can-show)
- [ADR-0005](../../architecture/adr/0005-presence-host-election.md), [ADR-0006](../../architecture/adr/0006-control-channel.md)
