// Package code translates presence tool calls made by hooks in the coding
// agent into domain events. It reads only an allowlist of hook fields and
// holds no policy.
//
// It may import internal/domain and internal/mcp. It must not import
// internal/host, internal/cli, any transport or any Discord package, and must
// never parse, store, log or forward prompts, tool inputs, tool outputs or
// paths.
//
// # The privacy boundary
//
// The allowlist in allowlist.go is the one list of what is read. A field
// that is not on an event's row is never decoded. Of what is read, a tool
// name becomes a tool kind, a model id becomes a family label, and a working
// directory becomes its last element, and only at the full privacy level. The
// level is applied to every event before it is queued.
//
// # Project profiles
//
// With a Resolver, the level is not fixed. Each hook that carries a working
// directory is resolved to the settings for it: a level and a display name.
// The directory is read at every level for this, used once and discarded; it
// is never stored. The settings last resolved hold until a hook carries
// another directory. Until the first hook that carries one, the session is at
// minimal, because the directory may name a profile more private than the
// global level. The Privacy option is the level for a directory with no
// profile, and for the whole session when there is no resolver.
//
// A move to a higher level is announced with a refresh that carries the level
// and the project. A move to a lower one ends the session and opens it again
// under the same id and start time at the lower level, because the domain has
// no event that takes a project or a model away, and so nothing published at
// the higher level may stay in the session at the host. Every event of a call
// is restricted at the level that call resolved.
//
// # Never impairing the caller
//
// The event tool does no I/O. It has a clock, pure functions and a bounded
// queue it never waits on. A goroutine the adapter owns takes events from
// the queue to the Publisher, so a stalled publisher costs events and never
// time. The tool's result is one constant.
//
// # Session identity
//
// The process is the session. It opens under a provisional id when the
// client initializes, takes the id the first hook carries, and takes a new
// one whenever a later hook carries a different id, as after a clear. Each
// move ends the old id and opens the new one with the original start time.
// The session ends when the adapter is closed.
package code
