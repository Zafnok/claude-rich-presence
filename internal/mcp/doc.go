// Package mcp is a minimal Model Context Protocol server over standard
// streams: the JSON-RPC subset this product needs. Tool calls return at once
// and never perform I/O on the calling path (ADR-0008).
//
// It must not import adapters, internal/host, internal/cli or any Discord
// package. Tools are registered with it by its callers.
//
// # Protocol versions
//
// Written against revision 2025-11-25 of the specification, read on
// 2026-10-03. Supported: 2025-11-25 and 2025-06-18, which do not differ in
// anything implemented here. A client that asks for any other version is
// answered with 2025-11-25 and decides for itself whether to go on.
//
// Revision 2026-07-28 removes the initialize handshake and ping, and is not
// supported. Nor is 2025-03-26, which requires JSON-RPC batches.
//
// # Supported subset
//
// Transport: JSON-RPC 2.0 over two byte streams, one UTF-8 message per line.
// Blank lines are skipped. A line longer than the limit is answered with an
// invalid request error and skipped. Nothing but responses is written to the
// output, each in a single write. When the input ends, the shutdown callback
// runs and Serve returns.
//
// Requests: initialize, ping, tools/list and tools/call. The server
// advertises the tools capability and nothing else. tools/list is never
// paged. A tool result is one text item and an optional error flag. A tool
// made with NewMetaTool is also given the _meta object of each call, which is
// how a client marks where a call came from.
//
// Notifications: notifications/initialized and notifications/cancelled are
// accepted. Every other notification is ignored, and so is a response,
// because the server never sends a request. No notification is ever answered.
//
// Not implemented: resources, prompts, sampling, roots, elicitation, logging,
// progress, completion, tasks, list-changed notifications, batches and the
// HTTP transport. If any is needed, ADR-0004 names the official Go SDK as the
// way forward.
//
// # Errors
//
//	-32700  the line is not JSON, or not UTF-8
//	-32600  the message is not an object, has a bad jsonrpc, id or method, or
//	        is too long; tools/list or tools/call before initialize; a second
//	        initialize
//	-32601  the method is not one of the four above
//	-32602  params is not an object or lacks what the method needs; the tool
//	        is unknown; a call's arguments or _meta is not an object; a
//	        cursor was sent
//
// An error for a message whose id could not be read carries a null id.
// Otherwise the id is echoed byte for byte. ping and unknown methods are
// answered before initialize as they are after it.
//
// A tool that fails, or whose handler panics, is not a protocol error: the
// call returns a result with isError set, and the server carries on.
//
// # Concurrency
//
// Requests are handled one at a time, in the order they arrive, on the
// goroutine that called Serve. The tools feed a state machine that depends on
// that order, and their handlers return at once by rule, so nothing is gained
// by running them side by side. It follows that a request is always finished
// before a cancellation for it can be read, so notifications/cancelled has
// nothing to cancel.
package mcp
