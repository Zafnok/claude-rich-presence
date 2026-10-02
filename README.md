# claude-rich-presence

Discord Rich Presence for Claude Code and Claude Desktop.

> **Status: architecture phase.** This repository currently contains the design, decisions and work plan only. There is no implementation yet. Start with [docs/architecture/viability.md](docs/architecture/viability.md).

> **Unofficial.** This project is not affiliated with, endorsed by, or sponsored by Anthropic or Discord. "Claude" and "Claude Code" are trademarks of Anthropic, PBC. "Discord" is a trademark of Discord Inc.

## What it will do

Show what you are doing with Claude in your Discord profile: that a session is open, whether Claude is working, waiting for you, or idle, and for how long. Optionally the model and the project name.

| Surface | Support | Detail shown |
|---|---|---|
| Claude Code in a terminal | Yes | Working, waiting, idle, elapsed time, optional model and project |
| Claude Desktop, Code tab (local sessions) | Yes | Same as above |
| Claude Code in VS Code or JetBrains | Expected, to be verified | Same as above |
| Claude Desktop, Chat | Partial | "Claude is open" and elapsed time only |
| Claude Code on the web, cloud sessions, claude.ai in a browser, mobile | No | Discord's interface is local to your machine |

Why Chat is partial and the web is unsupported is explained in the [viability assessment](docs/architecture/viability.md).

## How it works, in one paragraph

One small native program, `rich-presence`, is started by Claude itself as a local MCP server: by a Claude Code plugin in Claude Code, and by a desktop extension in Claude Desktop. Claude Code hooks report session events to it. Whichever copy starts first becomes the *presence host*: it keeps the single connection to the Discord client and renders one activity from all open sessions. The others forward their events to it and take over if it exits. Nothing is installed as a service, nothing runs after Claude closes, and nothing leaves your machine except the activity sent to your local Discord client.

## Documentation

| Read this | For |
|---|---|
| [docs/architecture/viability.md](docs/architecture/viability.md) | Whether and how this can be built, with evidence |
| [docs/architecture/README.md](docs/architecture/README.md) | The architecture: components, data flow, state model |
| [docs/architecture/adr/](docs/architecture/adr/README.md) | Each decision and its reasoning |
| [docs/architecture/quality-strategy.md](docs/architecture/quality-strategy.md) | Testing, 100% coverage, SonarQube |
| [docs/architecture/repository-layout.md](docs/architecture/repository-layout.md) | The target folder structure |
| [docs/architecture/risks.md](docs/architecture/risks.md) | Open risks and what retires each one |
| [docs/tickets/](docs/tickets/README.md) | The work plan: tickets, blockers, acceptance criteria, recommended model and effort |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to work in this repository |

## Principles

- **Never get in Claude's way.** Presence is cosmetic. It must not slow, block or break a session.
- **Documented interfaces only.** No transcript reading, log scraping, window inspection or private files.
- **Private by default.** No prompts, file paths or tool inputs, ever. Project names are opt-in. No telemetry, no network access.
- **No third-party runtime dependencies** unless an ADR justifies one.
- **Every line tested.** 100% statement coverage, enforced in CI.

## License

[MIT](LICENSE.md).
