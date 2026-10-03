// Package domain holds the core model: events, the session state machine and
// the session registry. It is pure: a function of its inputs.
//
// It must not import any other package of this module, nor os, net or
// anything else that touches the operating system or the real clock.
package domain
