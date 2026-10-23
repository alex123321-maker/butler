"""Export a source-blinded original450 pool for independent future grading."""
import hashlib,json,pathlib
p=pathlib.Path(__file__).parent
rankings=json.loads((p/'evidence/retrieval-top10.json').read_text())
queries=json.loads((p/'queries-frozen.json').read_text())['entries'];rows=[]
for q in queries:
    if q['split']=='kolobki':continue
    candidates=set(q['seed_relevance'])
    for result in rankings[q['id']].values():candidates.update(r['id'] for r in result)
    for ident in sorted(candidates,key=lambda i:hashlib.sha256((q['id']+'/'+i+'/pool140-v1').encode()).hexdigest()):
        rows.append({'query_id':q['id'],'query':q['text'],'candidate_id':ident,'grade':None})
with (p/'evidence/original450-review-pool.json').open('x') as f:json.dump({'version':'blind-pool-template-v1','status':'not_graded','source':'union top10 all six measured systems plus known seeds','purpose':'independent future judgment; no system names or rank scores in rows','entries':rows},f,ensure_ascii=False,indent=2)
print(json.dumps({'queries':120,'ungraded_pairs':len(rows)}))
