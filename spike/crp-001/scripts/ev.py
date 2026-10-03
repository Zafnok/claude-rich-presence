import json,sys
# ev.py <logdir>: compact event trace: server starts, hook tool calls, command-hook dumps
for l in open(sys.argv[1]+"/observations.jsonl"):
    r=json.loads(l); k=r["kind"]; t=r["ts"][11:23]
    if k=="server_start": print(t, r["pid"], "SERVER_START sid_env=", r["env"].get("CLAUDE_CODE_SESSION_ID","")[:8], "entry=", r["env"].get("CLAUDE_CODE_ENTRYPOINT"))
    elif k=="server_exit": print(t, r["pid"], "SERVER_EXIT", r.get("why"))
    elif k=="tools_call":
        a=dict(r["args"]); ev=a.pop("event"); sid=a.pop("session_id","")[:8]; a.pop("cwd",None)
        print(t, r["pid"], "CALL", r["tool"] if r["tool"]!="presence_event" else "", ev, "sid=",sid, {k:v for k,v in a.items() if v!=""}, "meta=" + json.dumps(r.get("meta")) if r.get("meta") else "")
    elif k=="hook_dump":
        v=r["values"]; print(t, "      dump", r["event"], "sid=", v.get("session_id","")[:8], {k:x for k,x in v.items() if k not in("session_id","hook_event_name","prompt_id","permission_mode")}, "keys+:", sorted(set(r["shape"])-{"cwd","hook_event_name","session_id","transcript_path","prompt_id","permission_mode"}))
