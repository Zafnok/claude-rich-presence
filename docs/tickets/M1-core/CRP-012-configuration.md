---
id: CRP-012
title: Configuration
milestone: M1 Core
type: feature
status: todo
priority: P0
blocked_by: [CRP-004]
blocks: [CRP-014, CRP-033, CRP-034, CRP-041]
model: claude-sonnet-5-5
effort: medium
size: M
---

# CRP-012: Configuration

## Goal

One validated configuration value, built from defaults, an optional file and environment variables, that can never stop the program from starting.

## Context

Settings arrive three ways: from Claude Code's plugin configuration and from Claude Desktop's extension configuration, both as environment variables on the adapter process, and from an optional file for people who want to set things by hand. Some settings belong to the adapter and some to the host.

## Scope

Package `internal/config`:

| Setting | Values | Default | Applied by |
|---|---|---|---|
| `enabled` | true, false | true | Adapter |
| `privacy` | `minimal`, `standard`, `full` | `standard` | Adapter |
| `discord_application_id` | A numeric string | The project's application id | Host |
| `idle_clear_after` | A duration, zero meaning never | 15 minutes | Host |
| `min_update_interval` | A duration, floor of 4 seconds | 15 seconds | Host |
| `log_level` | `error`, `warn`, `info`, `debug` | `warn` | Both |

- Precedence, highest first: environment variables named `RICH_PRESENCE_` plus the upper-cased setting name, the file, defaults.
- The file is JSON, named `config.json`, in a `rich-presence` directory under the operating system's user configuration directory.
- A function that resolves the configuration, log and runtime directories for the current operating system, taking the environment as a parameter.
- Validation that reports every problem and then **falls back to the default for that setting**. A missing file is normal. A malformed file, an unknown key or an invalid value produces a warning and never an error exit.

## Out of scope

- The control socket path rules, which are CRP-031. This ticket supplies the base directory.
- Wiring plugin and extension options to environment variables, which is CRP-042 and CRP-051.

## Acceptance criteria

- [ ] Each setting is tested for default, file, environment, and environment over file.
- [ ] Each invalid value falls back to its default and yields exactly one warning that names the setting and the source, without echoing the invalid value if it could be long.
- [ ] A malformed or unreadable file yields defaults plus one warning.
- [ ] Unknown keys in the file yield a warning each and are otherwise ignored.
- [ ] `min_update_interval` below the floor is raised to the floor with a warning.
- [ ] Directory resolution is tested for Windows, macOS and Linux inputs on every operating system, by passing the environment in.
- [ ] Reading the file goes through an interface, and read errors other than "not found" are covered.
- [ ] A fuzz test shows no input can make loading panic.

## Notes for the implementer

- JSON is chosen because the standard library reads it. Do not add a TOML or YAML dependency.
- The default application id is a placeholder constant until CRP-003 supplies the real one. It is public, not a secret.
- Durations in the file are strings such as `15m`, parsed by the standard library.

## Why this model and effort

Ordinary work with many small cases.

## References

- [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- [ADR-0006](../../architecture/adr/0006-control-channel.md)
