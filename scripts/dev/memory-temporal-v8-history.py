#!/usr/bin/env python3
"""Fixed prospective V8 wrapper over the validated CLOSED V7 resource leaf.
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
spec = importlib.util.spec_from_file_location('closed_v7_export', ROOT/'memory-closed-temporal-v7-history.py')
closed = importlib.util.module_from_spec(spec)
spec.loader.exec_module(closed)
util = closed.util
HISTORY = closed.BASE/'reports/closed-history-v1/history'
HISTORY_SHA = 'c1d3db9826b1602b7cb65475e3e6e65c5804cab4ec15ded1c128f31453ba2529'


def future():
    value = closed.sql("SELECT jsonb_build_object('schema','memory_temporal_native_v8','window_id','temporal-explicit-v8','schema_present',to_regnamespace('memory_temporal_native_v8') IS NOT NULL,'receipt_table_present',to_regclass('public.issue88_native_windows') IS NOT NULL)")
    value['receipt_count'] = closed.sql("SELECT count(*) FROM public.issue88_native_windows WHERE window_id='temporal-explicit-v8'") if value['receipt_table_present'] else 0
    if value['schema_present'] is not False or value['receipt_count'] != 0:
        raise SystemExit('V8 namespace or one-use receipt already exists')
    return value


def check_previous(path, sha):
    result = subprocess.run([sys.executable, str(ROOT/'memory-closed-temporal-v7-history.py'), 'check-current', str(path), sha], capture_output=True, timeout=300)
    if result.returncode or json.loads(result.stdout).get('current_day_unchanged') is not True:
        raise SystemExit('closed V7 seven-store population/originals changed')


def main():
    mode = sys.argv[1]
    if mode == 'check-current':
        path = pathlib.Path(sys.argv[2]).resolve()
        if not re.fullmatch('/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/history/history.json', str(path)):
            raise SystemExit('bounded V8 history reference required')
        raw = path.read_bytes()
        m = json.loads(raw)
        if hashlib.sha256(raw).hexdigest() != sys.argv[3] or m.get('format') != 'butler.temporal.native-history.v4' or m['day'] != datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d'):
            raise SystemExit('frozen V8 history/UTC day differs')
        name, data = util.referenced(path.parent, m['closed_v7_history'])
        check_previous(path.parent/name, hashlib.sha256(data).hexdigest())
        if future() != m['future_window']:
            raise SystemExit('V8 namespace proof changed')
        print(json.dumps({'read_only': True, 'seven_databases': 7, 'current_day_unchanged': True, 'auth_or_inference': False}))
        return
    if mode != 'prepare':
        raise SystemExit('prepare or check-current required')
    base, label = pathlib.Path(sys.argv[2]).resolve(), sys.argv[3]
    if not re.fullmatch('/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch('[A-Za-z0-9_-]+', label):
        raise SystemExit('bounded fresh V8 history target required')
    previous = util.frozen(HISTORY/'history.json', HISTORY_SHA)
    m = json.loads(previous)
    if m.get('format') != 'butler.memory.closed-owner-leaf.v1' or m['day'] != datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d'):
        raise SystemExit('same-day closed V7 predecessor required')
    check_previous(HISTORY/'history.json', HISTORY_SHA)
    proof = base/'reports'/label/'history'
    proof.mkdir(mode=0o700, parents=True)
    copied, pending = {}, [util.reference('history.json', previous)]
    while pending:
        ref = pending.pop()
        name, data = util.referenced(HISTORY, ref)
        target = 'closed-v7-history.json' if name == 'history.json' else name
        if target in copied:
            if copied[target] != data:
                raise SystemExit('recursive V8 history filename collision')
            continue
        util.write(proof/target, data)
        copied[target] = data
        pending.extend(util.references(json.loads(data)))
    result = {'format': 'butler.temporal.native-history.v4', 'owner': 'owner', 'day': m['day'], 'deadline': m['deadline'], 'schema': 'memory_temporal_native_v8', 'closed_v7_history': util.reference('closed-v7-history.json', previous), 'future_window': future(), 'rule': 'Latest validated inclusive145 once; retain every earlier failed/unexecuted/NULL original and separate adaptive226. Prospective whole12 on generic facts contract4; no quality inference from predecessor accounting.'}
    data = util.canonical(result)
    util.write(proof/'history.json', data)
    print(json.dumps({'read_only': True, 'history_sha256': hashlib.sha256(data).hexdigest(), 'seven_databases': 7, 'auth_or_inference': False}))


if __name__ == '__main__':
    main()
