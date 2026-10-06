# Threat model

What a local, unprivileged, always-running helper could do to its user, what stops it, and the test that shows it. Written for [CRP-062](../tickets/done/CRP-062-security-review.md) from a read of the code at commit `d554a88` on 2026-10-05, by a session that had not seen the implementation discussions.

Report a vulnerability as [SECURITY.md](../../SECURITY.md) describes.

## What the program is, for this purpose

One binary, `rich-presence`, started by Claude as a local MCP server in every session. It runs as the user, with no elevated privilege, for as long as the session lives. One copy is the presence host: it holds a lock file, listens on a Unix socket in a per-user runtime directory, and holds the connection to the local Discord client. The other copies connect to that socket and forward their session's events.

It has four inputs and two outputs.

| Direction | What | Who is on the other side |
|---|---|---|
| In | MCP messages on standard input: hook events through the `presence_event` tool, and `presence_status` | Claude Code or Claude Desktop, and through them the model |
| In | The configuration file and the environment | The user, and whatever started Claude |
| In and out | The control socket | Other copies of this binary, or any process of the same user |
| In and out | The Discord pipe or socket | The Discord client, or whatever is listening where Discord would |
| Out | The log file | The user, and whoever they paste it to |
| Out | Standard output of `status` and `doctor` | The same |

It opens no network connection. That is checked: see [No network](#no-network).

## Assets

| Asset | Why it matters |
|---|---|
| The user's work content: prompts, tool inputs and outputs, messages, file paths, the working directory | Must never leave the adapter, at any level |
| The project name | Published only at the `full` level, which the user chose |
| The fact and timing of activity, the model family, the number of sessions | Published at `standard`, which is the default |
| Claude's responsiveness and the model's context | The program sits on a path Claude waits on, and its tool results are read by the model |
| What the user's Discord profile shows | A social surface under the user's name |
| The user's files and account | The program runs as the user |

## Who is considered

| Actor | Can do | In scope |
|---|---|---|
| Another user on the same machine | Create files in shared directories such as `/tmp`, create named pipes, connect to anything not protected by permission | Yes |
| A process of the same user that is not this program | Everything the user can, including talking to Discord directly and reading the configuration | Only to check that this program gives it nothing it did not already have |
| The model, steered by content it read | Call the tools, choose directory names, edit files where the permission mode allows | Yes |
| Whoever controls a release asset or the bundle address | Replace the binary | Yes |
| A hostile Discord client, or something impersonating it | Send any frames, read what is sent | Yes |
| Root, or malware already running as the user with intent | Anything | No. Nothing here can defend against it |

## Severity

Used for the findings below and for reports under SECURITY.md.

| Severity | Meaning |
|---|---|
| High | Work content, or anything the privacy level withholds, reaches Discord or another user without a deliberate act by the user; or code runs, or files are written, that the user did not ask for; or Claude is blocked |
| Medium | Something the privacy level allows reaches a party it was not meant for; or a local user can break a trust boundary under conditions that are not the default |
| Low | A defence in depth is missing, or a local process of the same user can degrade presence or waste the host's memory |

## Threats, mitigations and tests

Tests are named as `package: Test`. Package paths are under `internal/` unless they start with `test/` or `tools/`.

### 1. Hook events into the adapter

| Threat | Mitigation | Test |
|---|---|---|
| An oversized message | A line over 1 MiB is discarded whole and answered with a fixed error; the server reads on. Each field has a byte limit, 256, or 4096 for the working directory and 128 for the session id, and a longer value makes the call malformed | `mcp: TestLineLengthLimit`, `TestDefaultLineLengthLimit`; `adapter/code: TestMalformedCallsPublishNothing` |
| A malformed message | Invalid JSON, a wrong type, an unknown event name or a session id with control characters is counted and dropped. The result is the same constant. A panic in a handler is recovered and what it carried is discarded | `mcp: FuzzDecode`, `FuzzServe`, `TestHandlerPanicIsAToolError`; `adapter/code: FuzzEventTool`, `TestMalformedCallsAreCounted`, `TestAnInternalErrorStillReturnsTheConstant` |
| Content leaking past the allowlist | Only the fields a row of the allowlist names are decoded; everything else stays undecoded bytes that are dropped with the call. The tool name becomes one of eight kinds, the model id one of four family words and a version, and the working directory its last element at `full` and nothing otherwise. The level is applied last, before the event leaves the adapter | `adapter/code: TestNothingLeaks`, `TestPrivacyLevels`, `TestEventTable`; `test/e2e: TestPrivacy` |
| A slow or stuck publisher delaying Claude | The tool only parses and offers to a bounded queue; a full queue drops the call | `adapter/code: TestAStalledPublisherNeverDelaysTheTool`; `host: TestPublishNeverWaits`; `test/e2e: TestEventLatency` |
| The working directory surviving in memory or on the wire below `full` | It is used once to choose the profile and discarded. Below `full` the session holds no project name | `adapter/code: TestNothingLeaks`; `test/e2e: TestPrivacy` |

### 2. Control socket

| Threat | Mitigation | Test |
|---|---|---|
| Another local user connecting to the host | The host creates the runtime directory with mode 0700 and refuses one it does not own or that group or others can access. On Windows the directory is under `%TEMP%` and inherits its access control | `control/transport: TestPrepareCreatesAnOwnerOnlyDirectory`, `TestPrepareRefusesGroupOrWorldAccess`, `TestPrepareRefusesAnotherUsersDirectory`, `TestAcquireRefusesAnUnsafeDirectory`, `TestRuntimeDirectoryUnderTempAdmitsNoOtherUser` |
| Another local user *being* the host | **Not mitigated for a follower. Finding F1** | None |
| A hostile follower: oversized lines, garbage, silence, a flood | A line over 64 KiB closes that connection only. A line that does not parse or validate is counted and skipped. A connection that does not say hello within a second is closed. Each connection has its own goroutine and the registry never waits on one | `host: TestAnOversizedLineClosesOnlyItsConnection`, `TestWhatCannotBeUsedIsSkippedAndTheConnectionStays`, `TestAConnectionThatDoesNotBeginWithAValidHelloIsClosed`, `TestAConnectionThatSaysNothingIsClosed`, `TestAStalledFollowerDelaysNobody`, `TestAPanicWhileServingAConnectionCostsOnlyThatConnection`; `control/protocol: FuzzDecode`, `FuzzDecoder`, `TestDecodeLineLimitIsExact` |
| A hostile follower using unlimited connections or sessions | **Not limited. Finding F2** | None |
| A hostile follower changing or ending another's session | A session belongs to the connection that created it. Only a sync moves it | `host: TestAnEventForAnotherConnectionsSessionIsDropped`, `TestASyncMovesASessionToTheConnectionThatSentIt` |
| A hostile follower making the host do something other than update presence | The host acts on five message types. Sync and event change the registry; status is answered from memory with a state word, two numbers and its own version; stand_down ends the term; anything else is ignored. Nothing received is logged, executed, or used as a path | `host: TestStatusIsAnsweredFromMemory`, `TestARefusedConnectionIsReadOnlyForStandDown`; `control/protocol: TestMessagesCarryOnlyTheListedFields`; `diag: TestInterfaceRefusesEventsAndFreeStrings` |
| A hostile follower placing text on Discord | It can: a project name of up to 128 bytes at `full`, and a model label of up to 64. A process of the same user could send the same to Discord itself, so nothing is gained, but the text is not cleaned. **Finding F3** | None |
| A forged stand_down | Honoured only from a peer whose hello carried a newer version, and once: a host that stood down and was not replaced by something newer does not stand down again. The cost is a gap in presence of under a second | `host: TestAStandDownThatIsNotFromANewerBinaryIsIgnored`, `TestANodeThatStoodDownForNobodyDoesNotStandDownAgain` |
| A hostile host answering a follower | A follower reads a welcome, whose version is limited to a version alphabet before it is logged, and status results, which hold a state word and numbers. Nothing else is read | `host: TestJoiningPassesOverWhatItDoesNotUnderstand`, `TestAFollowerReportsWhatItsHostLastSaid`; `diag: TestCleanVersion` |
| A symbolic link in place of the runtime directory | The directory is inspected without following links, and a link is refused | `control/transport: TestPrepareRefusesALink`, `TestPrepareRefusesAFile` |
| A relative or unusable runtime directory from the environment | Refused; presence is off and the server still runs | `control/transport: TestResolve`; `cli: TestMCPStartupProblemsLeaveStandardOutputToTheProtocol` |
| A planted lock file | The lock is an operating-system lock on an open file, not the file's existence, so a file left or planted there elects nobody and blocks nobody. The file is inside the owner-only directory | `control/transport: TestTwoProcessesContendForTheLock`, `TestKillingTheHolderFreesTheLockForAWaitingProcess`, `TestAcquireReportsALockFileItCannotOpen` |
| A planted or leftover socket file | Only the lock holder removes and replaces it; a follower that finds a dead one treats it as no host | `control/transport: TestListenReplacesALeftoverFile`, `TestSocketLeftByADeadProcessDoesNotPreventListening`, `TestOnlyTheLockHolderCanRemoveTheSocket`, `TestDialAfterTheListenerClosedIsNoHost` |

Accepted risks at this boundary:

- **Another user can deny presence** on a machine where the runtime directory is under a shared temporary directory, by creating the directory first. The host refuses it, and there is no second location. The user is told by `doctor` and can set `RICH_PRESENCE_RUNTIME_DIR`. Presence is not worth a fallback that would itself be guessable. Test: `diag: TestChecks`.
- **Windows has no ownership check.** Permission bits mean nothing there, so the directory is trusted for where it is. An override that points at a directory other users can write is the user's own doing. Test of the default: `control/transport: TestRuntimeDirectoryUnderTempAdmitsNoOtherUser`.
- **Any process of the same user can connect.** It is the same principal. The mitigations above are about limiting what a confused or buggy peer can do, not about keeping that user out.

### 3. Discord pipe

| Threat | Mitigation | Test |
|---|---|---|
| A malformed frame | A frame that cannot be decoded ends the connection, which is retried with backoff | `discord/codec: FuzzReadFrame`, `FuzzDecode`, `TestDecodeMalformed`; `discord/session: TestFramesThatEndTheConnection` |
| An oversized frame | A declared length over 64 KiB is refused before any payload is read | `discord/codec: TestReadFrameTooLarge`, `TestReadFrameLargestPayload` |
| A peer that accepts and then says nothing, or stops reading | The handshake has five seconds and each write five; either ends the connection. Setting an activity never waits on the connection | `discord/session: TestAnUnansweredHandshakeTimesOutAndIsRetried`, `TestSetReturnsAtOnceWhileAWriteIsBlockedAndTheWriteTimesOut`; `discord/transport: TestAnAttemptThatHangsIsAbandonedAndTheNextIsTried` |
| Text from the peer reaching the log | An error or a close is logged as a class and a numeric code. Its message is never logged | `discord/session: TestAnErrorResponseIsLoggedAndTheConnectionKept`; `diag: TestInterfaceRefusesEventsAndFreeStrings` |
| Being reconnected in a tight loop | Backoff from one second to sixty with jitter; an update is never written more often than the configured interval, whose floor is four seconds | `discord/session: TestBackoffGrowsToTheCapAndResetsWhenReady`, `TestReconnectsAndResendsNoFasterThanTheSchedulerAllows`, `TestNoTwoWritesAreCloserThanTheInterval`; `config: TestMinUpdateIntervalIsRaisedToTheFloor` |
| A process impersonating Discord at a lower index | **Accepted, see below.** On Unix the owner of the socket could be checked and is not. **Finding F4** | `discord/transport: TestFirstListeningIndexWins` shows the behaviour |

Accepted risk: **an impersonator receives what Discord would have.** Discord's protocol has no authentication in either direction, and the first endpoint that answers, from index 0 up, is used. On Windows any local process can create `discord-ipc-0` before Discord does; on Unix the same is possible wherever the directory Discord's documentation names is shared, which is `/tmp` when no runtime or temporary directory is set. The review looked at everything written to that connection:

- the application id, which is public;
- the process id of the host, which any local process can list;
- the activity: exactly what was about to be shown on the user's profile, already reduced to the privacy level. At `standard` that is status, model family and session count. At `full` it includes the project name, which a local impersonator would learn although they may not be among the people who can see the user's profile.

Nothing else flows that way: no session id, no path, no log, no configuration. What comes back can end the connection, be answered with a pong carrying the peer's own bytes, or be counted; it cannot change what is shown or reach the model. On Windows the pipe is opened with the anonymous impersonation level, so a pipe server learns nothing about the user's token; that is a property of go-winio's `DialPipeContext` at the pinned version, read in its source and not tested here, and is to be re-read when that dependency is upgraded.

### 4. Configuration file and environment

| Threat | Mitigation | Test |
|---|---|---|
| A hostile or mistyped value | Every setting is validated against a closed form; a bad one keeps its default and gives a warning that never repeats the value. The application id is digits only. The update interval has a floor | `config: FuzzLoad`, `TestInvalidValueFallsBackWithOneWarning`, `TestWarningsNeverEchoTheValue`, `TestApplicationIDLength`, `TestMinUpdateIntervalIsRaisedToTheFloor`, `TestBadFileYieldsDefaultsAndOneWarning` |
| A value Claude Desktop passed without filling in | Treated as not set | `config: TestUnsubstitutedEnvironmentValueIsNotSet` |
| A display name or area that carries formatting, a mention, a link or invisible characters | Cleaned to one line of plain text and cut to length | `config: TestCleanName`, `TestCleanAreas` |
| A repository link that is not what it seems | Accepted only as `https://host/owner/repository` on a listed host, from a restricted alphabet, with no user name, port, query, fragment or escape. It is not rendered yet | `config: TestValidateLink`, `FuzzValidateLink`, `TestRejectedLinkWarnsOnceWithoutTheValue` |
| A profile path used to read the file system | Paths are compared as text. Nothing under them is opened and links are not resolved | `config: TestProfilesReadNothingFromTheProject`, `FuzzEffective` |
| Profiles or link hosts set from the environment or by a tool | They come from the file alone, and no tool writes it | `config: TestEnvironmentDoesNotDefineProfiles`; `adapter/code: TestTheToolsAsAClientSeesThem` |
| A path setting that redirects the runtime directory | Must be absolute; then the directory checks of section 2 apply | `control/transport: TestResolve` |
| A configuration problem reaching the log or a public issue | The log records how many problems there are. `doctor` names settings and positions, and replaces an unknown key with a placeholder | `cli: TestMCPLogsConfigurationProblemsByCount`; `diag: TestUnknownConfigurationKeyIsNotRepeated` |

Accepted risks at this boundary:

- **The environment is trusted.** Whoever sets `RICH_PRESENCE_RUNTIME_DIR` or `RICH_PRESENCE_DISCORD_ENDPOINT` decides where the sockets are, and already controls the process.
- **The configuration file is read whole, with no size limit, before the server answers.** It is the user's own file in their own directory. A file made enormous or replaced by a device would slow the start of every session, and only the user can do that.
- **Text in a repository can talk Claude into editing the configuration file** in a permission mode without prompts, which opts that project in (risk R19). No tool of this program can write profiles, the link is validated, and the name is cleaned, so the result is bounded to a cleaned name and an owner and repository on a listed host. Guarding the user's own files against Claude is Claude's permission system's job.

### 5. Log file

| Threat | Mitigation | Test |
|---|---|---|
| Content leaking into the log | A log message must be a constant: the type system refuses a string variable. Attributes are made only by functions that take a constant, a number, a duration, or a version passed through a restricted alphabet. Panics are logged as a class, never with what they carried | `diag: TestInterfaceRefusesEventsAndFreeStrings`, `TestForbiddenFieldsNeverReachTheLog`, `TestCleanVersion`; `cli: TestMCPSurvivesAPanicWithoutWritingToStandardOutput`, `TestRunEndsWithAFixedMessageOnAPanic` |
| Unbounded growth | One file of at most 1 MiB plus one message, and one predecessor | `diag: TestFileStaysWithinCapAndKeepsOnePredecessor` |
| Others reading it | The directory is created 0700 and the file 0600 | `diag: TestLogFilesAreForTheUserAlone` |
| `doctor` or `status` output leaking when pasted | Paths have the home directory and the user's name replaced. Nothing about any session but a count is printed | `diag: TestReportNamesNoUserAndNoProject`, `TestRedact`; `cli: TestStatusAsksTheHost` |

Accepted risk: **a symbolic link at the log path is followed.** The file is opened for append without refusing links, and the mode of a directory that already exists is not checked. The path is inside the user's own configuration directory, so only the user, or something already running as them, can plant a link, and it would gain an append of fixed, content-free lines to a file that user can already write. No test.

### 6. The bundle and the plugin

| Threat | Mitigation | Test |
|---|---|---|
| A bundle assembled with the wrong or extra content | The bundle tool checks what it built and what it is given: the manifest, the exact file list, and that each binary is for the operating system and architecture its name claims. The build is reproducible | `tools/mcpb: TestCheckRejects`, `TestCheckRejectsWrongArchitectures`, `TestAssembleIsDeterministic`, `TestAssembleRefusesWhatDoesNotCheck` |
| A dependency or a CI action replaced upstream | Modules are limited to an allowlist and verified by the Go checksum database; actions are pinned to a commit; licences and known vulnerabilities are checked on every change and weekly | `tools/policycheck: TestRun`; the `Supply chain` workflow |
| The binary gaining a way to phone home | See [No network](#no-network) | `test/e2e: TestTheBinaryLinksNoNetworkClient` |

Accepted risks, each with the ticket that owns it. None of this exists yet, so none of it was reviewed:

- **Tampered release assets.** There is no release pipeline. [CRP-060](../tickets/M6-release/CRP-060-release-pipeline.md) is to publish checksums and a provenance attestation. Until a user verifies those, the trust root is the GitHub repository and account, as it is for the source.
- **A changed bundle address.** The plugin, which will point at the bundle by URL with no checksum field, is [CRP-042](../tickets/M4-claude-code/CRP-042-plugin-packaging.md). The address lives in the same repository as everything else, so changing it takes the same access as changing the code (risk R15).
- **Unsigned binaries.** [CRP-063](../tickets/M6-release/CRP-063-code-signing.md).

### 7. Claude itself

| Threat | Mitigation | Test |
|---|---|---|
| The model calling `presence_event` with invented input | A call that carries the marker Claude Code puts on the model's own calls is ignored. That marker is observed and not documented, so nothing relies on it: a call that gets through is parsed like a hook's, and can only do what a hook can | `adapter/code: TestACallMadeByTheModelIsIgnored`, `TestMalformedCallsPublishNothing`, `FuzzEventTool`, `TestSubagentsAreTrackedUpToALimit` |
| Tool output steering the model | `presence_event` returns the two bytes `{}` for every call, whatever happened. `presence_status` returns fixed words from closed vocabularies and integers; no value it was sent is ever printed. Protocol errors are fixed strings. Tool names and descriptions are constants | `adapter/code: TestTheResultIsByteIdentical`, `TestAnInternalErrorStillReturnsTheConstant`, `TestStatusTool`, `TestNothingLeaks`; `mcp: TestErrors`, `TestResultTextNeverBreaksTheLine` |
| The model changing what is published about other sessions | A process has one session and can speak only for it | `host: TestANodeHasOneSession`, `TestAnEventForAnotherConnectionsSessionIsDropped` |

The ticket asked for confirmation that a call made by the model can at worst make presence wrong. The review confirms it with one qualification:

- With invented input the model can change this session's status, tool kind and model family within their closed vocabularies, rename the session, end it and reopen it, and choose which project profile applies by naming a directory. None of that carries text of the model's choosing.
- **At the `full` level the last element of the working directory is published, and that is text the model can choose**: through an invented `cwd` if the marker fails, and in any case by creating a directory and working in it. That is up to 128 bytes on the user's profile that somebody other than the user wrote. It is what `full` means, it is opt-in, and it is not cleaned as a profile's display name is. **Finding F3** covers the cleaning and the documentation.
- Nothing the model sends is logged, written to a file, used as a path that is opened, or executed.

## No network

[ADR-0008](adr/0008-privacy-and-safety-by-default.md) says the program opens the local Discord pipe and the local control socket and nothing else. `test/e2e: TestTheBinaryLinksNoNetworkClient` holds it to that, for each of the four release targets:

1. The list of packages the binary is built from holds no HTTP, TLS, certificate, mail, RPC or MIME package, nothing from `golang.org/x/net`, and no module but this one, `golang.org/x/sys` and go-winio.
2. The standard library's `net` package has to be in the list, because Unix sockets live there, and it brings its own DNS resolver and TCP dialer with it. The one DNS package in the list is the one `net` imports, and nothing else imports it.
3. So the test also reads this module's source: `net` is imported only by the two transport packages, only names that cannot reach a network are used from it, and every dial and listen is written with the network `"unix"`. From go-winio only `DialPipeContext` is used.

`TestTheNetworkCheckCatchesWhatItShould` gives the same checks an HTTP client, a TCP dial, a name lookup and the rest, and expects each to be caught.

What this does not show: that go-winio or `golang.org/x/sys` contain no code that could open a network connection if called. They are read at upgrade under [ADR-0003](adr/0003-license-and-dependency-policy.md), and this module calls one function of the first.

## Findings

Each is a ticket. None is high.

| # | Severity | Finding | Ticket |
|---|---|---|---|
| F1 | Medium | A follower connects to the control socket without checking the directory it is in. The host refuses a runtime directory that another user owns, but then falls back to following, and dials the socket in that same directory. Where the directory is under a shared `/tmp` (Linux with no `XDG_RUNTIME_DIR`, macOS with no `TMPDIR`, and the short hashed name on both), another local user who creates it first can listen there and is sent every session's state: session id, status, tool kind, model family, timings, and the project name at `full`. `status` and `doctor` dial the same way but send no session | [CRP-065](../tickets/M6-release/CRP-065-follower-checks-runtime-directory.md) |
| F2 | Low | The host puts no limit on the number of connections it serves or on the number of sessions one connection may create. A process of the same user can grow the host's memory and its count of open files without bound, and inflate the session count that is shown | [CRP-066](../tickets/M6-release/CRP-066-limit-connections-and-sessions.md) |
| F3 | Low | Text that reaches Discord without coming from the configuration is not cleaned. The directory name published at `full` is checked for control characters only, so formatting characters, mention and link syntax and invisible characters such as a right-to-left override pass; and the host accepts a project name and a model label from a follower on length alone. The model can choose a directory name, so at `full` this is a way to put chosen text on the user's profile, and the documentation does not say so | [CRP-067](../tickets/M6-release/CRP-067-clean-published-text.md) |
| F4 | Low | On Unix the Discord socket is used whoever owns it. Where Discord's directory is a shared one, another local user can be "Discord" and is sent the activity. Discord's own design allows it and what is sent is meant for publication, but checking that the socket belongs to the current user costs one call and closes it | [CRP-068](../tickets/M6-release/CRP-068-check-discord-socket-owner.md) |

## What the review did not cover

- **Features that are not built**: the activity summary and its sanitiser (ADR-0011), repository link buttons, the status line bridge (ADR-0015), the plugin and its hook file, the release pipeline and signing. Each needs its own look when it lands; the summary most of all, because it is the one path for model-written text by design.
- **The Claude Desktop adapter**, `internal/adapter/desktop`. It was merged after the commit this review read, and has not been read. The no-network test does cover it, because that test reads whatever the binary is built from.
- **`/security-review`**, which the ticket names. It reviews the changes pending on a branch, and this branch changes documents and one test. The code was reviewed by reading it instead: every non-test file under `internal/control`, `internal/host`, `internal/discord`, `internal/adapter`, `internal/mcp`, `internal/diag`, `internal/config`, `internal/cli`, `internal/domain` and `internal/presence`.
- **Running anything hostile.** The findings come from reading code and tests on Windows. F1 and F4 concern Unix and were not reproduced on a Unix machine with two users; the tickets ask for a test that does.
- **macOS**, on any point where it differs from Linux, and the access control of `%TEMP%` on a Windows machine joined to a domain or with a redirected profile.
- **Dependencies' source**: go-winio and `golang.org/x/sys` beyond the one function named above, the Go standard library, and the Go toolchain.
- **The test helpers** under `internal/testutil` and the build tools under `tools/`, except to read what `tools/mcpb` checks. They are not in the shipped binary.
- **The GitHub repository's settings**: branch protection, who can publish a release, secret handling in workflows beyond what `tools/policycheck` enforces.
- **Discord's and Claude's own behaviour**: how Discord renders unusual text, and whether Claude Code keeps marking the model's tool calls.
- **Denial of service by the user's own processes** beyond F2, and side channels such as the timing of updates.
