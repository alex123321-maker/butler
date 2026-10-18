#!/usr/bin/env python3
"""Read-only whole92 currentness receipt; stdlib lab Docker/JSON glue only.

Copies immutable closed90 references, compares actual allocated durable rows,
and freezes seven-store/all-owner inventory before build and under auth flock.
Never queries settings/auth and never prints provider/source bodies or SQL errors.
"""
import datetime
import hashlib
import glob
import importlib.util
import json
import os
import pathlib
import re
import subprocess
import sys

sys.dont_write_bytecode = True
SCRIPTS = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('coherent_history_util', SCRIPTS/'memory-temporal-successor-history.py')
util = importlib.util.module_from_spec(spec)
spec.loader.exec_module(util)
TABLES = {'execution_budget_scopes', 'execution_budget_reservations', 'execution_model_attempts', 'memory_model_attempts', 'memory_aux_jobs', 'memory_model_policies', 'tasks', 'runs', 'accepted_inputs', 'action_effects', 'action_effect_evidence', 'action_effect_dispatches', 'tool_calls', 'run_final_results', 'memory_experience_lessons', 'skill_catalog', 'skill_versions', 'skill_states', 'experience_native_source_history'}
GLOBAL = {'experience_native_windows', 'experience_native_allocations', 'experience_native_entries', 'experience_native_templates', 'experience_native_case_databases', 'experience_native_history'}


def deny(message):
    raise SystemExit(message)


def raw(path):
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 512*1024*1024:
        deny('bounded regular immutable history artifact required')
    return path.read_bytes()


def frozen(path, sha):
    value = raw(path)
    if not re.fullmatch('[a-f0-9]{64}', sha) or hashlib.sha256(value).hexdigest() != sha:
        deny('exact history artifact changed')
    return value


def write(path, value):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'wb') as target:
        target.write(value)
        target.flush()
        os.fsync(target.fileno())


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=False).encode()


def relation(value):
    # Only a relation's top-level row order is immaterial. Nested arrays in
    # actual requests/actions are ordered evidence and are never normalized.
    return sorted((canonical(row) for row in value))


def reference(root, ref):
    if set(ref) != {'path', 'sha256'} or not isinstance(ref['path'], str):
        deny('typed exact artifact reference required')
    rel = pathlib.PurePosixPath(ref['path'])
    if rel.is_absolute() or '..' in rel.parts or len(rel.parts) > 20:
        deny('contained artifact reference required')
    target = root.joinpath(*rel.parts)
    cursor = root
    for part in rel.parts:
        cursor = cursor/part
        if cursor.is_symlink():
            deny('symlink history path denied')
    return target, frozen(target, ref['sha256'])


def walk_references(value):
    if isinstance(value, dict):
        if set(value) == {'path', 'sha256'}:
            yield value
        else:
            for child in value.values():
                yield from walk_references(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk_references(child)


def copy_closed(source, sha, destination):
    pending = [(source, pathlib.PurePosixPath('history.json'), sha)]
    copied, total = {}, 0
    while pending:
        path, name, digest = pending.pop()
        value = frozen(path, digest)
        if str(name) in copied:
            if copied[str(name)] != digest:
                deny('recursive history identity collision')
            continue
        total += len(value)
        if len(copied) >= 20000 or total > 4*1024*1024*1024:
            deny('bounded whole history inventory exceeded')
        write(destination.joinpath(*name.parts), value)
        copied[str(name)] = digest
        if path.suffix == '.json':
            for ref in walk_references(json.loads(value)):
                target, _ = reference(path.parent, ref)
                nested = name.parent/pathlib.PurePosixPath(ref['path'])
                pending.append((target, nested, ref['sha256']))
    return copied


def sql(database, query):
    if not re.fullmatch(r'(?:memory_read_test|experience90_[a-f0-9]{12}_(?:source|case_[0-9]{3})_test)', database):
        deny('exact closed90 owning database required')
    result = subprocess.run(['docker', 'exec', '-i', 'issue86-20261016-pg', 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'butler', '-d', database], input='BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;'+query+';COMMIT;', text=True, capture_output=True, timeout=45)
    if result.returncode:
        deny('closed90 read-only durable comparison unavailable')
    try:
        return json.loads(result.stdout)
    except ValueError:
        deny('closed90 durable projection malformed')


def packet_files(root, packet):
    files = {}
    for ref in packet['references']:
        _, value = reference(root, ref)
        if ref['path'] in files:
            deny('duplicate closed90 root reference')
        files[ref['path']] = json.loads(value) if ref['path'].endswith('.json') else value
    return files


def current90(root, packet):
    files = packet_files(root, packet)
    expected = files['actual/global-journal-final.json']
    if set(expected) != GLOBAL:
        deny('complete closed90 independent journal required')
    window = packet['preseal_sha256']
    if not re.fullmatch('[a-f0-9]{64}', window):
        deny('closed90 exact window identity required')
    expression = ','.join("'%s',(SELECT coalesce(jsonb_agg(to_jsonb(t)),'[]') FROM %s t WHERE window_id='%s')" % (table, table, window) for table in sorted(GLOBAL))
    actual = sql('memory_read_test', 'SET LOCAL search_path=experience_native_v1,public;SELECT jsonb_build_object('+expression+')')
    if any(relation(expected[key]) != relation(actual[key]) for key in GLOBAL):
        deny('closed90 independent durable journal changed')
    if len(actual['experience_native_windows']) != 1 or actual['experience_native_windows'][0]['closed'] is not True:
        deny('closed90 independent window is not closed')
    plan = files['plan/preseal.json']
    by_id = {item['id']: item['sequence'] for item in plan['schedule']}
    templates = actual['experience_native_templates']
    if len(templates) != 1 or templates[0]['state'] != 'frozen':
        deny('closed90 source is not an immutable frozen template')
    template = templates[0]['database_name']
    if not re.fullmatch(r'experience90_[a-f0-9]{12}_source_test', template):
        deny('closed90 source identity differs')
    source_state = sql('memory_read_test', "SELECT jsonb_build_object('template',(SELECT jsonb_build_object('name',datname,'template',datistemplate,'connections_allowed',datallowconn) FROM pg_database WHERE datname='%s'),'active',(SELECT count(*) FROM pg_stat_activity WHERE datname='%s'))" % (template, template))
    if source_state != {'template': {'name': template, 'template': True, 'connections_allowed': False}, 'active': 0}:
        deny('closed90 template became writable or has live readers')
    checked = 0
    costs = files['actual/unique-quoted-costs.json']
    exported = set(costs['allocated_database_exports'])
    if 'source-database.json' not in exported:
        deny('closed90 source export missing from union')
    for allocation in actual['experience_native_case_databases']:
        name = allocation['database_name']
        sequence = by_id.get(allocation['slot_id'])
        filename = 'case-%03d-database.json' % sequence if sequence is not None else ''
        if allocation['state'] != 'created' or filename not in exported or not re.fullmatch(r'experience90_[a-f0-9]{12}_case_[0-9]{3}_test', name):
            deny('closed90 exact allocated case population incomplete')
        expected_database = json.loads(raw(root/'actual'/filename))
        if set(expected_database) != TABLES:
            deny('complete closed90 private case export required')
        expression = ','.join("'%s',(SELECT coalesce(jsonb_agg(to_jsonb(t)),'[]') FROM %s t)" % (table, table) for table in sorted(TABLES))
        fresh = sql(name, 'SET LOCAL search_path=public;SELECT jsonb_build_object('+expression+')')
        if any(relation(fresh[table]) != relation(expected_database[table]) for table in TABLES):
            deny('closed90 actual allocated durable case changed')
        checked += 1
    if checked+1 != len(exported):
        deny('closed90 allocated/exported source-plus-case denominator differs')
    # Source connections are disabled by its immutable reset protocol. Do not
    # mutate that setting to reread it. Original source rows are rehashed and
    # recursively corroborated by the Go validator and copied case prefixes.
    return {'global_sha256': hashlib.sha256(canonical(actual)).hexdigest(), 'allocated_cases_checked': checked, 'source_template': source_state, 'source_rows_directly_requeried': False}


def nearest_core_inventory(root):
    pending, seen = [(root/'history/history.json', 0)], set()
    candidates, best = [], None
    while pending:
        path, depth = pending.pop(0)
        if path in seen or best is not None and depth > best:
            continue
        seen.add(path)
        data = json.loads(raw(path))
        if data.get('format') == 'butler.consolidation.current-owner-day.v2':
            candidates.append(data)
            best = depth
            continue
        for ref in walk_references(data):
            target, _ = reference(path.parent, ref)
            if target.suffix == '.json':
                pending.append((target, depth+1))
    if not candidates or any(canonical(value) != canonical(candidates[0]) for value in candidates):
        deny('unambiguous latest predecessor seven-store inventory unavailable')
    return candidates[0]


def capture(root, packet):
    day = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d')
    if packet.get('format') != 'butler.experience.closed-owner-leaf.v1' or packet.get('day') != day or packet.get('cost_population_complete') is not True:
        deny('same-day complete latest closed90 packet required')
    expected_core = nearest_core_inventory(root)
    core = util.current(day)
    # The90 global schema contains no product budgets; task/case charges have
    # separately allocated databases checked above. Any new charged root window
    # is rejected rather than absorbed into a posthoc larger carry.
    for database in core['databases']:
        retained = []
        for window in database['windows']:
            if database['container'] == 'issue86-20261016-pg' and window['schema'] == 'experience_native_v1':
                if any(window.get(key) for key in ('scopes', 'reservations', 'foreground', 'background')):
                    deny('unexpected product charge in closed90 global schema')
            else:
                retained.append(window)
        database['windows'] = retained
    if canonical(core) != canonical(expected_core):
        deny('root seven-store current charges differ from latest closed90 predecessor')
    return {'format': 'butler.coherent.currentness.v1', 'day': day, 'core': core, 'all_owners': util.all_owners(day), 'closed90': current90(root, packet)}


def main():
    os.umask(0o077)
    # This recipe is the first successor of CLOSED90, not a successor of a
    # failed92 window. A fresh label/base must never erase prior92 charges.
    # No such successor validator is implemented here; require explicit review.
    if glob.glob('/srv/storage/butler/agents/issue92-native-*/reports/*/auth-stage-begun.json'):
        deny('prior native92 auth window exists; CLOSED92 successor carry required before any new preparation/auth')
    mode = sys.argv[1]
    if mode == 'prepare':
        base, label, previous, sha = pathlib.Path(sys.argv[2]).resolve(), sys.argv[3], pathlib.Path(sys.argv[4]).resolve(), sys.argv[5]
        if not re.fullmatch('/srv/storage/butler/agents/issue92-native-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch('[a-z0-9-]+', label) or not re.fullmatch('/srv/storage/butler/agents/issue90-[A-Za-z0-9_-]+/reports/[a-z0-9-]+/closed-leaf-metadata.json', str(previous)):
            deny('exact isolated92 target and closed90 source required')
        proof = base/'reports'/label
        history = proof/'history'
        history.mkdir(mode=0o700, parents=True)
        copy_closed(previous, sha, history)
        packet = json.loads(frozen(history/'history.json', sha))
        current = capture(history, packet)
        write(proof/'current-before-build.json', canonical(current))
        print(json.dumps({'read_only': True, 'history_sha256': sha, 'current_unchanged': True, 'allocated_cases_checked': current['closed90']['allocated_cases_checked'], 'auth_or_inference': False}))
    elif mode == 'check-current':
        path, sha = pathlib.Path(sys.argv[2]).resolve(), sys.argv[3]
        if not re.fullmatch('/srv/storage/butler/agents/issue92-native-[A-Za-z0-9_-]+/reports/[a-z0-9-]+/history/history.json', str(path)):
            deny('exact prepared92 history path required')
        packet = json.loads(frozen(path, sha))
        current = capture(path.parent, packet)
        if canonical(current) != raw(path.parent.parent/'current-before-build.json'):
            deny('current owner/all-owner/closed90 durable population changed after freeze')
        print(json.dumps({'read_only': True, 'current_unchanged': True, 'allocated_cases_checked': current['closed90']['allocated_cases_checked'], 'auth_or_inference': False}))
    else:
        deny('prepare or check-current required')


if __name__ == '__main__':
    try:
        main()
    except (OSError, KeyError, TypeError, ValueError, subprocess.TimeoutExpired):
        deny('currentness proof unavailable; retain failed setup, do not import auth')
