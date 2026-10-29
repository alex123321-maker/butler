#!/usr/bin/env python3
"""Read-only recovery of an interrupted experimental journal; never resumes inference.
Stdlib host orchestration only; no credential/auth tables are read.
"""
import argparse,json,pathlib,re,subprocess,time

def run(*args): return subprocess.check_output(args,text=True)
def inspect(name,field):return json.loads(run('docker','inspect','--format','{{json '+field+'}}',name))
def main():
    p=argparse.ArgumentParser();p.add_argument('root',type=pathlib.Path);p.add_argument('--attempt',default='recovery');a=p.parse_args();r=a.root.resolve()
    assert str(r).startswith('/srv/gitea/butler-acceptance/issue115-utility-native-')
    assert re.fullmatch('recovery[-a-z0-9]*',a.attempt)
    out=r/a.attempt;out.mkdir(mode=0o700)
    plan=json.loads((r/'out/plan/plan.json').read_text());h=plan['SuiteSHA256'];assert re.fullmatch('[a-f0-9]{64}',h)
    source='issue115-native-pg-20261028-v1';old=pathlib.Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
    assert run('docker','inspect','--format','{{.Id}}',source).strip()==(old/'native-pg-container.txt').read_text().strip()
    states={}
    for name in (source,r.name+'-runner',r.name+'-pg'):
        try:states[name]=inspect(name,'.State')
        except subprocess.CalledProcessError:states[name]={'unavailable':True}
    with (out/'container-states.json').open('x') as f:json.dump(states,f,indent=2)
    assert not states[r.name+'-runner'].get('Running',False),'do not recover a live producer'
    started=not states[source]['Running']
    if started:run('docker','start',source)
    try:
        for _ in range(180):
            if subprocess.call(['docker','exec',source,'pg_isready','-h','127.0.0.1','-U','postgres','-p','55451'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)==0:break
            time.sleep(1)
        schema='utility_abc_'+h[:16]
        raw=run('docker','exec',source,'psql','-U','postgres','-p','55451','-d','behavior76_test','-Atc',f"SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY seq),'[]') FROM {schema}.entries e")
        entries=json.loads(raw)
        with (out/'journal.private.json').open('x') as f:f.write(raw)
    finally:
        try:clients=int(run('docker','exec',source,'psql','-U','postgres','-p','55451','-d','behavior76_test','-Atc',"SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend' AND pid<>pg_backend_pid()"))
        except subprocess.CalledProcessError:clients=None
        if started and clients==0:run('docker','stop','-t','15',source)
        with (out/'source-cleanup.json').open('x') as f:json.dump({'started_for_read_only_recovery':started,'remaining_clients':clients,'stopped_if_owned_and_idle':started and clients==0,'inference_admitted':False},f)
    rows=[]
    for e in entries:
        assert e['seq']==len(rows)+1
        body=e['body'];files=list((r/'out/actual'/f"slot-{e['slot']:03d}").glob('native-*.private.json'))
        match=[json.loads(x.read_text()) for x in files if json.loads(x.read_text())['sequence']==e['seq']]
        assert len(match)<=1
        if match:assert match[0]==body and match[0]['dispatched']==e['dispatched']
        rows.append({'sequence':e['seq'],'slot':e['slot'],'dispatched':e['dispatched'],'usage':body.get('usage') if body else None,'original_raw_file_present':bool(match),'pg_body_present':body is not None})
    statuses=[]
    for slot in plan['Slots']:
        d=r/'out/actual'/f"slot-{slot['Sequence']:03d}"
        status='unexecuted'
        if d.exists():status='interrupted'
        if (d/'execution.json').exists():
            v=json.loads((d/'execution.json').read_text());status='completed' if v['projection_error']=='<nil>' and v['run_error']=='<nil>' else 'failed'
        statuses.append(dict(slot,Status=status))
    report={'root':str(r),'recovery':'host restart reported by owner; producer is not resumed','native_admissions':len(rows),
      'native_methods_dispatched':sum(x['dispatched'] for x in rows),'known_full_tokens':sum(x['usage']['TotalTokens'] for x in rows if x['dispatched'] and x['usage'] is not None),
      'unknown_usage':sum(x['dispatched'] and x['usage'] is None for x in rows),'original_raw_files_missing':sum(not x['original_raw_file_present'] for x in rows),
      'scheduled':72,'scored_slots':sum((r/'out/actual'/f"slot-{x['Sequence']:03d}"/'score.json').exists() for x in statuses),
      'slot_status_source':'inferred from existing artifacts after reboot; does not replace a producer final marker',
      'slots':statuses,'attempts':rows,'invoice_cost':'unknown','native_quality_qualified':False,'all_original_results_preserved':True}
    # Public receipt deliberately retains counts/nullable usage, not raw model content.
    with (r/'recovery-report.json').open('x') as f:json.dump(report,f,indent=2)
    print(json.dumps({k:v for k,v in report.items() if k not in ('slots','attempts')}))
if __name__=='__main__':main()
