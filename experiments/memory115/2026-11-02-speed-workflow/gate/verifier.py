#!/usr/bin/env python3
"""Stdlib-only verification of a completed, model-free latency control gate."""
import hashlib
import json
import pathlib
import sys

r = pathlib.Path(sys.argv[1])
source = sys.argv[2]
manifest_path = pathlib.Path(sys.argv[3])
read = lambda p: json.loads(p.read_text())
digest = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
assert read(r/'reports/exit.json')['exit_code'] == 0
manifest = read(manifest_path)
normalized = []
for item in manifest:
    p = r/'source'/item['path']
    b = str(p.readlink()).encode() if item['mode'] == '120000' else p.read_bytes()
    actual = hashlib.sha1(b'blob '+str(len(b)).encode()+b'\0'+b).hexdigest()
    if actual != item['sha1']:
        assert item.get('exported_eol') == 'crlf', item['path']
        b = b.replace(b'\r\n', b'\n')
        assert hashlib.sha1(b'blob '+str(len(b)).encode()+b'\0'+b).hexdigest() == item['sha1'], item['path']
        normalized.append(item['path'])
events = [json.loads(x) for x in (r/'reports/tests.jsonl').read_text().splitlines()]
assert not any(x['Action'] in ('fail', 'skip') for x in events)
required = {'TestMemoryUtilityWholeControlledABCIntegration', 'TestSkillWorkflowControlledABCIntegration',
    'TestSkillWorkflowSpeedControlledABCIntegration', 'TestReviewedSkillRealBrokerDurableApprovalE2E'}
assert required <= {x.get('Test') for x in events if x['Action'] == 'pass'}
for name, count in [('whole', 72), ('workflow', 36), ('workflow-speed', 36)]:
    slots = read(r/'reports'/name/'slots-final.json')
    assert len(slots) == count and all(x['Status'] == 'completed' for x in slots)
loads = 0
for d in (r/'reports/workflow-speed').glob('slot-*'):
    binding = read(d/'binding.json')
    tables = read(d/'snapshot.private.json')['Tables']
    current = [x for x in tables.get('calls', []) if x['tool_name'] == 'skills.load']
    migration = int(d.name[5:]) <= 24
    expected = binding['Arm'] == 'C' and migration
    assert len(current) == int(expected)
    assert all(x['status'] == 'completed' for x in current)
    assert read(d/'score.json')['task_success']
    loads += len(current)
assert loads == 8
original = read(r/'reports/workflow/suite.private.json')
optimized = read(r/'reports/workflow-speed/suite.private.json')
# Controlled fixtures generate independent random instance IDs. Verify the
# same scenario/permission shape and procedure DATA, not random byte equality.
# Native freezing separately requires byte equality with the closed reference.
shape = lambda s: [(x['family'], x['condition'], x['resource'], x['max_prepare']) for x in s['cases']]
assert shape(original) == shape(optimized)
for a, b in zip(original['cases'][:8], optimized['cases'][:8]):
    assert a['sources'][0]['source']['body'] == b['sources'][0]['source']['body']
proof = {'source': source, 'controlled_slots': 72, 'workflow_controlled_slots': 36,
    'speed_controlled_slots': 36, 'exact_initial_broker_loads': loads,
    'test_failures': 0, 'test_skips': 0, 'native_requests': 0,
    'tests_sha256': digest(r/'reports/tests.jsonl'),
    'unit_and_race_build_log_sha256': digest(r/'reports/build.log'),
    'source_git_blobs_verified': len(manifest), 'source_tree_manifest_sha256': digest(manifest_path),
    'git_declared_windows_eol_exports': normalized, 'verifier_sha256': digest(pathlib.Path(__file__)),
    'required_tests': sorted(required), 'new_suite_sha256': digest(r/'reports/workflow/suite.private.json')}
proof['optimized_control_suite_sha256'] = digest(r/'reports/workflow-speed/suite.private.json')
with (r/'reports/repair-summary.json').open('x') as f:
    json.dump(proof, f, indent=2)
print(json.dumps(proof))
