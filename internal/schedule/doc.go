// Package schedule rate-limits and coalesces activity updates over an injected
// clock. It is pure.
//
// It is generic over any comparable value and imports nothing from this
// module. It must not read the real clock.
//
// Updates are delivered by callback, not by channel. The scheduler decides
// what to send at the moment its consumer is free to take it, so a slow
// consumer never receives a value that has since been replaced, and never
// holds up a submitter. See New.
package schedule
