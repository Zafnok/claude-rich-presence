---
id: CRP-051
title: MCPB bundle
milestone: M5 Claude Desktop
type: feature
status: todo
priority: P0
blocked_by: [CRP-001, CRP-033]
blocks: [CRP-042, CRP-052, CRP-060]
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-051: MCPB bundle

## Goal

One bundle file that Claude Code can fetch through the plugin and Claude Desktop can install as an extension, containing the right binary for each operating system.

## Context

The bundle is the single delivery artifact for both surfaces ([ADR-0007](../../architecture/adr/0007-integration-and-distribution.md)). It sits in the Desktop milestone by name, but the plugin depends on it too, so it is on the critical path.

The MCPB manifest selects by operating system, not by CPU architecture. That is why the Mac binary is universal and Linux ships `amd64` only.

## Scope

- `extension/manifest.json`:
  - name, display name, version, description with the unaffiliated notice, author, license, repository;
  - server type `binary`, running the `mcp` command;
  - per-operating-system overrides selecting the Windows, Mac and Linux binaries;
  - `user_config` for the privacy level and an optional Discord application id, passed to the server as the environment variables CRP-012 reads;
  - the tools the server exposes, declared as CRP-041 and CRP-050 define them;
  - compatibility limited to the three supported operating systems.
- A build step, as a Go program under `tools/` or documented commands, that assembles the bundle from built binaries, the manifest and the icon. The bundle is a zip archive with a fixed layout.
- The Mac universal binary: produced by merging the two architecture builds on a Mac runner.
- A CI job that builds the bundle on every pull request and checks its contents: manifest valid, every referenced file present, executable permission set on the Unix binaries.
- The icon, a placeholder until CRP-003 supplies the final artwork.

## Out of scope

- Publishing, which is CRP-060.
- Signing, which is CRP-063.

## Acceptance criteria

- [ ] The bundle built in CI passes the official MCPB validator, or, if using that tool is declined under the dependency policy, a check in this repository against the published manifest schema.
- [ ] Installed in Claude Code from a local path through the plugin, the server starts on Windows. Recorded in the pull request.
- [ ] The Unix binaries inside the bundle are executable after extraction.
- [ ] The Mac binary runs on both Mac architectures. Verified in CI on whichever architecture the runner has, and by inspection of the file's architectures for the other.
- [ ] The Mac binary's ad hoc code signature is valid after merging. If merging invalidates it, the build re-signs ad hoc.
- [ ] The bundle contains the project's license and the Go project's license notice.
- [ ] The manifest's version, the binary's reported version and the plugin's version come from one source.
- [ ] Building the bundle twice from the same commit gives byte-identical output, or the reasons it cannot are documented.
- [ ] Any Go code added for assembly is covered to 100.0%.

## Notes for the implementer

- Check the current MCPB manifest specification before writing the manifest. The format has a version field and has changed.
- Zip archives do not always preserve Unix permissions. Set them explicitly when writing the archive, and test by extracting.
- Go signs Mac binaries for Apple silicon ad hoc at link time, because the system refuses to run unsigned ones. Verify what survives the merge.
- If CRP-001 showed that Claude Code's bundle loader differs from the specification in any way, follow what Claude Code actually does and note it.

## Why this model and effort

Packaging against a published format, with a few platform details to verify.

## References

- [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md)
- [ADR-0002](../../architecture/adr/0002-implementation-language.md), on targets
- MCPB manifest specification: https://github.com/modelcontextprotocol/mcpb/blob/main/MANIFEST.md
- Findings of CRP-001, in `docs/research/`
