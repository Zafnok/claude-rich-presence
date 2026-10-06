// Package domain holds the core model: events, the session state machine and
// the session registry. It is pure: a function of its inputs.
//
// It also holds the one cleaning of text that reaches a Discord text line,
// CleanName. The adapters and the configuration clean with it, and Validate
// refuses a project name it would change, so the two cannot disagree.
//
// It must not import any other package of this module, nor os, net or
// anything else that touches the operating system or the real clock.
package domain
