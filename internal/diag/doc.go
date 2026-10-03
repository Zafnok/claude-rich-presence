// Package diag provides logging and the doctor checks. Work content is never
// logged: no prompts, paths or activity summaries.
//
// It may import internal/domain and internal/config. It must not import
// adapters, internal/host or internal/cli; what it writes to and inspects is
// handed to it.
package diag
