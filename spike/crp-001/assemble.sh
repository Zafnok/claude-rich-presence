#!/usr/bin/env bash
# assemble.sh <market-name> <plugin-version> <mcpServers-value> <bundle-to-embed: v1|v2|none> <win|linux|darwin>
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
market="$1"; pver="$2"; mcps="$3"; embed="$4"; os="$5"
out="$here/dist/$market"
rm -rf "$out"; mkdir -p "$out"
cp -r "$here/src/market/.claude-plugin" "$out/"
sed -i "s|@MARKET@|$market|" "$out/.claude-plugin/marketplace.json"
cp -r "$here/src/rich-presence" "$out/rich-presence"
sed -i "s|@VERSION@|$pver|; s|@MCPSERVERS@|$mcps|" "$out/rich-presence/.claude-plugin/plugin.json"
if [ "$embed" != none ]; then cp "$here/dist/bundles/rp-probe-$embed.mcpb" "$out/rich-presence/rp-probe.mcpb"; fi
cp -r "$here/src/rp-spike-dump" "$out/rp-spike-dump"
mkdir -p "$out/rp-spike-dump/tools" "$out/rp-spike-dump/hooks"
probe=rp-probe-$os
if [ "$os" = win ]; then probe=rp-probe.exe; fi
cp "$here/out/v1/server/$probe" "$out/rp-spike-dump/tools/$probe"
chmod +x "$out/rp-spike-dump/tools/$probe"
{
  echo '{ "hooks": {'
  sep=""
  for ev in SessionStart UserPromptSubmit PreToolUse PostToolUse PostToolUseFailure Notification Stop StopFailure PreCompact PostCompact PostModelSwitch SubagentStart SubagentStop SessionEnd; do
    printf '%s  "%s": [ { "hooks": [ { "type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/tools/%s", "args": ["dump", "cmdhook"], "timeout": 5 } ] } ]' "$sep" "$ev" "$probe"
    sep=$',\n'
  done
  printf '\n} }\n'
} > "$out/rp-spike-dump/hooks/hooks.json"
echo "assembled $out"
