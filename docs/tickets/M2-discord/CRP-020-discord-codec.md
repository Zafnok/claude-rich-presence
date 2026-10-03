---
id: CRP-020
title: Discord IPC codec
milestone: M2 Discord
type: feature
status: todo
priority: P0
blocked_by: [CRP-004, CRP-005, CRP-010]
blocks: [CRP-023]
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-020: Discord IPC codec

## Goal

Encode and decode Discord's local IPC frames and the handful of messages we use, as pure functions over byte streams.

## Context

A frame is a little-endian 32-bit opcode, a little-endian 32-bit length, and that many bytes of JSON. We implement this ourselves ([ADR-0004](../../architecture/adr/0004-in-house-protocol-implementations.md)). The `discord-ipc` skill holds the protocol reference and the sources to re-check.

## Scope

Package `internal/discord/codec`:

- Frame writer: opcode and payload to a single write.
- Frame reader over any byte stream, correct across partial reads, with a maximum payload size of 64 KiB.
- Opcodes: handshake, frame, close, ping, pong.
- Messages to encode: the handshake with protocol version and application id; the set-activity command with process id, a nonce, and the activity; the same command with a null activity, to clear; pong.
- Messages to decode: the ready event; an error response, with its code and message; a close frame, with its code and message; ping; and the acknowledgement of a command, matched by nonce.
- Activity fields, taken from the `Activity` type that CRP-010 defines in the domain package: both text lines, the start timestamp, large and small image keys with their hover text, and the activity type. Empty fields are omitted from the JSON, not sent as empty strings.

## Out of scope

- Opening pipes or sockets, which is CRP-021.
- Connection state, retries and ordering, which is CRP-023.
- Buttons, party, secrets, URLs, and any command other than set-activity.

## Acceptance criteria

- [ ] Encoded bytes for the handshake and for a set-activity command match fixtures written by hand from Discord's documentation.
- [ ] The reader returns complete frames when fed one byte at a time, and when fed several frames in one read.
- [ ] A declared length above the maximum is rejected before any payload is read.
- [ ] A stream that ends mid-header or mid-payload returns a distinct error.
- [ ] Unknown opcodes and unknown JSON fields are tolerated: reported to the caller as unknown, never a panic.
- [ ] Empty optional activity fields do not appear in the output.
- [ ] A fuzz test on the frame reader and on each message decoder runs clean, with a seed corpus that includes the fixtures.
- [ ] The package imports nothing that performs I/O beyond the stream interfaces it is given.

## Notes for the implementer

- Write the whole frame in one call. The official library notes that header and payload should go out together.
- The nonce generator is injected so tests are deterministic.
- Discord rejects a text line shorter than 2 characters. The renderer omits such lines; the codec only omits empty ones.
- Verify the opcode values, the handshake shape and the allowed activity types against the live sources in the `discord-ipc` skill before writing fixtures.

## Why this model and effort

A small, precisely specified binary format with good test hooks.

## References

- [ADR-0004](../../architecture/adr/0004-in-house-protocol-implementations.md)
- Discord RPC documentation: https://docs.discord.com/developers/topics/rpc
- Protocol notes in the official library: https://github.com/discord/discord-rpc/blob/master/documentation/hard-mode.md
