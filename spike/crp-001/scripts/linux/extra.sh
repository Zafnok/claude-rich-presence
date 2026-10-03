#!/usr/bin/env bash
# Linux extras: events with field recorder, clear/model/compact, kill, more latency, interactive TUI through a pty.
source ~/rps/envl.sh
SJ="--output-format stream-json --verbose"
mkdir -p "$R/work/l-c" "$R/work/l-k" "$R/work/tui"; echo hello > "$R/work/l-c/present.txt"

cd "$R/work/l-c"
MODE= run l-events -p 'Do these steps in order, one tool call per step. 1) Run the Bash command: echo step1   2) Use the Read tool on the file does-not-exist.txt in the current directory (it will fail; that is expected, continue).   3) Use the Agent tool with subagent_type general-purpose and the prompt: "Use the Read tool to read present.txt in the current directory and report its contents." Wait for it.   4) Reply with the word DONE.' --model haiku --plugin-dir "$PD" --plugin-dir "$PDD" --allowedTools "Bash(echo *)" Read Agent $SJ < /dev/null
python3 "$R/multi.py" | MODE= run l-multi2 -p --input-format stream-json --model haiku --plugin-dir "$PD" --plugin-dir "$PDD" $SJ

cd "$R/work/l-l" 2>/dev/null || { mkdir -p "$R/work/l-l"; cd "$R/work/l-l"; }
LAT='Run these Bash commands one at a time, each as its own separate tool call, in order, never combining them: echo 1, echo 2, echo 3, echo 4, echo 5, echo 6, echo 7, echo 8, echo 9, echo 10, echo 11, echo 12, echo 13, echo 14, echo 15, echo 16, echo 17, echo 18, echo 19, echo 20, echo 21, echo 22, echo 23, echo 24, echo 25, echo 26, echo 27, echo 28, echo 29, echo 30. Then reply DONE.'
for i in 2 3 4; do MODE= run "l-latency$i" -p "$LAT" --model haiku --plugin-dir "$PD" --allowedTools "Bash(echo *)" $SJ < /dev/null; done

# kill -9 mid-tool
cd "$R/work/l-k"; D="$R/logs/l-kill"; rm -rf "$D"; mkdir -p "$D"
RP_SPIKE_DIR="$D" "$CL" --debug-file "$D/debug.log" -p "Run the Bash command: sleep 40   then reply DONE" --model haiku --plugin-dir "$PD" --allowedTools "Bash(sleep *)" $SJ < /dev/null > "$D/out.jsonl" 2> "$D/stderr.txt" &
for i in $(seq 1 40); do grep -q PreToolUse "$D/observations.jsonl" 2>/dev/null && break; sleep 1; done
read CP SP <<< "$(python3 -c "
import json
for l in open('$D/observations.jsonl'):
    r=json.loads(l)
    if r['kind']=='server_start': print(r['parent'][0]['pid'], r['pid'])
")"
echo "kill -9 claude pid $CP (server $SP) at $(date +%T.%N)"; kill -9 "$CP"; sleep 8
python3 "$R/view.py" "$D/observations.jsonl" | tail -3; ps -p "$SP" -o pid,comm 2>&1 | tail -1

# interactive TUI through a pty
cd "$R/work/tui"; D="$R/logs/l-tui"; rm -rf "$D"; mkdir -p "$D"
RP_SPIKE_DIR="$D" python3 - "$CL" "$PD" "$PDD" "$D" <<'EOF'
import os,pty,sys,time,select,re
cl,pd,pdd,d=sys.argv[1:5]
pid,fd=pty.fork()
if pid==0:
    os.environ["TERM"]="xterm-256color"; os.environ["COLUMNS"]="140"; os.environ["LINES"]="45"
    os.execv(cl,[cl,"--plugin-dir",pd,"--plugin-dir",pdd,"--debug-file",d+"/debug.log","--model","haiku"])
out=open(d+"/screen.raw","wb")
def pump(sec):
    end=time.time()+sec
    while time.time()<end:
        r,_,_=select.select([fd],[],[],0.2)
        if r:
            try: b=os.read(fd,65536)
            except OSError: return False
            if not b: return False
            out.write(b); out.flush()
    return True
def send(s): os.write(fd,s.encode())
pump(8); send("\r"); pump(6)                      # trust dialog, if any
send("Run the Bash command: mkdir demo"); pump(1); send("\r"); pump(25)   # permission prompt should be showing
send("\r"); pump(15)                              # approve (default option)
pump(75)                                          # idle
send("/clear"); pump(1); send("\r"); pump(6)
send("/exit"); pump(1); send("\r"); pump(8)
try: os.kill(pid,9)
except Exception: pass
EOF
python3 - "$D" <<'EOF'
import re,sys
raw=open(sys.argv[1]+"/screen.raw","rb").read().decode("utf-8","replace")
txt=re.sub(r"\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[=>()][0-9A-B]?","",raw)
open(sys.argv[1]+"/screen.txt","w").write(txt)
hits=sorted(set(l.strip()[:200] for l in re.split(r"[\r\n]+",txt) if re.search(r"hook|rich-presence|presence_event|not connected|mcp_tool",l,re.I)))
print("screen lines mentioning hooks/plugin:", len(hits)); [print("  |",h) for h in hits[:25]]
EOF
python3 "$R/ev.py" "$D" | grep -v "#absent" | cut -c1-230
echo extra done
