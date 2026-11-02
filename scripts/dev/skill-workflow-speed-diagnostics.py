#!/usr/bin/env python3
"""Read-only synthetic trace diagnostics; never reads auth or provider headers."""
import collections
import hashlib
import json
import pathlib
import sys

parent = pathlib.Path(sys.argv[1])
output = pathlib.Path(sys.argv[2])
read = lambda p: json.loads(p.read_text())
result = {'native_requests': 0, 'profiles': {}, 'historical_results_modified': False}
for label in ('gemini-medium', 'gemini-low', 'luna-max', 'luna-high', 'luna-low'):
    r = parent/('issue115-utility-native-workflow-'+label+'-speed-20261102-r1')
    assert (r/'exit.json').exists()
    plan = read(r/'out/plan/plan.json')
    races = (r/'native.log').read_bytes().count(b'WARNING: DATA RACE')
    assert races == 0
    above_old_limit = []
    max_generated = 0
    tiers = collections.Counter()
    models = collections.Counter()
    for path in sorted((r/'out/actual').glob('slot-*/native-*.private.json')):
        call = read(path)
        tiers.update({e['ProviderServiceTier'] for e in call['events'] if e.get('ProviderServiceTier')})
        models.update({e['ProviderModelVersion'] for e in call['events'] if e.get('ProviderModelVersion')})
        usage = call.get('usage')
        if not usage:
            continue
        generated = usage['OutputTokens'] if plan['Provider'] == 'openai' else max(usage['OutputTokens'], usage['TotalTokens']-usage['InputTokens'])
        max_generated = max(max_generated, generated)
        if generated > 8192:
            above_old_limit.append({'slot': call['slot'], 'turn': call['turn'], 'generated_tokens': generated,
                'reasoning_output_tokens': usage.get('OutputReasoningTokens')})
    rows = []
    for slot in plan['Slots']:
        d = r/'out/actual'/('slot-%03d' % slot['Sequence'])
        score = read(d/'score.json')
        if score['task_success']:
            continue
        execution = read(d/'execution.json')
        snap = read(d/'snapshot.private.json')
        native = [read(p) for p in sorted(d.glob('native-*.private.json'))]
        calls = snap['Tables'].get('calls', [])
        proposed = collections.Counter(e['ToolCall']['ToolName'] for n in native
            for e in n['events'] if e.get('ToolCall'))
        before_matches = []
        for call in calls:
            if call['tool_name'] != 'extension.project.prepare-patch':
                continue
            args = call.get('args')
            if isinstance(args, str):
                try:
                    args = json.loads(args)
                except ValueError:
                    continue
            if not isinstance(args, dict):
                continue
            for change in args.get('body', {}).get('changes', []):
                if isinstance(change, dict) and change.get('path') == 'README.md':
                    before_matches.append(change.get('before') == snap['RecipientFiles'].get('alpha'))
        finals = []
        for f in snap['Tables'].get('finals', []):
            try:
                value = json.loads(f['content'])
            except (KeyError, ValueError, TypeError):
                finals.append({'valid_json': False})
                continue
            if not isinstance(value, dict):
                finals.append({'valid_json': True, 'object': False})
                continue
            sources = value.get('sources')
            finals.append({'valid_json': True, 'outcome': value.get('outcome'),
                'effect': value.get('effect'), 'check': value.get('check'),
                'source_count': len(sources) if isinstance(sources, list) else None,
                'url_citations': sum(str(x.get('id', '')).startswith(('http:', 'https:'))
                    for x in (sources if isinstance(sources, list) else []) if isinstance(x, dict))})
        rows.append({'sequence': slot['Sequence'], 'arm': slot['Arm'], 'reasons': score['reasons'],
            'projection_error': execution['projection_error'], 'engine_error_present': execution['run_error'] != '<nil>',
            'patch_before_matches_current_file': before_matches,
            'proposed_tools': dict(proposed), 'recorded_tools': dict(collections.Counter(x['tool_name']+':'+x['status'] for x in calls)),
            'native_errors': dict(collections.Counter(n['error_code'] for n in native if n.get('error_code'))),
            'finals': finals})
    result['profiles'][label] = {'native_race_warnings': races, 'failed_cases': rows,
        'max_generated_tokens': max_generated, 'calls_above_old_8192_output_budget': above_old_limit,
        'observed_service_tiers_by_call': dict(tiers), 'observed_models_by_call': dict(models)}
result['analyzer_sha256'] = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
with output.open('x') as f:
    json.dump(result, f, indent=2)
print(json.dumps({'failed_cases': {k: len(v['failed_cases']) for k, v in result['profiles'].items()}, 'native_requests': 0}))
