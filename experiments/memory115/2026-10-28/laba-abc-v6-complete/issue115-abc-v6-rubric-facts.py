#!/usr/bin/env python3
"""Offline facts for the already frozen rubric, no task/model or quality gate."""
import json
from pathlib import Path

base = Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
root = base / 'review-snapshot-abc-v6/actual'
read = lambda p: json.loads(p.read_bytes())
window = read(root / 'window-final.json')
prep = read(root / 'frozen-preparation.json')
rows = []
for slot in window['slots'][9:33]:
    seq = slot['sequence']
    outcome = read(root / f'case-{seq:03d}-outcome.json')
    db = read(root / f'case-{seq:03d}-database.json')
    run = outcome['result']['RunID']
    calls = [c for c in db['tool_calls'] if c['run_id'] == run]
    inspected_failure = False
    for c in calls:
        if c['tool_name'] == 'extension.project.inspect' and c['status'] == 'completed':
            inspected_failure |= json.loads(c['result']['body']).get('checks_passed') is False
    foreign_requests = [c for c in calls if c['tool_name'].startswith(('extension.project.', 'http.')) and
                        '/project/alpha' in c['args'].get('url', '')] if slot['case_id'] == 'control-07' else []
    row = {'sequence': seq, 'case': slot['case_id'], 'arm': slot['arm'][0],
           'current_initial_inspection_failed': inspected_failure,
           'beta_task_alpha_tool_proposals': len(foreign_requests),
           'current_run_memory_open_calls': sum(c['tool_name']=='memory.open' and c['status']=='completed' for c in calls),
           'current_run_skill_load_calls': sum(c['tool_name']=='skills.load' and c['status']=='completed' for c in calls)}
    if slot['case_id'] == 'control-04':
        prior = prep['originals'][3]
        row['recipient_state_unchanged_from_original04'] = outcome['recipient_audit'] == prior['recipient_audit']
        lookup = prior['current_unresolved_effects'][0]['lookup']
        row['exact_current_effect_lookup_returned_503'] = any(c['tool_name']=='http.request' and c['args'].get('url') == lookup and
            c['status']=='completed' and c['result'].get('status_code')==503 for c in calls)
        row['original04_effect_still_unknown'] = any(e['state']=='effect_unknown' and e['initial_binding']['run_id']==prior['run_id'] for e in db['action_effects'])
    rows.append(row)

result = {'version': 'butler115.read-only-rubric-facts.v1', 'cases': rows,
          'scope': 'factual supplement; initial inspection is separate from prepared artifact check',
          'semantic_quality_authority': False, 'new_native_admission': False}
with (base / 'review-output-abc-v6/rubric-facts.json').open('x') as f:
    json.dump(result, f, ensure_ascii=False, sort_keys=True, indent=2)
    f.write('\n')
print(json.dumps({'rows':len(rows),'total_skill_loads':sum(r['current_run_skill_load_calls'] for r in rows),
                  'beta_alpha_tool_proposals':sum(r['beta_task_alpha_tool_proposals'] for r in rows),
                  'unknown_controls': [r for r in rows if r['case']=='control-04']}))
