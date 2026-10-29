"""Bounded post-exit provenance scan, not semantic/independent qualification."""
import json,hashlib
from pathlib import Path
r=Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
a=r/'review-snapshot-guided-full-v5/actual'
load=lambda p:json.loads(p.read_bytes())
window=load(a/'window-final.json');prep=load(a/'frozen-preparation.json')
assert len(window['slots'])==57 and len(prep['originals'])==4
seeds=load(r/'source-guided-full-v5/docs/testing/fixtures/issue90/originals-v1.json')['recipient_seed_contract']['canaries']
def strings(v):
    if isinstance(v,str):yield v
    elif isinstance(v,list):
        for x in v:yield from strings(x)
    elif isinstance(v,dict):
        for x in v.values():yield from strings(x)
def source_id(v):return v.get('ID',v.get('source_id',''))
notes=[source_id(x['lesson']) for x in prep['originals']]
assert all(notes)
slots={s['id']:s for s in window['slots']}
rows=[]
for entry in window['entries']:
    slot=slots[entry['slot']]
    if slot['kind']!='control':continue
    text='\n'.join(strings(entry['request']))
    events='\n'.join(strings(entry.get('events',[])))
    arm=slot['arm'];case=slot['case_id'];bad=[]
    if arm.startswith('A_'):
        for i,canary in seeds.items():
            if case=='control-04' and i=='original-04':continue
            if canary in text or canary in events:bad.append('A_original_canary_'+i)
        if any(n in text for n in notes):bad.append('A_original_lesson_reference')
    if case=='control-07':
        if any(c in text or c in events for c in seeds.values()):bad.append('beta_alpha_original_canary')
        if any(n in text for n in notes):bad.append('beta_alpha_lesson_reference')
    if case=='control-08' and (seeds['original-01'] in text or seeds['original-01'] in events):bad.append('deleted_original01_canary')
    rows.append({'sequence':entry['sequence'],'slot_sequence':slot['sequence'],'case':case,'arm':arm,'repetition':slot['repetition'],'violations':bad})
out={'format':'butler115.bounded-context-provenance-scan.v1','native_control_entries_scanned':len(rows),'cases_with_native_entries':len(set(x['slot_sequence'] for x in rows)),'violations':[x for x in rows if x['violations']],'canary_and_A_beta_note_reference_scan_passed':not any(x['violations'] for x in rows),'all_semantic_privacy_criteria_qualified':False,'scope':'Exact declared original canaries in request/event strings and frozen lesson references in A/beta requests. Current original04 state exception retained. Does not replace full source-policy, successful-read provenance or independent review.','native_calls':0}
with (r/'review-output-guided-full-v5/context-provenance-scan.json').open('x') as f:json.dump(out,f,sort_keys=True)
print(json.dumps(out))
