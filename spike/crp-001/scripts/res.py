import json,sys
# res.py <logdir>: final answer, marker hits in the stream, hook outcomes
d=sys.argv[1]
hits=0; outcomes={}
for l in open(d+"/out.jsonl",encoding="utf-8"):
    m=json.loads(l); t=m.get("type")
    if t=="system" and m.get("subtype")=="hook_response":
        k=(m.get("hook_event"),m.get("outcome")); outcomes[k]=outcomes.get(k,0)+1
        if "RP-SPIKE" in json.dumps(m): hits+=0  # hook's own output echo, not model context
    elif t=="result":
        print("RESULT:", (m.get("result") or "")[:600].replace("\n"," | ")); print("turns",m.get("num_turns"),"ms",m.get("duration_ms"),"api_ms",m.get("duration_api_ms"))
    elif t in("user","assistant"):
        s=json.dumps(m)
        if "RP-SPIKE" in s: hits+=1; print("  IN-CONVERSATION:", t, s[s.find("RP-SPIKE")-200:s.find("RP-SPIKE")+120])
print("hook outcomes:", outcomes); print("conversation messages containing marker:", hits)
