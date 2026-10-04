---
name: add-dependency
description: Evaluate anything new that would enter go.mod, a CI workflow, the release pipeline or the test tooling of this repository against the dependency and license policy. Use before importing a new module, adding a GitHub Action, adding a build or lint tool, or when tempted to pull in a library for convenience.
---

# Add a dependency

The policy is ADR-0003. The default answer is no. The owner asked for no copyleft, no small single-maintainer dependencies, and low cost, and the architecture was designed so the standard library is enough.

## First: do you need it?

Ask in this order.

1. Is it in the Go standard library? Check before assuming it is not. The standard library covers JSON, Unix sockets on every operating system, structured logging, fuzzing, coverage, and flag parsing.
2. Is it in `golang.org/x/*`? Those are pre-approved.
3. Is what you need small? A few dozen lines you own and test are cheaper over time than a module you must watch.

If you still need it, classify it.

## Which kind is it?

| Kind | Example | Rule |
|---|---|---|
| Runtime | A module imported by anything under `cmd/` or `internal/`, other than `internal/testutil` | Standard library and `golang.org/x/*` only. Anything else needs an accepted ADR first |
| Test-linked | A module imported only by tests or `internal/testutil` | Same license allowlist. Prefer `golang.org/x/*`. Anything else needs an ADR |
| Standalone tool | A linter, a scanner, a validator run in CI | Any license, since running a tool does not affect ours. Must be pinned to an exact version. Prefer the Go project, GitHub, Google, SonarSource |
| GitHub Action | Anything under `uses:` | Pinned to a full commit hash. Prefer actions published by GitHub or by the vendor of the tool |

## Evaluating a candidate that needs an ADR

Gather these facts and put them in the ADR. Check them live; do not rely on memory.

| Question | Passing answer |
|---|---|
| License | On the allowlist: MIT, BSD-2-Clause, BSD-3-Clause, ISC, Apache-2.0, 0BSD, Unlicense |
| License of every transitive dependency | Also on the allowlist |
| Who maintains it | An organisation, or at least three people with recent commits |
| Activity | A release in the past twelve months, and issues being answered |
| Size of what it pulls in | Proportionate to the need |
| Cost of writing the needed part ourselves | Higher than the cost of tracking this dependency |
| Known vulnerabilities | None reachable, per `govulncheck` |

A candidate that fails the license rows is rejected. There is no exception for copyleft in linked code. A candidate that fails the maintainer or activity rows is rejected unless the ADR makes a specific case and the owner accepts it.

## Already decided

| Item | Status |
|---|---|
| Any third-party Discord presence library | Rejected. All are single-maintainer. See ADR-0004 |
| Discord's Social SDK | Rejected. Closed binary, restrictive terms, needs cgo |
| `github.com/Microsoft/go-winio` | Accepted for the Discord pipe on Windows only, imported by `internal/discord/transport` alone. See ADR-0017 |
| The official MCP Go SDK | Not used now. It is the upgrade path if we need more of MCP. See ADR-0004 |
| Assertion and mocking libraries | Not needed. Use the standard `testing` package |
| TOML or YAML parsers | Not needed. Configuration is JSON |
| A third-party release tool | Not needed. See CRP-060 |

## Procedure

1. Write the ADR with the `record-decision` skill, with the table above filled in.
2. Get it accepted by the owner.
3. Add the module, and add its entry to the allowlist file that CRP-007 establishes, naming the ADR.
4. If it is linked into the binary, add its license text to the third-party notices that the release ships.
5. Run the `quality-gate` skill.

Do not add the dependency first and write the ADR afterwards.
