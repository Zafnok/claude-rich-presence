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
| Ticket status | Update the `status` field in the ticket's own file, in the same pull request |

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

Runtime dependencies are limited to the Go standard library and `golang.org/x/*`. Anything else needs an ADR. Build and CI tools are pinned to an exact version or commit. The full policy is [ADR-0003](docs/architecture/adr/0003-license-and-dependency-policy.md).

## Security and privacy

Do not add code that reads prompts, transcripts, tool inputs or file paths, or that makes a network request. Report vulnerabilities privately to the repository owner through GitHub's security advisory form rather than in a public issue.

## License

By contributing you agree that your contribution is licensed under the [MIT License](LICENSE.md).
