# ADR-0009: 100% statement coverage and SonarQube Cloud, enforced in CI

## Status

Accepted.

## Context

The owner requires 100% test coverage and SonarQube.

- Go reports statement coverage. It has no branch coverage, and SonarQube has no branch data for Go.
- Platform-specific files are compiled on one operating system only, so no single machine can cover everything.
- Go can instrument a compiled binary and collect coverage from real executions, which makes `main` and the composition root coverable without tricks.
- SonarQube Cloud is free for public repositories. Its automatic analysis mode cannot import coverage, so analysis must run from CI.
- Whether the free plan allows a custom quality gate could not be confirmed.

## Decision

### Coverage

1. **100.0% statement coverage of all production packages, on each operating system in the CI matrix**, measured over the files that operating system compiles. A job that reports less fails.
2. **No exclusions.** No build tags, comments or configuration that remove production code from measurement. Code that cannot be tested is redesigned.
3. Coverage combines unit tests with end-to-end runs of an instrumented binary.
4. Test helpers shared between packages live under `internal/testutil` and are declared as test code, not production code.
5. The gate is a check in this repository, run in CI, that reads the Go coverage profile. It is the binding gate, independent of any SonarQube plan.

### Beyond the number

Statement coverage proves a line ran, not that it was checked. To make the number mean something:

1. Table-driven tests cover each side of every condition, not merely each statement.
2. Every decoder has a fuzz test with a seed corpus, run briefly in CI.
3. All tests run with the race detector.
4. Tests assert outcomes. A test with no assertion that exists to touch lines is rejected in review.

### SonarQube Cloud

1. CI-based analysis on every push to `main` and every pull request from the repository. Automatic analysis is turned off.
2. The three per-platform coverage profiles are passed to the scanner.
3. The quality gate requires 100% coverage on new code, no new issues, and no new duplication above 3%. If the plan permits a custom gate, it also requires 100% overall. If not, the default gate applies and rule 5 above remains the enforcement.
4. Pull requests from forks cannot access the token. They are gated by the in-repository checks, and analysed after merge.

### Static checks

`gofmt`, `go vet`, `govulncheck`, and one static analyser, chosen and pinned in [CRP-005](../../tickets/done/CRP-005-ci-pipeline.md).

## Consequences

- Operating-system calls must sit behind interfaces so their failure paths can be driven by fakes. This is already required by [ADR-0001](0001-core-architecture.md).
- Defensive branches that cannot occur are not written. If a state is impossible, the types should make it so.
- CI runs on three operating systems for every change. Free for a public repository.
- The owner must create the SonarQube Cloud organisation and project and add a token secret ([CRP-006](../../tickets/M0-foundation/CRP-006-sonarqube-cloud.md)).
- Spike code is thrown away and never merged, so it never dilutes the measure.

## Alternatives considered

| Alternative | Why not |
|---|---|
| A threshold below 100% | Not what was asked, and thresholds erode |
| Exclude `main`, platform files or error paths | Those are where presence tools actually fail |
| Rely on the SonarQube gate alone | Unavailable to fork pull requests, and dependent on plan features |
| A third-party mutation testing tool | Would add real assurance, but the available Go tools are individually maintained. Revisit later under [ADR-0003](0003-license-and-dependency-policy.md) |

The procedure for meeting these gates is in [quality-strategy.md](../quality-strategy.md).
