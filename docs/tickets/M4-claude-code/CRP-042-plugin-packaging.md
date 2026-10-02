---
id: CRP-042
title: Plugin and marketplace packaging
milestone: M4 Claude Code
type: feature
status: todo
priority: P0
blocked_by: [CRP-001, CRP-033, CRP-051]
blocks: [CRP-045, CRP-060, CRP-061]
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
- `plugin/hooks/hooks.json`: one `mcp_tool` hook per event in the event table, each with the literal event name, only its allowlisted fields, and a two-second timeout.
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

- [ ] `claude plugin validate --strict` passes for the plugin and the marketplace, in CI.
- [ ] The plugin directory contains no executable, no script, and no top-level `bin/`.
- [ ] Installed from a local marketplace with a locally built bundle, a real Claude Code session shows presence in a real Discord client, on Windows. What was run and what Discord showed is recorded in the pull request.
- [ ] The hook file and the adapter's allowlist are proven identical by a test.
- [ ] Every hook has an explicit timeout.
- [ ] Changing the privacy setting through Claude Code's plugin configuration changes what is published, after a session restart.
- [ ] Disabling the plugin stops the server and clears presence.
- [ ] The minimum Claude Code version the plugin needs is determined, stated in the plugin description and in the README.

## Notes for the implementer

- Check the current plugin manifest and marketplace references before writing the files. Field names here have changed between releases.
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
