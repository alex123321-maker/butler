#!/usr/bin/env python3
"""Fixed prospective V9 wrapper over the validated CLOSED V8 resource leaf.
Python stdlib supports read-only bounded lab pipes; no provider or auth access.
"""
import datetime
import hashlib
import importlib.util
import json
import pathlib
import re
import subprocess
import sys

sys.dont_write_bytecode = True
ROOT = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('closed_v8_export', ROOT/'memory-closed-temporal-v8-history.py')
closed = importlib.util.module_from_spec(spec)
spec.loader.exec_module(closed)
util = closed.util
HISTORY = closed.BASE/'reports/closed-history-v1/history'
HISTORY_SHA = 'dadc530e45c271d8b70ce1a69dd8d94c9ff252cd0d9ed9f1bcee875c9e8511a6'


def bounded_files(root):
    files = sorted(root.iterdir())
    if len(files) > 512:
        raise SystemExit('at most512 closed-history JSON files required')
    total = 0
    for item in files:
        if item.is_symlink() or not item.is_file() or not re.fullmatch(r'[A-Za-z0-9_.-]+\.json', item.name):
            raise SystemExit('only bounded regular closed-history JSON files required')
        size = item.stat().st_size
        if size > 32*1024*1024:
            raise SystemExit('closed-history JSON file exceeds32MiB')
        total += size
        if total > 128*1024*1024:
            raise SystemExit('closed-history JSON exceeds128MiB in total')
    return files


def future():
    value = closed.closed.sql("SELECT jsonb_build_object('schema','memory_temporal_native_v9','window_id','temporal-explicit-v9','schema_present',to_regnamespace('memory_temporal_native_v9') IS NOT NULL,'receipt_table_present',to_regclass('public.issue88_native_windows') IS NOT NULL)")
    value['receipt_count'] = closed.closed.sql("SELECT count(*) FROM public.issue88_native_windows WHERE window_id='temporal-explicit-v9'") if value['receipt_table_present'] else 0
    if value['schema_present'] is not False or value['receipt_count'] != 0:
        raise SystemExit('V9 namespace or one-use receipt already exists')
    return value


def check_previous(path, sha):
    result = subprocess.run([sys.executable, str(ROOT/'memory-closed-temporal-v8-history.py'), 'check-current', str(path), sha], capture_output=True, timeout=300)
    if result.returncode or json.loads(result.stdout).get('current_day_unchanged') is not True:
        raise SystemExit('closed V8 seven-store population/originals changed')


def main():
    mode = sys.argv[1]
    if mode == 'check-current':
        path = pathlib.Path(sys.argv[2]).resolve()
        if not re.fullmatch('/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/history/history.json', str(path)):
            raise SystemExit('bounded V9 history reference required')
        raw = path.read_bytes()
        m = json.loads(raw)
        if hashlib.sha256(raw).hexdigest() != sys.argv[3] or m.get('format') != 'butler.temporal.native-history.v5' or m['day'] != datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d'):
            raise SystemExit('frozen V9 history/UTC day differs')
        name, data = util.referenced(path.parent, m['latest_closed_history'])
        check_previous(path.parent/name, hashlib.sha256(data).hexdigest())
        if future() != m['future_window']:
            raise SystemExit('V9 namespace proof changed')
        print(json.dumps({'read_only': True, 'seven_databases': 7, 'current_day_unchanged': True, 'auth_or_inference': False}))
        return
    if mode != 'prepare':
        raise SystemExit('prepare or check-current required')
    base, label = pathlib.Path(sys.argv[2]).resolve(), sys.argv[3]
    if not re.fullmatch('/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch('[A-Za-z0-9_-]+', label):
        raise SystemExit('bounded fresh V9 history target required')
    previous = util.frozen(HISTORY/'history.json', HISTORY_SHA)
    m = json.loads(previous)
    if m.get('format') != 'butler.memory.closed-owner-leaf.v2' or m.get('version') != 8 or m['day'] != datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d'):
        raise SystemExit('same-day closed V8 predecessor required')
    check_previous(HISTORY/'history.json', HISTORY_SHA)
    proof = base/'reports'/label/'history'
    proof.mkdir(mode=0o700, parents=True)
    # The validated closed receipt also retains documentary absolute paths in
    # operator accounting. Those are not artifact capabilities to follow. Copy
    # the bounded flat JSON evidence directory; the Go loader independently
    # traverses and hashes every required typed manifest reference afterward.
    copied = {}
    total = 0
    for item in bounded_files(HISTORY):
        name = item.name
        try:
            with item.open('rb') as source:
                data = source.read(32*1024*1024+1)
        except PermissionError:
            data = util.artifact(item)
        if len(data) > 32*1024*1024:
            raise SystemExit('bounded closed V8 evidence required')
        total += len(data)
        if total > 128*1024*1024:
            raise SystemExit('closed-history JSON exceeds128MiB while copying')
        json.loads(data)
        if name == 'history.json' and data != previous:
            raise SystemExit('original closed V8 manifest changed')
        target = 'latest-closed-history.json' if name == 'history.json' else name
        util.write(proof/target, data)
        copied[target] = data
    result = {'format': 'butler.temporal.native-history.v5', 'owner': 'owner', 'day': m['day'], 'deadline': m['deadline'], 'schema': 'memory_temporal_native_v9', 'latest_closed_history': util.reference('latest-closed-history.json', previous), 'future_window': future(), 'rule': 'Latest validated inclusive175 once; retain every earlier failed/unexecuted/NULL original and separate adaptive226. Prospective whole12 on generic facts contract5; no quality inference from predecessor accounting.'}
    data = util.canonical(result)
    util.write(proof/'history.json', data)
    print(json.dumps({'read_only': True, 'history_sha256': hashlib.sha256(data).hexdigest(), 'seven_databases': 7, 'auth_or_inference': False}))


if __name__ == '__main__':
    main()
