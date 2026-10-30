#!/usr/bin/env python3
"""Read-only stdlib reporting of actual Core/Broker research traces.

Python is artifact analysis only, never a provider or product execution runtime.
All times are wall durations, not CPU or inferred hidden reasoning durations.
"""
import argparse
import collections
import csv
import decimal
import hashlib
import json
import pathlib
import runpy


def read(path):
    return json.loads(path.read_text())


def physical_artifact(slot, snapshot, case):
    if case['condition'] != 'prepared_artifact':
        return None
    binding = read(slot/'binding.json')
    digest = hashlib.sha256(json.dumps(snapshot['RecipientFiles'], sort_keys=True,
        separators=(',', ':'), ensure_ascii=False).encode()).hexdigest()
    if digest != binding['OriginalFilesSHA256']:
        return False
    calls = snapshot['Tables'].get('calls', [])
    prepared = [x for x in calls if x['tool_name'] == 'extension.project.prepare-patch']
    checks = [x for x in calls if x['tool_name'] == 'extension.project.check' and x['status'] == 'completed']
    if len(prepared) != 1 or prepared[0]['status'] != 'completed':
        return False
    try:
        receipt = json.loads(prepared[0]['result']['body'])
        proposal = prepared[0]['args']
        if isinstance(proposal, str):
            proposal = json.loads(proposal)
        proposal = proposal['body']
        # This is a separate physical measure, not repair of a model final.
        # The synthetic recipient rejects incomplete/wrong migrations and checks
        # only its current immutable, exact-content artifact.
        return any((b := json.loads(x['result']['body'])).get('checks_passed') is True
            and b.get('applied') is False and b.get('artifact_id') == receipt['artifact_id']
            and b.get('project_revision') == proposal['project_revision']
            and b.get('prepared_content') == proposal['changes'][0]['after']
            for x in checks)
    except (KeyError, ValueError, TypeError):
        return False


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=pathlib.Path)
    parser.add_argument('output', type=pathlib.Path)
    args = parser.parse_args()
    root = args.root
    plan = read(root/'out/plan/plan.json')
    suite = read(root/'out/plan/suite.private.json')
    basic = read(root/'public-report.json')
    assert plan['Protocol'] == 'skill-workflow-v1' and len(plan['Slots']) == 36
    assert basic['scheduled'] == 36
    pricing = runpy.run_path(str(pathlib.Path(__file__).with_name('memory-utility-provider-comparison.py')))['cost']
    rows = []
    method_rows = []
    for row, scheduled in zip(basic['rows'], plan['Slots']):
        assert row['sequence'] == scheduled['Sequence']
        slot = root/'out/actual'/('slot-%03d' % row['sequence'])
        case = suite['cases'][scheduled['Case']]
        timing = read(slot/'timing.json') if (slot/'timing.json').exists() else None
        snapshot = read(slot/'snapshot.private.json') if (slot/'snapshot.private.json').exists() else None
        native = [read(p) for p in sorted(slot.glob('native-*.private.json'))]
        calls = len(native)
        for n in native:
            method_rows.append({'model': plan['Model'], 'reasoning_effort': plan['ReasoningEffort'] or 'provider_default_high',
                'sequence': n['sequence'], 'slot': row['sequence'], 'case_id': case['id'], 'arm': row['arm'],
                'turn': n['turn'], 'method': n['kind'], 'dispatched': n['dispatched'],
                'provider_boundary_seconds': n['provider_boundary_ns']/1e9,
                'butler_admission_callback_seconds': n['butler_admission_callback_ns']/1e9,
                'provider_wait_seconds': n['provider_wait_ns']/1e9,
                'known_full_tokens': n['usage']['TotalTokens'] if n['usage'] is not None else None,
                'unknown_usage': n['dispatched'] and n['usage'] is None,
                'error_code': n.get('error_code'),
                'finish_reasons': json.dumps([e['AssistantFinal']['FinishReason'] for e in n['events'] if e.get('AssistantFinal')])})
        low = high = decimal.Decimal(0)
        for n in native:
            if plan['Provider'] == 'openai' and n['dispatched'] and n['usage'] is not None:
                bounds = pricing(n['usage'], plan['Model'])
                low += bounds[0]/1000000
                high += bounds[1]/1000000
        out = {k: row[k] for k in ('sequence', 'arm', 'condition', 'task_success', 'reasons',
            'native_methods_dispatched', 'unknown_usage', 'known_total_tokens', 'native_failures', 'tool_calls')}
        out['case_id'] = case['id']
        out['projection_error'] = row['projection_error']
        out['answer_states'] = [x['answer_state'] for x in snapshot['Tables'].get('finals', [])] if snapshot else []
        out['finish_reasons'] = dict(collections.Counter(e['AssistantFinal']['FinishReason']
            for n in native for e in n['events'] if e.get('AssistantFinal')))
        calls_observed = snapshot['Tables'].get('calls', []) if snapshot else []
        out['completed_skill_loads'] = sum(x['tool_name'] == 'skills.load' and x['status'] == 'completed'
            and isinstance(x.get('result'), dict) and bool(x['result'].get('digest')) for x in calls_observed)
        out['memory_tool_results'] = dict(collections.Counter(x['tool_name'] + ':' + x['status'] + ':'
            + (str(x['result'].get('status', 'unreported')) if isinstance(x.get('result'), dict) else 'unreported')
            for x in calls_observed if x['tool_name'].startswith('memory.')))
        out['opened_memory_results'] = sum(x['tool_name'] == 'memory.open' and x['status'] == 'completed'
            and isinstance(x.get('result'), dict) and x['result'].get('status') == 'ok'
            and bool(x['result'].get('items')) for x in calls_observed)
        out['native_method_attempts'] = calls
        out['physical_artifact_success'] = physical_artifact(slot, snapshot, case) if snapshot is not None else None
        out['known_usage_tariff_usd'] = {'minimum': str(low), 'maximum': str(high)} if plan['Provider'] == 'openai' else None
        out['timing'] = None
        if timing is not None:
            assert timing['version'] == 'wall-phases-v1'
            provider = sum(x['nanoseconds'] for x in timing['spans'] if x['kind'] == 'provider_wait')
            tools = sum(x['nanoseconds'] for x in timing['spans'] if x['kind'] == 'tool_rpc')
            assert provider == sum(n['provider_wait_ns'] for n in native)
            assert len([x for x in timing['spans'] if x['kind'] == 'provider_wait']) == calls
            system = timing['execution_ns'] - provider
            assert 0 <= tools <= system and 0 <= provider <= timing['execution_ns']
            # Full duration includes the fixture and observation phases once.
            summed = timing['preparation_ns'] + timing['execution_ns'] + timing['observer_ns']
            assert abs(timing['total_to_observation_ns']-summed) < 10000000
            by_tool = collections.Counter()
            for span in timing['spans']:
                if span['kind'] == 'tool_rpc':
                    by_tool[span['name']] += span['nanoseconds']/1e9
            out['timing'] = {
                'execution_seconds': timing['execution_ns']/1e9,
                'provider_wait_seconds': provider/1e9,
                'mean_provider_call_seconds': provider/1e9/calls if calls else None,
                'butler_total_seconds': system/1e9,
                'tool_rpc_seconds_within_butler': tools/1e9,
                'butler_other_seconds': (system-tools)/1e9,
                'preparation_seconds': timing['preparation_ns']/1e9,
                'observer_seconds': timing['observer_ns']/1e9,
                'full_phases_seconds': summed/1e9,
                'tool_rpc_seconds_by_name': dict(by_tool),
            }
        rows.append(out)

    def aggregate(items):
        timed = [x for x in items if x['timing'] is not None]
        keys = ('execution_seconds', 'provider_wait_seconds', 'butler_total_seconds',
            'tool_rpc_seconds_within_butler', 'butler_other_seconds', 'preparation_seconds',
            'observer_seconds', 'full_phases_seconds')
        total = {k: sum(x['timing'][k] for x in timed) for k in keys}
        calls = sum(x['native_method_attempts'] for x in timed)
        total['mean_provider_call_seconds'] = total['provider_wait_seconds']/calls if calls else None
        total['native_method_attempts_with_timing'] = calls
        total['timed_cases'] = len(timed)
        total['missing_timing_cases'] = len(items)-len(timed)
        total['scheduled'] = len(items)
        total['task_success'] = sum(x['task_success'] for x in items)
        total['migration_strict_success'] = sum(x['task_success'] for x in items if x['condition']=='prepared_artifact')
        total['migration_scheduled'] = sum(x['condition']=='prepared_artifact' for x in items)
        total['physical_artifact_success'] = sum(x['physical_artifact_success'] is True for x in items)
        total['cases_with_completed_skill_load'] = sum(x['completed_skill_loads'] > 0 for x in items)
        total['completed_skill_loads'] = sum(x['completed_skill_loads'] for x in items)
        total['cases_with_opened_memory'] = sum(x['opened_memory_results'] > 0 for x in items)
        memory_results = collections.Counter()
        for x in items:
            memory_results.update(x['memory_tool_results'])
        total['memory_tool_results'] = dict(memory_results)
        total['max_tokens_finals'] = sum(x['finish_reasons'].get('MAX_TOKENS', 0) + x['finish_reasons'].get('max_output_tokens', 0) for x in items)
        total['native_methods_dispatched'] = sum(x['native_methods_dispatched'] for x in items)
        total['unknown_usage'] = sum(x['unknown_usage'] for x in items)
        total['known_full_tokens'] = sum(x['known_total_tokens'] for x in items)
        total['known_usage_tariff_usd'] = {k: str(sum(decimal.Decimal(x['known_usage_tariff_usd'][k]) for x in items)) for k in ('minimum','maximum')} if plan['Provider']=='openai' else None
        return total

    result = {'protocol': plan['Protocol'], 'source': plan['Source'], 'model': plan['Model'],
        'reasoning_effort': plan['ReasoningEffort'] or 'provider_default_high',
        'suite_sha256_bytes': basic['suite_sha256_bytes'],
        'raw_pg_core_correlated_methods': basic['raw_pg_entries_correlated'],
        'core_missing_attempts': sum(x['core_missing_attempts'] for x in basic['rows']),
        'core_token_mismatches': sum(x['core_token_mismatches'] for x in basic['rows']),
        'all_slots_observed': basic['all_slots_observed'],
        'total': aggregate(rows), 'arms': {a: aggregate([x for x in rows if x['arm']==a]) for a in 'ABC'},
        'rows': rows, 'pure_model_reasoning_seconds': None, 'invoice_cost': None,
        'notes': ['Provider wait includes network, transport adapter and remote processing; measured Butler admission callbacks are excluded.',
            'Butler total includes tool RPC; adding them would double count. This is wall elapsed time, not CPU usage.',
            'Full phases include fixture/skill preparation, execution and observation; later teardown/reporting and authoring labor excluded.',
            'Unknown usage and missing timing remain unknown, including failed calls; totals state their measured denominator.',
            'One public development cohort; physical artifact checks and strict answer scores are distinct. No old result is regraded.']}
    # Go's terminal test duration covers fixture teardown and gaps as well as
    # measured task phases. Do not substitute test2json's postprocessing clock.
    events = [json.loads(x) for x in (root/'native.jsonl').read_text().splitlines()]
    ends = [x for x in events if x.get('Test') == 'TestMemoryUtilityWholeNativeABC'
        and x.get('Action') in ('pass', 'fail') and 'Elapsed' in x]
    result['native_window_wall_seconds'] = ends[0]['Elapsed'] if len(ends) == 1 else None
    result['window_seconds_outside_measured_phases'] = (result['native_window_wall_seconds']
        - result['total']['full_phases_seconds']) if result['native_window_wall_seconds'] is not None else None
    result['reporter_sha256'] = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
    assert len({x['sequence'] for x in method_rows}) == len(method_rows)
    if method_rows:
        method_rows.sort(key=lambda x: x['sequence'])
        with args.output.with_suffix('.calls.csv').open('x', newline='') as f:
            writer = csv.DictWriter(f, fieldnames=list(method_rows[0]))
            writer.writeheader()
            writer.writerows(method_rows)
    with args.output.open('x') as f:
        json.dump(result, f, indent=2)
    print(json.dumps({k: v for k, v in result.items() if k not in ('rows','notes')}))


if __name__ == '__main__':
    main()
