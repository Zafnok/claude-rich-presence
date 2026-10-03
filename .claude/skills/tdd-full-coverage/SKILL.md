---
name: tdd-full-coverage
description: How to write Go in this repository so that statement coverage stays at exactly 100% with no exclusions. Use whenever writing, changing or reviewing Go code or tests here, when the coverage gate fails, or when a line seems impossible to test.
---

# Test-first Go at 100% coverage

The rule is in ADR-0009: 100.0% statement coverage on each operating system, no exclusions. This skill is how to get there without contortions. The short version: if a line is hard to test, the design is wrong, not the rule.

## The loop

1. Write the test for the next behaviour. Run it and watch it fail for the right reason.
2. Write the least code that passes.
3. Refactor with the tests green.
4. Check coverage for the package before moving on. Do not leave uncovered lines to collect later.

## Structure that makes coverage natural

| Situation | Do this |
|---|---|
| Code calls the operating system: files, sockets, pipes, locks, environment, process ids | Define a small interface in the package that uses it. Give it a real implementation in a platform file and a fake in the test. The logic never calls the system directly |
| Code needs the time or a timer | Take a clock interface. Tests use `internal/testutil/fakeclock`. Never call the real clock from logic, and never sleep in a test |
| Code needs randomness: jitter, nonces | Take the source as a parameter |
| `main` | One statement that calls `cli.Run` with the process's arguments, streams and environment. `Run` returns an exit code and is tested directly. `main` itself is covered by end-to-end runs of an instrumented binary |
| An error branch after a system call | The fake returns that error. Every `if err != nil` has a test that takes it |
| A goroutine | It has an owner that starts it and waits for it. The test ends by stopping the owner and asserting nothing is left |
| A branch that "cannot happen" | Do not write it. Change the types so the state cannot be represented, or handle it in a way a test can reach |
| Platform files, `_windows.go` and `_unix.go` | Only the system call and its error mapping. All decisions live in the shared file, where every operating system tests them |
| A helper needed by tests in several packages | It goes in `internal/testutil`, which is test code and is not in the measured set |

## What a good test looks like here

- **Table-driven**, with a name per case. Cover each side of every condition, not just each line. Go measures statements; a line with two conditions can be "covered" by one case and wrong in the other.
- **Asserts an outcome.** A test that only calls a function to light up lines is rejected in review.
- **Deterministic.** No real time, no real randomness, no dependence on test order.
- **Runs under the race detector.** All tests do, in CI.
- **Fuzz every decoder.** Any function that parses bytes from outside the process gets a fuzz test with a seed corpus: Discord frames, control messages, MCP requests, configuration, hook input.

## Things not to do

- Do not add build tags, comments or configuration to exclude code from coverage.
- Do not move production logic into `internal/testutil` to hide it from measurement.
- Do not test through a real Discord, a real Claude, or the network.
- Do not use a fixed sleep to wait for something. Poll a condition with a deadline, or use the fake clock.
- Do not add a mocking or assertion library. The standard `testing` package is enough, and a dependency needs an ADR.

## Checking coverage

Use the commands in the Development section of `CONTRIBUTING.md`. The gate tool, `tools/covercheck`, prints every uncovered block as `file:line`. A package is done when it prints nothing.

Remember that your machine covers only the files your operating system compiles. Files for other operating systems are measured in CI. If you changed one, say in the pull request that it is unverified locally.

## When you believe a line truly cannot be covered

Stop. Describe the line and why. The usual answers are: put an interface in front of it, pass the dependency in, or delete it. If none applies, that is a decision for the `record-decision` skill and the owner, not something to work around.
