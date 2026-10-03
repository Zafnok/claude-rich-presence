// Package codec encodes and decodes Discord IPC frames: the handshake, the
// set-activity command and responses. It is pure and works on bytes.
//
// A frame is a little-endian 32-bit opcode, a little-endian 32-bit payload
// length and that many bytes of JSON. The supported subset is:
//
//   - opcodes: handshake, frame, close, ping and pong;
//   - encoded: the handshake, the set-activity command with an activity or
//     with null to clear, and pong;
//   - decoded: the ready event, an error response, a close frame, ping and
//     the acknowledgement of a command.
//
// Anything else that is well formed decodes as KindUnknown. Buttons, party,
// secrets, URLs and every other command are not supported.
//
// It must not import net, os or any other package of this module except
// internal/domain.
package codec
