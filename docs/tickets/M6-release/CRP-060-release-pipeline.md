---
id: CRP-060
title: Release pipeline
milestone: M6 Release
type: chore
status: todo
priority: P1
blocked_by: [CRP-003, CRP-005, CRP-007, CRP-042, CRP-043, CRP-051]
blocks: [CRP-063, CRP-064]
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-060: Release pipeline

## Goal

Pushing a version tag produces a complete, verifiable release and points the plugin at it, with no manual steps.

## Context

The plugin on `main` names the bundle by URL. That URL must never point at something that does not exist yet, or users who update in the gap get a broken plugin. So assets are published first, and the plugin is updated second.

## Scope

A workflow triggered by a tag of the form `vMAJOR.MINOR.PATCH`:

1. Verify the tag is on `main` and that CI passed for that commit.
2. Build every target from [ADR-0002](../../architecture/adr/0002-implementation-language.md) with cgo disabled, trimmed paths, and the version set from the tag.
3. Merge the Mac universal binary on a Mac runner.
4. Assemble the bundle with the build step from CRP-051.
5. Produce a checksums file covering every asset.
6. Produce a build provenance attestation for every asset, using GitHub's attestation action.
7. Create the GitHub Release with the bundle, the standalone binaries, the checksums, the license, and third-party notices.
8. Only after the release is published: update the plugin manifest's bundle URL and version, and the marketplace entry, and land that change on `main`.
9. Generate release notes from Conventional Commits since the previous tag.

Also:

- The third-party notices file: the Go project's license, and any module approved under [ADR-0003](../../architecture/adr/0003-license-and-dependency-policy.md).
- A documented procedure for the owner: how to cut a release, and how to withdraw one.

## Out of scope

- Signing and notarisation, which is CRP-063.
- Package managers such as Homebrew, Scoop or winget. Not in the first release.

## Acceptance criteria

- [ ] A tag on a commit that is not on `main`, or whose CI did not pass, produces no release.
- [ ] A dry run, on a pre-release tag in a fork or with a pre-release flag, produces every asset and attaches them to a pre-release.
- [ ] Every asset's checksum matches the checksums file, and `gh attestation verify` succeeds for each.
- [ ] The plugin change in step 8 is made only after the release assets are downloadable, and the bundle URL it writes returns the bundle.
- [ ] After a release, a fresh install of the plugin from the marketplace in Claude Code starts the released binary, and the version it reports matches the tag. Recorded for the first release.
- [ ] The workflow's permissions are the minimum for each job, and only the publishing job can write.
- [ ] Every action is pinned to a commit hash.
- [ ] The release procedure and the withdrawal procedure are documented in `CONTRIBUTING.md`.

## Notes for the implementer

- The Windows binary links Microsoft's go-winio ([ADR-0017](../../architecture/adr/0017-go-winio-for-windows-pipes.md)). Its MIT notice ships with the Windows artifacts, beside ours and the Go project's.
- Step 8 writes to `main`. Decide between a direct commit by the workflow and an automatically opened pull request, considering branch protection from CRP-007, and record the choice.
- Do not introduce a third-party release tool. The Go toolchain, the GitHub CLI and GitHub's own actions are sufficient.
- Withdrawing a release means pointing the plugin back at the previous bundle, not deleting assets that installed plugins still reference.
- The first release should be a pre-release, exercised end to end before a stable tag.

## Why this model and effort

Mostly conventional, but ordering and permissions mistakes here reach every user.

## References

- [ADR-0003](../../architecture/adr/0003-license-and-dependency-policy.md), [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md)
- [Risk register](../../architecture/risks.md), R15
- GitHub artifact attestations: https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations
