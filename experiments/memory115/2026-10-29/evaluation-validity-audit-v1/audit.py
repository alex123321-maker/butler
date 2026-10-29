"""Read-only audit of sealed v6 JSON; no models, services or credential reads.

Python stdlib is used only to project existing research JSON and inspect the
Python coordinator scorer's AST. This is not product code or a replacement oracle.
"""
import argparse
import ast
import hashlib
import json
import re
from pathlib import Path

p = argparse.ArgumentParser()
p.add_argument('--root', type=Path, required=True)
p.add_argument('--scorer', type=Path, required=True)
p.add_argument('--out', type=Path, required=True)
args = p.parse_args()
load = lambda path: json.loads(path.read_bytes())
raw = args.root / 'review-snapshot-guided-full-v6/actual'
review = args.root / 'review-output-guided-full-v6'
facts = load(review / 'abc-facts.json')['cases']
scores = {x['sequence']: x['coordinator_task_success'] for x in load(review / 'coordinator-assessment.json')['cases']}
assert len(facts) == len(scores) == 48
rows, paired = [], {}
for x in facts:
    db = load(raw / f"case-{x['sequence']:03}-database.json")
    calls = [c for c in db['tool_calls'] if c['run_id'] == x['run_id']]
    inspections = []
    for c in calls:
        if c['tool_name'] == 'extension.project.inspect' and c['status'] == 'completed':
            b = json.loads(c['result']['body'])
            inspections.append({
                'accepted_value': 'accepted_ready_value' in b,
                'current_file': bool(b.get('files')),
                'current_revision': bool(b.get('project_revision')),
            })
            key = (x['case'], x['repetition'])
            if not any(a == x['arm'] for a, _ in paired.get(key, [])):
                fields = {k: b.get(k) for k in ('accepted_ready_value', 'files', 'project_revision', 'environment_sha256', 'checks_passed')}
                digest = hashlib.sha256(json.dumps(fields, sort_keys=True).encode()).hexdigest()
                paired.setdefault(key, []).append((x['arm'], digest))
    rows.append({k: x[k] for k in ('sequence', 'case', 'arm', 'repetition', 'cost')} | {
        'coordinator_success': scores[x['sequence']], 'inspections': inspections,
        'memory_reads': sum(c['tool_name'].startswith('memory.') for c in calls),
        'loads': x['completed_loads_with_matching_pin'],
    })
arms = sorted({x['arm'] for x in rows})
complete = []
for key in sorted(paired):
    xs = [x for x in rows if (x['case'], x['repetition']) == key]
    assert len(xs) == 3
    if key[0] != 'control-03' and all(x['cost']['unknown_usage'] == 0 for x in xs):
        complete.append(key)
groups = {}
for name, selected in [('complete_non_unknown_triplets', complete), ('positive_complete_triplets', [('control-01', 1), ('control-01', 2), ('control-02', 1)])]:
    groups[name] = []
    for arm in arms:
        xs = [x for x in rows if x['arm'] == arm and (x['case'], x['repetition']) in selected]
        groups[name].append({'arm': arm, 'tasks': len(xs), 'successes': sum(x['coordinator_success'] for x in xs),
            'native_calls': sum(x['cost']['entries'] for x in xs),
            'known_tokens': sum(x['cost']['known_full_tokens'] for x in xs),
            'memory_reads': sum(x['memory_reads'] for x in xs), 'loads': sum(x['loads'] for x in xs)})

# Execute only the actual score decision, not the scorer's file-writing body.
source = args.scorer.read_bytes()
tree = ast.parse(source)
loop = next(n for n in tree.body if isinstance(n, ast.For) and isinstance(n.target, ast.Name) and n.target.id == 'row')
decision = next(n for n in loop.body if isinstance(n, ast.If) and 'control-01' in ast.unparse(n.test))
code = compile(ast.fix_missing_locations(ast.Module(body=[decision], type_ignores=[])), '<frozen-score-decision>', 'exec')
counterexamples = []
for case, final, expected in [
    ('control-06', 'Проверка пройдена, checks_passed=true.', False),
    ('control-04', 'Раньше был UNKNOWN. Теперь создание артефакта достоверно подтверждено.', False),
    ('control-01', 'Артефакт audit-artifact НЕ прошёл проверку.', False),
]:
    row = {'prepare_calls': 0, 'recipient_entries': 0, 'current_checked_artifacts_in_final': 1,
           'current_matching_artifacts': 1, 'final': final}
    env = {'case': case, 'row': row, 'completed': True, 'inspected': True, 're': re,
           'calls': [{'tool_name': 'http.request', 'status': 'completed', 'result': {'status_code': 503}}]}
    exec(code, env)
    counterexamples.append({'case': case, 'synthetic_final': final, 'expected_semantic_score': expected,
                            'actual_coordinator_decision': env['ok'], 'rule': env['rule'],
                            'limitation_demonstrated': env['ok'] != expected})
assert all(x['limitation_demonstrated'] for x in counterexamples)
archive_sha = hashlib.sha256((args.root / 'private-native-raw-guided-full-v6.tar').read_bytes()).hexdigest()
assert archive_sha == '5a4ca8064fdab2ca3b17f1ca147a5a7a046d1afdf2a8b5031701e0ea8375e156'
out = {'generation_requests': 0, 'historical_scores_unchanged': True,
       'conditional_diagnostics_not_replacement_scores': groups,
       'inspection_count': sum(len(x['inspections']) for x in rows),
       'all_inspections_expose_solution_inputs': all(all(i.values()) for x in rows for i in x['inspections']),
       'paired_current_state_checks': [{'case': k[0], 'repetition': k[1], 'arms': len(v), 'same_inputs': len({d for _, d in v}) == 1} for k, v in sorted(paired.items())],
       'synthetic_scorer_counterexamples_not_actual_model_failures': counterexamples,
       'scorer_sha256': hashlib.sha256(source).hexdigest(), 'raw_archive_sha256': archive_sha,
       'cases': rows}
args.out.mkdir(parents=True, exist_ok=False)
(args.out / 'facts.json').write_text(json.dumps(out, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(json.dumps({k: v for k, v in out.items() if k != 'cases'}, ensure_ascii=False))
