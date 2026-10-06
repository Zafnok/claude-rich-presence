---
id: CRP-042
title: Plugin and marketplace packaging
milestone: M4 Claude Code
type: feature
status: in-progress
priority: P0
blocked_by: [CRP-001, CRP-033, CRP-051]
blocks: [CRP-045, CRP-047, CRP-048, CRP-052, CRP-060, CRP-061, CRP-073, CRP-075, CRP-079]
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-042: Plugin and marketplace packaging

## Goal

A Claude Code user runs two commands, adding the marketplace and installing the plugin, and presence works.

## Context

The plugin is JSON and Markdown only. It points at the MCPB bundle and declares the hooks ([ADR-0007](../../architecture/adr/0007-integration-and-distribution.md)). This repository is its own marketplace. CRP-001's findings fix the details of hook fields and of how the bundle is referenced.

## Scope

- `.claude-plugin/marketplace.json` at the repository root, listing one plugin whose source is `./plugin`.
- `plugin/.claude-plugin/plugin.json`:
  - name `rich-presence`, description including the unaffiliated notice, license, repository;
  - `mcpServers` set to the bundle's release URL;
  - `userConfig` for the privacy level, offered as a fixed choice of three, and for an optional Discord application id. These reach the server as environment variables that CRP-012 reads.
- `plugin/hooks/hooks.json`: one `mcp_tool` hook per event in the event table, each with the literal event name, only its allowlisted fields, and a two-second timeout. As ADR-0007's rules 6 to 9 require: the `SessionStart` hook has the matcher `clear|compact`; there is no `SessionEnd` hook; no event has more than one handler; the server address ends with the bundle manifest's `name`.
- A `.gitignore` entry for `plugin/.mcpb-cache/`, and `metadata.description` in the marketplace manifest, which strict validation requires.
- `plugin/skills/`:
  - `status`: asks Claude to call `presence_status` and report the result in plain language;
  - `privacy`: explains the three levels and how to change the setting.
- A CI step running `claude plugin validate --strict` on the plugin and the marketplace.
- A test that compares the hook file's events and fields against the allowlist table from CRP-041 and fails on any difference.
- A development workflow, documented: how to load the plugin from a local directory with a locally built bundle.

## Out of scope

- Building the bundle, which is CRP-051.
- Writing the release URL at release time, which is CRP-060.
- End-user documentation beyond the plugin's own description, which is CRP-061.

## Acceptance criteria

- [x] `claude plugin validate --strict` passes for the plugin and the marketplace, in CI.
- [x] The plugin directory contains no executable, no script, and no top-level `bin/`.
- [ ] Installed from a local marketplace with a locally built bundle, a real Claude Code session shows presence in a real Discord client, on Windows. What was run and what Discord showed is recorded in the pull request.
- [x] The hook file and the adapter's allowlist are proven identical by a test.
- [x] Every hook has an explicit timeout.
- [ ] Starting an interactive session shows no hook error under the banner, and exiting prints none. Recorded in the pull request. CRP-001 saw both errors with a hook file that lacked the matcher and declared `SessionEnd`.
- [x] A test checks that the server address in the hook file matches the bundle manifest's `name`.
- [ ] Changing the privacy setting through Claude Code's plugin configuration changes what is published, after a session restart.
- [ ] Disabling the plugin stops the server and clears presence.
- [x] A test checks that the plugin manifest's `version` equals the contents of the repository's `VERSION` file, which is also the version in the bundle manifest and in the binaries ([CRP-051](../done/CRP-051-mcpb-bundle.md)).
- [x] The minimum Claude Code version the plugin needs is determined, stated in the plugin description and in the README.

## State on 2026-10-05

Built and checked with Claude Code 2.1.288 on Windows. The boxes ticked above are held by tests in `internal/adapter/code/plugin_test.go` and by the `Plugin` job in CI. The others wait for:

- the owner, for the four criteria that need an interactive session and a real Discord client. The steps are in the pull request. They also need a Discord application id, which [CRP-003](../M0-foundation/CRP-003-naming-branding-discord-app.md) has not supplied yet: the built-in one is a placeholder that Discord rejects, so the check must set the plugin's application id option to one the owner controls.

Found here, and not as the ticket assumed:

- A plugin option does reach the bundled server, through the bundle's `${user_config.KEY}` of the same name. But a `default` on the bundle's own entry overrode the user's choice: with the bundle's default of `standard`, choosing `full` in the plugin still gave the server `standard`. The default was removed from `extension/manifest.json`, and `mcpb check` now refuses one.
- Seen with a stand-in server that records its environment, in an isolated configuration directory: unset gives `standard` and an empty application id; `privacy=full` and an application id set through `claude plugin configure` both arrive; after `claude plugin disable`, no server is started.
- The minimum is Claude Code 2.1.271, the first version with fixed-choice options. An older one cannot load the plugin. The lowest version the wiring has actually run on is 2.1.284, in CRP-001.

## Notes for the implementer

- Check the current plugin manifest and marketplace references before writing the files. Field names here have changed between releases.
- Read the [CRP-001 findings](../../research/crp-001-claude-code-adapter.md) first. Points that bear on this ticket: a marketplace added from a local directory loads the plugin in place and writes `.mcpb-cache/` into it; a URL bundle is fetched on first load, not at install, and a failed fetch is silent; a new bundle URL takes effect only when the plugin `version` changes; a `user_config` default in the bundle reached the server with no prompt, while plugin-level `userConfig` reaching a bundled server was not tested, so test it here before relying on it.
- The fixed-choice form of `userConfig` needs a recent Claude Code. If that minimum is unacceptable, use a free-text string and validate it in CRP-012, which already falls back to the default on bad input.
- The server is addressed in hooks by its scoped name, which includes the plugin name. A rename under [ADR-0010](../../architecture/adr/0010-naming-and-branding.md) must change both.
- The manual check needs the owner's machine and Discord. Ask the owner to run it and record what they saw.

## Why this model and effort

Configuration against documented formats, with a few consistency checks.

## References

- [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md), [ADR-0010](../../architecture/adr/0010-naming-and-branding.md)
- Findings of CRP-001, in `docs/research/`
- Plugin manifest reference: https://code.claude.com/docs/en/plugins-reference
- Plugin marketplaces: https://code.claude.com/docs/en/plugin-marketplaces
