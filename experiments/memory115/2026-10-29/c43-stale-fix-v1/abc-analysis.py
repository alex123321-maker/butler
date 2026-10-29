"""Read-only accounting of sealed synthetic traces; no inference or auth inputs."""
import json,pathlib,collections,sys
sys.path.insert(0,'/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
from guided_full_owned_export_v5 import current_run
r=pathlib.Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1/review-snapshot-guided-full-v5/actual')
out=pathlib.Path('/srv/gitea/butler-acceptance/issue115-c43-stale-20261029-v1/reports')
slots={x['id']:x for f in r.glob('slot-*-start.json') for x in [json.loads(f.read_text())]}
entries=collections.defaultdict(list)
for f in r.glob('entry-*-end.json'):
 x=json.loads(f.read_text()); entries[x['slot']].append(x)
cases=[]
for slot,s in sorted(slots.items(),key=lambda v:v[1]['sequence']):
 n=s['sequence']
 if n<10:continue
 o=json.loads((r/f'case-{n:03d}-outcome.json').read_text());d=json.loads((r/f'case-{n:03d}-database.json').read_text());run=current_run(d,o)
 ts=[t for t in d['tool_calls'] if t['run_id']==run]
 es=sorted(entries[slot],key=lambda x:x['sequence'])
 counts=collections.Counter(t['tool_name'] for t in ts)
 searches=[json.dumps(t['args'],sort_keys=True,ensure_ascii=False) for t in ts if t['tool_name']=='memory.search']
 opens=[t for t in ts if t['tool_name']=='memory.open']
 repeated_opens=collections.Counter(json.dumps(t['args'],sort_keys=True) for t in opens)
 seen=set();sent=0;repeated=0
 for e in es:
  request=e['request']
  for item in request.get('InputItems',request.get('ToolResults',[])):
   raw=json.dumps(item,sort_keys=True,ensure_ascii=False).encode(); sent+=len(raw)
   if raw in seen:repeated+=len(raw)
   seen.add(raw)
 cases.append({'sequence':n,'case':s['case_id'],'arm':s['arm'],'repetition':s['repetition'],'native_calls':len(es),'input_tokens':sum((e.get('usage')or{}).get('InputTokens',0) for e in es),'output_tokens':sum((e.get('usage')or{}).get('OutputTokens',0) for e in es),'thinking_tokens':sum((e.get('usage')or{}).get('ThinkingTokens',0) for e in es),'full_tokens':sum((e.get('usage')or{}).get('TotalTokens',0) for e in es),'native_unknown':sum(e.get('usage')is None for e in es),'provider_ms':sum(e['milliseconds'] for e in es),'tool_calls':dict(sorted(counts.items())),'repeated_identical_searches':len(searches)-len(set(searches)),'repeated_identical_open_calls':sum(v-1 for v in repeated_opens.values()),'open_max_chars':[t['args'].get('max_chars') for t in opens],'open_offsets':[t['args'].get('offset',0) for t in opens],'request_item_bytes':sent,'repeated_identical_request_item_bytes':repeated})
 cases[-1]['input_token_sequence']=[(e.get('usage')or{}).get('InputTokens') for e in es]
 cases[-1]['open_calls_per_source']=sorted(collections.Counter(t['args'].get('source_id') for t in opens).values(),reverse=True)
 for k in ['request_item_bytes','repeated_identical_request_item_bytes']:cases[-1].pop(k)
arms={}
for arm in sorted(set(c['arm'] for c in cases)):
 cs=[c for c in cases if c['arm']==arm]; bad=[c for c in cs if c['case']=='control-03']
 def sums(a):return {k:sum(c[k] for c in a) for k in ['native_calls','input_tokens','output_tokens','thinking_tokens','full_tokens','provider_ms','repeated_identical_searches','repeated_identical_open_calls']}
 arms[arm]={'all_controls':sums(cs),'known_failed_path':sums(bad),'other_controls':sums([c for c in cs if c['case']!='control-03']),'tool_calls':dict(sum((collections.Counter(c['tool_calls']) for c in cs),collections.Counter())),'open_max_chars':dict(collections.Counter(str(n) for c in cs for n in c['open_max_chars']))}
result={'format':'butler115.sealed-trace-attribution.v1','new_model_requests':0,'all_controls':48,'shared_preparation_full_tokens':302954,'arms':arms,'cases':cases,'request_limit':'Resume records contain only the new tool result; no full HTTP wire replay or duplicate prompt byte estimate is made. Input tokens are provider-reported; repeated reads include distinct pages and differently worded searches, not necessarily identical calls.','causal_limit':'Two repetitions, narrow synthetic corpus, no randomized intervention or newly generated counterfactual. C43 is a source candidate pinning defect, not proof that loaded skills degrade reasoning.'}
(out/'abc-trace-attribution.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print(json.dumps(arms,ensure_ascii=False,indent=2))
