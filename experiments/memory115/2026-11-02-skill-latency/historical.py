"""Read-only historical timing attribution; never reads credentials or calls models."""
import collections, datetime, hashlib, json, pathlib, sys

root = pathlib.Path(sys.argv[1])
read = lambda p: json.loads(p.read_text())
def stamp(v):
    return datetime.datetime.fromisoformat(v.replace('Z', '+00:00')).timestamp()
report = read(root/'public-report.json')
plan = read(root/'out/plan/plan.json')
rows = []
for row, scheduled in zip(report['rows'], plan['Slots']):
    d = root/'out/actual'/('slot-%03d' % row['sequence'])
    for name in ['snapshot.private.json','timing.json']:
        assert hashlib.sha256((d/name).read_bytes()).hexdigest() == row['artifact_hashes'][name]
    s,t = read(d/'snapshot.private.json'),read(d/'timing.json')
    paths = sorted(d.glob('native-*.private.json'))
    for path in paths:
        assert hashlib.sha256(path.read_bytes()).hexdigest() == row['artifact_hashes'][path.name]
    methods = [read(p) for p in paths]
    last = methods[-1]
    end = stamp(last['started']) + last['provider_boundary_ns']/1e9
    # started precedes boundary start by admission work; elapsed_ms supplies
    # the original pre-serialization boundary to within its millisecond rounding.
    end = stamp(last['started']) + last['elapsed_ms']/1000
    run = s['Tables']['runs'][0]
    wait = sum(n['provider_wait_ns'] for n in methods)/1e9
    write_lag = sum(max(0, (d/('native-%02d.private.json'%n['turn'])).stat().st_mtime
        - stamp(n['started']) - n['elapsed_ms']/1000) for n in methods)
    rows.append({'sequence':row['sequence'],'case_index':scheduled['Case'],'arm':row['arm'],
        'methods':len(methods),'execution_seconds':t['execution_ns']/1e9,
        'provider_wait_seconds':wait,'outside_provider_seconds':t['execution_ns']/1e9-wait,
        'last_response_to_run_finished_seconds':stamp(run['finished_at'] or run['updated_at'])-end,
        'last_response_to_raw_file_write_seconds':(d/('native-%02d.private.json'%last['turn'])).stat().st_mtime-end,
        'all_raw_write_lag_seconds':write_lag,
        'last_response_events':len(last['events']),
        'last_response_delta_events':sum(e['EventType']=='assistant_delta' for e in last['events']),
        'before_first_method_seconds':stamp(methods[0]['started'])-stamp(run['started_at']),
        'between_methods_seconds':sum(stamp(b['started'])-stamp(a['started'])-a['elapsed_ms']/1000 for a,b in zip(methods,methods[1:])),
        'inside_method_local_seconds':sum(n['elapsed_ms']/1000-n['provider_wait_ns']/1e9 for n in methods),
        'outside_run_envelope_seconds':t['execution_ns']/1e9-(stamp(run['finished_at'] or run['updated_at'])-stamp(run['started_at'])),
        'tool_rpc_seconds':sum(x['nanoseconds'] for x in t['spans'] if x['kind']=='tool_rpc')/1e9})
keys=['methods','execution_seconds','provider_wait_seconds','outside_provider_seconds',
    'last_response_to_run_finished_seconds','all_raw_write_lag_seconds','tool_rpc_seconds',
    'before_first_method_seconds','between_methods_seconds','inside_method_local_seconds','outside_run_envelope_seconds']
arms={a:{k:sum(x[k] for x in rows if x['arm']==a) for k in keys} for a in 'ABC'}
print(json.dumps({'root':str(root),'native_requests':0,'arms':arms,
    'C_minus_B':{k:arms['C'][k]-arms['B'][k] for k in keys},'rows':rows,
    'limitations':['Historical file mtimes indicate the write instant, not fsync completion; write lag also includes JSON serialization and PG persistence. Clock subtraction is approximate, unlike original monotonic totals.','Individual historical pinned-RPC spans were not recorded. Tail intervals localize delay but do not alone measure the entire RPC cost.']},indent=2))
