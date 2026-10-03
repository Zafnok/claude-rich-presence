#!/usr/bin/env bash
# CRP-001 spike scenarios, batch 1. Works on Windows (Git Bash, env2.sh) and Linux (envl.sh).
# Usage: scen1.sh <env-file> <tag>
source "$1"; T="$2"
SJ="--output-format stream-json --verbose"
mkdir -p "$R/work/$T-m" "$R/work/$T-l" "$R/work/$T-g"

ASK='First run the Bash command: echo hello   Then, without any further tool calls, answer this: does the text RP-SPIKE appear anywhere in what you can see in this conversation (system messages, reminders, hook output, context added to my message, tool results)? Reply with exactly FOUND or NOTFOUND on the first line, and if FOUND, quote every line that contains it.'

# A4: tool result text returned to every hook
cd "$R/work/$T-m"
MODE=marker  run "$T-marker-haiku"  -p "$ASK" --model haiku  --plugin-dir "$PD" --allowedTools "Bash(echo *)" $SJ < /dev/null
MODE=marker  run "$T-marker-sonnet" -p "$ASK" --model sonnet --plugin-dir "$PD" --allowedTools "Bash(echo *)" $SJ < /dev/null
MODE=jsonctx run "$T-jsonctx"       -p "$ASK" --model sonnet --plugin-dir "$PD" --allowedTools "Bash(echo *)" $SJ < /dev/null
MODE=error   run "$T-error"         -p "$ASK" --model haiku  --plugin-dir "$PD" --allowedTools "Bash(echo *)" $SJ < /dev/null

# A5: latency sample, lean (no field recorder)
cd "$R/work/$T-l"
LAT='Run these Bash commands one at a time, each as its own separate tool call, in order, never combining them: echo 1, echo 2, echo 3, echo 4, echo 5, echo 6, echo 7, echo 8, echo 9, echo 10, echo 11, echo 12, echo 13, echo 14, echo 15, echo 16, echo 17, echo 18, echo 19, echo 20, echo 21, echo 22, echo 23, echo 24, echo 25, echo 26, echo 27, echo 28, echo 29, echo 30. Then reply DONE.'
MODE= run "$T-latency"        -p "$LAT" --model haiku --plugin-dir "$PD" --allowedTools "Bash(echo *)" $SJ < /dev/null
MODE= run "$T-latency-noplug" -p "$LAT" --model haiku --allowedTools "Bash(echo *)" $SJ < /dev/null
# A5: hung server and dead server during a tool-using turn
MODE=hang run "$T-hang-tools" -p 'Run the Bash command: echo 1   then the Bash command: echo 2   then reply DONE.' --model haiku --plugin-dir "$PD" --allowedTools "Bash(echo *)" $SJ < /dev/null
MODE=exit run "$T-exit-tools" -p 'Run the Bash command: echo 1   then the Bash command: echo 2   then reply DONE.' --model haiku --plugin-dir "$PD" --allowedTools "Bash(echo *)" $SJ < /dev/null

# if-filter on an mcp_tool hook
cd "$R/work/$T-g"
if [ ! -d repo ]; then
  git init -q --bare remote.git && git init -q -b main repo && cd repo && git -c user.name=spike -c user.email=s@l commit -q --allow-empty -m init && git remote add origin ../remote.git && git push -q origin main && cd ..
fi
cd repo
IFP='Run each of these Bash commands exactly as written, one at a time, each as its own separate tool call, in this order, and do not skip any even if it seems pointless:
1. git status
2. git push origin main
3. echo git push origin main
4. git log -1 --oneline
5. git status && git push origin main
6. echo $(git rev-parse HEAD)
7. git pushx origin main
8. git fetch origin
Then reply DONE.'
MODE= run "$T-if" -p "$IFP" --model haiku --plugin-dir "$PD" --allowedTools "Bash(git *)" "Bash(echo *)" $SJ < /dev/null
echo batch1 done
