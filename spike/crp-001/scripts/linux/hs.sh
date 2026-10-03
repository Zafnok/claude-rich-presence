#!/usr/bin/env bash
source ~/rps/envl.sh
cp /mnt/c/path/to/rps/pd/rich-presence/rp-probe.mcpb "$PD/rp-probe.mcpb"; rm -rf "$PD/.mcpb-cache"
mkdir -p "$R/work/l-hs"; cd "$R/work/l-hs"
Q='First run the Bash command: echo hello   Then, without further tool calls: quote every line you can see in this conversation, outside this message of mine and outside your own replies, that contains the words hook and success together. Give each such line on its own line prefixed with LINE: and finish with COUNT=<number of such lines>. If there are none, reply COUNT=0.'
for m in empty emptyjson; do
  MODE=$m run "l-hs-$m" -p "$Q" --model sonnet --plugin-dir "$PD" --allowedTools "Bash(echo *)" --output-format stream-json --verbose < /dev/null >/dev/null
  echo "== $m: $(python3 "$R/res.py" "$R/logs/l-hs-$m" | head -1 | cut -c1-330)"
  grep -o '"ver":"[^"]*"' "$R/logs/l-hs-$m/observations.jsonl" | sort -u
done
# all events with the {} reply: clear, model switch, compact
python3 "$R/multi.py" | MODE=emptyjson run l-multi-emptyjson -p --input-format stream-json --model haiku --plugin-dir "$PD" --output-format stream-json --verbose
python3 "$R/res.py" "$R/logs/l-multi-emptyjson" | grep "hook outcomes"; python3 "$R/ev.py" "$R/logs/l-multi-emptyjson" | grep -c CALL; cat "$R/logs/l-multi-emptyjson/stderr.txt"
