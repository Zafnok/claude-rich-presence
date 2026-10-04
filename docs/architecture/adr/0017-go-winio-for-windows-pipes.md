# ADR-0017: Microsoft's go-winio for the Discord pipe on Windows

## Status

Accepted. Supersedes the consequence "No third-party code in the binary" of [ADR-0004](0004-in-house-protocol-implementations.md), for Windows only. The rest of ADR-0004 stands.

## Context

The connection to Discord on Windows is a named pipe. One goroutine blocks in a read for as long as presence shows, while another writes ([ADR-0004](0004-in-house-protocol-implementations.md)). [ADR-0002](0002-implementation-language.md) and risk R12 assumed the standard library could do this from Go 1.26, and [ADR-0003](0003-license-and-dependency-policy.md) conditionally approved `github.com/Microsoft/go-winio` in case it could not. [CRP-021](../../tickets/done/CRP-021-discord-transport.md) tested the assumption.

| Fact | How it is known |
|---|---|
| From Go 1.25, a file made from a handle opened for overlapped I/O is served by the runtime's completion port and supports deadlines. From Go 1.26, `os.OpenFile` accepts `FILE_FLAG_OVERLAPPED` | Go 1.25 and 1.26 release notes, read 2026-10-03 |
| With a pipe opened that way, a read deadline times out, a write completes while a read is pending, and closing unblocks a pending read | Observed on Windows 11 with Go 1.27.0, 2026-10-03: the tests of CRP-021 pass against the fake Discord server without the race detector |
| With the race detector on, the same tests fail. Both `Read` and `Write` of the file end in `internal/poll.(*FD).addOffset`, which adds to one field of the file, and a read holds only the read lock while a write holds only the write lock. The report names those two frames and nothing of ours | Observed, same machine and date. Read in `internal/poll/fd_windows.go` of Go 1.27.0 |
| The same code is on the main branch of Go | Read on 2026-10-03 |
| The offset has no meaning for a pipe, so the race is unlikely to change behaviour. It is still a data race, and every test in this repository runs under the race detector ([ADR-0009](0009-quality-gates.md)) | Inference from the Windows documentation of the overlapped structure, and the quality rule |
| No lock of ours can separate the two accesses without also preventing a write during a pending read, which is the requirement | Reasoning from the code above |
| With go-winio v0.6.2 the same tests pass under the race detector, five runs in a row | Observed, same machine and date |

The candidate, checked on 2026-10-03:

| Question | Answer |
|---|---|
| License | MIT |
| Linked into the binary | Its root package, three internal packages and `pkg/guid`, plus `golang.org/x/sys/windows`. Its module file also names `github.com/sirupsen/logrus`, which none of those packages import, so it is not linked and does not enter our module file |
| Who maintains it | Microsoft. 1,082 stars, 218 forks, used by Docker and containerd |
| Activity | Commits up to 2026-09-30, including a race fix on 2026-09-25. **The latest tagged release, v0.6.2, is from April 2024**, so it does not meet the "release in the past twelve months" rule of ADR-0003 |
| Size of what it pulls in | Proportionate: one small module, Windows only |
| Cost of writing it ourselves | About 150 lines of overlapped I/O with completion handling, in a platform file whose error branches tests cannot reach, against the 100% coverage rule |
| Known vulnerabilities | None, per `govulncheck` |

The owner accepted the module on 2026-10-03 on the grounds that Microsoft maintains it actively, with the missing recent release known.

## Decision

1. On Windows, the Discord transport opens the pipe with `winio.DialPipeContext` and uses the connection it returns.
2. `github.com/Microsoft/go-winio` is imported only by `internal/discord/transport`, only in files built for Windows, and by tests. No other package may import it.
3. Only its pipe dialling is used in production code. Its listener may be used in tests.
4. It is the only module outside the standard library and `golang.org/x/*`. Any other still needs its own ADR.
5. Revisit when the Go standard library no longer races: remove the module and return to `os.OpenFile`. The tests that would prove it are the ones in the transport package, run with the race detector.

## Consequences

- The Windows binary contains third-party code, and the release must ship Microsoft's MIT notice beside ours and the Go project's ([CRP-060](../../tickets/M6-release/CRP-060-release-pipeline.md)). The supply-chain check must list the module with this ADR ([CRP-007](../../tickets/M0-foundation/CRP-007-supply-chain-policy.md)).
- We depend on a module whose tagged releases are infrequent. A fix we need may exist only on its main branch.
- go-winio starts one goroutine of its own, for its completion port, the first time a pipe is opened, and never stops it. A test that asserts no goroutine is left must allow for it on Windows.
- A deadline error from the Windows connection is go-winio's own value, and an error after close is too. Callers must test for a timeout through the `Timeout` method, which both platforms provide, not by comparing with a standard library value.
- While a pipe exists but has no free instance, go-winio retries every 10 milliseconds until the context ends. The transport bounds each attempt, so a busy pipe costs one attempt timeout and is then passed over.
- macOS and Linux are unchanged and still use the standard library alone.

## Alternatives considered

| Alternative | Why not |
|---|---|
| The standard library, as planned | A data race inside it when a read and a write overlap. The race detector fails, and the rule is not to skip or weaken that check |
| A lock around reads and writes | Would stop a write while a read is pending, which is how the connection is used |
| Our own overlapped I/O on `golang.org/x/sys/windows` | No new module, but a large platform file with unreachable error branches, repeating what go-winio already does and Microsoft maintains |
| Two handles to one pipe, one per direction | A duplicated handle shares the completion port association, and the standard library then falls back to synchronous I/O on it, without deadlines |
| Wait for a fix in Go | No fix exists on the main branch, and every later Discord ticket waits on this one |
