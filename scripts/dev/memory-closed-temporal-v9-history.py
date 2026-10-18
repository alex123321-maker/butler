#!/usr/bin/env python3
"""Read-only CLOSED V9 latest owner/day leaf for the next lab collector.
Stdlib-only bounded files/pipes preserve original NULLs and prior lineage.
No credential reader, provider call, grading or production admission.
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
spec = importlib.util.spec_from_file_location('closed_v9_util', ROOT/'memory-temporal-v9-history.py')
v9 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(v9)
closed, util = v9.closed.closed, v9.util
BASE = pathlib.Path('/srv/storage/butler/agents/issue88-native-v9-20261018')
PRIOR = BASE/'reports/native-v9-preseal-v1'
HISTORY = PRIOR/'history'


def originals():
    # Exists only after the serialized exact return/main200/fresh snapshot.
    status = json.loads(util.artifact(PRIOR/'native-status'))
    if status not in (0, 1):
        raise SystemExit('closed original V9 collector status required')
    leaf = util.artifact(PRIOR/'closed-leaf-metadata.json')
    metadata = json.loads(leaf)
    pins_raw = util.artifact(PRIOR/'frozen-pins.json')
    pins = json.loads(pins_raw)
    if pins.get('window_revision') != 9 or pins.get('facts_contract_revision') != '5' or pins.get('facts_contract_sha256') != '43ee9a114c83813205dd973bfe22e2f0f54982ce2b36de3bf23fa0bd8d05f49e' or metadata.get('source') != pins.get('revision') or metadata.get('format') != 'butler.temporal.closed-leaf.v9' or metadata.get('collector_status') != status:
        raise SystemExit('exact closed V9 source/status required')
    if metadata['artifact_hashes'].get('frozen-pins.json') != hashlib.sha256(pins_raw).hexdigest():
        raise SystemExit('exact original V9 pins witness required')
    files = {'closed-v9-operator.json': leaf, 'closed-v9-frozen-pins.json': pins_raw}
    for filename in ('full-ledger.json', 'all-original-outcomes.json', 'closed-handback.json'):
        files['closed-v9-'+filename] = util.frozen(PRIOR/filename, metadata['artifact_hashes'][filename])
    c = json.loads(files['closed-v9-closed-handback.json'])
    main, returned = c['main_auth_safe'], c['return_safe']
    if c['native_status'] != status or returned.get('opaque_main_cas') is not True or returned.get('active_work_checked') is not True or main.get('resolved') is not True or main.get('unsafe_refresh') is not False or main.get('expired') is not False or c['main_snapshot_safe'].get('fresh_main_snapshot_saved') is not True or not any(t.get('stage') == 'paid_discovery' and t.get('outcome') == 'completed' and t.get('http_status') == 200 for t in main['trace']):
        raise SystemExit('exact closed V9 handback proof required')
    for filename, field in [('return-safe.json', 'return_safe'), ('main-auth-safe.json', 'main_auth_safe'), ('main-snapshot-safe.json', 'main_snapshot_safe')]:
        if json.loads(util.artifact(PRIOR/filename)) != c[field]:
            raise SystemExit('original V9 handback witness changed')
    return files


def predecessor():
    return closed.sql("""SET LOCAL search_path=memory_temporal_native_v9,public;
SELECT jsonb_build_object('window',(SELECT to_jsonb(w) FROM temporal_native_window w WHERE singleton),
'entries',(SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'run_id',run_id,'kind',kind,'finished_at',finished_at,'observation_json',jsonb_build_object('id',observation_json->'id','run_id',observation_json->'run_id','kind',observation_json->'kind','operation_id',observation_json->'operation_id','reservation_id',observation_json->'reservation_id','usage',observation_json->'usage')) ORDER BY id),'[]') FROM temporal_native_entries))""")


def capture(day):
    current, all_owners = util.current(day), util.all_owners(day)
    matches = [w for db in current['databases'] if db['container'] == 'butler-issue85-postgres' and db['database'] == 'issue85_native_test' for w in db['windows'] if w['schema'] == 'memory_temporal_native_v9']
    if len(matches) != 1:
        raise SystemExit('one exact closed V9 current-owner leaf required')
    prior = predecessor()
    if prior['window'].get('sealed') is not True or prior['window'].get('closed') is not True or len(prior['entries']) != prior['window'].get('paid_entries'):
        raise SystemExit('complete closed V9 population required')
    prior.update({'format': 'butler.consolidation.predecessor88.v5', **{key: matches[0][key] for key in ('scopes', 'reservations', 'foreground', 'background')}})
    return current, all_owners, prior


def main():
    mode = sys.argv[1]
    if mode == 'check-current':
        path = pathlib.Path(sys.argv[2]).resolve()
        if not re.fullmatch(r'/srv/storage/butler/agents/issue(?:88|90)-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/history/(?:history|latest-closed-history)\.json', str(path)):
            raise SystemExit('bounded closed V9 history path required')
        raw = path.read_bytes()
        m = json.loads(raw)
        if hashlib.sha256(raw).hexdigest() != sys.argv[3] or m.get('format') != 'butler.memory.closed-owner-leaf.v2' or m.get('version') != 9 or m['day'] != datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d'):
            raise SystemExit('frozen closed V9 manifest or UTC day changed')
        values = capture(m['day'])
        for key, value in zip(('current', 'all_owners', 'closed_stage'), values):
            if util.referenced(path.parent, m[key])[1] != util.canonical(value):
                raise SystemExit('current closed V9 population changed; fresh review required')
        files = originals()
        for key, name in [('original_ledger', 'closed-v9-full-ledger.json'), ('original_outcomes', 'closed-v9-all-original-outcomes.json'), ('closed_handback', 'closed-v9-closed-handback.json'), ('operator_accounting', 'closed-v9-operator.json'), ('original_pins', 'closed-v9-frozen-pins.json')]:
            if util.referenced(path.parent, m[key])[1] != files[name]:
                raise SystemExit('closed V9 original witness changed')
        for ref in m['original_entries']:
            name, data = util.referenced(path.parent, ref)
            if data != util.artifact(PRIOR/name.removeprefix('closed-v9-')):
                raise SystemExit('closed V9 immutable native entry changed')
        if closed.future() != m['future90']:
            raise SystemExit('future90 namespace/receipt changed')
        print(json.dumps({'read_only': True, 'seven_databases': 7, 'current_day_unchanged': True, 'auth_or_inference': False}))
        return
    if mode != 'prepare':
        raise SystemExit('prepare or check-current required')
    base, label = pathlib.Path(sys.argv[2]).resolve(), sys.argv[3]
    if not re.fullmatch('/srv/storage/butler/agents/issue(?:88|90)-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch('[A-Za-z0-9_-]+', label):
        raise SystemExit('bounded closed V9 history target required')
    files = originals()
    previous = util.frozen(HISTORY/'history.json', json.loads(files['closed-v9-frozen-pins.json'])['retained_history_sha256'])
    m = json.loads(previous)
    day = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d')
    if m.get('format') != 'butler.temporal.native-history.v5' or m['day'] != day:
        raise SystemExit('same-day original V9 lineage required')
    proof = base/'reports'/label/'history'
    proof.mkdir(mode=0o700, parents=True)
    copied_count = copied_bytes = 0
    def write_bounded(name, data):
        nonlocal copied_count, copied_bytes
        if len(data) > 32*1024*1024 or copied_count+1 > 512 or copied_bytes+len(data) > 128*1024*1024:
            raise SystemExit('closed V9 evidence exceeds finite copy limits')
        util.write(proof/name, data)
        copied_count += 1
        copied_bytes += len(data)
    for item in v9.bounded_files(HISTORY):
        with item.open('rb') as source:
            data = source.read(32*1024*1024+1)
        if len(data) > 32*1024*1024 or item.name == 'history.json' and data != previous:
            raise SystemExit('bounded immutable original V9 history required')
        json.loads(data)
        target = 'temporal-v9-history.json' if item.name == 'history.json' else item.name
        write_bounded(target, data)
    current, all_owners, prior = capture(day)
    files.update({'closed-v9-current-seven.json': util.canonical(current), 'closed-v9-all-owners-seven.json': util.canonical(all_owners), 'closed-v9-current.json': util.canonical(prior)})
    entries = []
    original_entries = json.loads(files['closed-v9-full-ledger.json'])['entries'] or []
    if len(original_entries) > 144:
        raise SystemExit('bounded original V9 native population required')
    for row in original_entries:
        name = 'entry-%03d.json' % row['id']
        data = util.artifact(PRIOR/name)
        if json.loads(data) != row['observation_json']:
            raise SystemExit('native V9 original entry differs from ledger')
        target = 'closed-v9-'+name
        write_bounded(target, data)
        entries.append(util.reference(target, data))
    for name, data in files.items():
        write_bounded(name, data)
    result = {'format': 'butler.memory.closed-owner-leaf.v2', 'version': 9, 'owner': 'owner', 'day': day, 'deadline': m['deadline'], 'schema': 'memory_temporal_native_v9', 'window': 'temporal-explicit-v9', 'previous_history': util.reference('temporal-v9-history.json', previous), 'original_entries': entries, 'future90': closed.future(), 'rule': 'One validated latest inclusive closed V9 leaf; old175 contained once; independent native/product unknown counts, all12 outcomes and every old failure retained. Historical161/adaptive226 separate. No qualification or production admission.'}
    for key, name in [('current', 'closed-v9-current-seven.json'), ('all_owners', 'closed-v9-all-owners-seven.json'), ('closed_stage', 'closed-v9-current.json'), ('original_ledger', 'closed-v9-full-ledger.json'), ('original_outcomes', 'closed-v9-all-original-outcomes.json'), ('closed_handback', 'closed-v9-closed-handback.json'), ('operator_accounting', 'closed-v9-operator.json')]:
        result[key] = util.reference(name, files[name])
    data = util.canonical(result)
    write_bounded('history.json', data)
    v9.bounded_files(proof)
    print(json.dumps({'read_only': True, 'history_sha256': hashlib.sha256(data).hexdigest(), 'seven_databases': 7, 'auth_or_inference': False}))


if __name__ == '__main__':
    main()
