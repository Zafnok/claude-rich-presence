# Repository layout

The target structure. Today only the Markdown exists. Each other path names the ticket that creates it.

```text
claude-rich-presence/
├── README.md
├── LICENSE.md
├── CLAUDE.md                         guidance for Claude sessions in this repository
├── CONTRIBUTING.md
├── SECURITY.md                       CRP-062
├── go.mod                            CRP-004
├── sonar-project.properties          CRP-006
│
├── .claude/
│   └── skills/                       skills that help build this project
│
├── .claude-plugin/
│   └── marketplace.json              CRP-042  this repository is its own plugin marketplace
│
├── .github/
│   ├── workflows/                    CRP-005 ci, CRP-006 sonar, CRP-007 supply chain, CRP-060 release
│   └── dependabot.yml                CRP-007
│
├── cmd/
│   └── rich-presence/                CRP-033  main: a few lines that call internal/cli
│
├── internal/
│   ├── domain/                       CRP-010  events, session state machine, registry
│   ├── presence/                     CRP-011  focus selection, activity rendering, privacy levels
│   ├── schedule/                     CRP-013  rate limiting and coalescing over an injected clock
│   ├── config/                       CRP-012  defaults, file, environment, validation
│   ├── discord/
│   │   ├── codec/                    CRP-020  frames, handshake, set-activity, responses
│   │   ├── transport/                CRP-021  dialers per operating system, pipe discovery
│   │   └── session/                  CRP-023  connect, reconnect, apply, clear
│   ├── control/
│   │   ├── protocol/                 CRP-030  message types and codec
│   │   └── transport/                CRP-031  socket path, listener, dialer, host lock
│   ├── host/                         CRP-032  election, serving followers, failover, wiring
│   ├── mcp/                          CRP-040  minimal stdio JSON-RPC server
│   ├── adapter/
│   │   ├── code/                     CRP-041  presence_event tool to domain events
│   │   ├── desktop/                  CRP-050  lifecycle only
│   │   └── statusline/               CRP-074  status line data to session facts
│   ├── diag/                         CRP-034  logging, doctor checks
│   ├── cli/                          CRP-033  command dispatch and the composition root
│   └── testutil/
│       ├── fakediscord/              CRP-022  scripted Discord IPC server
│       ├── fakeclock/                CRP-013
│       └── mcpclient/                CRP-043  scripted MCP client for end-to-end tests
│
├── tools/
│   └── covercheck/                   CRP-005  fails the build below 100.0% statement coverage
│
├── test/
│   └── e2e/                          CRP-043  black-box tests of the built binary
│
├── plugin/                           CRP-042  the Claude Code plugin: JSON and Markdown only
│   ├── .claude-plugin/plugin.json
│   ├── hooks/hooks.json
│   └── skills/                       user-facing skills: status, privacy
│
├── extension/                        CRP-051  MCPB bundle sources
│   ├── manifest.json
│   └── icon.png                      CRP-003 supplies the artwork
│
└── docs/
    ├── architecture/                 this directory
    │   └── adr/
    ├── protocol/                     CRP-030  control protocol reference
    ├── research/                     spike findings: CRP-001, CRP-002, CRP-045
    ├── user/                         CRP-061  install, configure, privacy, troubleshooting
    └── tickets/
```

## Rules

| Rule | Reason |
|---|---|
| Everything under `internal/` | Nothing here is a library for others. It keeps the public surface at zero |
| Dependencies point inward: `adapter`, `host`, `discord/session`, `cli` may import `domain`; `domain` imports none of ours | [ADR-0001](adr/0001-core-architecture.md) |
| Only `internal/cli` constructs real operating-system implementations | One composition root |
| Platform-specific code is confined to `discord/transport` and `control/transport`, in files suffixed `_windows.go` and `_unix.go`, behind an interface declared in a platform-neutral file | Keeps conditional compilation out of logic |
| `plugin/` has no top-level `bin/` directory and no executables | [ADR-0007](adr/0007-integration-and-distribution.md) |
| Spike code is never merged. Findings go to `docs/research/` | [ADR-0009](adr/0009-quality-gates.md) |
| Names are defined once: a constants file in `internal/cli` and the release workflow | [ADR-0010](adr/0010-naming-and-branding.md) |

## Why the plugin and extension are separate directories

They are different manifests for different hosts. `plugin/` is fetched by Claude Code from git and contains no binary; it points at the bundle. `extension/` is the source of the bundle that the release pipeline builds, which is also what Claude Desktop installs directly.

## Files that are not Markdown and do not exist yet

The architecture pull request contains Markdown only, by instruction. That leaves a few repository basics to the first implementation ticket, [CRP-004](../tickets/M0-foundation/CRP-004-repo-scaffolding.md): `.gitignore`, `.gitattributes`, `.editorconfig`, and `go.mod`.
