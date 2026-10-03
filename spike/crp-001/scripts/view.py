import json,sys
for l in open(sys.argv[1]):
    r=json.loads(l); k=r['kind']
    if k=='heartbeat': continue
    extra=''
    if k=='tools_call': extra=r['mode']+' '+json.dumps(r['args'])
    if k=='tools_call_replied': extra=str(r['server_us'])+'us'
    if k in('server_exit','other_method','signal','hanging','exiting_on_call'): extra=json.dumps({a:b for a,b in r.items() if a in('why','method','params','signal','event')})
    print(r['ts'][11:26], r['pid'], k, extra[:330])
