// Package codec encodes and decodes Discord IPC frames: the handshake, the
// set-activity command and responses. It is pure and works on bytes.
//
// It must not import net, os or any other package of this module except
// internal/domain.
package codec
