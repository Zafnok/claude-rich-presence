# Control protocol

The messages a session process (a **follower**) and the presence **host** exchange over the control channel. The channel itself, a per-user Unix domain socket, is decided in [ADR-0006](../architecture/adr/0006-control-channel.md). Who becomes host is decided in [ADR-0005](../architecture/adr/0005-presence-host-election.md).

The code is the package [`internal/control/protocol`](../../internal/control/protocol). Every example below is a golden file in its `testdata/golden` directory, and a test fails if this document and those files differ.

Current protocol version: **1**.

## Framing

1. One JSON object per line, encoded as UTF-8, ended by a line feed. The sender writes each line in a single write.
2. A line is at most 65,536 bytes (64 KiB), not counting the line feed. A receiver that meets a longer line closes the connection.
3. A blank line is skipped. White space around the object, including a carriage return before the line feed, is ignored.
4. Every object has a string field `type`. The other fields sit beside it at the top level.

## Conversation

| Step | Who | Message |
|---|---|---|
| 1 | Follower | `hello`, always first |
| 2 | Host | `welcome`, or `refuse` |
| 3 | Follower | `sync` with its current session, straight after `welcome` |
| 4 | Follower | `event`, one for each thing that happens, for as long as the connection lasts |

A follower may send `sync` again at any time. It replaces what the host holds for that connection.

A session exists on the host exactly while its follower's connection is open. There is no "goodbye" message: closing the connection is the signal. A session belongs to one connection: the one whose `event` created it, or the last to `sync` it. It is removed when that connection closes. Only a `sync` moves a session to another connection, and an `event` for a session that belongs to another connection is ignored. So a follower that reconnects before the host has seen its old connection close keeps its session, and whatever the host still reads from the old connection can neither take the session back nor end it.

After a `refuse` the only message the host still reads is `stand_down`. The follower sends it or not, and closes the connection. The host does not wait for long: it closes a connection that has not been welcomed one second after accepting it, whether the connection was refused or has said nothing.

`status` and `stand_down` are requests that can be sent on any connection after `welcome`. The `status` command opens a connection of its own, says `hello`, asks, reads the answer and closes without ever sending a `sync`. A follower may ask for `status` as often as it likes. This binary asks after each thing it sends, so that what it reports of its host is recent.

## Messages

| Type | Direction | Carries |
|---|---|---|
| [`hello`](#hello) | Follower to host | Protocol version, binary version |
| [`welcome`](#welcome) | Host to follower | Protocol version, binary version |
| [`refuse`](#refuse) | Host to follower | A reason code |
| [`sync`](#sync) | Follower to host | The follower's complete current session, or none |
| [`event`](#event) | Follower to host | One presence event |
| [`stand_down`](#stand_down) | Follower to host | Nothing |
| [`status`](#status) | Either to host | Nothing |
| [`status_result`](#status_result) | Host to requester | Discord connection state, session count, host binary version, uptime |

In the tables below, a required field must be present and not empty. A message without one is rejected.

### `hello`

The first message on every connection.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `protocol` | integer, 1 or more | Yes | The protocol version the sender will speak on this connection |
| `version` | [binary version](#binary-version) | Yes | The sender's binary version |

```json
{"type":"hello","protocol":1,"version":"1.4.0"}
```

### `welcome`

The host accepts the `hello`.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `protocol` | integer, 1 or more | Yes | The highest protocol version the host speaks. The connection speaks the version in the `hello` |
| `version` | [binary version](#binary-version) | Yes | The host's binary version |

```json
{"type":"welcome","protocol":1,"version":"1.3.2"}
```

### `refuse`

The host does not accept the `hello`. Whatever the reason, the follower goes back to the election and tries again later.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `reason` | reason code | Yes | Why |

| Reason | Meaning |
|---|---|
| `unsupported_protocol` | The host does not speak the protocol version in the `hello` |
| `standing_down` | The host is giving up the lock and takes no new followers |
| `other` | Anything else. A reason the receiver does not know is read as `other` |

```json
{"type":"refuse","reason":"unsupported_protocol"}
```

### `sync`

The follower's complete current session. It is idempotent: the host replaces what it holds for this connection with what the message says.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `session` | object or `null` | No | The session. `null` or absent means the follower has no session now, and the host drops the one it held for this connection |

The session object:

| Field | Type | Required | Meaning |
|---|---|---|---|
| `id` | string, at most 128 bytes | Yes | The session id |
| `surface` | [word](#words) | Yes | `code` or `desktop` |
| `status` | [word](#words) | Yes | `idle`, `working`, `waiting` or `compacting` |
| `tool` | [word](#words) | No | The kind of tool in use, only while `working`. See the tool kinds under [`event`](#event) |
| `model` | string, at most 64 bytes | No | A short model family label |
| `project` | string, at most 128 bytes | No | The project name, as the adapter's privacy level allows |
| `privacy` | [word](#words) | Yes | `minimal`, `standard` or `full` |
| `link` | string | No | The repository link of the session's project profile. See [The link](#the-link) |
| `start` | [time](#times) | Yes | When the session began. It feeds the elapsed timer and survives a change of host |
| `last_activity` | [time](#times) | Yes | When the session last did something |
| `subagents` | integer, 0 or more | No | How many subagents are running. Absent means 0 |

```json
{"type":"sync","session":{"id":"3f2a9c1e-7b64-4d0a-9e51-0c8d2b6a4f17","surface":"code","status":"working","tool":"editing","model":"opus","project":"example-project","privacy":"standard","start":1790985600000,"last_activity":1790985723500,"subagents":2}}
```

With no session:

```json
{"type":"sync","session":null}
```

### `event`

One thing that happened in one session.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `event` | object | Yes | The event |

The event object:

| Field | Type | Required | Meaning |
|---|---|---|---|
| `session_id` | string, at most 128 bytes | Yes | The session id |
| `surface` | [word](#words) | Yes | `code` or `desktop` |
| `at` | [time](#times) | Yes | When it happened |
| `kind` | [word](#words) | Yes | What happened, from the list below |
| `tool` | [word](#words) | No | The kind of tool. The domain requires it on `tool_started` |
| `model` | string, at most 64 bytes | No | A short model family label. The domain requires it on `model_changed` |
| `project` | string, at most 128 bytes | No | The project name |
| `privacy` | [word](#words) | No | `minimal`, `standard` or `full` |
| `link` | string | No | The repository link of the session's project profile. Read on `session_opened` only. See [The link](#the-link) |

Kinds: `session_opened`, `session_refreshed`, `turn_started`, `tool_started`, `tool_finished`, `attention_needed`, `idle`, `turn_finished`, `compaction_started`, `compaction_finished`, `model_changed`, `subagent_started`, `subagent_stopped`, `session_ended`.

Tool kinds: `editing`, `running`, `reading`, `searching`, `browsing`, `delegating`, `tools`, `generic`. A tool's name is never sent.

```json
{"type":"event","event":{"session_id":"3f2a9c1e-7b64-4d0a-9e51-0c8d2b6a4f17","surface":"code","at":1790985723500,"kind":"tool_started","tool":"editing"}}
```

### `stand_down`

Asks the host to give up the lock, so that a newer binary can win the election ([ADR-0005](../architecture/adr/0005-presence-host-election.md), "Version skew"). It has no fields and no reply. Every protocol version has it.

A follower sends it once on a connection, straight after a `welcome` whose binary version is older than its own, and then carries on as any follower. A follower that could not take over, because its own attempt at the lock failed for a reason other than the lock being held, does not send it. The host acts on it only when the sender is the newer binary:

| Connection | The sender is newer when |
|---|---|
| Welcomed | The binary version in its `hello` is [newer](#binary-version) than the host's own |
| Refused | The protocol version in its `hello` is higher than the highest the host speaks |

Any other `stand_down` is ignored. The host judges for itself, so two processes can never ask each other to stand down in turn.

A host that stands down refuses every `hello` with `standing_down` from then on, closes its connections, releases the lock, and waits 800 milliseconds before it tries the lock again. A sender that means to take over must try the lock within that time, and before the other followers do: a follower that has lost its host first tries the lock 50 to 100 milliseconds later. This binary, when it has asked and then loses the host, tries six times in the first 40 milliseconds.

A process that has stood down ignores `stand_down` in every later term as host until it has followed a newer binary. If no newer binary became host, whoever asked did not take over, and standing down again would only clear presence again. This bounds what a sender that never takes the lock can cause: each older process stands down for it once.

```json
{"type":"stand_down"}
```

### `status`

Asks the host for a summary. It has no fields. The host answers with one `status_result` on the same connection.

```json
{"type":"status"}
```

### `status_result`

The host's summary of itself. It says nothing about any session: no project name, no path, no session id, no model. It is safe to paste into a public issue.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `discord` | state code | Yes | The host's connection to Discord |
| `sessions` | integer, 0 or more | No | How many sessions the host holds. Always sent; absent is read as 0 |
| `version` | [binary version](#binary-version) | Yes | The host's binary version |
| `uptime_seconds` | integer, 0 or more | No | How long this process has been host. Always sent; absent is read as 0 |

| State | Meaning |
|---|---|
| `connected` | The handshake with Discord is complete |
| `connecting` | A connection attempt is under way |
| `disconnected` | There is no connection, as when Discord is not running |
| `unknown` | A state the receiver does not know is read as `unknown` |

```json
{"type":"status_result","discord":"connected","sessions":3,"version":"1.4.0","uptime_seconds":5400}
```

## Field types

### Binary version

A string of 1 to 64 bytes from the letters `A` to `Z` and `a` to `z`, the digits, and `.` `-` `+` `_` `(` `)`. That covers a release such as `1.4.0`, a version the Go toolchain derives such as `v0.0.0-20261003120000-0123456789ab+dirty`, and `(devel)`. It cannot hold a path separator or a space.

Two versions are ordered as follows, to decide which binary is newer. A leading `v` and everything from a `+` on are ignored. What is left is numbers separated by dots, then optionally a hyphen and pre-release identifiers separated by dots. Numbers compare as numbers, and a missing one counts as zero, so `1.4` and `1.4.0` are the same. A pre-release is older than its release. Pre-release identifiers compare one by one: numbers as numbers, a number before anything else, the rest as text, and a shorter list before a longer one that begins with it. That is the order of semantic versioning, and it puts two versions derived by the Go toolchain in the order of their timestamps.

A version that does not read that way, such as `(devel)`, is neither newer nor older than any other. Two such builds never ask each other to stand down.

### Times

An integer: milliseconds since 1970-01-01 00:00 UTC. It must be greater than 0.

### Words

A string of at most 32 bytes from a closed vocabulary: surface, status, event kind, tool kind, privacy level. The codec carries the word and checks only its length. The receiver's domain model decides whether it knows the word. This is what lets a vocabulary grow without a new protocol version: see rule 3 below.

## The link

`link` was added without a new protocol version, because it is additive in both directions. A host from before the field ignores it, as it ignores any field it does not know, and shows no button. A host that knows the field reads a follower that does not send it as having no link.

The link comes from a project profile in the user's own configuration and from nowhere else ([ADR-0012](../architecture/adr/0012-project-profiles-and-repository-link.md)). The host trusts no follower's link. It validates each with the function the configuration uses, against the hosts in its own configuration, and publishes the result. A link that fails is dropped alone, with a warning that does not repeat it: the session is kept and the connection stays open. For that reason the field has no length limit of its own here. It is bounded by the limit on a line, and what the host accepts is at most 512 bytes.

A session's link is set when the session opens, by a `session_opened` event or by a sync, and by nothing after. A follower whose link changes ends the session and opens it again.

## What the channel cannot carry

The fields above are all there is. No message has a field for prompt text, tool input or output, an assistant message, a file path, a transcript path or a tool name, and a test fails if a field is added without being listed. The only strings that are not from a closed vocabulary are the session id, the model label, the project name, the repository link and the binary version. Each has a length limit, and the link is validated by the host as [The link](#the-link) describes. The activity summary ([ADR-0011](../architecture/adr/0011-model-authored-activity-summary.md)) is not in protocol version 1.

## Errors

A receiver sorts a bad line into one of four kinds. None of them quotes the line.

| Kind | When | What the receiver does |
|---|---|---|
| Line too long | The line is over 64 KiB | Closes the connection |
| Malformed | The line is not a JSON object, or a known field has the wrong JSON type | Ignores the line |
| Missing field | `type`, or a required field of a known type, is absent or empty | Ignores the line |
| Invalid value | A value is out of range: a string over its limit, a negative number, a binary version outside its alphabet | Ignores the line |

An error in `hello` is different, because nothing can follow it: the host closes the connection. So does a first message that is not `hello`.

## Compatibility

Different versions of the binary run side by side during an upgrade, so these rules hold from the first release.

1. **Unknown message types are ignored.** A receiver that does not know a `type` reads the line, does nothing and carries on. It does not look at the other fields, so a later message type is free to reuse a field name.
2. **Unknown fields are ignored**, at every level. A field can be added to a message without a new protocol version, provided an older receiver that ignores it still behaves correctly.
3. **Unknown words are dropped by the domain, not the codec.** An event whose `kind` an older host does not know decodes, fails the host's validation and is skipped. The connection stays open. A session in a `sync` with an unknown word is skipped the same way.
4. **Unknown reason and state codes** are read as `other` and `unknown`.
5. **Anything an older host cannot safely ignore increments the protocol version.** Examples: removing or renaming a field, changing a field's type or meaning, adding a required field, or adding a message the host must act on for presence to be right.
6. **The host accepts every version it understands**, from the oldest it supports to its own. It answers a `hello` with any other version with `refuse` and the reason `unsupported_protocol`. Today the oldest and the newest are both 1.
7. **The connection speaks the version in the `hello`.** A newer host does not send an older follower anything that version lacks.
8. **A follower treats every `refuse` as "retry the election later".** It never treats one as fatal. A follower refused for `unsupported_protocol` knows it is the newer binary and may send `stand_down` on the refused connection before closing it.
9. **The first three messages never change shape**: `hello`, `welcome` and `refuse` keep their meaning in every protocol version, and so does `stand_down`. Two binaries of any versions can always get as far as agreeing that they disagree.
