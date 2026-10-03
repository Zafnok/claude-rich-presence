// Package protocol defines the control channel between a session process and
// the presence host: message types and their codec. It is pure.
//
// The format is specified in docs/protocol/control.md, and the golden files
// under testdata hold one example of each message. A change to either is a
// change to the wire format.
//
// It may import internal/domain. It must not import net, os or any other
// package of this module.
package protocol
