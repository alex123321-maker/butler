#!/usr/bin/env python3
"""Synthetic opaque bytes, actual disposable PG and a late-exiting container.
No main/native retained database access, provider, auth clone or real secrets.
"""
import base64
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import subprocess
import sys

sys.dont_write_bytecode = True
base, label, pg, network = pathlib.Path(sys.argv[1]).resolve(), sys.argv[2], sys.argv[3], sys.argv[4]
if not re.fullmatch(r'/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch('[a-z0-9-]+', label) or not re.fullmatch('butler-issue88-[a-z0-9-]+-history-pg', pg) or not re.fullmatch('butler-issue88-[a-z0-9-]+-history-net', network):
    raise SystemExit('dedicated disposable recovery controls only')
meta = json.loads(subprocess.check_output(['docker', 'inspect', pg]))[0]
if 'POSTGRES_DB=temporal_v9_history_test' not in meta['Config']['Env']:
    raise SystemExit('exact disposable recovery database required')
spec = importlib.util.spec_from_file_location('temporal_recovery', pathlib.Path(__file__).with_name('memory-temporal-auth-recovery.py'))
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)
original = base64.b64encode(b'synthetic-unused-opaque-row-not-a-credential'*2)
latest = base64.b64encode(b'synthetic-late-durable-row-not-a-credential'*2)
count = 0
def execute(query):
    value = subprocess.run(['docker','exec','-i',pg,'psql','-X','-qAt','-h','127.0.0.1','-U','butler','-d','temporal_v9_history_test','-v','ON_ERROR_STOP=1'],input=query,text=True,capture_output=True,timeout=30)
    if value.returncode: raise SystemExit('disposable recovery SQL failed')
    return value.stdout.strip()
def read(query):
    global count
    count += 1
    value=execute('BEGIN TRANSACTION READ ONLY;'+query+';ROLLBACK;')
    return True if value=='t' else False if value=='f' else json.loads(value)
recovery.sql = read  # Test-only injectable storage seam; production fixed SQL unchanged.
execute("CREATE TABLE IF NOT EXISTS public.issue88_native_windows(window_id TEXT PRIMARY KEY);INSERT INTO public.issue88_native_windows VALUES('temporal-explicit-v9') ON CONFLICT DO NOTHING;CREATE SCHEMA IF NOT EXISTS memory_temporal_native_v9;CREATE TABLE memory_temporal_native_v9.system_settings(key TEXT PRIMARY KEY,value TEXT);CREATE TABLE memory_temporal_native_v9.temporal_native_entries(id BIGINT)")
execute("INSERT INTO memory_temporal_native_v9.system_settings VALUES('BUTLER_PROVIDER_AUTH_ANTIGRAVITY','"+original.decode()+"')")
def fixture(suffix):
    name=label+'-'+suffix
    proof, private = base/'reports'/name, base/'private'/name
    proof.mkdir(mode=0o700);private.mkdir(mode=0o700)
    pins=proof/'frozen-pins.json';recovery.exclusive(pins,b'{"window_revision":9,"facts_contract_revision":"5"}')
    raw=(hashlib.sha256(pins.read_bytes()).hexdigest()+'  '+str(pins)+'\n').encode()
    recovery.exclusive(proof/'prepared-pins.sha256',raw)
    recovery.exclusive(private/'auth-import.value',original);recovery.exclusive(private/'expected-main.value',original)
    return name,proof,private,hashlib.sha256(raw).hexdigest()
def invoke(f):
    name,proof,private,digest=f
    sys.argv=['recovery',str(base),name,digest]
    recovery.main()
def denies(f):
    try: invoke(f)
    except SystemExit: return
    raise SystemExit('unsafe synthetic recovery accepted')
results=[]
f=fixture('late');worker='butler-issue88-'+f[0]+'-native-v9'
created=False
try:
    statement="UPDATE memory_temporal_native_v9.system_settings SET value='"+latest.decode()+"' WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY';"
    command=['docker','run','-d','--pull','never','--name',worker,'--network',network,'--cpus','.1','--memory','64m','--pids-limit','32','pgvector/pgvector:pg17','sh','-c','sleep 3; exec psql -h postgres -U butler -d temporal_v9_history_test -v ON_ERROR_STOP=1 -qAt -c "$1"','controls',statement]
    subprocess.run(command,capture_output=True,check=True,timeout=30);created=True
    before=count;denies(f)
    if count!=before or (f[2]/'auth-return.value').exists(): raise SystemExit('running collector reached opaque storage')
    results.append('actual_running_denied_before_sql')
    subprocess.run(['docker','wait',worker],capture_output=True,check=True,timeout=30)
    invoke(f)
    if (f[2]/'auth-return.value').read_bytes()!=latest:raise SystemExit('early snapshot returned instead of final durable refresh')
    results.append('actual_exited_reads_late_durable_refresh')
    denies(f);results.append('one_use_return_denies_repeat')
    subprocess.run(['docker','rm',worker],capture_output=True,check=True,timeout=30);created=False
    f2=fixture('absent');invoke(f2)
    if (f2[2]/'auth-return.value').read_bytes()!=latest:raise SystemExit('removed collector lost durable refresh')
    results.append('absent_collector_reads_durable_refresh')
    execute('DELETE FROM memory_temporal_native_v9.system_settings')
    f3=fixture('imported-missing');recovery.exclusive(f3[1]/'provider-auth-imported.json',b'{}');denies(f3)
    results.append('missing_row_after_import_denies_unused_fallback')
    f4=fixture('unused');invoke(f4)
    if (f4[2]/'auth-return.value').read_bytes()!=original:raise SystemExit('unused original bytes changed')
    results.append('preimport_unused_original_retained')
    f5=fixture('attempt-missing');execute('INSERT INTO memory_temporal_native_v9.temporal_native_entries VALUES(1)');denies(f5)
    results.append('native_entry_without_recoverable_row_denies')
finally:
    if created: subprocess.run(['docker','rm','-f',worker],capture_output=True,timeout=30)
out=base/'reports'/label/'auth-recovery-lab-results.json'
recovery.exclusive(out,json.dumps({'pass':len(results),'fail':0,'skip':0,'checks':results,'native_calls':0,'auth_imported':False,'synthetic_opaque_only':True,'retained_stores_modified':False},sort_keys=True).encode())
print(json.dumps({'pass':len(results),'fail':0,'skip':0,'native_calls':0,'auth_imported':False}))
