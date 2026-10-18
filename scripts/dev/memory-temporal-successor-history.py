#!/usr/bin/env python3
"""Read-only v6 successor evidence. Standard-library subprocess pipes and
exclusive private files preserve original bytes, nullable usage and seven stores.
This lab exporter is neither a credential reader nor production admission.
"""
import datetime
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent
PRIOR89 = pathlib.Path('/srv/storage/butler/agents/issue89-20261016/reports/native-v2-preseal-v2')
PRIOR89_HISTORY = pathlib.Path('/srv/storage/butler/agents/issue89-20261016/reports/native-v2-history-v1/history')
HISTORY89_SHA = '976e353b5b01eea7817666f8aebccddca68f4f3a293873aa256ba9817c8949fb'
PRIOR87 = pathlib.Path('/srv/storage/butler/agents/issue87-native-v5b-20261018/reports/native-v5')
LEDGER87_SHA = 'c51b06b2a04ce92d6e901ef59e35584715c4d128e0e35f85ab32f46b84f724a4'
LEDGER89_SHA = 'a407e19989b187d88ffa5cd97eeb84247d50bf33bbdb2c144d0200fedac81a62'
SCREEN89_SHA = '3a3f011dc8580db6eff68f4e79d236eb49eee35dd5cc34c1d49eeda55103d6c6'


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()


def write(path, raw):
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'wb') as out:
        out.write(raw)
        out.flush()
        os.fsync(out.fileno())


def artifact(path):
    result = subprocess.run(['docker', 'run', '--rm', '--pull', 'never', '--network', 'none', '--cpus', '.1', '--memory', '64m', '--pids-limit', '32', '--mount', 'type=bind,src='+str(path.parent)+',dst=/from,readonly', 'golang:1.27.2-bookworm', 'cat', '/from/'+path.name], capture_output=True, timeout=40)
    if result.returncode or len(result.stdout) > 32*1024*1024:
        raise SystemExit('bounded original successor artifact unavailable')
    json.loads(result.stdout)
    return result.stdout


def reference(name, raw):
    return {'Path': name, 'SHA256': hashlib.sha256(raw).hexdigest()}


def referenced(root, ref):
    name, sha = ref.get('Path', ref.get('path')), ref.get('SHA256', ref.get('sha256'))
    if not isinstance(name, str) or pathlib.Path(name).name != name or not re.fullmatch('[a-f0-9]{64}', str(sha)):
        raise SystemExit('bounded immutable successor reference required')
    raw = artifact(root / name)
    if hashlib.sha256(raw).hexdigest() != sha:
        raise SystemExit('original successor bytes changed')
    return name, raw


def references(value):
    if isinstance(value, dict):
        if set(value) >= {'Path', 'SHA256'} or set(value) >= {'path', 'sha256'}:
            yield value
        for child in value.values():
            yield from references(child)
    elif isinstance(value, list):
        for child in value:
            yield from references(child)


def copy_lineage(source, proof, first):
    copied = {}
    pending = [first]
    while pending:
        ref = pending.pop()
        name, raw = referenced(source, ref)
        if name in copied:
            if copied[name] != raw:
                raise SystemExit('successor lineage basename collision')
            continue
        write(proof / ('consolidation-v2-history.json' if name == 'history.json' else name), raw)
        copied[name] = raw
        pending.extend(references(json.loads(raw)))
    return copied


def current(day):
    result = subprocess.run([sys.executable, str(ROOT/'memory-consolidation-history.py'), 'snapshot-current-seven', day], capture_output=True, timeout=240)
    if result.returncode:
        raise SystemExit('complete seven-store current owner inventory unavailable')
    value = json.loads(result.stdout)
    if value.get('format') != 'butler.consolidation.current-owner-day.v2' or value.get('day') != day or len(value.get('databases', [])) != 7:
        raise SystemExit('current seven-store inventory contract differs')
    return value


def all_owners(day):
    spec = importlib.util.spec_from_file_location('adaptive_history', ROOT/'memory-adaptive-history.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.DATABASES += (('butler-issue87-native-v5-pg', 'memory_adaptive_test', 5432),)
    value = module.inventory(day)
    value['format'] = 'butler.temporal.seven-store-ledgers.v6'
    value['prospective_owner'] = 'owner'
    for database in value['databases']:
        if database['container'] != 'butler-issue87-native-v5-pg':
            continue
        if len(database['windows']) != 1 or database['windows'][0]['schema'] != 'adaptive_native_v5':
            raise SystemExit('exact seventh-store population required')
        window = database['windows'][0]
        extra = module.sql(database['container'], database['database'], database['port'], "SET LOCAL search_path=adaptive_native_v5,public;SELECT jsonb_build_object('window',(SELECT to_jsonb(w) FROM adaptive_native_window w),'native_entries',(SELECT coalesce(jsonb_agg(jsonb_build_object('entry_id',entry_id,'run_id',run_id,'operation_id',operation_id,'reservation_id',reservation_id,'kind',kind,'native_usage',native_usage,'full_total_known',full_total_known,'usage_invalid',usage_invalid,'observed_at',observed_at) ORDER BY entry_id),'[]') FROM adaptive_native_entries))")
        window['adaptive_window'], window['native_adaptive'] = extra['window'], extra['native_entries']
    return value


def predecessor_metadata():
    # The original two-call v1 and complete v5 stay separate recursive witnesses.
    # This projection reads only the closed eleven-call v2 latest leaf.
    query = """BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL search_path=consolidation_native_v2,public;
SELECT jsonb_build_object('closed',(SELECT closed FROM consolidation_screen_window WHERE singleton),'paid_entries',(SELECT entries FROM consolidation_screen_window WHERE singleton),'calls',(SELECT coalesce(jsonb_agg(jsonb_build_object('entry_id',entry_id,'operation_id',operation_id,'reservation_id',observation->>'ReservationID','observation_json',jsonb_build_object('EntryID',observation->'EntryID','OperationID',observation->'OperationID','ReservationID',observation->'ReservationID','Usage',observation->'Usage','FinishedAt',observation->'FinishedAt')) ORDER BY entry_id),'[]'::jsonb) FROM consolidation_screen_calls));
COMMIT;"""
    result = subprocess.run(['docker', 'exec', '-i', 'issue86-20261016-pg', 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'butler', '-d', 'memory_read_test'], input=query, text=True, capture_output=True, timeout=40)
    if result.returncode:
        raise SystemExit('closed latest consolidation metadata unavailable')
    value = json.loads(result.stdout)
    if value.get('closed') is not True or value.get('paid_entries') != 11 or len(value.get('calls', [])) != 11:
        raise SystemExit('complete original eleven-call latest leaf required')
    return value


def frozen(path, sha):
    raw = artifact(path)
    if hashlib.sha256(raw).hexdigest() != sha:
        raise SystemExit('closed original witness differs')
    return raw


def main():
    mode = sys.argv[1]
    if mode == 'probe':
        # Content-free table/column names only, for exact fixed query review.
        spec = importlib.util.spec_from_file_location('adaptive_history', ROOT/'memory-adaptive-history.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        print(json.dumps(module.sql('butler-issue87-native-v5-pg', 'memory_adaptive_test', 5432, "SELECT jsonb_build_object('tables',(SELECT jsonb_agg(tablename ORDER BY tablename) FROM pg_tables WHERE schemaname='adaptive_native_v5' AND tablename LIKE 'adaptive_native%'))")))
        return
    if mode == 'check-current':
        path = pathlib.Path(sys.argv[2]).resolve()
        if not re.fullmatch('/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/history/history.json', str(path)):
            raise SystemExit('isolated v6 history path required')
        raw = path.read_bytes()
        if hashlib.sha256(raw).hexdigest() != sys.argv[3]:
            raise SystemExit('frozen v6 history differs')
        manifest = json.loads(raw)
        day = manifest['day']
        if datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d') != day:
            raise SystemExit('prospective UTC day expired')
        for key, fresh in (('current', canonical(current(day))), ('all_owners', canonical(all_owners(day)))):
            _, before = referenced(path.parent, manifest[key])
            if fresh != before:
                raise SystemExit('seven-store population changed; new reviewed freeze required')
        _, prior_raw = referenced(path.parent, manifest['predecessor89'])
        prior, live = json.loads(prior_raw), predecessor_metadata()
        if any(prior[key] != live[key] for key in ('closed', 'paid_entries', 'calls')):
            raise SystemExit('closed latest native predecessor changed')
        for key, source, sha in (('original89_ledger', PRIOR89/'screen/full-ledger.json', LEDGER89_SHA), ('original89_screen', PRIOR89/'screen/screen.json', SCREEN89_SHA), ('original87_ledger', PRIOR87/'full-ledger.json', LEDGER87_SHA)):
            _, before = referenced(path.parent, manifest[key])
            if before != frozen(source, sha):
                raise SystemExit('original native witness changed')
        print(json.dumps({'read_only': True, 'current_day_unchanged': True, 'seven_databases': 7, 'day': day, 'auth_or_inference': False}))
        return
    if mode != 'prepare':
        raise SystemExit('prepare, check-current or probe required')
    base, label = pathlib.Path(sys.argv[2]).resolve(), sys.argv[3]
    if not re.fullmatch('/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch('[A-Za-z0-9_-]+', label):
        raise SystemExit('bounded isolated v6 target required')
    proof = base/'reports'/label/'history'
    proof.mkdir(mode=0o700, parents=True)
    now = datetime.datetime.now(datetime.timezone.utc)
    day = now.strftime('%Y-%m-%d')
    deadline = datetime.datetime.combine(now.date()+datetime.timedelta(days=1), datetime.time(), datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    previous = frozen(PRIOR89_HISTORY/'history.json', HISTORY89_SHA)
    old = json.loads(previous)
    if old.get('format') != 'butler.consolidation.native-history.v3' or old.get('day') != day or old.get('deadline') != deadline:
        raise SystemExit('same-day complete consolidation-v2 history required')
    copied = copy_lineage(PRIOR89_HISTORY, proof, reference('history.json', previous))
    inventory, all_inventory = current(day), all_owners(day)
    matches = [window for database in inventory['databases'] if database['container'] == 'issue86-20261016-pg' and database['database'] == 'memory_read_test' for window in database['windows'] if window['schema'] == 'consolidation_native_v2']
    if len(matches) != 1:
        raise SystemExit('one exact latest49 leaf required')
    prior = predecessor_metadata()
    prior.update({'format': 'butler.temporal.predecessor89.v2', **{key: matches[0][key] for key in ('scopes', 'reservations', 'foreground', 'background')}})
    prior['model_jobs'] = prior['background']
    files = {'current-v6-owner-day.json': canonical(inventory), 'all-seven-owner-ledgers.json': canonical(all_inventory), 'predecessor89-v2-current.json': canonical(prior), 'native89-v2-full-ledger.json': frozen(PRIOR89/'screen/full-ledger.json', LEDGER89_SHA), 'native89-v2-screen.json': frozen(PRIOR89/'screen/screen.json', SCREEN89_SHA), 'native87-v5-full-ledger.json': frozen(PRIOR87/'full-ledger.json', LEDGER87_SHA)}
    adaptive_history = frozen(PRIOR87/'history.json', '8d1ae45467dea7433dde736b19ee6c621601293a42e50ff7e17d7c96aae35e8f')
    adaptive = json.loads(adaptive_history)
    files['native87-v5-history.json'] = adaptive_history
    files['native87-v5-prior-six-store.json'] = frozen(PRIOR87/'six-store-ledgers.json', adaptive['inventory_sha256'])
    files['native87-v4-full-ledger.json'] = frozen(PRIOR87/'historical-v4-full-ledger.json', adaptive['historical_ledger_sha256'])
    files['native87-v4-native-calls.json'] = frozen(PRIOR87/'historical-v4-native-calls.json', adaptive['historical_native_sha256'])
    for ref in adaptive['closed_latest_issue89']:
        name, data = referenced(PRIOR87, ref)
        files[name] = data
    for name, raw in files.items():
        if name in copied:
            raise SystemExit('v6 successor artifact basename collision')
        write(proof/name, raw)
    manifest = {'format': 'butler.temporal.native-history.v2', 'owner': 'owner', 'day': day, 'deadline': deadline, 'schema': 'memory_temporal_native_v6', 'previous89_history': reference('consolidation-v2-history.json', previous), 'current': reference('current-v6-owner-day.json', files['current-v6-owner-day.json']), 'all_owners': reference('all-seven-owner-ledgers.json', files['all-seven-owner-ledgers.json']), 'predecessor89': reference('predecessor89-v2-current.json', files['predecessor89-v2-current.json']), 'original89_ledger': reference('native89-v2-full-ledger.json', files['native89-v2-full-ledger.json']), 'original89_screen': reference('native89-v2-screen.json', files['native89-v2-screen.json']), 'original87_ledger': reference('native87-v5-full-ledger.json', files['native87-v5-full-ledger.json']), 'rule': 'Recursively retain old161 and original89v1/88v5/89v2 with all failures and NULLs. Carry latest owner49 vector once. Distinct adaptive owner226 quotes remain explicit evidence, never additional owner49 spending. Seven live stores and original full native87 ledger remain immutable. Experimental partition, not production cross-database admission.'}
    manifest['original87_history'] = reference('native87-v5-history.json', adaptive_history)
    manifest['original87_inventory'] = reference('native87-v5-prior-six-store.json', files['native87-v5-prior-six-store.json'])
    manifest['original87_old_ledger'] = reference('native87-v4-full-ledger.json', files['native87-v4-full-ledger.json'])
    manifest['original87_old_native'] = reference('native87-v4-native-calls.json', files['native87-v4-native-calls.json'])
    manifest['original87_predecessors'] = [reference(ref['path'], files[ref['path']]) for ref in adaptive['closed_latest_issue89']]
    raw = canonical(manifest)
    write(proof/'history.json', raw)
    print(json.dumps({'read_only': True, 'history_sha256': hashlib.sha256(raw).hexdigest(), 'seven_databases': 7, 'day': day, 'auth_or_inference': False}))


if __name__ == '__main__':
    main()
