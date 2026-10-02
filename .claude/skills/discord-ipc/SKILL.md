---
name: discord-ipc
description: Reference for Discord's local IPC protocol as this project uses it for Rich Presence, with what is documented, what is only observed, and how to re-verify. Use when writing or reviewing anything under internal/discord, the fake Discord server, activity payloads, rate limiting, or when presence does not appear in Discord.
---

# Discord local IPC for Rich Presence

We implement this protocol ourselves (ADR-0004). This is the working reference. It was assembled on 2026-10-02. **Re-check the live sources at the bottom before relying on any detail**, and update this file when something has changed.

## Endpoint

| Operating system | Where |
|---|---|
| Windows | Named pipe `\\?\pipe\discord-ipc-N` |
| macOS, Linux | Unix socket `discord-ipc-N` in the first set of `XDG_RUNTIME_DIR`, `TMPDIR`, `TMP`, `TEMP`, else `/tmp` |
| Linux, Flatpak | Under that directory: `app/com.discordapp.Discord/`, and the Canary equivalent |
| Linux, Snap | Under that directory: `snap.discord/` |

`N` runs from 0 to 9. Try each in order; the first that connects wins. Several Discord builds may be running.

The Flatpak and Snap locations are not in Discord's documentation. They come from another project's source.

## Frame

| Bytes | Content |
|---|---|
| 4 | Opcode, little-endian unsigned 32-bit |
| 4 | Payload length in bytes, little-endian unsigned 32-bit |
| length | UTF-8 JSON |

Write header and payload in one call. The official library caps a frame at 64 KiB; we enforce the same.

| Opcode | Name | Use |
|---|---|---|
| 0 | Handshake | First frame from the client |
| 1 | Frame | Commands, responses, events |
| 2 | Close | Either side, with a code and message |
| 3 | Ping | Answer with pong, same payload |
| 4 | Pong | |

## Conversation

1. Client connects and sends a handshake carrying the protocol version, which is 1, and the application id as a string.
2. Discord answers with a frame carrying a dispatch whose event is `READY`. A bad application id gets a close frame instead.
3. Client sends a frame whose command is `SET_ACTIVITY`, with arguments holding the client's process id and the activity, and a unique nonce.
4. Discord answers with a frame echoing the nonce, or with an error event for that nonce.
5. To clear, send the same command with a null activity.
6. **When the connection closes, Discord clears the activity.** The connection must stay open for as long as presence should show.

No OAuth step is used. Discord's own library sends only the handshake and then sets the activity. The documentation text says commands need authentication; behaviour says otherwise for this command. This is a dependency on behaviour (risk R7).

## Activity fields we use

| Field | Notes |
|---|---|
| `details` | First line. 2 to 128 characters |
| `state` | Second line. 2 to 128 characters |
| `timestamps.start` | Unix time. Discord shows elapsed time |
| `assets.large_image`, `assets.large_text` | Asset key uploaded to the application, and its hover text |
| `assets.small_image`, `assets.small_text` | As above, shown as a badge |
| `type` | Over this interface only 0 (playing), 2 (listening), 3 (watching) and 5 (competing) are accepted. We use 0 |

Omit a field rather than send it empty. A text field shorter than 2 characters is rejected.

Fields that exist and we do not use in the first release: buttons (at most two), party, secrets, link fields for the text lines and images, and the display-type field.

The title Discord shows is the **application's name**, set in the Developer Portal, not anything in the payload. Asset keys are lower-cased by Discord.

## Rate limit

Two official figures exist: five updates per 20 seconds, and one update per 15 seconds, the latter described by Discord staff as enforced by the client. Which one applies to this interface today is unverified. We design for one per 15 seconds and always deliver the latest state (CRP-013).

## Things that go wrong

| Symptom | Cause |
|---|---|
| Nothing connects | Discord is not running, is the web or mobile app, or lives in a sandbox path not in the candidate list |
| Connects, then closed immediately | Unknown or invalid application id |
| Activity set, nothing visible | The user turned off activity sharing in Discord's settings, a text line is under 2 characters, or the asset key does not exist |
| Presence disappears | Our connection closed |
| Updates seem ignored | Rate limited |
| On Windows, a write hangs while a read is pending | The pipe was opened for synchronous I/O. It must be opened for overlapped I/O (CRP-021) |

## Unverified

- Which rate limit applies.
- Whether Discord clears the activity when the process named in it exits while the socket stays open. We always name our own process, so it does not arise.
- The exact rules of the Developer Portal's application-name filter.

## How to re-verify

1. Read the RPC page of Discord's developer documentation for transport, opcodes, the handshake and the set-activity command.
2. Read the protocol notes in Discord's old library for framing details.
3. Compare against the open reimplementation of the server for behaviour on close and on errors.
4. Against a real Discord client, with the owner's help: a handshake and one activity, observed in the profile.

## Sources

- https://docs.discord.com/developers/topics/rpc
- https://github.com/discord/discord-rpc/blob/master/documentation/hard-mode.md
- https://github.com/discord/discord-rpc/blob/master/src/rpc_connection.h
- https://github.com/discord/discord-api-docs/issues/668
- https://docs.discord.com/developers/developer-tools/game-sdk
- https://github.com/OpenAsar/arrpc
- https://github.com/qwertyquerty/pypresence/blob/master/pypresence/utils.py, for the Flatpak and Snap paths

## In this repository

- Decision to implement in-house: `docs/architecture/adr/0004-in-house-protocol-implementations.md`
- Tickets: CRP-013, CRP-020, CRP-021, CRP-022, CRP-023
- Application id and artwork: CRP-003
