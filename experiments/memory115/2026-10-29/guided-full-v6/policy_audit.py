"""Offline, content-free policy/load/cost audit over sealed synthetic exports."""
import json, collections
from pathlib import Path
from guided_full_owned_export_v5 import current_run
r=Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
p=r/'review-snapshot-guided-full-v6/actual';out=r/'review-output-guided-full-v6'
load=lambda p:json.loads(p.read_bytes())
facts=load(out/'abc-facts.json');window=load(p/'window-final.json')
assert len(facts['cases'])==48 and facts['all48_owning_outcomes_present']
native_ops={e['operation_id'] for e in window['entries']}
counts=collections.Counter();limits=set();stops=[];without_native=[];loads=[];max_models=0;max_tools=0;rows=[];c43=None
for row in facts['cases']:
 seq=row['sequence'];db=load(p/f'case-{seq:03}-database.json');o=load(p/f'case-{seq:03}-outcome.json')
 run=current_run(db,o);task=o['task']['task_id'];result=o.get('result') or {}
 state=next(x for x in db['runs'] if x['run_id']==run)
 calls=[c for c in db['tool_calls'] if c['run_id']==run]
 reads=[c for c in calls if c['tool_name'] in ('memory.search','memory.open','memory.neighbors')]
 if seq==43:
  assert row['case']=='control-04' and row['arm'].startswith('C_') and row['repetition']==2
  c43={'sequence':seq,'run_state':row['run_state'],'flow_error':row['flow_error'],'read_statuses':[{'tool':c['tool_name'],'tool_status':c['status'],'data_status':(c.get('result') or {}).get('status')} for c in reads],'prepare_calls':row['prepare_calls'],'completed_broker_loads':row['completed_loads_with_matching_pin'],'actual_current_verifier_503':any(c['tool_name']=='http.request' and c['status']=='completed' and (c.get('result') or {}).get('status_code')==503 for c in calls),'native_entries':row['cost']['entries'],'old_failed_C43_modified':False,'isolated_causal_attribution':False}
 for c in reads:counts[(row['arm'],c['tool_name'],c['status'],str((c.get('result') or {}).get('status','')))]+=1
 code=result.get('ErrorCode') or state.get('error_message') or ''
 if row['run_state']!='RUN_STATE_COMPLETED' or row['flow_error']:
  # Public fixed labels only. Never project arbitrary error text or response bodies.
  label=next((s for s in ('memory_evidence_read_bound','memory_evidence_search_exhausted','memory_evidence_range_already_read','budget_model_turns_task','memory_source_stale','effect_unknown') if s in str(code)+' '+str(row['flow_error'])), 'other_failure')
  stops.append({'sequence':seq,'arm':row['arm'],'case':row['case'],'label':label,'run_state':row['run_state'],'noncompleted_tools':[{'tool':c['tool_name'],'status':c['status']} for c in calls if c['status']!='completed']})
 for scope in db['execution_budget_scopes']:
  if scope['kind']=='task' and scope['subject_id']==task:
   used=scope['used_json'];bound=scope['limits_json'];limits.add((bound['model_turns'],bound['external_calls']))
   max_models=max(max_models,used['model_turns']);max_tools=max(max_tools,used['external_calls'])
 for a in db['execution_model_attempts']:
  if a.get('run_id')==run and a['operation_id'] not in native_ops:
   assert a['actual_input_tokens'] is None and a['actual_output_tokens'] is None
   without_native.append({'sequence':seq,'usage':None,'native_entry_present':False,'reservation_refunded':False})
 if row['completed_loads_with_matching_pin']:
  loads.append({'sequence':seq,'case':row['case'],'arm':row['arm'],'repetition':row['repetition'],'completed_broker_loads':len(row['loaded_skill_digests']),'matching_current_pins':row['completed_loads_with_matching_pin'],'before_first_task_action':row['loads_completed_before_first_task_action'],'run_completed':row['run_state']=='RUN_STATE_COMPLETED' and not row['flow_error']})
 rows.append({'sequence':seq,'arm':row['arm'],'case':row['case'],'reads':len(reads),'searches':sum(c['tool_name']=='memory.search' for c in reads),'opens':sum(c['tool_name']=='memory.open' for c in reads),'memory_tool_noncompleted':sum(c['status']!='completed' for c in reads)})
assert all(x['reads']<=8 and x['searches']<=3 for x in rows)
assert limits=={(24,32)}
assert not any(x['reads'] for x in rows if x['arm'].startswith('A_'))
audit={'format':'butler115.evidence-policy-window-audit.v1','native_calls_created_by_audit':0,'current_memory_calls':[{'arm':k[0],'tool':k[1],'tool_status':k[2],'data_status':k[3],'count':v} for k,v in sorted(counts.items())],'noncompleted_runs':stops,'product_attempts_without_native_entry':without_native,'observed_task_limits':[{'model_turns':a,'external_calls':b} for a,b in sorted(limits)],'maximum_observed_task_model_entries':max_models,'maximum_observed_task_external_calls':max_tools,'all_control_read_counts_within_8_and_3':True,'all_A_reads_zero':True,'controls':rows,'original_raw_outcomes_changed':False,'production_updated':False}
with (out/'failure-and-memory-audit.json').open('x') as f:json.dump(audit,f,sort_keys=True,indent=2)
assert c43 is not None
with (out/'c43-native-regression.json').open('x') as f:json.dump(c43,f,sort_keys=True,indent=2)
proof={'format':'butler115.exact-current-broker-loads.v3','loads':loads,'total_matching_loads':sum(x['matching_current_pins'] for x in loads),'total_before_task_action':sum(x['before_first_task_action'] for x in loads),'successful_loads_are_not_task_success':True,'new_requests_created_by_audit':0}
with (out/'broker-load-proof.json').open('x') as f:json.dump(proof,f,sort_keys=True,indent=2)
print(json.dumps({'memory_calls':audit['current_memory_calls'],'stops':stops,'matching_loads':proof['total_matching_loads'],'without_native':without_native}))
