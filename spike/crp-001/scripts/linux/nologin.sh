#!/usr/bin/env bash
# Linux no-login tests: isolated config, git marketplace, URL bundle, update, hooks, hang/exit.
source ~/rps/envl.sh
export CLAUDE_CONFIG_DIR="$R/cfg"
rm -rf "$R/cfg" "$R/market" "$R/logs/nl-"*; mkdir -p "$R/cfg" "$R/work/a" "$R/logs"
uname -srm; "$CL" --version
echo "--- validate"; "$CL" plugin validate --strict "$PD" 2>&1 | tail -1
cd "$R/work/a"
echo "--- L1 plugin-dir local bundle"
D="$R/logs/nl-pd"; mkdir -p "$D"; RP_SPIKE_DIR="$D" timeout 60 "$CL" --plugin-dir "$PD" mcp list 2>&1 | tail -1
ls -la "$PD/.mcpb-cache"/*/server 2>&1 | tail -4

echo "--- L2 marketplace + URL bundle"
pkill -f "$R/httpsd" 2>/dev/null; nohup "$R/httpsd" "$R/bundles" "$R/cert.pem" "$R/requests.log" > "$R/httpsd.out" 2>&1 &
sleep 1
cp -r "$R/market-src" "$R/market"; cd "$R/market"; git init -q -b main; git add -A; git -c user.name=spike -c user.email=s@l commit -q -m v0.0.1
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0="url.file://$R/market.insteadOf" GIT_CONFIG_VALUE_0="https://spike.invalid/market.git"
export NODE_EXTRA_CA_CERTS="$R/cert.pem"
cd "$R/work/a"
"$CL" plugin marketplace add https://spike.invalid/market.git 2>&1 | tail -1
"$CL" plugin install rich-presence@rp-spike-url 2>&1 | tail -1
D="$R/logs/nl-url1"; mkdir -p "$D"; RP_SPIKE_DIR="$D" timeout 60 "$CL" mcp list 2>&1 | tail -1
grep -o '"ver":"[^"]*"' "$D/observations.jsonl" | sort -u
cd "$R/market"; sed -i 's|"version": "0.0.1"|"version": "0.0.3"|; s|rp-probe-v1.mcpb|rp-probe-v3.mcpb|' rich-presence/.claude-plugin/plugin.json; git -c user.name=spike -c user.email=s@l commit -qam v0.0.3
cd "$R/work/a"
"$CL" plugin marketplace update rp-spike-url 2>&1 | tail -1
"$CL" plugin update rich-presence@rp-spike-url 2>&1 | tail -1
D="$R/logs/nl-url3"; mkdir -p "$D"; RP_SPIKE_DIR="$D" timeout 60 "$CL" mcp list 2>&1 | tail -1
grep -o '"ver":"[^"]*"' "$D/observations.jsonl" | sort -u
grep -v market.git "$R/requests.log" | tail -4
ls "$R/cfg/plugins/cache/rp-spike-url/rich-presence/"

echo "--- L3 unauthenticated -p: hooks, hang, exit"
for mode in empty hang exit; do
  D="$R/logs/nl-$mode"; mkdir -p "$D"; echo "$mode" > "$D/mode.txt"
  s=$(date +%s.%N)
  RP_SPIKE_DIR="$D" timeout 120 "$CL" -p "Reply OK" --model haiku --debug-file "$D/debug.log" < /dev/null 2>&1 | tail -2
  e=$(date +%s.%N); echo "== $mode wall $(python3 -c "print(round($e-$s,2))")s"
  python3 "$R/view.py" "$D/observations.jsonl"
done
python3 - <<EOF
import json
for l in open("$R/logs/nl-empty/observations.jsonl"):
    r=json.loads(l)
    if r["kind"]=="server_start":
        print("exe:",r["exe"]); print("cwd:",r["cwd"]); print("parent:",r["parent"]); print("env:",r["env"]); print("names:",r["env_names"]); print("stdin:",r["stdin_mode"])
    if r["kind"]=="initialize": print(json.dumps(r["params"]))
EOF
