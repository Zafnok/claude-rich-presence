# CRP-001 spike prototype

Throwaway code for [CRP-001](../../docs/tickets/done/CRP-001-spike-claude-code-adapter.md). **Never merged.** The findings are in `docs/research/crp-001-claude-code-adapter.md` on `main`.

| Path | What it is |
|---|---|
| `probe/` | The logging MCP server (`probe mcp`) and the hook-input field recorder (`probe dump`). Standard library only. A `mode.txt` file in the log directory switches the reply: `empty`, `marker`, `jsonctx`, `error`, `hang`, `exit`, `slow:<ms>`, `suppress`, `emptyjson`, `emptytext`, `nocontent` |
| `mkbundle/` | Zips a directory into an `.mcpb` with executable bits set |
| `httpsd/` | Local HTTPS file server with a self-signed certificate and a redirecting path |
| `src/` | Bundle manifest, the test plugin and its hook file, the field-recorder plugin, the marketplace manifest. `@…@` placeholders are filled by `assemble.sh` |
| `assemble.sh` | Builds a marketplace directory from `src/` and built binaries |
| `scripts/` | The scenario runs and the log summarisers. They expect an environment file that defines `R`, `CL`, `PD`, `PDD` and a `run` function; `scripts/linux/envl.sh` is the Linux one |

The hook file here is the one the spike started with. It still declares `SessionEnd` and an unmatched `SessionStart`, which the findings say not to do.
