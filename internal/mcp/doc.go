// Package mcp is a minimal Model Context Protocol server over standard
// streams: the JSON-RPC subset this product needs. Tool calls return at once
// and never perform I/O on the calling path (ADR-0008).
//
// It must not import adapters, internal/host, internal/cli or any Discord
// package. Tools are registered with it by its callers.
package mcp
