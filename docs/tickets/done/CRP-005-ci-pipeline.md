---
id: CRP-005
title: CI pipeline and the coverage gate
milestone: M0 Foundation
type: chore
status: done
priority: P0
blocked_by: [CRP-004]
blocks: [CRP-006, CRP-007, CRP-010, CRP-012, CRP-013, CRP-020, CRP-021, CRP-022, CRP-031, CRP-040, CRP-060]
model: claude-sonnet-5-5
effort: medium
size: M
---

# CRP-005: CI pipeline and the coverage gate

## Goal

Every pull request is built, checked and tested on three operating systems, and fails if statement coverage on any of them is below 100.0%. This is the binding gate of [ADR-0009](../../architecture/adr/0009-quality-gates.md).

## Context

Platform-specific files compile on one operating system only, so coverage must be gated per operating system. The gate must not depend on SonarQube, which fork pull requests cannot reach.

## Scope

- A GitHub Actions workflow on pull requests and pushes to `main`, with a matrix of `ubuntu-latest`, `macos-latest` and `windows-latest`.
- Stages, in the order given in the [quality strategy](../../architecture/quality-strategy.md#ci-pipeline): format and vet, static analysis, cross-build of all release targets, tests with the race detector, a short fuzz run per fuzz target, coverage gate.
- `tools/covercheck`: a small Go program that reads a Go coverage profile, prints each uncovered block as `file:line`, and exits non-zero unless total coverage is exactly 100.0%. It takes the list of measured packages from one place, shared with the test command.
- Choose and pin one static analyser, with a line of justification under [ADR-0003](../../architecture/adr/0003-license-and-dependency-policy.md).
- Upload each operating system's coverage profile as an artifact, for CRP-006.
- Wiring for end-to-end coverage: the workflow can merge coverage from an instrumented binary into the profile. CRP-043 supplies the tests; this ticket supplies the mechanism and proves it with the `version` command.

## Out of scope

- SonarQube, which is CRP-006.
- License and vulnerability checks, which are CRP-007.
- Release builds, which are CRP-060.

## Acceptance criteria

- [x] A pull request that leaves one statement untested fails on the coverage gate, and the log names the file and line.
- [x] A pull request that breaks formatting, `go vet`, the static analyser, or any cross-build fails at that stage.
- [x] All three operating systems run and must pass.
- [x] `main` in `cmd/rich-presence` shows as covered, through an instrumented binary run of `version`.
- [x] Every third-party action is pinned to a full commit hash, and the workflow's token has read-only permissions by default.
- [x] `tools/covercheck` is itself covered to 100.0% and included in the measured set.
- [x] `internal/testutil` is excluded from the measured set, and nothing else is.
- [x] A run on an unchanged tree takes under ten minutes.

## Notes for the implementer

- Go can build a coverage-instrumented binary and write coverage data to a directory named by an environment variable when it runs. The standard toolchain converts and merges that data with unit-test profiles.
- Fuzz targets are discovered, not listed by hand, so new ones are picked up.
- Cancel superseded runs on the same pull request to save minutes.
- Check current action versions and the Go coverage tooling flags against live documentation before writing the workflow.

## Why this model and effort

Standard pipeline work with a few details that must be exactly right.

## References

- [ADR-0009](../../architecture/adr/0009-quality-gates.md)
- [Quality strategy](../../architecture/quality-strategy.md)
- Go coverage for integration tests: https://go.dev/doc/build-cover
