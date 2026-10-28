#!/usr/bin/env python3
"""Post-exit JSON/provenance projection only; no model, DB, acceptance or repair.

Python stdlib is used for offline nested JSON/byte analysis, alongside the existing
Python closure utilities. This is neither a core service nor another eval runner.
Semantic review of the unchanged independent rubric is still required.
"""
import hashlib
import json
from pathlib import Path

base = Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
root = base / 'review-snapshot-abc-v6' / 'actual'
source = base / 'source-abc-v6'
out = base / 'review-output-abc-v6' / 'request-provenance-facts.json'

def read(path):
    return json.loads(path.read_bytes())

window = read(root / 'window-final.json')
prep = read(root / 'frozen-preparation.json')
facts = read(base / 'review-output-abc-v6' / 'abc-facts.json')
originals = read(source / 'docs/testing/fixtures/issue90/originals-v1.json')
assert window['finished_at'] and len(window['slots']) == 57
assert len(prep['originals']) == 4 and len(facts['cases']) == 24
assert facts['all24_owning_outcomes_present']

markers = []
for i, original in enumerate(prep['originals']):
    observation = original['observation']
    ids = [observation.get(k, '') for k in ('task_id', 'run_id', 'result_id', 'owner_result_id')]
    ids += [original['original_source']['source_id'], original['lesson']['source_id']]
    artifacts = [v['Artifact'] for v in original['recipient_audit']['entries'].values() if v['Artifact']]
    canary = originals['recipient_seed_contract']['canaries'][f'original-0{i+1}']
    markers.append({'ids': list(filter(None, ids)), 'artifacts': artifacts, 'canary': [canary]})

skill_markers = [prep['skill']['digest'], prep['skill']['body_source']['source_id']]
rows = []
by_sequence = {r['sequence']: r for r in facts['cases']}
for slot in window['slots'][9:33]:
    seq = slot['sequence']
    case = slot['case_id']
    arm = slot['arm'][0]
    entries = [e for e in window['entries'] if e['slot'] == slot['id']]
    assert entries
    requests = [json.dumps(e['request'], ensure_ascii=False, sort_keys=True) for e in entries]
    text = '\n'.join(requests)
    hits = []
    for i, group in enumerate(markers):
        forbidden = []
        if arm == 'A' or case == 'control-07':
            forbidden += group['ids']
            if case != 'control-04':
                forbidden += group['artifacts'] + group['canary']
        if case == 'control-08' and i == 0:
            forbidden += group['artifacts'] + group['canary']
        if any(m in text for m in forbidden):
            hits.append(f'original-0{i+1}')
    outcome = read(root / f'case-{seq:03d}-outcome.json')
    config = outcome['recipient_audit']['config']
    if case == 'control-04':
        expected = prep['originals'][3]['recipient_audit']['config']['initial_files']
    else:
        key = 'fresh_control_readme_template_v2' if case == 'control-05' else 'fresh_control_readme_template_v1'
        seed = hashlib.sha256(f'butler-experience90-control-seed-v1\0{case}\0{slot["repetition"]}'.encode()).hexdigest()
        body = originals['recipient_seed_contract'][key].replace('{sha256_control_seed}', seed)
        expected = {'alpha': body, 'beta': body}
    row = by_sequence[seq]
    initial_files_match = config['initial_files'] == expected
    deleted_source_opened = case == 'control-08' and any(m in row['memory_open_sources'] for m in markers[0]['ids'])
    evaluator_metadata_seen = any(m in text for m in ('"oracle"', '"requested_outcome"', '"positive_experience_expected_applicable"', 'control-01', 'control-02', 'control-03', 'control-04', 'control-05', 'control-06', 'control-07', 'control-08'))
    b_skill_marker_hit = arm == 'B' and any(m in text for m in skill_markers)
    incompatible_skill_load = case in ('control-05', 'control-07', 'control-08') and bool(row['loaded_skill_digests'])
    rows.append({'sequence': seq, 'case': case, 'arm': arm,
                 'captured_request_count': len(requests),
                 'captured_request_utf8_bytes': sum(len(r.encode()) for r in requests),
                 'forbidden_original_marker_hits': hits,
                 'B_skill_provenance_marker_hit': b_skill_marker_hit,
                 'incompatible_or_revoked_skill_loaded': incompatible_skill_load,
                 'deleted_source_successfully_opened': deleted_source_opened,
                 'evaluator_metadata_or_control_identity_seen': evaluator_metadata_seen,
                 'initial_files_match_frozen_reset': initial_files_match,
                 'model_output_quality_verdict': 'requires_semantic_review'})

result = {'version': 'butler115.read-only-provenance-projection.v1',
          'plan_sha256': facts['plan_sha256'], 'cases': rows,
          'exact_marker_and_reset_checks_passed': all(not r['forbidden_original_marker_hits'] and
              not r['B_skill_provenance_marker_hit'] and not r['incompatible_or_revoked_skill_loaded'] and
              not r['deleted_source_successfully_opened'] and not r['evaluator_metadata_or_control_identity_seen'] and
              r['initial_files_match_frozen_reset'] for r in rows),
          'scope': 'captured request provenance markers and exact recipient resets; not independent quality qualification',
          'new_native_admission': False, 'raw_text_published': False}
with out.open('x') as f:
    json.dump(result, f, ensure_ascii=False, sort_keys=True, indent=2)
    f.write('\n')
print(json.dumps({'rows': len(rows), 'exact_marker_and_reset_checks_passed': result['exact_marker_and_reset_checks_passed']}))
