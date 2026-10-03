import json,sys,time
def u(s): sys.stdout.write(json.dumps({"type":"user","message":{"role":"user","content":s}})+"\n"); sys.stdout.flush()
u("Reply with only OK, no tools."); time.sleep(12)
u("/clear"); time.sleep(6)
u("Reply with only OK again, no tools."); time.sleep(12)
u("/model sonnet"); time.sleep(6)
u("Reply with only OK a third time, no tools."); time.sleep(12)
u("/compact"); time.sleep(20)
u("Reply with only OK a fourth time, no tools."); time.sleep(12)
