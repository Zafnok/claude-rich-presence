# Repository settings

Settings that live in GitHub rather than in the repository, so a pull request cannot change them. The owner applies them once, in the repository's Settings page. Nothing here can be applied by a pull request or by Claude, because it needs the repository administrator's permissions.

Checked against GitHub's documentation for branch protection rules on 2026-10-04. The names below are the ones the Settings page shows; if GitHub renames one, the intent is the sentence after it.

## Branch protection for `main`

Settings, Branches, Add branch ruleset (or Add classic branch protection rule), target `main`.

| Setting | Value | Why |
|---|---|---|
| Require a pull request before merging | On | Nothing reaches `main` without CI having run on it |
| Require status checks to pass | On | See the list below |
| Require branches to be up to date before merging | On | The checks ran on what will be merged |
| Block force pushes | On | History on `main` cannot be rewritten |
| Restrict deletions | On | `main` cannot be deleted |
| Include administrators (do not allow bypass) | On | The owner is held to the same checks |

Required status checks. Each is one job that stands for a whole matrix, so it is the only name to require and survives a change to the matrix:

| Check | Workflow | What it stands for |
|---|---|---|
| `CI` | [ci.yml](../.github/workflows/ci.yml) | Format, vet, Staticcheck, build for every release target, tests with the race detector, fuzz smoke, the coverage gate, the module allowlist and the action pinning check, on Linux, macOS and Windows |
| `Supply chain` | [supply-chain.yml](../.github/workflows/supply-chain.yml) | The license check for every release target, and `govulncheck` on Linux, macOS and Windows |

CRP-006 adds the SonarQube Cloud check to this list when it lands.

A required check must have run once before GitHub offers it in the search box, so open the pull request that adds these workflows first, then add the rule.

## Actions

Settings, Actions, General.

| Setting | Value | Why |
|---|---|---|
| Workflow permissions | Read repository contents | The workflows also declare `permissions: contents: read`; this is the default underneath them |
| Allow GitHub Actions to create and approve pull requests | Off | Nothing here needs it |

## Dependabot

Settings, Code security. Turn on Dependabot alerts and Dependabot security updates. The weekly version updates are configured in [.github/dependabot.yml](../.github/dependabot.yml) and need no setting. Both ecosystems open one grouped pull request a week.

## When this changes

If a workflow job is renamed, or a new required workflow is added, update the table above in the same pull request, and ask the owner to update the rule.
