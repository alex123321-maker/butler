#!/usr/bin/env python3
"""Fixed read-only v7 lab successor. Python stdlib is used for bounded Docker
pipes and immutable JSON byte copies; it adds no production admission authority.
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
spec = importlib.util.spec_from_file_location('temporal_v6_export', ROOT/'memory-temporal-successor-history.py')
old = importlib.util.module_from_spec(spec)
spec.loader.exec_module(old)
PRIOR = pathlib.Path('/srv/storage/butler/agents/issue89-native-v3-20261018/reports/prospective-v3-preseal-v2')
HISTORY = pathlib.Path('/srv/storage/butler/agents/issue89-native-v3-20261018/reports/history-v1/history')
HISTORY_SHA = 'cba9612964da448146ae65c08546a8f007c24583e307a2dde1f752463aedea5f'
LEDGER_SHA = 'c04da70cd2688898bbe0440a52226c2879dad6ca6c4929e881219f3fb3f28aff'
SCREEN_SHA = '7305b4ed2e24684a7702203955d659c2925f4f47edfb8d96051674b511a538fb'
CLOSURE_SHA = 'da668e875e995cbf1f14f426e0d667f76ed7aee0b59687823a643d518a1177c1'


def sql(query):
    result = old.subprocess.run(['docker','exec','-i','issue86-20261016-pg','psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','butler','-d','memory_read_test'],input='BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;'+query+';COMMIT;',text=True,capture_output=True,timeout=40)
    if result.returncode:
        raise SystemExit('closed v3 metadata unavailable')
    return json.loads(result.stdout)


def predecessor():
    value = sql("""SET LOCAL search_path=consolidation_native_v3,public;
SELECT jsonb_build_object('closed',(SELECT closed FROM consolidation_screen_window WHERE singleton),'paid_entries',(SELECT entries FROM consolidation_screen_window WHERE singleton),
'slots',(SELECT coalesce(jsonb_agg(jsonb_build_object('ID',slot_id,'State',state,'Definition',jsonb_build_object('ID',definition->'ID','CaseID',definition->'CaseID','Kind',definition->'Kind','Arm',definition->'Arm','State',definition->'State','Session',definition->'Session','RunID',definition->'RunID','Limit',definition->'Limit','StartedAt',definition->'StartedAt','FinishedAt',definition->'FinishedAt')) ORDER BY definition->>'CaseID',CASE definition->>'Arm' WHEN 'preparation' THEN 0 WHEN 'baseline' THEN 1 ELSE 2 END),'[]') FROM consolidation_screen_slots),
'calls',(SELECT coalesce(jsonb_agg(jsonb_build_object('entry_id',entry_id,'operation_id',operation_id,'reservation_id',observation->>'ReservationID','run_id',run_id,'slot_id',slot_id,'kind',kind,'observation_json',jsonb_build_object('EntryID',observation->'EntryID','OperationID',observation->'OperationID','ReservationID',observation->'ReservationID','Usage',observation->'Usage','FinishedAt',observation->'FinishedAt')) ORDER BY created_at,entry_id),'[]') FROM consolidation_screen_calls))""")
    if value.get('closed') is not True or not isinstance(value.get('paid_entries'),int) or not 0 <= value['paid_entries'] <= 99 or len(value.get('calls',[])) != value['paid_entries'] or len(value.get('slots',[])) != 9:
        raise SystemExit('closed complete nine-slot metadata required')
    # Order is anchored to the original pre-auth opaque schedule, not SQL IDs.
    schedule = json.loads(old.artifact(PRIOR/'screen/initial-schedule.json'))
    by_id = {s['ID']:s for s in value['slots']}
    if len(by_id)!=9 or set(by_id)!={s['ID'] for s in schedule['slots']}:
        raise SystemExit('original preauth/live slot population differs')
    value['slots'] = [by_id[s['ID']] for s in schedule['slots']]
    return value


def closed_handback():
    raw = old.frozen(PRIOR/'closed-handback.json',CLOSURE_SHA)
    value = json.loads(raw)
    returned, main, snapshot = value['return_safe'], value['main_auth_safe'], value['main_snapshot_safe']
    if value['native_status'] not in (0,1) or returned.get('opaque_main_cas') is not True or returned.get('active_work_checked') is not True or main.get('resolved') is not True or main.get('unsafe_refresh') is not False or main.get('expired') is not False or snapshot.get('fresh_main_snapshot_saved') is not True or not any(t.get('stage')=='paid_discovery' and t.get('outcome')=='completed' and t.get('http_status')==200 for t in main['trace']):
        raise SystemExit('exact original closed handback unavailable')
    for name,key in [('return-safe.json','return_safe'),('main-auth-safe.json','main_auth_safe'),('main-snapshot-safe.json','main_snapshot_safe')]:
        if json.loads(old.artifact(PRIOR/name)) != value[key]:
            raise SystemExit('original handback witness changed')
    if int(old.artifact(PRIOR/'native-status')) != value['native_status']:
        raise SystemExit('original native status changed')
    return raw


def future():
    def query(text):
        result=old.subprocess.run(['docker','exec','-i','butler-issue85-postgres','psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','butler','-p','18585','-d','issue85_native_test'],input='BEGIN TRANSACTION READ ONLY;'+text+';ROLLBACK;',text=True,capture_output=True,timeout=40)
        if result.returncode:
            raise SystemExit('fresh v7 namespace/receipt proof unavailable')
        return json.loads(result.stdout)
    state=query("SELECT jsonb_build_object('schema','memory_temporal_native_v7','window_id','temporal-explicit-v7','schema_present',to_regnamespace('memory_temporal_native_v7') IS NOT NULL,'receipt_table_present',to_regclass('public.issue88_native_windows') IS NOT NULL)")
    state['receipt_count']=query("SELECT count(*) FROM public.issue88_native_windows WHERE window_id='temporal-explicit-v7'") if state['receipt_table_present'] else 0
    if state['schema_present'] is not False or state['receipt_count']!=0:
        raise SystemExit('v7 namespace or oneuse receipt exists; no clone')
    return state


def main():
    mode=sys.argv[1]
    if mode=='check-current':
        path=pathlib.Path(sys.argv[2]).resolve()
        if not re.fullmatch('/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/history/history.json',str(path)):
            raise SystemExit('bounded v7 history required')
        raw=path.read_bytes()
        if hashlib.sha256(raw).hexdigest()!=sys.argv[3]:raise SystemExit('frozen v7 history differs')
        m=json.loads(raw)
        if m.get('format')!='butler.temporal.native-history.v3' or m['day']!=datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%d'):
            raise SystemExit('frozen v7 UTC day expired')
        for key,value in [('current',old.current(m['day'])),('all_owners',old.all_owners(m['day']))]:
            if old.referenced(path.parent,m[key])[1]!=old.canonical(value):raise SystemExit('seven-store population changed; new freeze required')
        prior=json.loads(old.referenced(path.parent,m['predecessor89'])[1])
        if any(prior[k]!=v for k,v in predecessor().items()):raise SystemExit('closed v3 population changed')
        if future()!=m['future_window'] or closed_handback()!=old.referenced(path.parent,m['predecessor89_closure'])[1]:raise SystemExit('future/closure changed')
        for key,path89,sha in [('original89_ledger',PRIOR/'screen/full-ledger.json',LEDGER_SHA),('original89_screen',PRIOR/'screen/screen.json',SCREEN_SHA)]:
            if old.referenced(path.parent,m[key])[1]!=old.frozen(path89,sha):raise SystemExit('original v3 witness changed')
        if old.referenced(path.parent,m['original89_schedule'])[1]!=old.artifact(PRIOR/'screen/initial-schedule.json'):raise SystemExit('original v3 schedule changed')
        print(json.dumps({'read_only':True,'current_day_unchanged':True,'seven_databases':7,'day':m['day'],'auth_or_inference':False}))
        return
    if mode!='prepare':raise SystemExit('prepare or check-current required')
    base,label=pathlib.Path(sys.argv[2]).resolve(),sys.argv[3]
    if not re.fullmatch('/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+',str(base)) or not re.fullmatch('[A-Za-z0-9_-]+',label):raise SystemExit('bounded v7 target required')
    proof=base/'reports'/label/'history'
    proof.mkdir(mode=0o700,parents=True)
    now=datetime.datetime.now(datetime.timezone.utc)
    day=now.strftime('%Y-%m-%d');deadline=datetime.datetime.combine(now.date()+datetime.timedelta(days=1),datetime.time(),datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    previous=old.frozen(HISTORY/'history.json',HISTORY_SHA)
    m=json.loads(previous)
    if m.get('format')!='butler.consolidation.native-history.v4' or m['day']!=day or m['deadline']!=deadline:raise SystemExit('same-day recursive original v3 history required')
    copied={};pending=[old.reference('history.json',previous)]
    while pending:
        ref=pending.pop();name,raw=old.referenced(HISTORY,ref)
        target='consolidation-v3-history.json' if name=='history.json' else name
        if target in copied:
            if copied[target]!=raw:raise SystemExit('recursive lineage collision')
            continue
        old.write(proof/target,raw);copied[target]=raw;pending.extend(old.references(json.loads(raw)))
    closure=closed_handback()
    inventory,all_inventory=old.current(day),old.all_owners(day)
    matches=[w for db in inventory['databases'] if db['container']=='issue86-20261016-pg' and db['database']=='memory_read_test' for w in db['windows'] if w['schema']=='consolidation_native_v3']
    if len(matches)!=1:raise SystemExit('exact latest inclusive v3 leaf required')
    prior=predecessor();prior.update({'format':'butler.temporal.predecessor89.v3',**{key:matches[0][key] for key in ('scopes','reservations','foreground','background')}});prior['model_jobs']=prior['background']
    files={'current-v7-owner-day.json':old.canonical(inventory),'all-seven-v7-owner-ledgers.json':old.canonical(all_inventory),'predecessor89-v3-current.json':old.canonical(prior),'native89-v3-full-ledger.json':old.frozen(PRIOR/'screen/full-ledger.json',LEDGER_SHA),'native89-v3-screen.json':old.frozen(PRIOR/'screen/screen.json',SCREEN_SHA),'native89-v3-initial-schedule.json':old.artifact(PRIOR/'screen/initial-schedule.json'),'native89-v3-closed-handback.json':closure}
    for name,raw in files.items():
        if name in copied:raise SystemExit('new v7 witness collision')
        old.write(proof/name,raw)
    result={'format':'butler.temporal.native-history.v3','owner':'owner','day':day,'deadline':deadline,'schema':'memory_temporal_native_v7','previous89_history':old.reference('consolidation-v3-history.json',previous),'future_window':future(),'rule':'Latest closed inclusive owner109 once; recursive old73 and every failed/unexecuted/unknown original retained. Distinct adaptive226 separately visible and never rebilled. Fixed lab recipe, not production cross-database admission.'}
    for key,name in [('current','current-v7-owner-day.json'),('all_owners','all-seven-v7-owner-ledgers.json'),('predecessor89','predecessor89-v3-current.json'),('original89_ledger','native89-v3-full-ledger.json'),('original89_screen','native89-v3-screen.json'),('original89_schedule','native89-v3-initial-schedule.json'),('predecessor89_closure','native89-v3-closed-handback.json')]:result[key]=old.reference(name,files[name])
    raw=old.canonical(result);old.write(proof/'history.json',raw)
    print(json.dumps({'read_only':True,'history_sha256':hashlib.sha256(raw).hexdigest(),'seven_databases':7,'day':day,'auth_or_inference':False}))


if __name__=='__main__':main()
