#!/usr/bin/env python3
"""Content-free lab closure and exact private artifact references, no auth.

This is a companion receipt, not a replacement for original PG/transport rows
or independent semantic review. Python stdlib only; model text stays private.
"""
import hashlib
import json
import os
import pathlib
import re
import sys


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'))


def write(path, value):
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as out:
        out.write(json.dumps(value, indent=2)+'\n')
        out.flush()
        os.fsync(out.fileno())


def reference(root, path):
    if not path.is_file() or path.is_symlink() or path.stat().st_size > 512*1024*1024:
        raise ValueError('bounded regular closure artifact required')
    digest = hashlib.sha256()
    with path.open('rb') as src:
        for block in iter(lambda: src.read(1024*1024), b''):
            digest.update(block)
    return {'path': str(path.relative_to(root)), 'sha256': digest.hexdigest()}


def read(path):
    if path.is_symlink() or path.stat().st_size > 64*1024*1024:
        raise ValueError('bounded regular private JSON required')
    return json.loads(path.read_bytes())


def product_population(actual, costs, entries):
    records = {}
    for name in costs['allocated_database_exports']:
        if not re.fullmatch(r'(?:source|case-0[1-5][0-9])-database.json', name) or '/' in name:
            raise ValueError('exact allocated database export identity required')
        database = read(actual/name)
        for table in ('execution_model_attempts', 'memory_model_attempts'):
            for row in database[table]:
                operation = row['operation_id']
                value = (table, canonical(row))
                if operation in records and records[operation] != value:
                    raise ValueError('cloned product identity changed')
                records[operation] = value
    observed, seen = 0, set()
    for _, (table, raw) in records.items():
        row = json.loads(raw)
        observed += row.get('observed_at') is not None and (row.get('usage') is not None if table == 'memory_model_attempts' else row.get('actual_input_tokens') is not None and row.get('actual_output_tokens') is not None)
    for entry in entries:
        operation = entry['operation_id']
        if operation in seen or operation not in records:
            raise ValueError('native entry lacks unique owning product admission')
        seen.add(operation)
        table, raw = records[operation]
        row = json.loads(raw)
        if entry['reservation_id'] != row['reservation_id'] or entry['run_id'] != (operation if table == 'memory_model_attempts' else row['run_id']):
            raise ValueError('native entry product reservation/run differs')
    if len(records) != costs['quotes']['new_model_reservation_rows']:
        raise ValueError('actual product population differs from unique new model quotes')
    return len(records), observed


def summarize(proof, expected):
    plan = read(proof/'plan/preseal.json')
    if reference(proof, proof/'plan/preseal.json')['sha256'] != expected or len(plan['schedule']) != 57 or sum(x['maximum_provider_entries'] for x in plan['schedule']) != 640:
        raise ValueError('exact full57/640 preseal required')
    actual = proof/'actual'
    refs = [reference(proof, path) for path in sorted(actual.rglob('*')) if path.is_file()] if actual.is_dir() else []
    write(proof/'private-artifacts.json', {'format': 'butler.experience.private-artifacts.v1', 'files': refs})
    window_path = actual/'window-final.json'
    window = read(window_path) if window_path.exists() else None
    if window is not None and (len(window['slots']) != 57 or len(window['entries'] or []) > 640):
        raise ValueError('original whole-window population changed')
    # Missing end/final evidence remains explicitly uncertain after an abrupt
    # process termination. An already started slot is never called unexecuted.
    outcomes = []
    for slot in plan['schedule']:
        sequence = slot['sequence']
        end, start = actual/('slot-%03d-end.json' % sequence), actual/('slot-%03d-start.json' % sequence)
        if window is not None:
            outcome = window['slots'][sequence-1]
            if outcome['id'] != slot['id']:
                raise ValueError('original slot identity changed')
            status = outcome['status']
        elif end.exists():
            status = read(end)['status']
        else:
            status = 'unknown_collector_terminated' if start.exists() else 'not_executed'
        outcomes.append({'sequence': sequence, 'id': slot['id'], 'kind': slot['kind'], 'case_id': slot['case_id'], 'arm': slot['arm'], 'repetition': slot['repetition'], 'status': status})
    write(proof/'all-original-outcomes.json', {'slots': outcomes, 'denominator': 57, 'original_final_window_present': window is not None, 'status_semantics': 'Collector stage status; task/run/effect outcomes and UNKNOWN/no-final originals remain in owning rows. Completed stages are not task-success counts.'})
    usage, carry, complete = None, None, False
    costs_path = actual/'unique-quoted-costs.json'
    if window is not None and costs_path.exists():
        costs = read(costs_path)
        entries = window['entries'] or []
        products, observed = product_population(actual, costs, entries)
        new = {'native_entries': len(entries), 'native_known_usage': 0, 'native_unknown_usage': 0, 'native_missing_total': 0, 'known_full_token_lower_bound': 0, 'product_attempts': products, 'product_observed': observed, 'product_null': products-observed, 'product_without_native_entry': products-len(entries)}
        for entry in entries:
            value = entry.get('usage')
            if value is None:
                new['native_unknown_usage'] += 1
            else:
                new['native_known_usage'] += 1
                total = value['TotalTokens']
                if total <= 0:
                    new['native_missing_total'] += 1
                else:
                    new['known_full_token_lower_bound'] += total
        usage = {'new': new, 'inclusive': {key: plan['history']['inclusive_usage'][key]+value for key, value in new.items()}}
        carry = costs['quotes']['inclusive_quote']
        if carry['model_turns'] != usage['inclusive']['product_attempts']:
            raise ValueError('inclusive quote/independent product admission count differs')
        complete = True
    safe = {}
    for name, key in [('return-safe.json', 'return_safe'), ('main-auth-safe.json', 'main_auth_safe'), ('main-snapshot-safe.json', 'main_snapshot_safe')]:
        path = proof/name
        safe[key] = read(path) if path.exists() else None
    handback = {**safe, 'native_status': int((proof/'native-status').read_text()), 'handback_status': int((proof/'handback-status').read_text())}
    write(proof/'closed-handback.json', handback)
    paths = [proof/'plan/preseal.json', proof/'plan/schedule.json', proof/'history/history.json', proof/'private-artifacts.json', proof/'all-original-outcomes.json', proof/'closed-handback.json', proof/'runner.jsonl']
    if costs_path.exists():
        paths.append(costs_path)
    if (actual/'global-journal-final.json').exists():
        paths.append(actual/'global-journal-final.json')
    if (actual/'domain-claim.json').exists():
        paths.append(actual/'domain-claim.json')
    packet = {'format': 'butler.experience.closed-owner-leaf.v1', 'owner': plan['owner'], 'day': plan['history']['day'], 'deadline': plan['deadline'], 'preseal_sha256': expected, 'previous_history_sha256': plan['history']['sha256'], 'denominator': 57, 'maximum_new_entries': 640, 'cost_population_complete': complete, 'inclusive_carry': carry, 'usage': usage, 'references': [reference(proof, path) for path in paths], 'release_qualified': False, 'independent_quality_review_required': True, 'invoice_cost': 'unknown', 'control_plane_handback_is_inference': False}
    write(proof/'closed-leaf-metadata.json', packet)
    return packet


if __name__ == '__main__':
    base, label, expected = pathlib.Path(sys.argv[1]).resolve(), sys.argv[2], sys.argv[3]
    if not re.fullmatch(r'/srv/storage/butler/agents/issue90-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch(r'[a-z0-9-]+', label) or not re.fullmatch(r'[a-f0-9]{64}', expected):
        raise SystemExit('exact isolated native closure reference required')
    try:
        result = summarize(base/'reports'/label, expected)
    except (OSError, ValueError, KeyError, TypeError):
        raise SystemExit('native closure incomplete; preserve all original rows and private evidence')
    print(json.dumps({'closed_receipt_written': True, 'cost_population_complete': result['cost_population_complete'], 'release_qualified': False}))
