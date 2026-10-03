import sys,json,datetime as dt
ts=lambda s: dt.datetime.strptime(s[:23],"%Y-%m-%dT%H:%M:%S.%f")
durs=[];srv=[]
for d in sys.argv[1:]:
    pend=[]
    for l in open(d+"/debug.log",encoding="utf-8",errors="replace"):
        if "Hooks: mcp_tool calling" in l: pend.append(ts(l))
        elif pend and ("Hook output does not start" in l or "mcp_tool hook error" in l): durs.append((ts(l)-pend.pop(0)).total_seconds()*1000)
    srv+= [json.loads(l)["server_us"] for l in open(d+"/observations.jsonl") if '"tools_call_replied"' in l]
durs.sort();srv.sort()
p=lambda a,q: a[min(len(a)-1,int(round(q*(len(a)-1))))]
print("client round trip ms: n=%d p50=%s p90=%s p99=%s max=%s"%(len(durs),p(durs,.5),p(durs,.9),p(durs,.99),durs[-1]))
print("server handling us:   n=%d p50=%s p99=%s max=%s"%(len(srv),p(srv,.5),p(srv,.99),srv[-1]))
from collections import Counter; print("histogram ms:", sorted(Counter(int(x) for x in durs).items()))
