#!/usr/bin/env python3
"""Read-only CLOSED V7 resource lineage for later lab cohorts.

Python stdlib only: bounded Docker pipes and exclusive fsynced JSON copies.
No settings, credentials, provider calls or production admission authority.
"""
import datetime
import hashlib
import importlib.util
import json
import pathlib
import re
import sys

sys.dont_write_bytecode = True
ROOT = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('closed_v7_util', ROOT/'memory-temporal-successor-history.py')
util = importlib.util.module_from_spec(spec)
spec.loader.exec_module(util)
BASE = pathlib.Path('/srv/storage/butler/agents/issue88-native-v7-20261018')
PRIOR = BASE/'reports/native-v7-preseal-v3'
HISTORY = PRIOR/'history'
HISTORY_SHA = '2b8230458e2ae4c1d950fa1de4adc9ab3cdc7aad4c04769d9a03007b3208c95e'
ORIGINALS = {
    'full-ledger.json': 'df418c4dc7631c9219a4b593ff87d0c14d12ee997cc94b55a3cbfbebd4e98a11',
    'all-original-outcomes.json': 'be122ba559417ffff11ec78ced94a56a320821aca842ed146017257df4795539',
    'closed-handback.json': 'da668e875e995cbf1f14f426e0d667f76ed7aee0b59687823a643d518a1177c1',
    'closed-leaf-metadata.json': '7f50800185b05979d65d36734e0fe9e46c53491eefc5d817619e0abe590be019',
}


def sql(query, container='butler-issue85-postgres', database='issue85_native_test', port=18585):
    result = util.subprocess.run(['docker', 'exec', '-i', container, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'butler', '-p', str(port), '-d', database], input='BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;'+query+';COMMIT;', text=True, capture_output=True, timeout=40)
    if result.returncode:
        raise SystemExit('closed V7 read-only metadata unavailable')
    return json.loads(result.stdout)


def predecessor():
    return sql("""SET LOCAL search_path=memory_temporal_native_v7,public;
SELECT jsonb_build_object('window',(SELECT to_jsonb(w) FROM temporal_native_window w WHERE singleton),
'entries',(SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'run_id',run_id,'kind',kind,'finished_at',finished_at,'observation_json',jsonb_build_object('id',observation_json->'id','run_id',observation_json->'run_id','kind',observation_json->'kind','operation_id',observation_json->'operation_id','reservation_id',observation_json->'reservation_id','usage',observation_json->'usage')) ORDER BY id),'[]') FROM temporal_native_entries))""")


def future():
    state = sql("SELECT jsonb_build_object('Container','issue86-20261016-pg','Database','memory_read_test','Schema','experience_native_v1','Window','experience-explicit-v1','Receipt','public.issue90_native_windows','Present',to_regnamespace('experience_native_v1') IS NOT NULL,'Table',to_regclass('public.issue90_native_windows') IS NOT NULL)", 'issue86-20261016-pg', 'memory_read_test', 5432)
    state['Count'] = sql("SELECT count(*) FROM public.issue90_native_windows WHERE window_id='experience-explicit-v1'", 'issue86-20261016-pg', 'memory_read_test', 5432) if state['Table'] else 0
    if state['Present'] is not False or state['Count'] != 0:
        raise SystemExit('future90 namespace or original cohort receipt already exists')
    return state


def originals():
    files = {name: util.frozen(PRIOR/name, sha) for name, sha in ORIGINALS.items()}
    c = json.loads(files['closed-handback.json'])
    main, returned = c['main_auth_safe'], c['return_safe']
    if c.get('native_status') not in (0, 1) or returned.get('opaque_main_cas') is not True or returned.get('active_work_checked') is not True or main.get('resolved') is not True or main.get('unsafe_refresh') is not False or main.get('expired') is not False or c['main_snapshot_safe'].get('fresh_main_snapshot_saved') is not True or not any(t.get('stage') == 'paid_discovery' and t.get('outcome') == 'completed' and t.get('http_status') == 200 for t in main['trace']):
        raise SystemExit('closed V7 exact handback required')
    for filename, field in [('return-safe.json', 'return_safe'), ('main-auth-safe.json', 'main_auth_safe'), ('main-snapshot-safe.json', 'main_snapshot_safe')]:
        if json.loads(util.artifact(PRIOR/filename)) != c[field]:
            raise SystemExit('closed V7 original handback changed')
    if json.loads(util.artifact(PRIOR/'native-status')) != c['native_status']:
        raise SystemExit('original V7 collector status changed')
    return files


def capture(day):
    inventory, all_owners = util.current(day), util.all_owners(day)
    matches = [w for db in inventory['databases'] if db['container'] == 'butler-issue85-postgres' and db['database'] == 'issue85_native_test' for w in db['windows'] if w['schema'] == 'memory_temporal_native_v7']
    if len(matches) != 1:
        raise SystemExit('one exact closed V7 owner-day leaf required')
    prior = predecessor()
    if prior['window'].get('sealed') is not True or prior['window'].get('closed') is not True or len(prior['entries']) != prior['window'].get('paid_entries'):
        raise SystemExit('closed V7 window incomplete')
    prior.update({'format': 'butler.consolidation.predecessor88.v3', **{key: matches[0][key] for key in ('scopes', 'reservations', 'foreground', 'background')}})
    return inventory, all_owners, prior


def main():
    mode = sys.argv[1]
    if mode == 'check-current':
        path = pathlib.Path(sys.argv[2]).resolve()
        if not re.fullmatch('/srv/storage/butler/agents/issue(?:88|90)-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/history/(?:history|closed-v7-history).json', str(path)):
            raise SystemExit('bounded closed V7 history path required')
        raw = path.read_bytes()
        m = json.loads(raw)
        if hashlib.sha256(raw).hexdigest() != sys.argv[3] or m.get('format') != 'butler.memory.closed-owner-leaf.v1' or m['day'] != datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d'):
            raise SystemExit('frozen closed V7 history or UTC day changed')
        values = capture(m['day'])
        for key, value in zip(('current', 'all_owners', 'closed_v7'), values):
            if util.referenced(path.parent, m[key])[1] != util.canonical(value):
                raise SystemExit('closed V7 current population changed; new review required')
        files = originals()
        for key, filename in [('original_v7_ledger', 'full-ledger.json'), ('original_v7_outcomes', 'all-original-outcomes.json'), ('closed_v7_handback', 'closed-handback.json'), ('operator_accounting', 'closed-leaf-metadata.json')]:
            if util.referenced(path.parent, m[key])[1] != files[filename]:
                raise SystemExit('closed V7 original witness changed')
        for ref in m['original_v7_entries']:
            name, data = util.referenced(path.parent, ref)
            if data != util.artifact(PRIOR/name.removeprefix('closed-v7-')):
                raise SystemExit('closed V7 original entry changed')
        if future() != m['future90']:
            raise SystemExit('future90 namespace changed')
        print(json.dumps({'read_only': True, 'seven_databases': 7, 'current_day_unchanged': True, 'auth_or_inference': False}))
        return
    if mode != 'prepare':
        raise SystemExit('prepare or check-current required')
    base, label = pathlib.Path(sys.argv[2]).resolve(), sys.argv[3]
    if not re.fullmatch('/srv/storage/butler/agents/issue(?:88|90)-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch('[A-Za-z0-9_-]+', label):
        raise SystemExit('bounded closed V7 history target required')
    proof = base/'reports'/label/'history'
    proof.mkdir(mode=0o700, parents=True)
    previous = util.frozen(HISTORY/'history.json', HISTORY_SHA)
    m = json.loads(previous)
    day = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d')
    if m['day'] != day:
        raise SystemExit('same-day closed V7 lineage required; historical rollover needs separate review')
    copied, pending = {}, [util.reference('history.json', previous)]
    while pending:
        ref = pending.pop()
        name, data = util.referenced(HISTORY, ref)
        target = 'temporal-v7-history.json' if name == 'history.json' else name
        if target in copied:
            if copied[target] != data:
                raise SystemExit('recursive history filename collision')
            continue
        util.write(proof/target, data)
        copied[target] = data
        pending.extend(util.references(json.loads(data)))
    inventory, all_owners, prior = capture(day)
    files = originals()
    ledger = json.loads(files['full-ledger.json'])
    entries = []
    for row in ledger['entries'] or []:
        name = 'entry-%03d.json' % row['id']
        data = util.artifact(PRIOR/name)
        if json.loads(data) != row['observation_json']:
            raise SystemExit('original V7 native entry diverges from ledger')
        target = 'closed-v7-'+name
        util.write(proof/target, data)
        entries.append(util.reference(target, data))
    setup = util.frozen(BASE/'reports/native-v7-preseal-v2/setup-failure.json', '32af3be43422d80da034f2b5eaa534e67f37cbb7546f3499410cb179e3b9df39')
    setup_outcomes = util.artifact(BASE/'reports/native-v7-preseal-v2/all-original-outcomes.json')
    files.update({'closed-current-seven.json': util.canonical(inventory), 'closed-all-owners-seven.json': util.canonical(all_owners), 'closed-v7-current.json': util.canonical(prior), 'closed-v7-setup-failure.json': setup, 'closed-v7-setup-outcomes.json': setup_outcomes})
    for name, data in files.items():
        if name in copied:
            raise SystemExit('closed V7 witness filename collision')
        util.write(proof/name, data)
    result = {'format': 'butler.memory.closed-owner-leaf.v1', 'owner': 'owner', 'day': day, 'deadline': m['deadline'], 'previous_v7_history': util.reference('temporal-v7-history.json', previous), 'original_v7_entries': entries, 'future90': future(), 'rule': 'One recursively validated inclusive closed V7 owner-day leaf; independent product/native NULLs, old failures, all12 original outcomes, historical161 and separate adaptive226 retained. No authority or qualification.'}
    for key, name in [('current', 'closed-current-seven.json'), ('all_owners', 'closed-all-owners-seven.json'), ('closed_v7', 'closed-v7-current.json'), ('original_v7_ledger', 'full-ledger.json'), ('original_v7_outcomes', 'all-original-outcomes.json'), ('closed_v7_handback', 'closed-handback.json'), ('operator_accounting', 'closed-leaf-metadata.json'), ('zero_auth_setup_failure', 'closed-v7-setup-failure.json'), ('zero_auth_setup_outcomes', 'closed-v7-setup-outcomes.json')]:
        result[key] = util.reference(name, files[name])
    data = util.canonical(result)
    util.write(proof/'history.json', data)
    print(json.dumps({'read_only': True, 'history_sha256': hashlib.sha256(data).hexdigest(), 'seven_databases': 7, 'day': day, 'auth_or_inference': False}))


if __name__ == '__main__':
    main()
