// Package schedule rate-limits and coalesces activity updates over an injected
// clock. It is pure.
//
// It may import internal/domain. It must not read the real clock, and must not
// import adapters, transports, internal/host or internal/cli.
package schedule
