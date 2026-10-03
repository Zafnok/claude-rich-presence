#!/usr/bin/env bash
# CRP-001 spike scenarios, batch 2: lifecycle, compaction, resume, clear, model switch, context cost, model-initiated calls.
# Usage: scen2.sh <env-file> <tag>
source "$1"; T="$2"
SJ="--output-format stream-json --verbose"
mkdir -p "$R/work/$T-s"; cd "$R/work/$T-s"
sid() { python3 -c "
import json,sys
for l in open(sys.argv[1],encoding='utf-8'):
    m=json.loads(l)
    if m.get('type')=='result': print(m.get('session_id'))
" "$R/logs/$1/out.jsonl" | tail -1; }
command -v python3 >/dev/null 2>&1 || python3() { python "$@"; }

MODE= run "$T-s1" -p "Remember the word PINEAPPLE. Reply OK." --model haiku --plugin-dir "$PD" --plugin-dir "$PDD" $SJ < /dev/null
SID=$(sid "$T-s1"); echo "session: $SID"
MODE= run "$T-compact" -p "/compact" --resume "$SID" --model haiku --plugin-dir "$PD" --plugin-dir "$PDD" $SJ < /dev/null
MODE= run "$T-resume" -p "Run the Bash command: echo resumed   then tell me the word I asked you to remember." --resume "$SID" --model haiku --plugin-dir "$PD" --plugin-dir "$PDD" --allowedTools "Bash(echo *)" $SJ < /dev/null
MODE= run "$T-continue" -p "Reply OK" --continue --model haiku --plugin-dir "$PD" --plugin-dir "$PDD" $SJ < /dev/null

# multi-turn over stream-json input: prompt, /clear, prompt, /model, prompt
umsg() { python3 -c "import json,sys; print(json.dumps({'type':'user','message':{'role':'user','content':sys.argv[1]}}))" "$1"; }
{ umsg "Reply OK"; sleep 12; umsg "/clear"; sleep 6; umsg "Reply OK again"; sleep 12; umsg "/model sonnet"; sleep 6; umsg "Reply OK a third time"; sleep 15; } | MODE= run "$T-multi" -p --input-format stream-json --model haiku --plugin-dir "$PD" --plugin-dir "$PDD" $SJ

# context cost and model-initiated calls
MODE= run "$T-context" -p "/context" --model haiku --plugin-dir "$PD" $SJ < /dev/null
MODE= run "$T-context-noplug" -p "/context" --model haiku $SJ < /dev/null
MODE= run "$T-ask-status-denied" -p "Is my Discord presence working? Use whatever tool you have for that." --model haiku --plugin-dir "$PD" $SJ < /dev/null
MODE= run "$T-ask-status-allowed" -p "Is my Discord presence working? Use whatever tool you have for that." --model haiku --plugin-dir "$PD" --allowedTools "mcp__plugin_rich-presence_presence__presence_status" $SJ < /dev/null
MODE= run "$T-unprompted" -p "Write a haiku about autumn, then run the Bash command: echo done   and then explain in two sentences what a mutex is." --model sonnet --plugin-dir "$PD" --allowedTools "Bash(echo *)" "mcp__plugin_rich-presence_presence__presence_status" "mcp__plugin_rich-presence_presence__presence_event" $SJ < /dev/null
echo batch2 done
