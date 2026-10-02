---
name: record-decision
description: Record an architecture decision as an ADR in docs/architecture/adr, or supersede an existing one. Use when a ticket forces a choice no ADR covers, when evidence contradicts an accepted ADR, when a spike settles a Proposed ADR, or before adding a dependency.
---

# Record a decision

## Does this need an ADR?

| Situation | Where it goes |
|---|---|
| It constrains later tickets, adds a dependency, changes a wire format, changes what data leaves the adapter, or reverses an earlier decision | An ADR |
| It is local to one package and easy to change later | The pull request description |
| It is a fact discovered about a vendor's behaviour | `docs/research/`, and the relevant skill if it is a reference others need |

When unsure, write the ADR. A short ADR costs little; a decision nobody can find costs more.

## Writing one

1. Take the next number. Name the file `NNNN-short-slug.md` in `docs/architecture/adr/`.
2. Use the five sections, in order: Status, Context, Decision, Consequences, Alternatives considered. The format is described in `docs/architecture/adr/README.md`.
3. In **Context**, state each fact with how it is known: read in documentation, with the link and date; observed, with where; or unverified. Do not present an inference as a fact.
4. In **Decision**, write rules a reviewer could check code against.
5. In **Consequences**, include what gets worse. An ADR with only benefits has not been thought through.
6. In **Alternatives considered**, give each real alternative one honest reason it lost.
7. Add the row to the table in `docs/architecture/adr/README.md`.

## Status

| Status | Meaning |
|---|---|
| Proposed | Depends on something not yet known. Name the ticket that will settle it |
| Accepted | Binding |
| Superseded by ADR-NNNN | Kept for history |

## Changing a decision

Accepted ADRs are not edited to say something different. Write a new ADR that supersedes the old one, set the old one's status to `Superseded by ADR-NNNN`, and update the index. Small clarifications that do not change the decision may be edited in place.

When a spike settles a Proposed ADR, change its status, replace the "pending" wording with what was found, and link the findings in `docs/research/`.

## After recording

Check which tickets the decision affects and amend them with the `write-ticket` skill. If `CLAUDE.md` lists a rule the decision changes, update it in the same pull request.
