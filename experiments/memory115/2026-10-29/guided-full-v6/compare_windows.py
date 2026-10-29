"""Same scoring/denominators, unique accounting versus allocated arm costs."""
import json
from pathlib import Path
r=Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
load=lambda p:json.loads(p.read_bytes())
old=load(r/'review-output-guided-full-v5/coordinator-assessment.json')
new=load(r/'review-output-guided-full-v6/coordinator-assessment.json')
audit=load(r/'review-output-guided-full-v6/failure-and-memory-audit.json')
assert old['all48_owning_outcomes_present'] and new['all48_owning_outcomes_present']
assert all(x['denominator']==16 for x in old['arms']+new['arms'])
rows=[]
for before,after in zip(old['arms'],new['arms']):
 assert before['arm']==after['arm']
 reads=[x for x in audit['controls'] if x['arm']==after['arm']]
 rows.append({'arm':after['arm'],'old_tasks':before['coordinator_task_success'],'new_tasks':after['coordinator_task_success'],'old_full_known_tokens':before['allocated_full_known_tokens'],'new_full_known_tokens':after['allocated_full_known_tokens'],'task_delta':after['coordinator_task_success']-before['coordinator_task_success'],'full_known_token_delta':after['allocated_full_known_tokens']-before['allocated_full_known_tokens'],'new_control_memory_reads':sum(x['reads'] for x in reads),'new_control_searches':sum(x['searches'] for x in reads),'old_actual_task_elapsed_millis':before['actual_task_elapsed_millis'],'new_actual_task_elapsed_millis':after['actual_task_elapsed_millis']})
old_cases={x['sequence']:x for x in old['cases']}
assert all(all(old_cases[c['sequence']][k]==c[k] for k in ('case','arm','repetition')) for c in new['cases'])
changed=[{'sequence':c['sequence'],'case':c['case'],'arm':c['arm'],'old_success':old_cases[c['sequence']]['coordinator_task_success'],'new_success':c['coordinator_task_success']} for c in new['cases'] if old_cases[c['sequence']]['coordinator_task_success']!=c['coordinator_task_success']]
old_facts=load(r/'review-output-guided-full-v5/abc-facts.json')['cases']
new_facts=load(r/'review-output-guided-full-v6/abc-facts.json')['cases']
case_costs=[]
for case in sorted({c['case'] for c in new['cases']}):
 for arm in sorted({c['arm'] for c in new['cases']}):
  before=[c['cost'] for c in old_facts if c['case']==case and c['arm']==arm]
  after=[c['cost'] for c in new_facts if c['case']==case and c['arm']==arm]
  assert len(before)==len(after)==2 and all(before+after)
  case_costs.append({'case':case,'arm':arm,'old_native_entries':sum(c['entries'] for c in before),'new_native_entries':sum(c['entries'] for c in after),'old_known_full_tokens':sum(c['known_full_tokens'] for c in before),'new_known_full_tokens':sum(c['known_full_tokens'] for c in after),'new_memory_reads':sum(c['reads'] for c in audit['controls'] if c['case']==case and c['arm']==arm)})
v={'format':'butler115.closed-v5-vs-evidence-v6.v1','denominator_each_arm':16,'arms':rows,'changed_case_outcomes':changed,'new_within_window_comparisons':new['comparisons'],'new_unique_usage':new['new_usage'],'inclusive_primary_usage':new['inclusive_primary_usage'],'separate_diagnostics':new['separate_diagnostics'],'invoice_cost':'unknown','isolated_causal_effect_of_read_policy':False,'limitations':['v6 also contains the previously model-free C43 source-pin fix.','Source tasks, frozen notes and skills are regenerated; model sampling and external latency also vary.','Two repetitions; coordinator scoring, not independent whole-issue acceptance.','Full known tokens include provider total/thinking; unknown spend and product NULL are not zero.','Shared preparation is allocated to each eligible arm for comparison but incurred once in unique window accounting.'],'old_results_changed':False,'new_model_calls_created_by_comparison':0,'production_updated':False}
v['case_costs_two_repetitions']=case_costs
with (r/'review-output-guided-full-v6/comparison-v5-v6.json').open('x') as f:json.dump(v,f,sort_keys=True,indent=2)
print(json.dumps({'arms':rows,'changed_case_outcomes':changed},ensure_ascii=False))
