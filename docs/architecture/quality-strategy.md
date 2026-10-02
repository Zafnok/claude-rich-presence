# Quality strategy

How the gates in [ADR-0009](adr/0009-quality-gates.md) are met in practice.

## Test levels

| Level | Scope | Uses | Proportion |
|---|---|---|---|
| Unit | One package. Pure logic and orchestration over fakes | Table-driven tests, fake clock, fake ports | Most tests |
| Fuzz | Every decoder: Discord frames, control messages, MCP requests, config, hook input | Go's built-in fuzzing with a seed corpus | One per decoder |
| Integration | Real sockets and pipes on the real operating system, against the fake Discord server | `internal/testutil/fakediscord` | A few per transport |
| End to end | The built, instrumented binary driven over standard streams by a scripted MCP client, with a fake Discord server | `test/e2e` | A handful of scenarios |

No test talks to a real Discord client or a real Claude. Those are covered by manual checks in the validation tickets and by the spikes.

## Reaching 100% in Go

Go counts statements. Reaching 100% is mostly a matter of structure.

| Obstacle | Approach |
|---|---|
| `main` cannot be called from a test | `main` is one line that passes the process's arguments, streams and environment to a `Run` function and exits with its result. `Run` is tested directly, and `main` itself is covered by end-to-end runs of an instrumented binary |
| Operating-system calls fail in ways that are hard to provoke | Each call sits behind an interface. A fake returns the error on demand |
| Platform files compile on one operating system | Each CI job must reach 100% over the files it compiles. The three profiles together cover the repository |
| Time and randomness | An injected clock and an injected source for jitter and nonces |
| Goroutines and shutdown paths | Every goroutine has an owner that waits for it. Tests use the fake clock and channels, never sleeps |
| "Impossible" branches | Do not write them. Make the state unrepresentable, or handle it in a way a test can reach |
| Test helpers counted as production code | Shared helpers live in `internal/testutil` and are excluded from the measured package list. They are test code, and are declared as such to SonarQube |

Patterns to follow are in the `tdd-full-coverage` skill.

## What is measured

- Measured: every package under `cmd/`, `internal/` except `internal/testutil`, and `tools/`.
- Not measured, because it is not Go: `plugin/`, `extension/`, workflows. These are validated instead: `claude plugin validate --strict` for the plugin and marketplace, a manifest check for the extension, and the end-to-end tests exercise them.

## CI pipeline

Runs on every pull request and on `main`, on Linux, macOS and Windows.

| Stage | Fails when |
|---|---|
| Format and vet | `gofmt` would change a file, or `go vet` reports anything |
| Static analysis | The pinned analyser reports anything |
| Build | Any release target fails to compile, with cgo disabled |
| Unit and integration tests | Any test fails, with the race detector on |
| Fuzz smoke | Any fuzz target fails within a short fixed budget |
| End-to-end tests | Any scenario fails, or the latency budget is exceeded |
| Coverage gate | Merged unit and end-to-end coverage for that operating system is below 100.0% |
| Supply chain | A module outside policy appears, a license is off the allowlist, or `govulncheck` finds a reachable vulnerability |
| Plugin validation | `claude plugin validate --strict` fails. Runs once the plugin exists |
| SonarQube Cloud | The quality gate fails. Skipped for fork pull requests |

## SonarQube Cloud setup

| Item | Value |
|---|---|
| Analysis mode | CI-based. Automatic analysis disabled |
| Sources | `cmd`, `internal`, `tools` |
| Tests | Files ending `_test.go`, and `internal/testutil` |
| Coverage | The three per-platform Go coverage profiles |
| Exclusions from coverage | None for production Go code |
| Quality gate | 100% on new code, no new issues, duplication on new code at most 3%; 100% overall if the plan permits a custom gate |
| Secret | `SONAR_TOKEN`, added by the owner |

Property names and scanner details are settled in [CRP-006](../tickets/M0-foundation/CRP-006-sonarqube-cloud.md) against the current SonarQube documentation.

## Review

| Change | Review |
|---|---|
| Any pull request | `/code-review` before requesting human review |
| Host election, failover, Discord session manager, transports | Reviewed with Opus 5.5 at high effort, focusing on races, shutdown and error paths |
| Anything touching what leaves the adapter | Checked against the allowlist in [ADR-0008](adr/0008-privacy-and-safety-by-default.md) |
| Before the first release | [CRP-062](../tickets/M6-release/CRP-062-security-review.md) |

## Manual verification

Automation cannot prove that presence appears in a real Discord client. Each ticket that changes visible behaviour ends with a short manual check, recorded in the pull request: what was run, on which operating system, and what Discord showed.
