#!/usr/bin/env python3
"""Read-only stdlib research reporting; never reads auth/settings or edits raw runs."""
import argparse, collections, hashlib, json, pathlib

def read(p): return json.loads(p.read_text())
def digest(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def unique_object(pairs):
    d={}
    for k,v in pairs:
        if k in d: raise ValueError('duplicate field')
        d[k]=v
    return d
def main():
    p=argparse.ArgumentParser();p.add_argument('root',type=pathlib.Path);p.add_argument('output',type=pathlib.Path);a=p.parse_args()
    r=a.root;plan=read(r/'out/plan/plan.json');suite=read(r/'out/plan/suite.private.json');actual=r/'out/actual'
    schedule=plan['Slots'];assert len(schedule)==72 and len(suite['cases'])==24
    final={x['Sequence']:x for x in read(actual/'slots-final.json')}
    journal=read(r/'out/journal-complete.private.json');assert [v['seq'] for v in journal]==list(range(1,len(journal)+1))
    seen={};rows=[];totals={arm:collections.Counter() for arm in 'ABC'}
    for scheduled in schedule:
        seq,arm=scheduled['Sequence'],scheduled['Arm'];case=suite['cases'][scheduled['Case']];d=actual/f'slot-{seq:03d}'
        score=read(d/'score.json') if (d/'score.json').exists() else {'task_success':False,'reasons':['missing_observation']}
        execution=read(d/'execution.json') if (d/'execution.json').exists() else {}
        prep=read(d/'preparation.json') if (d/'preparation.json').exists() else {}
        snapshot=read(d/'snapshot.private.json') if (d/'snapshot.private.json').exists() else {'Tables':{}}
        tables=snapshot['Tables'];native=[]
        value_match=None
        if case['family']!='regression' and len(tables.get('finals',[]))==1:
            try:
                f=json.loads(tables['finals'][0]['content'],object_pairs_hook=unique_object)
                if isinstance(f,dict) and set(f)=={'outcome','value','sources','effect','artifact','check'}:
                    value_match=f['outcome']==case['expected']['outcome'] and f['value']==case['expected']['value']
            except (ValueError,TypeError): pass
        for file in sorted(d.glob('native-*.private.json')):
            v=read(file);assert v['slot']==seq and v['sequence'] not in seen;seen[v['sequence']]=v;native.append(v)
        core={x['operation_id']:x for x in tables.get('model_attempts',[])}
        core_missing=0;core_token_mismatches=0
        for v in native:
            observed=core.get(v['operation_id'])
            if observed is None:
                core_missing+=1;continue
            assert observed['run_id']==v['run_id'] and observed['reservation_id']==v['reservation_id']
            if v['usage'] is not None:
                ci,co=observed['actual_input_tokens'],observed['actual_output_tokens']
                core_token_mismatches+=ci is None or co is None or ci!=v['usage']['InputTokens'] or co!=v['usage']['OutputTokens']
        row={'sequence':seq,'arm':arm,'family':case['family'],'condition':case['condition'],'slot_status':final.get(seq,scheduled)['Status'],
             'task_success':score['task_success'],'reasons':score['reasons'],'abstained':score.get('abstained',False),'format_valid':score.get('format_valid',False),
             'projection_error':execution.get('projection_error'),'answer_value_match_diagnostic_only':value_match,
             'elapsed_ms':execution.get('elapsed_ms'),'cold_setup_ms':prep.get('elapsed_ms'),'source_writes':prep.get('source_writes'),
             'tool_calls':dict(collections.Counter(x['tool_name'] for x in tables.get('calls',[]))),
             'native_methods_dispatched':sum(x['dispatched'] for x in native),'unknown_usage':sum(x['dispatched'] and x['usage'] is None for x in native),
             'known_total_tokens':sum(x['usage']['TotalTokens'] for x in native if x['dispatched'] and x['usage'] is not None),
             'known_input_tokens':sum(x['usage']['InputTokens'] for x in native if x['dispatched'] and x['usage'] is not None),
             'known_output_tokens':sum(x['usage']['OutputTokens'] for x in native if x['dispatched'] and x['usage'] is not None),
             'known_thinking_tokens':sum(x['usage'].get('ThinkingTokens',0) or 0 for x in native if x['dispatched'] and x['usage'] is not None),
             'native_failures':sum('error_code' in x for x in native),
             'core_missing_attempts':core_missing,'core_token_mismatches':core_token_mismatches,
             'artifact_hashes':{f.name:digest(f) for f in d.glob('*.json')}}
        if (d/'preparation-snapshot.private.json').exists():
            t=read(d/'preparation-snapshot.private.json');row['scripted_preparation_model_attempts']=len(t['execution_model_attempts']);row['scripted_preparation_tool_calls']=len(t['tool_calls'])
        rows.append(row);c=totals[arm];c['scheduled']+=1;c['task_success']+=row['task_success'];c['abstained']+=row['abstained'];c['format_valid']+=row['format_valid']
        c['observed_slots']+=(d/'score.json').exists()
        c['answer_value_match_diagnostic_only']+=value_match is True
        c['thinking_count_unreported_methods']+=sum(x['dispatched'] and x['usage'] is not None and x['usage'].get('ThinkingTokens') is None for x in native)
        c['execution_time_unknown_slots']+=row['elapsed_ms'] is None;c['cold_time_unknown_slots']+=row['cold_setup_ms'] is None
        for key in ('native_methods_dispatched','unknown_usage','known_total_tokens','known_input_tokens','known_output_tokens','known_thinking_tokens','native_failures','source_writes','elapsed_ms','cold_setup_ms','scripted_preparation_model_attempts','scripted_preparation_tool_calls','core_missing_attempts','core_token_mismatches'):c[key]+=row.get(key) or 0
        for tool,count in row['tool_calls'].items():c['tool:'+tool]+=count
    assert len({(x['Case'],x['Arm']) for x in schedule})==72
    for entry in journal:
        raw=seen.get(entry['seq']);assert entry['body']==raw,('raw/PG journal mismatch',entry['seq'])
        assert entry['dispatched']==raw['dispatched'] and entry['slot']==raw['slot']
    assert len(seen)==len(journal)
    historical={}
    for name,want in plan['HistoricalReceipts'].items():
        path=r/'previous'/name;assert digest(path)==want;v=read(path)
        historical[name]={'sha256':want,'as_reported':{k:v[k] for k in ('inclusive_usage','new_usage','known_full_tokens','native_dispatched','native_methods_dispatched','native_admissions','scored_slots','scheduled','unknown_usage','invoice_cost','calls','tokens') if k in v}}
    report={'root':str(r),'source':plan['Source'],'model':plan['Model'],'scheduled':72,'arms':{k:dict(v) for k,v in totals.items()},
       'difference_B_minus_A_success':totals['B']['task_success']-totals['A']['task_success'], 'difference_C_minus_B_success':totals['C']['task_success']-totals['B']['task_success'],
       'all_slots_reached':all(x['slot_status'] in ('completed','failed') for x in rows),'all_slots_observed':all(x['observed_slots']==24 for x in totals.values()),
       'native_admissions':len(journal),'native_methods_dispatched':sum(v['dispatched'] for v in journal),
       'historical_receipts_verified':historical,'historical_totals_not_added_due_to_shared_allocations':True,
       'plan_sha256':digest(r/'out/plan/plan.json'),'suite_sha256_bytes':digest(r/'out/plan/suite.private.json'),
       'pg_journal_sha256':digest(r/'out/journal-complete.private.json'),'raw_pg_entries_correlated':len(journal),
       'native_quality_qualified':False,'invoice_cost':'unknown','preparation_native_requests':0,'preparation_mode':plan['Preparation'],
       'limitations':['public development corpus, no independent holdout','scripted prepared notes and reviewed skill; autonomous learning and manual authorship cost are unmeasured','strict typed exact-citation/open metric; value-match is a posthoc diagnostic and not task success','implicit provider caching unverified'], 'rows':rows}
    a.output.parent.mkdir(parents=True,exist_ok=True)
    with a.output.open('x') as f:json.dump(report,f,indent=2)
    print(json.dumps({k:v for k,v in report.items() if k not in ('rows','historical_receipts_verified')}))
if __name__=='__main__':main()
