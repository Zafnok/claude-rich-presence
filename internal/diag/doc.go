// Package diag provides logging, counters and the doctor checks. Work content
// is never logged: no prompts, paths, project names or activity summaries.
//
// The logging interface is the guard. A message must be a constant, and an
// attribute can only be built from a constant name, a number, a duration or a
// version, so there is no call that would write an event or a string taken
// from one.
//
// A Logger writes to a file, so it is never called on a path that Claude
// waits on (ADR-0008).
//
// It may import internal/domain, internal/config and internal/discord/codec.
// It must not import adapters, internal/host or internal/cli; what it
// inspects is handed to it as Ports.
package diag
