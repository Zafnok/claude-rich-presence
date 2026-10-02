---
id: CRP-063
title: Code signing and notarisation
milestone: M6 Release
type: owner-task
status: todo
priority: P2
blocked_by: [CRP-002, CRP-052, CRP-060]
blocks: []
model: owner
effort: low
size: S
---

# CRP-063: Code signing and notarisation

## Goal

Decide whether and how to sign the released binaries, based on what the spikes and validation actually observed, and on what it costs.

## Context

The first release ships unsigned (R5). Whether that is a problem depends on evidence from CRP-002 and CRP-052: whether macOS blocks the binary inside an installed bundle, and whether Windows or antivirus products warn about it. Signing can cost money, and keeping cost down is a project goal, so this is the owner's decision.

## Scope

- Summarise what CRP-002 and CRP-052 observed on each operating system.
- Investigate and record current terms and cost for:

  | Option | Question |
  |---|---|
  | Free code signing for open-source projects on Windows | Does the project qualify, and what does the programme require of the build? |
  | An Apple Developer membership with notarisation | Annual cost, and what the release workflow would need |
  | Signing the bundle itself with the MCPB tooling | What it protects against, and whether Claude Desktop or Claude Code checks it |
  | Staying unsigned | What users must do on each operating system, written as install instructions |

- A recommendation, and the owner's decision.

## Out of scope

- Implementing the chosen option. That becomes its own ticket.

## Acceptance criteria

- [ ] `docs/research/crp-063-code-signing.md` records the observations, the options with current cost and requirements, the recommendation and the decision.
- [ ] If an option is chosen, an implementation ticket exists.
- [ ] If staying unsigned, the install documentation says exactly what users will see and do, and R5 is moved to accepted limitations.

## Notes for the implementer

- A model can research the options and draft the comparison. Sonnet 5.5 at low effort is enough. Check terms and prices on the providers' own pages on the day; they change.
- Do not enrol in any programme or create any account on the owner's behalf.

## Why this model and effort

It is a cost decision informed by a short investigation.

## References

- [Risk register](../../architecture/risks.md), R5
- [ADR-0003](../../architecture/adr/0003-license-and-dependency-policy.md), section on cost
