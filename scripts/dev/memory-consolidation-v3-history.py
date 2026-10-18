#!/usr/bin/env python3
"""Read-only fixed v3 lab recipe; standard-library pipes retain original JSON
bytes and seven-store NULLs. No credentials or production admission are read.
"""
import datetime
import importlib.util
import json
import pathlib
import re
import subprocess
import sys

sys.dont_write_bytecode = True
ROOT = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('temporal_lineage', ROOT/'memory-temporal-successor-history.py')
old = importlib.util.module_from_spec(spec)
spec.loader.exec_module(old)
SCHEMA = 'consolidation_native_v3'
WINDOW = 'consolidation-explicit-v3'


def sql(container, database, text, port=5432):
    result = subprocess.run(['docker', 'exec', '-i', container, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'butler', '-p', str(port), '-d', database], input='BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;'+text+';ROLLBACK;', text=True, capture_output=True, timeout=40)
    if result.returncode:
        raise SystemExit('bounded read-only successor inventory unavailable')
    return json.loads(result.stdout)


def future_window():
    state = sql('issue86-20261016-pg', 'memory_read_test', "SELECT jsonb_build_object('schema','consolidation_native_v3','window_id','consolidation-explicit-v3','schema_present',to_regnamespace('consolidation_native_v3') IS NOT NULL,'receipt_table_present',to_regclass('public.issue89_native_windows') IS NOT NULL)")
    state['receipt_count'] = sql('issue86-20261016-pg', 'memory_read_test', "SELECT count(*) FROM public.issue89_native_windows WHERE window_id='consolidation-explicit-v3' OR schema_name='consolidation_native_v3'") if state['receipt_table_present'] else 0
    if state['schema_present'] is not False or state['receipt_count'] != 0:
        raise SystemExit('v3 namespace or one-use receipt exists; no clone allowed')
    return state


def metadata():
    value = sql('butler-issue85-postgres', 'issue85_native_test', """SET LOCAL search_path=memory_temporal_native_v6,public;
SELECT jsonb_build_object('window',(SELECT to_jsonb(w) FROM temporal_native_window w),
'entries',(SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'run_id',run_id,'kind',kind,'finished_at',finished_at,
'observation_json',jsonb_build_object('id',observation_json->'id','run_id',observation_json->'run_id','kind',observation_json->'kind','operation_id',observation_json->'operation_id','reservation_id',observation_json->'reservation_id','usage',observation_json->'usage')) ORDER BY id),'[]') FROM temporal_native_entries))""", 18585)
    window = value.get('window', {})
    if window.get('closed') is not True or window.get('sealed') is not True or type(window.get('paid_entries')) is not int or not 1 <= window['paid_entries'] <= 144 or len(value.get('entries', [])) != window['paid_entries']:
        raise SystemExit('complete closed temporal-v6 original entry population required')
    return value


def copy_lineage(source, target, raw):
    copied = {'temporal-v6-history.json': raw}
    old.write(target/'temporal-v6-history.json', raw)
    pending = list(old.references(json.loads(raw)))
    while pending:
        name, data = old.referenced(source, pending.pop())
        if name in copied:
            if copied[name] != data:
                raise SystemExit('recursive original basename collision')
            continue
        old.write(target/name, data)
        copied[name] = data
        pending.extend(old.references(json.loads(data)))


def closed_handback(report):
    result = subprocess.run(['docker', 'run', '--rm', '--pull', 'never', '--network', 'none', '--cpus', '.1', '--memory', '64m', '--pids-limit', '32', '--mount', 'type=bind,src='+str(report)+',dst=/from,readonly', 'golang:1.27.2-bookworm', 'cat', '/from/native-status'], capture_output=True, timeout=40)
    if result.returncode or result.stdout.strip() != b'0':
        raise SystemExit('successful closed native runner and handback required before export')
    returned = json.loads(old.artifact(report/'return-safe.json'))
    main = json.loads(old.artifact(report/'main-auth-safe.json'))
    snapshot = json.loads(old.artifact(report/'main-snapshot-safe.json'))
    if returned.get('opaque_main_cas') is not True or main.get('resolved') is not True or main.get('unsafe_refresh') is not False or main.get('expired') is not False or snapshot.get('fresh_main_snapshot_saved') is not True:
        raise SystemExit('exact CAS/main/snapshot successor handback proof required')
    if not any(x.get('stage') == 'paid_discovery' and x.get('outcome') == 'completed' and x.get('http_status') == 200 for x in main.get('trace', [])):
        raise SystemExit('actual main paid-discovery HTTP200 required')


def main():
    mode = sys.argv[1]
    if mode == 'check-current':
        path = pathlib.Path(sys.argv[2]).resolve()
        if not re.fullmatch('/srv/storage/butler/agents/issue89-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/history/history.json', str(path)):
            raise SystemExit('isolated v3 history path required')
        raw = path.read_bytes()
        if old.reference('', raw)['SHA256'] != sys.argv[3]:
            raise SystemExit('frozen successor history differs')
        manifest = json.loads(raw)
        day = manifest['day']
        if manifest.get('format') != 'butler.consolidation.native-history.v4' or datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d') != day or future_window() != manifest['future_window']:
            raise SystemExit('prospective v3 day/namespace/receipt changed')
        for key, value in (('current', old.current(day)), ('all_owners', old.all_owners(day))):
            _, before = old.referenced(path.parent, manifest[key])
            if old.canonical(value) != before:
                raise SystemExit('seven-store population changed; new freeze required')
        _, prior = old.referenced(path.parent, manifest['predecessor88'])
        prior, live = json.loads(prior), metadata()
        if any(prior[key] != live[key] for key in ('window', 'entries')):
            raise SystemExit('closed latest temporal original metadata changed')
        print(json.dumps({'read_only':True,'current_day_unchanged':True,'seven_databases':7,'schema_absent':True,'receipt_count':0,'auth_or_inference':False}))
        return
    if mode != 'prepare':
        raise SystemExit('prepare or check-current required')
    base, label, report = pathlib.Path(sys.argv[2]).resolve(), sys.argv[3], pathlib.Path(sys.argv[4]).resolve()
    if not re.fullmatch('/srv/storage/butler/agents/issue89-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch('[A-Za-z0-9_-]+', label) or not re.fullmatch('/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+', str(report)):
        raise SystemExit('bounded isolated successor references required')
    closed_handback(report)
    previous = old.artifact(report/'history/history.json')
    previous_manifest = json.loads(previous)
    now = datetime.datetime.now(datetime.timezone.utc)
    day = now.strftime('%Y-%m-%d')
    deadline = datetime.datetime.combine(now.date()+datetime.timedelta(days=1), datetime.time(), datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    if previous_manifest.get('format') != 'butler.temporal.native-history.v2' or previous_manifest.get('schema') != 'memory_temporal_native_v6' or previous_manifest.get('day') != day or previous_manifest.get('deadline') != deadline:
        raise SystemExit('same-day temporal-v6 recursive original history required')
    inventory, all_inventory, native, future = old.current(day), old.all_owners(day), metadata(), future_window()
    matches = [w for db in inventory['databases'] if db['container'] == 'butler-issue85-postgres' and db['database'] == 'issue85_native_test' for w in db['windows'] if w['schema'] == 'memory_temporal_native_v6']
    if len(matches) != 1:
        raise SystemExit('one exact latest inclusive temporal-v6 owner scope required')
    native.update({'format':'butler.consolidation.predecessor88.v2', **{key:matches[0][key] for key in ('scopes','reservations','foreground','background')}})
    proof = base/'reports'/label/'history'
    proof.mkdir(mode=0o700, parents=True)
    copy_lineage(report/'history', proof, previous)
    files = {'v3-current-owner-day.json':old.canonical(inventory), 'v3-all-seven-owner-ledgers.json':old.canonical(all_inventory), 'v3-predecessor88-current.json':old.canonical(native), 'v6-native88-full-ledger.json':old.artifact(report/'full-ledger.json'), 'v6-native88-outcomes.json':old.artifact(report/'all-original-outcomes.json')}
    for name,data in files.items():
        old.write(proof/name,data)
    entries=[]
    for entry in native['entries']:
        number=entry['id']
        if type(number) is not int or not 1 <= number <= 144:
            raise SystemExit('bounded original temporal-v6 entry required')
        name='v6-entry-%03d.json'%number
        data=old.artifact(report/('entry-%03d.json'%number))
        old.write(proof/name,data)
        entries.append(old.reference(name,data))
    manifest={'format':'butler.consolidation.native-history.v4','owner':'owner','day':day,'deadline':deadline,'schema':SCHEMA,'predecessor88_history':old.reference('temporal-v6-history.json',previous),'current':old.reference('v3-current-owner-day.json',files['v3-current-owner-day.json']),'all_owners':old.reference('v3-all-seven-owner-ledgers.json',files['v3-all-seven-owner-ledgers.json']),'predecessor88':old.reference('v3-predecessor88-current.json',files['v3-predecessor88-current.json']),'original88_ledger':old.reference('v6-native88-full-ledger.json',files['v6-native88-full-ledger.json']),'original88_outcomes':old.reference('v6-native88-outcomes.json',files['v6-native88-outcomes.json']),'original88_entries':entries,'future_window':future,'rule':'Latest inclusive owner/day leaf once; original161/89v1/88v5/89v2/88v6 failures, NULLs and deadlines retained recursively. Distinct adaptive226 quotes are separate evidence, never rebilled. Lab partition only, not production cross-database admission.'}
    raw=old.canonical(manifest)
    old.write(proof/'history.json',raw)
    print(json.dumps({'read_only':True,'history_sha256':old.reference('',raw)['SHA256'],'seven_databases':7,'day':day,'auth_or_inference':False}))


if __name__ == '__main__':
    main()
