"""Offline stdlib extraction of frozen synthetic input, not a native115 replay.
No source values are printed. This is an analysis utility, not a core service.
"""
import hashlib,json
from pathlib import Path
r=Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
p=r/'skill-context-20261029-v1'
(p/'fixture').mkdir(mode=0o700,parents=True,exist_ok=True)
(p/'out').mkdir(mode=0o700,exist_ok=True)
(p/'delta').mkdir(mode=0o700,exist_ok=True)
w=json.loads((r/'review-snapshot-abc-v6/actual/window-final.json').read_bytes())
slot=next(s for s in w['slots'] if s['case_id']=='control-01' and s['arm'].startswith('C'))
request=next(e['request'] for e in w['entries'] if e['slot']==slot['id'])
raw=json.dumps(request,ensure_ascii=False,sort_keys=True,separators=(',',':')).encode()+b'\n'
with (p/'fixture/request.json').open('xb') as f:f.write(raw)
manifest=(r/'review-snapshot-abc-v6/actual/skill-model-manifest.json').read_bytes()
with (p/'fixture/manifest.json').open('xb') as f:f.write(manifest)
pilot=(r/'skill-prompt-20261029-v1/out/result.json').read_bytes()
meta={'request_sha256':hashlib.sha256(raw).hexdigest(),'manifest_sha256':hashlib.sha256(manifest).hexdigest(),'previous_pilot_sha256':hashlib.sha256(pilot).hexdigest(),'tools':len(request['ToolDefinitions']),'request_bytes':len(raw),'manifest_bytes':len(manifest),'source':'closed ABC6 C/control-01 initial request; historical synthetic fixture only','prior_closed_abc_native':503,'previous_separate_force_native':1,'previous_separate_prompt_native':16,'planned_new_native_maximum':3,'business_actions':0,'repetitions':1,'measure':'first native tool proposal only; no full task, no skill execution, no product acceptance'}
with (p/'fixture/inventory.json').open('x') as f:json.dump(meta,f,sort_keys=True,indent=2);f.write('\n')
print(json.dumps(meta,sort_keys=True))
