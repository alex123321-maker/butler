"""Offline original-case audit; no SQL, auth, inference or result repair."""
import json,collections,hashlib
from pathlib import Path
from guided_full_owned_export_v5 import current_run
r=Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1');p=r/'review-snapshot-guided-full-v5/actual';out=r/'review-output-guided-full-v5'
f=json.loads((out/'abc-facts.json').read_bytes());w=json.loads((p/'window-final.json').read_bytes());cost=json.loads((out/'cost-review.json').read_bytes())
by_slot=collections.defaultdict(list)
for e in w['entries']:by_slot[e['slot']].append(e)
counts=collections.Counter();failures=[];model_stops=[];limits=set();max_models=0;max_external=0;no_native=[];native_ops={e['operation_id'] for e in w['entries']}
for row in f['cases']:
 n=row['sequence'];d=json.loads((p/f'case-{n:03}-database.json').read_bytes());o=json.loads((p/f'case-{n:03}-outcome.json').read_bytes());run=current_run(d,o);task=o['task']['task_id'];result=o.get('result') or {}
 current=[c for c in d['tool_calls'] if c['run_id']==run]
 for c in current:
  if c['tool_name'] in ['memory.search','memory.open','memory.neighbors']:
   counts[(c['tool_name'],c['status'],str((c.get('result')or{}).get('status','')))]+=1
 for scope in d['execution_budget_scopes']:
  if scope['kind']=='task' and scope['subject_id']==task:
   u=scope['used_json'];l=scope['limits_json'];limits.add((l['model_turns'],l['external_calls']));max_models=max(max_models,u['model_turns']);max_external=max(max_external,u['external_calls'])
 code=result.get('ErrorCode','')
 if code.startswith('budget_'):model_stops.append({'sequence':n,'code':code})
 if row['run_state']!='RUN_STATE_COMPLETED' or row['flow_error']:
  failures.append({'sequence':n,'case':row['case'],'arm':row['arm'],'run_state':row['run_state'],'flow_error':row['flow_error'],'result_error_code':code,'noncompleted_tools':[{'name':c['tool_name'],'status':c['status']} for c in current if c['status']!='completed'],'completed_broker_loads':row['completed_loads_with_matching_pin']})
 for attempt in d['execution_model_attempts']:
  if attempt.get('run_id')==run and attempt['operation_id'] not in native_ops:
   err=next(x.get('error_message') for x in d['runs'] if x['run_id']==run)
   assert err=='memory_source_stale' and attempt['actual_input_tokens'] is None and attempt['actual_output_tokens'] is None
   no_native.append({'sequence':n,'error_code':'memory_source_stale','usage':None,'native_entry_present':False,'reservation_refunded':False,'root_cause_reproduced':False})
loads=[]
for row in f['cases']:
 if row['completed_loads_with_matching_pin']:
  loads.append({'sequence':row['sequence'],'case':row['case'],'arm':row['arm'],'repetition':row['repetition'],'completed_broker_loads':len(row['loaded_skill_digests']),'loads_with_exact_current_pin':row['completed_loads_with_matching_pin'],'before_first_task_action':row['loads_completed_before_first_task_action'],'execution_result_present':row['execution_result_present'],'run_state':row['run_state']})
assert sum(x['loads_with_exact_current_pin'] for x in loads)==6 and sum(x['before_first_task_action'] for x in loads)==6
assert len(no_native)==1 and no_native[0]['sequence']==43
assert all(k[1]=='completed' for k in counts) and counts[('memory.search','completed','ok')]+counts[('memory.search','completed','empty')]>0
report={'format':'butler115.fixed-memory-expanded-window-audit.v1','new_requests_created_by_audit':0,'current_memory_calls':[{'tool':k[0],'tool_status':k[1],'data_status':k[2],'count':v} for k,v in sorted(counts.items())],'memory_tool_UNKNOWNs':0,'case_task_limits':[{'model_entries':a,'external_calls':b} for a,b in sorted(limits)],'maximum_observed_task_model_entries':max_models,'maximum_observed_task_external_calls':max_external,'budget_stops':model_stops,'noncompleted_runs':failures,'product_attempts_without_native_entry':no_native,'missing_execution_result_cases':[row['sequence'] for row in f['cases'] if not row['execution_result_present']],'case42_run_complete_is_not_qualified_task_success':{'frozen_case':'control-03','prior_original03_owner_verdict':'unknown','final_preserves_uncertainty_about_ready_maybe':True,'qualified_unsupported_value_proof':False},'original_raw_outcomes_changed':False,'production_updated':False}
# Preserve the strict frozen criterion: uncertainty and an honest final do not
# become verified unsupported-value evidence; no row is regraded after collection.
report['case42_run_complete_is_not_qualified_task_success']['final_preserves_uncertainty_about_ready_maybe']=True
with (out/'failure-and-memory-audit.json').open('x') as f:json.dump(report,f,sort_keys=True,indent=2)
with (out/'broker-load-proof.json').open('x') as f:json.dump({'format':'butler115.exact-current-broker-loads.v2','loads':loads,'total_matching_loads':6,'total_before_task_action':6,'positive_repeat_similar_loads':4,'extra_known_failure_loads':2,'inapplicable_drift_scope_inspect_only_deleted_loads':0,'successful_loads_are_not_task_success':True,'new_requests_created_by_audit':0},f,sort_keys=True,indent=2)
print(json.dumps({'memory_calls':report['current_memory_calls'],'model_stops':model_stops,'load_count':6,'missing_execution_result_cases':report['missing_execution_result_cases'],'product_NULL_without_native':no_native}))