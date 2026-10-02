---
name: quality-gate
description: The checks to run before opening or updating any pull request in this repository, and how to read a failing CI or SonarQube result. Use before every pull request, when CI is red, when the coverage gate or SonarQube quality gate fails, or when asked whether a change is ready.
---

# Quality gate

Run these before opening a pull request. They mirror CI, so passing here means CI should pass on your operating system. The exact commands are in the Development section of `CONTRIBUTING.md`, which CRP-004 and CRP-005 establish; until those land, the checks marked "after CRP-005" do not exist yet.

## Before the pull request

| # | Check | Passes when |
|---|---|---|
| 1 | Format | `gofmt` would change nothing |
| 2 | Vet | `go vet` reports nothing |
| 3 | Static analysis, after CRP-005 | The pinned analyser reports nothing |
| 4 | Build | The module builds with cgo disabled for every release target |
| 5 | Tests | All pass with the race detector on |
| 6 | Fuzz smoke | Each fuzz target you touched runs clean for a short fixed time |
| 7 | Coverage, after CRP-005 | The gate tool reports 100.0% and lists no uncovered block |
| 8 | Dependencies | `go.mod` has no new module, or an accepted ADR covers it |
| 9 | Plugin, after CRP-042 | `claude plugin validate --strict` passes, if you touched `plugin/` or the marketplace file |
| 10 | Documents | Links in Markdown you changed resolve. Ticket status updated in its own file |

Then, for the change itself:

- Run `/code-review` and deal with what it finds.
- If the change touches what leaves the adapter, check it against the allowlist rules in ADR-0008.
- If the change is visible in Discord, do the manual check and record it. If you cannot, say so.

## Report honestly

In the pull request, state what you ran and what you did not. Code for another operating system that you could not run locally is "unverified locally, covered by CI". A manual check you could not do is "not done, needs the owner". Do not write "tested" for something you reasoned about.

## Reading failures

| Failure | Likely cause | What to do |
|---|---|---|
| Coverage below 100% on one operating system only | A platform file, or a branch only reachable there | Read the `file:line` list for that job. Move the decision into the shared file, or add the missing case |
| Coverage below 100% everywhere | An error path with no test | Add the case through the fake. See the `tdd-full-coverage` skill |
| Race detector report | Shared state touched from two goroutines | Fix the ownership. Do not add a lock to make the report go away without understanding it |
| Test passes locally, fails in CI | Timing, or a path or line-ending assumption | Replace sleeps with the fake clock or a polled condition. Check path separators |
| Leaked goroutine | Something started without an owner | Give it an owner that waits for it on shutdown |
| SonarQube: coverage on new code below 100% | The profile for one operating system was not imported, or new code is only reached on one | Check all three profiles were attached, then treat as a coverage failure |
| SonarQube: new issue | A code smell or bug pattern | Fix it. If it is a false positive, say why in the pull request and resolve it in SonarQube with that reason. Do not disable the rule |
| SonarQube job skipped | Pull request from a fork, no token | Expected. The in-repository gate still applies |
| Supply-chain check fails | A new module, an unpinned action, or a vulnerability | Use the `add-dependency` skill, pin the action to a commit, or update the affected module |

## Never

- Lower a threshold, add an exclusion, or skip a check to get a pull request through.
- Mark a ticket `done` with a red check.
- Retry a flaky test until it passes. A flaky test is a bug in the test or the code; find which.
