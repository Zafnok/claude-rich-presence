# ADR-0002: Go, standard library only at runtime

## Status

Accepted.

## Context

The program must:

- run on Windows, macOS and Linux with nothing preinstalled. Claude Code ships as a native binary and brings no Node or Python; the development machine has neither `node` nor `go` on `PATH`;
- speak a Windows named pipe and Unix sockets, hold a file lock, and serve JSON over standard streams;
- start fast, because a copy starts with every Claude session;
- be covered to 100% and analysed by SonarQube;
- avoid single-maintainer dependencies and copyleft;
- be cheap to build, in tooling and in the model time spent writing it.

Facts established during assessment:

- Go 1.26 can open a Windows named pipe for overlapped I/O through the standard library, so deadlines work without third-party code.
- Go's standard library supports Unix domain sockets on Windows.
- Go measures coverage of compiled binaries run end to end, not only of unit tests, so `main` can be covered honestly.
- SonarQube Cloud has analysed Go for years and imports Go coverage profiles directly. Its Rust support is recent.

## Decision

1. Implement in **Go**, minimum version **1.26**, building with the current stable release.
2. **No cgo.** `CGO_ENABLED=0` for every target.
3. **Runtime dependencies: the standard library only.** `golang.org/x/*` is pre-approved if needed. Anything else requires an ADR ([ADR-0003](0003-license-and-dependency-policy.md)).
4. Targets: `windows/amd64`, `darwin/amd64` and `darwin/arm64` as one universal binary, `linux/amd64`. `linux/arm64` and `windows/arm64` binaries are built and published but not included in the bundle. Windows on Arm runs the `amd64` binary under emulation.

## Consequences

- One static file per platform, a few megabytes, starting in single-digit milliseconds.
- Cross-compilation for every target from one Linux runner, except the macOS universal merge.
- The whole dependency surface is the Go project's own code, under BSD-3-Clause.
- Statement coverage is the only coverage Go reports. There is no branch coverage. [ADR-0009](0009-quality-gates.md) addresses this.
- Reaching 100% in Go means every `if err != nil` path needs a test, which forces the port structure of [ADR-0001](0001-core-architecture.md).
- Contributors need Go installed. It is not on the development machine today; installing it is the first step of [CRP-004](../../tickets/M0-foundation/CRP-004-repo-scaffolding.md).

## Alternatives considered

| Language | For | Against |
|---|---|---|
| Rust | Smallest binary, fastest start. Toolchain already on the development machine | JSON needs third-party crates, and a Windows named-pipe or Unix-socket server needs `windows-sys` or an async runtime. Cross-compiling to macOS needs macOS runners or extra tooling. 100% line coverage with `llvm-cov` is harder to reach and keep. SonarQube support is newer. More model time per ticket |
| TypeScript on Node | Claude Desktop bundles Node for extensions. Largest pool of prior art | Claude Code does not provide Node to hooks or plugin servers, and the development machine proves users may not have it. Bundling a runtime into a single executable gives a binary of tens of megabytes and slow starts |
| Python | Simple | Needs an interpreter on the user's machine |
| C or C++ | No runtime | Memory safety, per-platform build complexity, and no gain over Go for this workload |

The existing Rust toolchain on the development machine was weighed as a signal of preference. It did not outweigh the dependency and tooling costs above. The owner confirmed on 2026-10-02 that installed tooling is not a constraint and that the language should be chosen on fitness alone, so the choice of Go stands.
