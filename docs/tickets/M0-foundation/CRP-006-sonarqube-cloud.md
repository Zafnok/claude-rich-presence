---
id: CRP-006
title: SonarQube Cloud integration
milestone: M0 Foundation
type: chore
status: todo
priority: P1
blocked_by: [CRP-005]
blocks: []
model: claude-sonnet-5-5
effort: low
size: S
---

# CRP-006: SonarQube Cloud integration

## Goal

SonarQube Cloud analyses every push to `main` and every same-repository pull request, with coverage imported, and its quality gate is a required check.

## Context

SonarQube Cloud is free for public repositories. Its automatic analysis cannot import coverage, so analysis runs from CI. Whether the free plan allows a custom quality gate is unconfirmed (R14); the binding coverage gate is CRP-005 either way.

## Scope

Owner actions, which need the owner's GitHub login:

1. Create the SonarQube Cloud organisation bound to the GitHub account and import the repository.
2. Turn automatic analysis off.
3. Create a token and add it as the repository secret `SONAR_TOKEN`.
4. Set the quality gate described below, if the plan allows.

Then:

- `sonar-project.properties` with the settings in the [quality strategy](../../architecture/quality-strategy.md#sonarqube-cloud-setup).
- A CI job that downloads the three coverage artifacts from CRP-005 and runs the official scanner action, pinned to a commit hash.
- The job is skipped, not failed, when the token is unavailable, which is the case for fork pull requests.
- A quality gate badge in the README.

## Out of scope

- Any coverage exclusion for production Go code.

## Acceptance criteria

- [ ] The SonarQube Cloud project shows 100% coverage on `main`, matching the CI gate.
- [ ] Test files and `internal/testutil` are classified as test code, not as uncovered source.
- [ ] A pull request that introduces a code smell or an uncovered new line fails the SonarQube check.
- [ ] The quality gate requires 100% coverage on new code, no new issues, and at most 3% duplication on new code. If a custom gate is not available on the plan, that is recorded in [ADR-0009](../../architecture/adr/0009-quality-gates.md) with what the default gate enforces instead.
- [ ] A pull request from a fork passes CI without the SonarQube job.
- [ ] The scanner action is pinned to a commit hash, and the token is used in no other job.

## Notes for the implementer

- Confirm the current property names for Go coverage and test reports against SonarQube's documentation before writing the file.
- Coverage profiles from Windows use the module path, not file-system paths, so they should merge cleanly. Confirm in the first analysis that files from all three profiles are attributed.
- Ask the owner to perform steps 1 to 4. Do not create accounts or tokens on the owner's behalf.

## Why this model and effort

Configuration against documented settings.

## References

- [ADR-0009](../../architecture/adr/0009-quality-gates.md)
- Coverage parameters: https://docs.sonarsource.com/sonarqube-cloud/analyzing-source-code/test-coverage/test-coverage-parameters
- Automatic analysis limits: https://docs.sonarsource.com/sonarqube-cloud/analyzing-source-code/automatic-analysis
