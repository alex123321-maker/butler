#!/usr/bin/env python3
"""Close the original whole92 denominator after exited-process auth handback.

Stdlib evidence glue only. Never reads PG settings/private auth, invokes a model,
grades semantics, rewrites the preseal or turns mechanical success into GO.
"""
import hashlib
import json
import os
import pathlib
import re
import sys


def data(path):
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 512*1024*1024:
        raise ValueError('bounded regular immutable evidence required')
    return path.read_bytes()


def read(path):
    return json.loads(data(path))


def write(path, value):
    raw = json.dumps(value, ensure_ascii=False, indent=2).encode()+b'\n'
    with os.fdopen(os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), 'wb') as target:
        target.write(raw)
        target.flush()
        os.fsync(target.fileno())


def reference(root, path):
    return {'path': path.relative_to(root).as_posix(), 'sha256': hashlib.sha256(data(path)).hexdigest()}


def optional(root, name):
    path = root/name
    return read(path) if path.exists() else None


def safe_request(request, kind):
    allowed = ('Context', 'InputItems', 'ToolDefinitions', 'StreamingEnabled') if kind == 'start' else ('RunID', 'ToolCallRef', 'ToolResultJSON', 'InputItems')
    projected = {key: request[key] for key in allowed if key in request}
    if 'Context' in projected:
        context = projected['Context']
        projected['Context'] = {key: context[key] for key in ('RunID', 'ProviderName', 'ModelName', 'SupportsStreaming', 'SupportsToolCalls', 'SupportsStatefulResume') if key in context}
    if 'InputItems' in projected:
        items = []
        for item in projected['InputItems']:
            kept = {key: item[key] for key in ('Role', 'Content', 'ContentType', 'Name') if key in item}
            origin = (item.get('Metadata') or {}).get('butler_provenance')
            if origin is not None:
                kept['Metadata'] = {'butler_provenance': {key: origin[key] for key in ('kind', 'source_refs', 'parent_refs', 'sensitivity') if key in origin}}
            items.append(kept)
        projected['InputItems'] = items
    return projected


def complete_handback(handback):
    process, recovery, fence = handback.get('collector_exit') or {}, handback.get('recovery') or {}, handback.get('recovery_fence') or {}
    returned, probe, snapshot = handback.get('return') or {}, handback.get('main_control_plane') or {}, handback.get('main_snapshot') or {}
    trace = probe.get('trace')
    if not isinstance(trace,list) or not all(isinstance(event,dict) for event in trace):
        return False
    paid = [event for event in trace if event.get('stage') == 'paid_discovery'] if isinstance(trace, list) else []
    completed200 = any(event.get('outcome') == 'completed' and event.get('http_status') == 200 for event in paid)
    validpaid = all(event.get('outcome') == 'started' and (event.get('http_status') or 0) == 0 or event.get('outcome') in ('response_decoded', 'completed') and event.get('http_status') == 200 for event in paid)
    original = 'latest_original_owning_durable_encrypted_setting_after_process_exit'
    return bool(handback['native_status'] == handback['handback_status'] == 0 and process.get('Running') is False and process.get('Status') in ('exited', 'dead') and recovery.get('opaque_return_ready') is True and recovery.get('source') == original and recovery.get('raw_material_printed') is False and fence.get('collector_running') is False and fence.get('original_database') == 'coherent_native_test' and fence.get('restored_auth_ignored') is True and fence.get('source') == original and returned.get('opaque_main_cas') is True and returned.get('active_work_checked') is True and probe.get('resolved') is True and probe.get('unsafe_refresh') is False and probe.get('expired') is False and completed200 and validpaid and snapshot.get('fresh_main_snapshot_saved') is True)


def native_usage(entries, schedule):
    counts = {slot['name']: 0 for slot in schedule}
    usage = {'native_entries': len(entries), 'native_known_usage': 0, 'native_unknown_usage': 0, 'native_missing_total': 0, 'known_full_token_lower_bound': 0}
    operations, reservations = set(), set()
    for sequence, entry in enumerate(entries, 1):
        if entry['sequence'] != sequence or entry['slot'] not in counts or not entry['operation_id'] or not entry['reservation_id'] or entry['operation_id'] in operations or entry['reservation_id'] in reservations:
            raise ValueError('native sequence/slot/operation/reservation differs')
        operations.add(entry['operation_id']); reservations.add(entry['reservation_id'])
        counts[entry['slot']] += 1
        value = entry.get('usage')
        if value is None:
            usage['native_unknown_usage'] += 1
        else:
            if any(type(value.get(key)) is not int or value[key] < 0 for key in ('InputTokens', 'OutputTokens', 'TotalTokens')):
                raise ValueError('native observed usage malformed')
            usage['native_known_usage'] += 1
            if value['TotalTokens'] == 0:
                usage['native_missing_total'] += 1
            else:
                usage['known_full_token_lower_bound'] += value['TotalTokens']
    if any(counts[slot['name']] > slot['maximum_entries'] for slot in schedule):
        raise ValueError('per-slot method population exceeded')
    return usage, counts


def close(proof, expected):
    seal = read(proof/'preseal.json')
    if reference(proof, proof/'preseal.json')['sha256'] != expected or seal['format'] != 'butler.coherent.native-preseal.v1' or len(seal['schedule']) != 15 or sum(slot['maximum_entries'] for slot in seal['schedule']) != 75 or len(seal['ordered_checkpoints']) != 25 or seal['controlled_product_admissions_maximum'] != 5:
        raise ValueError('original full15/25/75+5 preseal required')
    window = optional(proof, 'native-window-final.json')
    entries = window.get('entries') or [] if window is not None else []
    if window is not None and (len(window['slots']) != 15 or len(entries) > 75):
        raise ValueError('original native window denominator changed')
    measured, counts = native_usage(entries, seal['schedule'])
    outcomes = []
    for index, slot in enumerate(seal['schedule']):
        start, end = proof/('slot-%02d-start.json' % (index+1)), proof/('slot-%02d-end.json' % (index+1))
        if window is not None:
            outcome = window['slots'][index]
            if outcome['name'] != slot['name'] or outcome['maximum_entries'] != slot['maximum_entries']:
                raise ValueError('original slot identity changed')
            state = outcome['state']
        elif end.exists():
            state = read(end)['state']
        else:
            state = 'unknown_collector_terminated' if start.exists() else 'not_executed'
        outcomes.append({'sequence': index+1, 'name': slot['name'], 'maximum_entries': slot['maximum_entries'], 'state': state})
    write(proof/'all-original-outcomes.json', {'denominator': 15, 'mechanical_checkpoints': 25, 'slots': outcomes, 'original_final_window_present': window is not None})
    costs = optional(proof, 'full-quoted-costs.json')
    outcome = optional(proof, 'native-outcome.json')
    timeline = optional(proof, 'timeline/timeline-lifecycle.json')
    cost_complete = costs is not None and outcome is not None and outcome.get('cost_population_complete') is True
    if cost_complete:
        new = costs['new_usage']
        if new['native_entries'] != len(entries) or new['native_known_usage']+new['native_unknown_usage'] != len(entries) or new['product_attempts'] != new['native_entries']+new['product_without_native_entry'] or not 0 <= new['product_without_native_entry'] <= 5 or costs['inclusive_quote']['model_turns'] != costs['inclusive_usage']['product_attempts']:
            raise ValueError('native/product/controlled/carry populations differ')
        if any(new[key] != value for key, value in measured.items()):
            raise ValueError('native original entries/full observed totals differ from Go cost projection')
    handback = {key: optional(proof, filename) for key, filename in [('collector_exit', 'collector-exit-safe.json'), ('recovery', 'recovery-safe.json'), ('recovery_fence', 'auth-recovery-safe.json'), ('return', 'return-safe.json'), ('main_control_plane', 'main-auth-safe.json'), ('main_snapshot', 'main-snapshot-safe.json')]}
    handback.update(native_status=int((proof/'native-status').read_text()), handback_status=int((proof/'handback-status').read_text()))
    write(proof/'closed-handback.json', handback)
    mechanical = bool(cost_complete and window is not None and not window['capture_error'] and not window['whole_failed'] and window['mechanics_tail_reached'] and all(slot['state'] == 'completed' and counts[slot['name']] > 0 for slot in outcomes) and outcome.get('mechanical_pass') is True and timeline is not None and timeline.get('pass') is True and timeline.get('native_calls') == len(entries) and [step['Name'] for step in timeline['steps']] == seal['ordered_checkpoints'] and new['product_without_native_entry'] == 5 and complete_handback(handback))
    # Visible input/output and exact tool results only. Private original capture
    # remains hashed independently; session/auth/transport option metadata is not
    # needed by semantic reviewers and is excluded from this projection.
    safe_entries = []
    for entry in entries:
        safe_entries.append({**{key: value for key, value in entry.items() if key != 'request'}, 'request': safe_request(entry['request'],entry['kind'])})
    write(proof/'native-review-safe.json', {'format': 'butler.coherent.native-review-safe.v1', 'preseal_sha256': expected, 'denominator': 15, 'mechanical_checkpoints': 25, 'slots': outcomes, 'entries': safe_entries, 'full_costs': costs, 'quality_review_required': True, 'semantic_qualified': False, 'reasoning_auth_transport_metadata_excluded': True})
    paths = [path for path in sorted(proof.rglob('*')) if path.is_file() and not any(part in ('postgres', 'bin', 'history') for part in path.relative_to(proof).parts)]
    write(proof/'private-artifacts.json', {'format': 'butler.coherent.private-artifacts.v1', 'files': [reference(proof, path) for path in paths]})
    packet = {'format': 'butler.coherent.closed-whole.v1', 'owner': seal['owner'], 'day': seal['history']['day'], 'deadline': seal['deadline'], 'preseal_sha256': expected, 'previous_history_sha256': seal['history']['sha256'], 'denominator': 15, 'mechanical_checkpoints': 25, 'maximum_native_entries': 75, 'maximum_controlled_product_admissions': 5, 'mechanical_pass': mechanical, 'cost_population_complete': cost_complete, 'usage': {'new': costs['new_usage'], 'inclusive': costs['inclusive_usage']} if cost_complete else None, 'inclusive_carry': costs['inclusive_quote'] if cost_complete else None, 'references': [reference(proof, proof/name) for name in ['preseal.json', 'scheduled-before-auth.json', 'history/history.json', 'all-original-outcomes.json', 'closed-handback.json', 'private-artifacts.json', 'native-review-safe.json']], 'release_qualified': False, 'independent_all15_semantic_review_required': True, 'invoice_cost': 'unknown', 'control_plane_probe_is_not_inference': True}
    write(proof/'closed-whole-metadata.json', packet)
    return packet


if __name__ == '__main__':
    try:
        base, label, expected = pathlib.Path(sys.argv[1]).resolve(), sys.argv[2], sys.argv[3]
        if not re.fullmatch('/srv/storage/butler/agents/issue92-native-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch('[a-z0-9-]+', label) or not re.fullmatch('[a-f0-9]{64}', expected):
            raise ValueError('exact isolated closure identity required')
        result = close(base/'reports'/label, expected)
        print(json.dumps({'closed_receipt_written': True, 'mechanical_pass': result['mechanical_pass'], 'cost_population_complete': result['cost_population_complete'], 'release_qualified': False}))
    except (OSError, KeyError, TypeError, ValueError):
        raise SystemExit('native92 closure incomplete; retain all original rows and evidence')
