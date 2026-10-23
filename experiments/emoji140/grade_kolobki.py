"""Explicit single-AI grades over the entire inspected 52-item cohort."""
import json,pathlib
p=pathlib.Path(__file__).parent
queries=[q for q in json.loads((p/'queries-frozen.json').read_text())['entries'] if q['split']=='kolobki']
ids=[r['id'] for r in json.loads((p/'evidence/corpus-kolobki.json').read_text())['entries']]
related={'k03':[12],'k04':[11,23,47],'k07':[30],'k08':[30],'k09':[27],'k10':[35],'k11':[29,40],'k13':[30,50,51],'k14':[2,10],'k16':[26],'k20':[38]}
rows=[]
for q in queries:
    grades={i:0 for i in ids};grades.update(q['seed_relevance'])
    for n in related.get(q['id'],[]):grades['kolobki-%03d'%n]=1
    rows.append({'query_id':q['id'],'grades':grades})
with (p/'kolobki-qrels.json').open('x') as f:json.dump({'version':'kolobki-ai-qrels-v1','rater':'same Codex AI as annotations and queries; no independent human','coverage':'all52 candidates for each of25 queries; based on five inspected eight-frame sheets','rankings_seen':False,'continuous_playback':False,'scale':{'0':'wrong','1':'related but unsuitable','2':'suitable','3':'exact'},'entries':rows},f,ensure_ascii=False,indent=2)
print(json.dumps({'queries':len(rows),'explicit_grades':len(rows)*len(ids)}))
