#!/usr/bin/env python3
"""Read-only stdlib projection of the two authorized native windows.
Exports fixed task observations/counts; never exports settings or credentials.
"""
import hashlib
import json
from pathlib import Path
import subprocess

BASE = Path('/srv/storage/butler/agents')
WINDOWS = [('initial', 'tool-discovery-native-20261019', 'butler-discovery-native-20261019'),
           ('cost_pair', 'tool-discovery-cost-pair-20261019', 'butler-discovery-cost-pair-20261019')]


def rows(project, query):
    command = ['docker', 'exec', '-i', project + '-postgres-1', 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'butler', '-d', 'butler_discovery_native_test']
    result = subprocess.run(command, input=('select coalesce(json_agg(row_to_json(x)),\'[]\') from (' + query + ') x').encode(), stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)
    return json.loads(result.stdout)


report = {'provider': 'antigravity', 'model': 'gemini-3.8-flash-high', 'output_ceiling': 32768, 'timeout_seconds': 300,
          'monetary_total': None, 'monetary_reason': 'subscription_allocation_invoice_and_host_cost_not_observed',
          'price_basis': 'subscription_no_overage', 'windows': {}}
for label, directory, project in WINDOWS:
    root = BASE / directory
    run_rows = rows(project, 'select run_id,session_key,status,error_type,started_at,finished_at from runs order by started_at')
    attempts = rows(project, 'select operation_id,run_id,input_allowance,output_allowance,actual_input_tokens,actual_output_tokens,created_at,observed_at from execution_model_attempts order by created_at')
    tools = rows(project, 'select tool_call_id,run_id,tool_name,args,status,result,started_at,finished_at from tool_calls order by started_at')
    finals = rows(project, 'select run_id,content,verdict,answer_state from run_final_results order by created_at')
    schemas = rows(project, "select run_id,details_json,created_at from task_activity where activity_type='tools_loaded' order by created_at")
    effects = rows(project, "select initial_binding->>'run_id' as run_id,initial_binding->>'tool_name' as tool_name,state,dispatch_outcome from action_effects order by created_at")
    charges = rows(project, 'select operation_id,request_json->\'charge\' as charge from execution_budget_reservations order by created_at')
    complete = json.loads((root / 'reports/complete-usage.json').read_text())
    by_op = {r['operation_id']: r for r in complete}
    cases = []
    for path in sorted((root / 'reports').glob('*-result.json')):
        result = json.loads(path.read_text())
        if not result.get('input'):
            continue
        run_id = result['input']['run_id']
        calls = [t for t in tools if t['run_id'] == run_id]
        observed = [a for a in attempts if a['run_id'] == run_id]
        final = next((f for f in finals if f['run_id'] == run_id), None)
        real = [t for t in calls if t['tool_name'].startswith('doctor.')]
        required = result['case']['tools']
        success = result['input']['status'] == 'completed' and final is not None
        success = success and all(any(t['tool_name'] == name and t['status'] == 'completed' for t in calls) for name in required)
        for t in real:
            checks = (t['result'] or {}).get('checks', [])
            success = success and bool(checks) and all(c.get('status') == 'healthy' for c in checks)
        if result['case']['id'] == 'containers':
            success = success and 'healthy' in (final or {}).get('content', '').lower()
        if result['case']['id'] == 'database':
            answer = (final or {}).get('content', '').lower()
            success = success and 'postgres' in answer and 'redis' in answer and 'healthy' in answer
        if result['case']['id'] == 'no-tool':
            success = success and not calls and final['content'].strip() == '391'
        if result['case']['id'] == 'provider':
            # Preserve complete final for independent Russian semantic review.
            success = success and 'не' in (final or {}).get('content', '').lower()
        measured = [by_op[a['operation_id']] for a in observed if a['operation_id'] in by_op]
        usage_complete = bool(observed) and len(measured) == len(observed)
        public_calls = []
        for t in calls:
            public = {k: t[k] for k in ('tool_call_id', 'tool_name', 'args', 'status', 'started_at', 'finished_at')}
            if t['tool_name'].startswith('doctor.'):
                public['checks'] = [{k: c.get(k) for k in ('name', 'status', 'checked_at')} for c in (t['result'] or {}).get('checks', [])]
            else:
                public['receipt'] = t['result']
            public_calls.append(public)
        cases.append({'label': path.name.removesuffix('-result.json'), 'arm': result['arm'], 'case': result['case']['id'],
                      'run_id': run_id, 'input_status': result['input']['status'], 'error_code': result['input']['error_code'],
                      'independent_case_pass': bool(success), 'elapsed_seconds': result['elapsed_seconds'],
                      'model_attempts': len(observed), 'known_input_tokens': sum(a['actual_input_tokens'] or 0 for a in observed),
                      'known_visible_output_tokens': sum(a['actual_output_tokens'] or 0 for a in observed),
                      'complete_native_total_tokens': sum(a['total_tokens'] for a in measured) if usage_complete else None,
                      'complete_usage_coverage': len(measured), 'tool_calls': public_calls, 'final': final,
                      'schema_observations': [s for s in schemas if s['run_id'] == run_id]})
    report['windows'][label] = {'cases': cases, 'attempts': attempts, 'complete_usage': complete, 'effects': effects,
                              'reservations': charges, 'model_attempts': len(attempts),
                              'unknown_input_output_attempts': sum(a['actual_input_tokens'] is None or a['actual_output_tokens'] is None for a in attempts),
                              'unknown_total_attempts': sum(a['operation_id'] not in by_op for a in attempts),
                              'source_archive_sha256': hashlib.sha256((root / 'source.tar.gz').read_bytes()).hexdigest()}

pair = report['windows']['cost_pair']
assert pair['model_attempts'] <= 8
assert len(pair['cases']) == 2
assert all(c['independent_case_pass'] and c['complete_native_total_tokens'] is not None for c in pair['cases'])
eager = next(c for c in pair['cases'] if c['arm'] == 'eager')
deferred = next(c for c in pair['cases'] if c['arm'] == 'deferred')
chain = [t['tool_name'] for t in deferred['tool_calls']]
assert chain == ['tools.search', 'tools.load', 'doctor.check_container']
assert all(t['status'] == 'completed' for t in deferred['tool_calls'])
report['comparison'] = {'full_usage_pair_pass': True, 'native_chain': chain,
                        'eager_total_tokens': eager['complete_native_total_tokens'], 'deferred_total_tokens': deferred['complete_native_total_tokens'],
                        'token_reduction_percent': 100 * (1 - deferred['complete_native_total_tokens'] / eager['complete_native_total_tokens']),
                        'eager_elapsed_seconds': eager['elapsed_seconds'], 'deferred_elapsed_seconds': deferred['elapsed_seconds'],
                        'latency_ratio': deferred['elapsed_seconds'] / eager['elapsed_seconds'],
                        'full_quality_noninferiority': 'not_established', 'distinct_tasks': 4,
                        'initial_v4': {arm: {'passed': sum(c['independent_case_pass'] for c in report['windows']['initial']['cases'] if c['label'].startswith('v4-') and c['arm'] == arm),
                                            'total': sum(c['label'].startswith('v4-') and c['arm'] == arm for c in report['windows']['initial']['cases'])}
                                       for arm in ('eager', 'deferred')}}
report['cumulative_model_attempts'] = sum(w['model_attempts'] for w in report['windows'].values())
output = BASE / 'tool-discovery-cost-pair-20261019/reports/comparison.json'
output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
output.chmod(0o600)
print(json.dumps(report['comparison']))
