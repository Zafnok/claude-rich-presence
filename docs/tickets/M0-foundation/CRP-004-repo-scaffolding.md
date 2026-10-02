---
id: CRP-004
title: Repository scaffolding and toolchain
milestone: M0 Foundation
type: chore
status: todo
priority: P0
blocked_by: []
blocks: [CRP-005, CRP-010, CRP-012, CRP-013, CRP-020, CRP-021, CRP-022, CRP-031, CRP-040]
model: claude-sonnet-5-5
effort: low
size: S
---

# CRP-004: Repository scaffolding and toolchain

## Goal

A Go module that builds and tests on Windows, macOS and Linux, laid out as [repository-layout.md](../../architecture/repository-layout.md) describes, so every later ticket has a place to put its code.

## Context

The repository holds only Markdown. Go is not installed on the development machine.

## Scope

- Install Go, the current stable release, minimum 1.26. Record the installation command for each operating system in `CONTRIBUTING.md`.
- `go.mod` with module path `github.com/Zafnok/claude-rich-presence` and the minimum Go version.
- `cmd/rich-presence` with a `main` that only passes arguments, streams and environment to `internal/cli.Run` and exits with its return value.
- `internal/cli` with `Run`, handling `version` and an unknown-command error, and one constants file that holds the product, binary, plugin and tool names.
- The package directories from the layout, each with a package comment stating its responsibility and what it must not import.
- `.gitignore` covering build output, coverage files, `.claude/worktrees/`, and editor files.
- `.gitattributes` normalising line endings to LF for text and marking binary types.
- `.editorconfig`.
- A "Development" section in `CONTRIBUTING.md`: how to build, test, and measure coverage.

## Out of scope

- CI. That is CRP-005.
- Any behaviour beyond `version`.

## Acceptance criteria

- [ ] `go build ./...` and `go test -race ./...` succeed on Windows. They are expected to succeed on macOS and Linux and are confirmed there by CRP-005.
- [ ] `go vet ./...` and `gofmt -l .` report nothing.
- [ ] `rich-presence version` prints the version and exits 0. An unknown command prints usage to standard error and exits 2.
- [ ] Statement coverage of the skeleton is 100.0%, with `Run` tested directly.
- [ ] `go.mod` has no `require` entries.
- [ ] `CGO_ENABLED=0 go build` succeeds for `windows/amd64`, `darwin/amd64`, `darwin/arm64` and `linux/amd64`.
- [ ] The names of the product, binary, plugin and tools appear in exactly one Go file.

## Notes for the implementer

- Keep `main` to a single statement so end-to-end runs cover it later.
- Version information comes from the build: use the standard library's build info, with a linker-set variable as the override for releases.
- Do not add a `Makefile`. `make` is not present on Windows by default. Plain `go` commands, documented, are enough.

## Why this model and effort

Mechanical setup with no design decisions left open.

## References

- [Repository layout](../../architecture/repository-layout.md)
- [ADR-0002](../../architecture/adr/0002-implementation-language.md)
- [Quality strategy](../../architecture/quality-strategy.md)
