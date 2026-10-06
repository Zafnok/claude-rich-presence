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

Runtime dependencies are limited to the Go standard library and `golang.org/x/*`. Anything else needs an ADR. Build and CI tools are pinned to an exact version or commit. The full policy is [ADR-0003](docs/architecture/adr/0003-license-and-dependency-policy.md), and CI enforces it:

- A module that supplies a package on any release target and is neither under `golang.org/x/` nor listed in [.github/allowed-modules.txt](.github/allowed-modules.txt) fails the build. Each line of that file names the accepted ADR that approved the module. Add the line in the same pull request as the module, after the ADR is accepted. The check is [tools/policycheck](tools/policycheck/main.go).
- Every linked module must have a license on the ADR-0003 allowlist, checked with a pinned `go-licenses` for each release target.
- Every `uses:` in a workflow must be a full commit hash, with the tag in a comment. Dependabot keeps them current.
- `govulncheck` runs on every pull request and every Monday.

The checks are in [ci.yml](.github/workflows/ci.yml) and [supply-chain.yml](.github/workflows/supply-chain.yml). The settings the owner applies to the repository are in [docs/repository-settings.md](docs/repository-settings.md).

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
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
```

```bash
CGO_ENABLED=1 go test -race ./...
```

`gofmt -l .` must print nothing. The static analyser is [Staticcheck](https://staticcheck.dev/), pinned to the version in [the workflow](.github/workflows/ci.yml); use the same one.

### End-to-end tests

`go test ./...` includes them. To run them alone:

```bash
go test -count=1 ./test/e2e
```

They build the binary themselves and start it as real processes, with a fake Claude Code and a fake Discord. Each scenario has its own runtime directory, home directory and Discord endpoint name, so they never touch a presence host or a Discord that is running on your machine. A scenario that fails prints what the fake Discord recorded, the product's log and what each process wrote to standard error.

The latency scenario allows the event tool 10 milliseconds at the 99th percentile on your machine and 100 in CI, where the `CI` environment variable is set. The reason for the difference is written beside the two numbers in [latency_test.go](test/e2e/latency_test.go).

### Fuzz

Every fuzz target runs briefly in CI. To run the ones in a package you touched:

```bash
pkg=./tools/covercheck; for t in $(go test -list '^Fuzz' $pkg | grep '^Fuzz'); do go test -run '^$' -fuzz "^$t\$" -fuzztime 10s $pkg || break; done
```

### Measure coverage

Statement coverage must be 100.0% of the measured set, which is every package except those under `internal/testutil`. That rule is written once, in [tools/covercheck](tools/covercheck/main.go), and the test command takes its package list from there.

`main` cannot be called from a test, so it is covered by running binaries built with coverage instrumentation. The end-to-end tests in [test/e2e](test/e2e/doc.go) build the product that way and run it as real processes, and the two tools are run by hand below. The gate reads the unit-test profile and the profile of those runs together.

```bash
rm -rf coverage && mkdir -p coverage/e2e
go build -cover -covermode=atomic -o bin/ ./tools/covercheck ./tools/mcpb ./tools/policycheck
measured=$(go list ./... | GOCOVERDIR=coverage/e2e bin/covercheck packages)
export RICH_PRESENCE_E2E_COVERDIR="$(go list -m -f '{{.Dir}}')/coverage/e2e"
CGO_ENABLED=1 go test -count=1 -race -coverpkg="$measured" -coverprofile=coverage/unit.txt ./...
code=0; GOCOVERDIR=coverage/e2e bin/mcpb || code=$?; test "$code" -eq 2   # no command is a usage error, which covers its main function
for target in windows/amd64 darwin/amd64 darwin/arm64 linux/amd64; do
  GOOS=${target%/*} GOARCH=${target#*/} CGO_ENABLED=0 go list -deps -test -json ./... |
    GOCOVERDIR=coverage/e2e bin/policycheck modules .github/allowed-modules.txt
done
GOCOVERDIR=coverage/e2e bin/policycheck actions .github/workflows/*.yml
go tool covdata textfmt -i=coverage/e2e -o coverage/e2e.txt
{ cat coverage/unit.txt; tail -n +2 coverage/e2e.txt; } > coverage/profile.txt
go run ./tools/covercheck check -module "$(go list -m)" coverage/profile.txt
```

`RICH_PRESENCE_E2E_COVERDIR` must be an absolute path. `-count=1` is there because a cached result of the end-to-end tests starts no process, and so writes no coverage.

The last command prints each uncovered block as `file:line` and fails unless coverage is 100.0%. To see a file line by line:

```bash
go tool cover -html=coverage/profile.txt
```

The unit tests must write their profile with `-coverprofile`. A package that no test reaches appears there with nothing covered; in the binary coverage data that `-test.gocoverdir` writes, it does not appear at all, and the gate would not see it.

`bin/` and `coverage/` are ignored by git.

### Continuous integration

[The workflow](.github/workflows/ci.yml) runs the steps above, in that order, on Linux, macOS and Windows for every pull request and every push to `main`. Each operating system must reach 100.0% over the files it compiles. Its coverage profile is kept as an artifact named `coverage-` followed by the runner name. The check named `CI` passes only when all three pass.

### The bundle

`rich-presence.mcpb` is the one archive that Claude Code fetches through the plugin and Claude Desktop installs as an extension ([ADR-0007](docs/architecture/adr/0007-integration-and-distribution.md)). Its parts are in [extension/](extension/): the manifest, the icon (a placeholder until [CRP-003](docs/tickets/M0-foundation/CRP-003-naming-branding-discord-app.md) supplies one) and the third-party notices. [tools/mcpb](tools/mcpb/main.go) assembles it and checks it.

The version has one source, the [VERSION](VERSION) file. `mcpb build` writes it into the manifest in place of `@VERSION@`, and the binaries get it from the linker. The plugin manifest and the release pipeline of [CRP-060](docs/tickets/M6-release/CRP-060-release-pipeline.md) read the same file.

```bash
version=$(cat VERSION)
ldflags="-s -w -buildid= -X $(go list -m)/internal/cli.version=$version"
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o dist/rich-presence.exe ./cmd/rich-presence
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o dist/rich-presence-linux ./cmd/rich-presence
```

The Mac binary is universal and must be merged on a Mac, with `lipo -create -output dist/rich-presence-darwin` over the two `darwin` builds and then `codesign --force --sign - dist/rich-presence-darwin`. The `bundle-darwin` job of CI does exactly that. Then:

```bash
go run ./tools/mcpb build -version "$version" -out dist/rich-presence.mcpb \
  -manifest extension/manifest.json -icon extension/icon.png \
  -license LICENSE.md -notices extension/THIRD-PARTY-NOTICES.md \
  -windows dist/rich-presence.exe -darwin dist/rich-presence-darwin -linux dist/rich-presence-linux
go run ./tools/mcpb check -version "$version" dist/rich-presence.mcpb
```

`check` is this repository's check against the published manifest schema (version 0.3), plus the decisions of the ADRs: the layout, the server name, the settings, the executable bit of the Unix binaries and the architectures inside each binary. It is used instead of the official validator, which would be a new tool in CI under the dependency policy.

The archive is reproducible: the same binaries give the same bytes. The Windows and Linux binaries are too, since they are built with `-trimpath` and no build id. The Mac binary is signed ad hoc on the runner, and whether `codesign` gives the same bytes twice is not checked, so CI compares two assemblies of the Windows and Linux builds only.

### The plugin

The Claude Code plugin is [plugin/](plugin/), and this repository is its marketplace through [.claude-plugin/marketplace.json](.claude-plugin/marketplace.json). The plugin is JSON and Markdown only: a manifest that names the bundle's release URL, one hook per event, and two skills. The tests in [internal/adapter/code/plugin_test.go](internal/adapter/code/plugin_test.go) hold these files to the adapter's allowlist, the bundle manifest and the [VERSION](VERSION) file, and CI runs Claude Code's own validator on them:

```bash
claude plugin validate --strict ./plugin
```

```bash
claude plugin validate --strict .
```

Two facts about settings, found by [CRP-042](docs/tickets/M4-claude-code/CRP-042-plugin-packaging.md) with Claude Code 2.1.288:

- An option in the plugin's `userConfig` reaches the bundled server through the `${user_config.KEY}` reference of the same name in the bundle's manifest. So the two files declare the same keys.
- A `default` on the bundle's own `user_config` entry wins over the value the user chose in the plugin. So the bundle declares no defaults, and `mcpb check` refuses one. The server's own defaults apply to an empty value.

#### Run the plugin with a bundle you built

The committed manifest names a release URL, which Claude Code downloads. To try a local build, make a copy of the marketplace whose plugin names a bundle file instead. Nothing tracked is changed, and `dist/` is ignored.

1. Build `dist/rich-presence.mcpb` as [The bundle](#the-bundle) describes. Off a Mac, take the Mac binary from CI instead of building it: `gh run download --name bundle-darwin --dir dist` on any run of the CI workflow.
2. Make the copy:

   ```bash
   rm -rf dist/marketplace && mkdir -p dist/marketplace
   cp -r .claude-plugin plugin dist/marketplace/
   rm -rf dist/marketplace/plugin/.mcpb-cache
   cp dist/rich-presence.mcpb dist/marketplace/plugin/
   sed -i.bak 's#"mcpServers": "[^"]*"#"mcpServers": "./rich-presence.mcpb"#' dist/marketplace/plugin/.claude-plugin/plugin.json
   rm dist/marketplace/plugin/.claude-plugin/plugin.json.bak
   ```

3. For one session, with nothing installed:

   ```bash
   claude --plugin-dir dist/marketplace/plugin
   ```

   Or install it as a user would, from the local marketplace:

   ```bash
   claude plugin marketplace add ./dist/marketplace
   claude plugin install rich-presence@rich-presence
   ```

4. `claude mcp list` shows `plugin:rich-presence:presence` as connected. In a session, `/rich-presence:status` reports what the server sees.
5. To remove it: `claude plugin marketplace remove rich-presence`.

A marketplace added from a directory is loaded in place, so after rebuilding the bundle, repeat step 2 and start a new session. Claude Code writes `.mcpb-cache/` into the plugin directory it loads, with absolute paths inside. That is why the copy is made under `dist/`, and why `plugin/.mcpb-cache/` is ignored in case the repository root itself is ever added as a marketplace.

The privacy level is set with `/plugin`, under the plugin's configuration, or from a shell with `echo '{"privacy":"full"}' | claude plugin configure rich-presence@rich-presence --values-stdin`. It is read when the server starts, so it applies to the next session.
