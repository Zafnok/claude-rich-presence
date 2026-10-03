// Package statusline translates the status line data the host application
// passes on standard input into session facts (ADR-0015). It reads only an
// allowlist of fields.
//
// It may import internal/domain. It must not import internal/host,
// internal/cli, any transport or any Discord package, and must never read
// stored credentials or call a network endpoint.
package statusline
