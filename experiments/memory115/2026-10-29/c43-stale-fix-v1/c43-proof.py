import json,hashlib,pathlib,subprocess,sys
sys.path.insert(0,'/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
from guided_full_owned_export_v5 import current_run
root=pathlib.Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
r=root/'review-snapshot-guided-full-v5/actual'
o=json.loads((r/'case-043-outcome.json').read_text());d=json.loads((r/'case-043-database.json').read_text());run=current_run(d,o)
slot=json.loads((r/'slot-043-start.json').read_text())['id']
entry=next(json.loads(f.read_text()) for f in sorted(r.glob('entry-*-end.json')) if json.loads(f.read_text())['slot']==slot)
names=[x['Name'] for x in entry['request']['ToolDefinitions']]
q="SELECT jsonb_agg(to_jsonb(l)) FROM memory_experience_lessons l JOIN memory_experience_pins p USING(source_id) WHERE p.run_id='"+run+"';"
p=subprocess.run(['docker','exec','-i','issue115-c43-pg-20261029-v1','psql','-X','-q','-U','postgres','-d','memory115_c43_test','-At','-v','ON_ERROR_STOP=1'],input=q,text=True,capture_output=True,check=True)
lessons=json.loads(p.stdout)
timings=json.loads((r/'processes'/('source-timing-'+hashlib.sha256(b'/execution/actual/case-043').hexdigest()+'.json')).read_text())
proof={'format':'butler115.c43-dependency-pin-proof.v1','new_model_requests':0,'historical_case':43,'environment':o['environment'],'recorded_run':run,'execution_result_was_nil':o.get('result')is None,'durable_run':[{'state':x['current_state'],'error_type':x['error_type'],'error_message':x['error_message']} for x in d['runs'] if x['run_id']==run],'current_permitted_tool_names':names,'pinned_lessons':[{'source_id':x['source_id'],'environment_matches':x['environment_hash']==o['environment'],'dependencies':x['dependencies_json'],'missing_current_tools':[v['tool'] for v in x['dependencies_json'] if v['tool']not in names]} for x in lessons],'timing_failure':[x for x in timings if x.get('failure')],'tools':[{'name':x['tool_name'],'status':x['status'],'data_status':(x.get('result')or{}).get('status')} for x in d['tool_calls'] if x['run_id']==run],'replay_scope':'Exact retained lesson/body/graph and current tool-name constraint, fresh transactional run/lease/grants; rolled back. Old installation expiry preserved, not renewed. Fresh PG regression independently proves the current-dependency guard fails while base sources and original dependency guards pass.','replay_before':{'data_status':'empty','incompatible_lesson_pins':1},'replay_after':{'data_status':'empty','incompatible_lesson_pins':0}}
path=pathlib.Path('/srv/gitea/butler-acceptance/issue115-c43-stale-20261029-v1/reports/c43-proof.json');path.write_text(json.dumps(proof,ensure_ascii=False,indent=2)+'\n');print(json.dumps(proof,ensure_ascii=False,indent=2))
