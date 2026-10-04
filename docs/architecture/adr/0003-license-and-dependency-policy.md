# ADR-0003: MIT license and dependency policy

## Status

Accepted.

## Context

The owner asked for a permissive license, no copyleft or other license that would affect ours, no dependencies on small single-maintainer repositories, and low cost.

Every maintained Discord presence library we found, in every language, is effectively maintained by one person ([viability](../viability.md)). The Go libraries in particular depend on a named-pipe package last touched in 2016.

## Decision

### License

The project is licensed under the **MIT License**. The text is in [LICENSE.md](../../../LICENSE.md).

Reasons for MIT over the other permissive options:

| Option | Assessment |
|---|---|
| **MIT** | Shortest and most widely understood. Compatible with everything, including GPLv2-only consumers. Matches the Claude plugin and MCP ecosystems |
| Apache-2.0 | Adds an explicit patent grant and a NOTICE mechanism. Nothing here is patentable, so the grant buys little, and the NOTICE obligation is extra bookkeeping for binary distribution |
| BSD-3-Clause | Equivalent to MIT in practice, less common in this ecosystem |
| Unlicense, 0BSD | Public-domain style dedications are less well tested legally in some jurisdictions |

Contributions are accepted under the same license, inbound equals outbound, with no contributor agreement.

### Runtime dependencies

Code that is compiled into the released binary.

1. **Pre-approved**: the Go standard library, and `golang.org/x/*` modules maintained by the Go project.
2. **Conditionally approved**: `github.com/Microsoft/go-winio` (MIT, owned by Microsoft), only if the standard-library named-pipe path proves inadequate in [CRP-021](../../tickets/done/CRP-021-discord-transport.md), and only by a superseding note on [ADR-0004](0004-in-house-protocol-implementations.md).
3. **Everything else requires an ADR** that shows all of:
   - a license on the allowlist below;
   - an organisation or at least three active maintainers behind it, with a release in the past twelve months;
   - no transitive dependency that fails this policy;
   - that writing the needed part ourselves would cost more than maintaining it.

### Test, build and CI dependencies

Tools that are not compiled into the binary.

1. The same license allowlist applies to anything linked into test binaries.
2. Standalone tools may carry any license, since running a tool does not affect ours, but must be pinned to an exact version, and GitHub Actions to a full commit hash.
3. Prefer tools from the Go project (`go vet`, `govulncheck`), GitHub, Google or SonarSource over individually maintained ones. A tool from an individual needs a line of justification in the pull request that introduces it.

### License allowlist

Allowed: MIT, BSD-2-Clause, BSD-3-Clause, ISC, Apache-2.0, 0BSD, Unlicense.

Not allowed in anything linked: GPL, LGPL, AGPL, MPL, EPL, CDDL, SSPL, BUSL, Commons Clause, or any custom or missing license.

### Enforcement

CI fails if `go.mod` gains a module outside the pre-approved set without a matching ADR, if any linked module's license is off the allowlist, or if `govulncheck` reports a reachable vulnerability. Implemented in [CRP-007](../../tickets/M0-foundation/CRP-007-supply-chain-policy.md).

### Cost

Everything used is free for a public repository: GitHub Actions, SonarQube Cloud, GitHub Releases, build provenance attestations. The only items that could cost money are code-signing certificates and Apple notarisation, deferred to [CRP-063](../../tickets/M6-release/CRP-063-code-signing.md) as an owner decision.

## Consequences

- The released binary's notices are our own MIT notice plus the Go project's BSD-3-Clause notice. The release pipeline must ship both.
- We write and maintain a Discord IPC client and a minimal MCP server ourselves ([ADR-0004](0004-in-house-protocol-implementations.md)).
- Adding convenience libraries is deliberately slow. That is the intent.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Use an existing Discord presence library | All are single-maintainer; see context |
| Allow MPL-2.0 or LGPL with care | File-level and linking copyleft create obligations for a statically linked Go binary that are easy to get wrong. Nothing we need is only available under them |
| A contributor license agreement | Unnecessary friction for a small MIT project |
