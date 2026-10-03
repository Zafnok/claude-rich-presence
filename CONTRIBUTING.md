# Contributing

## Before you start

1. Read the [architecture overview](docs/architecture/README.md) and the [ADR index](docs/architecture/adr/README.md).
2. Pick a ticket from [docs/tickets/](docs/tickets/README.md) whose blockers are all `done`.
3. If the work has no ticket, write one first (see [TEMPLATE.md](docs/tickets/TEMPLATE.md)).

## Workflow

| Step | Convention |
|---|---|
| Branch | From `main`, named `crp-NNN-short-slug` or any name that contains the ticket id |
| Commits | [Conventional Commits](https://www.conventionalcommits.org/), with the ticket id in the scope or footer, for example `feat(discord): frame codec (CRP-020)` |
| Tests | Written first. Statement coverage stays at 100% on every operating system in the CI matrix |
| Pull request | One ticket per pull request. Link the ticket. State what you verified and how |
| Ticket status | Update the `status` field in the ticket's own file, in the same pull request. When it becomes `done`, move the file to `docs/tickets/done/` and repoint the links to it |

## Definition of done

A ticket is `done` when all of these hold:

- Every acceptance criterion in the ticket is met and demonstrated by a test or a recorded observation.
- CI is green on Linux, macOS and Windows: build, `go vet`, static analysis, race-enabled tests.
- Statement coverage is 100.0% on each operating system, with no exclusions added.
- The SonarQube Cloud quality gate passes, once CRP-006 has set it up.
- No runtime dependency was added, or an accepted ADR covers it.
- Documentation that the change makes stale is updated in the same pull request.

## Decisions

If a ticket forces a choice that an ADR does not already cover, record it. Small choices go in the pull request description. Choices that constrain later work get an ADR; see the [ADR index](docs/architecture/adr/README.md) for the format.

## Dependencies

Runtime dependencies are limited to the Go standard library and `golang.org/x/*`. Anything else needs an ADR. Build and CI tools are pinned to an exact version or commit. The full policy is [ADR-0003](docs/architecture/adr/0003-license-and-dependency-policy.md).

## Security and privacy

Do not add code that reads prompts, transcripts, tool inputs or file paths, or that makes a network request. Report vulnerabilities privately to the repository owner through GitHub's security advisory form rather than in a public issue.

## License

By contributing you agree that your contribution is licensed under the [MIT License](LICENSE.md).

## Development

### Install Go

The minimum is Go 1.26. Build with the current stable release.

| Operating system | Command |
|---|---|
| Windows | `winget install GoLang.Go` |
| macOS | `brew install go` |
| Linux | Download the archive from [go.dev/dl](https://go.dev/dl/) and follow [go.dev/doc/install](https://go.dev/doc/install), or use your distribution's package if it is new enough |

Check with `go version`.

The race detector needs cgo and so a C compiler, for tests only. The shipped binary is always built with `CGO_ENABLED=0`. macOS has one after `xcode-select --install`, and Linux after installing `gcc`. On Windows, Go cannot use Microsoft's compiler; install the mingw-w64 `gcc` from [MSYS2](https://www.msys2.org/):

```bash
winget install MSYS2.MSYS2
```

```bash
C:/msys64/usr/bin/bash.exe -lc "pacman -S --needed --noconfirm mingw-w64-ucrt-x86_64-gcc"
```

Then add `C:\msys64\ucrt64\bin` to your `PATH`, or prefix a single command with it: `PATH="/c/msys64/ucrt64/bin:$PATH"`.

There is no `Makefile`. Every task is a plain `go` command, run from the repository root. The commands below are written for a POSIX shell, which on Windows is Git Bash.

### Build

```bash
go build -o bin/ ./cmd/rich-presence
```

Check that every release target compiles without cgo:

```bash
for t in windows/amd64 darwin/amd64 darwin/arm64 linux/amd64; do GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 go build ./... || break; done
```

A release sets the version through the linker. Without it, the binary reports the version the Go toolchain derives from the git state.

```bash
go build -ldflags "-X github.com/Zafnok/claude-rich-presence/internal/cli.version=1.2.3" -o bin/ ./cmd/rich-presence
```

### Format, vet and test

```bash
gofmt -l .
```

```bash
go vet ./...
```

```bash
CGO_ENABLED=1 go test -race ./...
```

`gofmt -l .` must print nothing.

### Measure coverage

Statement coverage must be 100.0%. `main` cannot be called from a test, so it is covered by running a binary built with coverage instrumentation. The unit tests and that run write to one directory, and the result is the merge of both.

```bash
rm -rf coverage && mkdir coverage
go test -cover ./... -args -test.gocoverdir="$PWD/coverage"
go build -cover -o bin/ ./cmd/rich-presence
GOCOVERDIR=coverage bin/rich-presence version
go tool covdata percent -i=coverage
```

To list what is not covered, by function:

```bash
go tool covdata textfmt -i=coverage -o coverage/profile.txt
go tool cover -func=coverage/profile.txt
```

`bin/` and `coverage/` are ignored by git. [CRP-005](docs/tickets/M0-foundation/CRP-005-ci-pipeline.md) replaces the last step with a gate tool that fails below 100.0% and prints each uncovered block.
