// Package code translates presence tool calls made by hooks in the coding
// agent into domain events. It reads only an allowlist of hook fields and
// holds no policy.
//
// It may import internal/domain and internal/mcp. It must not import
// internal/host, internal/cli, any transport or any Discord package, and must
// never parse, store, log or forward prompts, tool inputs, tool outputs or
// paths.
package code
