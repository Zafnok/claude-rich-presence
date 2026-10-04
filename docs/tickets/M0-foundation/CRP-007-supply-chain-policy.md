---
id: CRP-007
title: Supply-chain policy enforcement
milestone: M0 Foundation
type: chore
status: todo
priority: P1
blocked_by: [CRP-005]
blocks: [CRP-060]
model: claude-sonnet-5-5
effort: low
size: S
---

# CRP-007: Supply-chain policy enforcement

## Goal

CI enforces the dependency and license policy of [ADR-0003](../../architecture/adr/0003-license-and-dependency-policy.md), so it cannot be broken by accident.

## Context

The policy allows only the standard library and `golang.org/x/*` at runtime without an ADR, forbids copyleft in anything linked, and requires pinned tools.

## Scope

- A check that fails when `go.mod` requires a module that is neither pre-approved nor listed in an allowlist file. Each allowlist entry names the ADR that approved it.
- A license check over all linked modules against the allowlist in ADR-0003, using a tool from the Go project or Google, pinned.
- `govulncheck` on every pull request and on a weekly schedule.
- A check that every action in every workflow is pinned to a full commit hash.
- Dependabot configuration for Go modules and GitHub Actions, weekly.
- Repository settings, documented for the owner to apply: branch protection on `main` requiring the CI checks, and disallowing force pushes.

## Out of scope

- `SECURITY.md` and the threat model, which are CRP-062.
- Third-party notices in release artifacts, which are CRP-060.

## Acceptance criteria

- [ ] A pull request adding an unapproved module fails with a message that points to ADR-0003 and the `add-dependency` skill.
- [ ] A pull request adding an approved module with a matching allowlist entry passes.
- [ ] A linked module with a license off the allowlist fails the check. Demonstrated in the pull request description with a temporary example, not merged.
- [ ] A workflow using an action by tag instead of commit hash fails the check.
- [ ] `govulncheck` runs on pull requests and weekly.
- [ ] Dependabot opens grouped weekly pull requests for modules and actions.
- [ ] Any Go code added for these checks lives under `tools/`, is covered to 100.0%, and adds no dependency.

## Notes for the implementer

- The allowlist file starts with one entry: `github.com/Microsoft/go-winio`, approved by [ADR-0017](../../architecture/adr/0017-go-winio-for-windows-pipes.md). It is linked on Windows only, so run the license check for Windows as well as for the runner's own operating system.
- The module check can be a few lines of Go over the output of the standard module listing command. Keep it in `tools/` so it is tested like everything else.
- Branch protection needs the owner's repository permissions. Write the exact settings down and ask the owner to apply them.

## Why this model and effort

Small, well-specified checks.

## References

- [ADR-0003](../../architecture/adr/0003-license-and-dependency-policy.md)
- `govulncheck`: https://go.dev/doc/security/vuln/
