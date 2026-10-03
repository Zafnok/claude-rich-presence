# Linux env for spike runs. DEFAULT config (~/.claude) for logged-in runs; set CLAUDE_CONFIG_DIR yourself for isolated tests.
for v in $(env | grep -E '^(CLAUDE|ANTHROPIC|MCP_)' | cut -d= -f1); do unset "$v"; done
export R="$HOME/rps"
export CL="$HOME/.local/bin/claude"
export PD="$R/pd/rich-presence"
export PDD="$R/pd/rp-spike-dump"
run() { local n="$1"; shift; local D="$R/logs/$n"; rm -rf "$D"; mkdir -p "$D"; [ -n "$MODE" ] && echo "$MODE" > "$D/mode.txt"; RP_SPIKE_DIR="$D" "$CL" --debug-file "$D/debug.log" "$@" > "$D/out.jsonl" 2> "$D/stderr.txt"; echo "exit=$? $n"; }
