// Package host is the presence host: election, serving followers, failover,
// and the wiring from events to Discord updates. Everything outside reaches it
// through ports.
//
// It may import the core packages and the codecs. It must not import
// internal/cli, any adapter, or a transport package directly.
package host
