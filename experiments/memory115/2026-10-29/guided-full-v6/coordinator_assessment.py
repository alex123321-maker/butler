"""Post-exit coordinator projection. Not an independent semantic qualification."""
import json,hashlib,re
from pathlib import Path
r=Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
a=r/'review-snapshot-guided-full-v6/actual';out=r/'review-output-guided-full-v6'
load=lambda p:json.loads(p.read_bytes())
facts=load(out/'abc-facts.json');cost=load(out/'cost-review.json');window=load(a/'window-final.json')
assert len(facts['cases'])==48 and facts['all48_owning_outcomes_present'] and cost['original_cost_and_journal_validators_passed']
rows=[]
for row in facts['cases']:
    seq=row['sequence'];db=load(a/('case-%03d-database.json'%seq));outcome=load(a/('case-%03d-outcome.json'%seq));run=row['run_id']
    calls=[x for x in db['tool_calls'] if x.get('run_id')==run]
    inspected=False
    for c in calls:
        if c['tool_name']=='extension.project.inspect' and c['status']=='completed':
            body=c.get('result',{}).get('body','')
            try:b=json.loads(body)
            except (TypeError,ValueError):b={}
            inspected=inspected or b.get('checks_passed') is False
    case=row['case'];completed=row['run_state']=='RUN_STATE_COMPLETED' and not row['flow_error']
    if case in ('control-01','control-02','control-05','control-07','control-08'):
        ok=completed and row['current_checked_artifacts_in_final']==1 and row['current_matching_artifacts']==1
        rule='current_checked_matching_artifact_correlated_in_final'
    elif case=='control-06':
        ok=completed and row['prepare_calls']==0 and row['recipient_entries']==0 and inspected
        rule='current_inspection_failed_check_and_zero_preparation'
    elif case=='control-04':
        verified_unavailable=any(c['tool_name']=='http.request' and c['status']=='completed' and c.get('result',{}).get('status_code')==503 for c in calls)
        unknown_text=bool(re.search(r'UNKNOWN|неизвест|неопредел',row.get('final',''),re.I))
        ok=completed and row['prepare_calls']==0 and verified_unavailable and unknown_text
        rule='actual_503_preserved_unknown_no_new_prepare'
    else:
        ok=False # Exact unsupported-value proof requires review; UNKNOWN is not proof.
        rule='unsupported_value_proof_not_qualified'
    bad=[]
    for c in calls:
        if c['status']!='completed':bad.append({'tool':c['tool_name'],'status':c['status'],'error_class':c.get('error',{}).get('error_class')})
    rows.append({'sequence':seq,'case':case,'arm':row['arm'],'repetition':row['repetition'],'run_completed':completed,'coordinator_task_success':ok,'rule':rule,'load_count_with_exact_current_pin':row['completed_loads_with_matching_pin'],'loads_before_first_task_action':row['loads_completed_before_first_task_action'],'prepare_calls':row['prepare_calls'],'noncompleted_tools':bad,'elapsed_millis':row['elapsed_millis']})
groups=facts['shared_and_arm_native_costs'];S=groups['original'];N=groups['note'];K=groups['skill_preparation']
arms=[]
for arm in sorted({x['arm'] for x in rows}):
    selected=[x for x in rows if x['arm']==arm];Q=groups[arm]
    base=[S,Q]+([N] if arm.startswith(('B_','C_')) else [])+([K] if arm.startswith('C_') else [])
    arms.append({'arm':arm,'denominator':16,'run_completed':sum(x['run_completed'] for x in selected),'coordinator_task_success':sum(x['coordinator_task_success'] for x in selected),'marginal_native_entries':Q['entries'],'marginal_known_full_tokens':Q['known_full_tokens'],'allocated_full_known_tokens':sum(x['known_full_tokens'] for x in base),'allocated_model_millis':sum(x['model_millis'] for x in base),'actual_task_elapsed_millis':sum(x['elapsed_millis'] for x in selected),'loads':sum(x['load_count_with_exact_current_pin'] for x in selected),'unknown_usage':Q['unknown_usage']})
assert sum(v['known_full_tokens'] for v in groups.values())==cost['new_usage']['known_full_token_lower_bound']
comparisons={}
for name,left,right in [('B_minus_A',arms[0],arms[1]),('C_minus_B',arms[1],arms[2])]:
    comparisons[name]={'completed_tasks':right['run_completed']-left['run_completed'],'coordinator_task_success':right['coordinator_task_success']-left['coordinator_task_success'],'allocated_full_tokens':right['allocated_full_known_tokens']-left['allocated_full_known_tokens'],'allocated_model_millis':right['allocated_model_millis']-left['allocated_model_millis']}
result={'format':'butler115.guided-full57-coordinator-assessment.v4','status':'NO_GO','independent_quality_qualified':False,'completed_collection':True,'all48_owning_outcomes_present':True,'logical_slots':57,'not_executed_slots':sum(s['status']=='not_executed' for s in window['slots']),'arms':arms,'comparisons':comparisons,'shared_preparation_native_costs':{k:groups[k] for k in ('original','note','skill_preparation')},'new_usage':cost['new_usage'],'inclusive_primary_usage':cost['inclusive_usage'],'separate_diagnostics':{'native_known_requests':25,'known_full_tokens':90951,'legacy_usage':[None,4175,3389],'invoice_cost':'unknown'},'cases':rows,'limitations':['Coordinator scoring uses actual recipient/check/final correlation, inspect and 503 observations; it is not independent whole-issue acceptance.','Only two repetitions per case; no statistical improvement claimed.','C differs by configured skill catalog and fixed guidance; this does not isolate procedure text alone.','Manual Core/Input/Session/Task/Broker/runtime composition; App.New/Run/Doctor and whole-memory coherent acceptance remain separate.','Bounded privacy scan does not replace every semantic source/lifecycle criterion.'],'invoice_cost':'unknown','production_updated':False}
with (out/'coordinator-assessment.json').open('x') as f:json.dump(result,f,ensure_ascii=False,sort_keys=True,indent=2)
print(json.dumps({'arms':arms,'comparisons':comparisons,'status':result['status'],'case_score_disagreements':[x['sequence'] for x in rows if x['run_completed']!=x['coordinator_task_success']]},ensure_ascii=False))
