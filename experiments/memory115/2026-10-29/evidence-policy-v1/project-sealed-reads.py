"""Read-only projection of sealed synthetic receipts for Go prefix replay."""
import json,pathlib,sys
sys.path.insert(0,'/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
from guided_full_owned_export_v5 import current_run
r=pathlib.Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1/review-snapshot-guided-full-v5/actual')
cases=[]
for f in sorted(r.glob('slot-*-start.json')):
 slot=json.loads(f.read_text());n=slot['sequence']
 if n<10:continue
 o=json.loads((r/f'case-{n:03d}-outcome.json').read_text());d=json.loads((r/f'case-{n:03d}-database.json').read_text());run=current_run(d,o)
 effects={e['initial_binding'].get('tool_call_id') for e in d['action_effects'] if e['initial_binding'].get('run_id')==run and e['state']=='effect_confirmed'}
 reads=[]
 for c in sorted(d['tool_calls'],key=lambda x:(x['started_at'],x['tool_call_id'])):
  if c['run_id']!=run or c['tool_name']not in ['memory.search','memory.open','memory.neighbors']:continue
  reads.append({'Tool':c['tool_name'],'ArgsJSON':json.dumps(c['args'],ensure_ascii=False,separators=(',',':')),'ResultJSON':json.dumps(c['result'],ensure_ascii=False,separators=(',',':')),'Status':c['status'],'Confirmed':c['tool_call_id']in effects})
 cases.append({'sequence':n,'case':slot['case_id'],'arm':slot['arm'],'reads':reads})
path=pathlib.Path('/srv/gitea/butler-acceptance/issue115-evidence-20261029-v1/build/read-traces.json');path.write_text(json.dumps(cases,ensure_ascii=False,separators=(',',':'))+'\n')
print(json.dumps({'cases':len(cases),'reads':sum(len(c['reads']) for c in cases),'confirmed':sum(h['Confirmed'] for c in cases for h in c['reads']),'new_model_requests':0}))
