"""Freeze AI query seeds before any retrieval rankings; no human-gold claim."""
import json,pathlib,hashlib
p=pathlib.Path(__file__).parent
def save(name,obj):
    with (p/name).open('x') as f:json.dump(obj,f,ensure_ascii=False,indent=2)
corpus=json.loads((p/'evidence/corpus-kolobki.json').read_text())['entries'];byid={r['id']:r for r in corpus}
rows=[]
for line in (p/'ai-kolobki-labels.txt').read_text().splitlines():
    n,subjects,actions,expression,ocr,notes=line.split('|');ident='kolobki-'+n
    rows.append(dict(byid[ident],observed={'subjects':subjects.split(';'),'actions':actions.split(';'),'expression':expression,'text':ocr or None,'style':'пиксельная анимация; жёлтые колобки с чёрным контуром','temporal_sequence':actions.split(';')},inferred={'possible_meanings':[],'ambiguity':bool(notes)},review_notes=notes))
assert len(rows)==52
save('ai-kolobki-annotations.json',{'version':'ai-kolobki-v1','rater':'single Codex AI; not human gold','evidence':'all52 eight-frame dark contact sheets, manually inspected before rankings','entries':rows})
queries=[]
allids={r['id'] for r in json.loads((p/'evidence/corpus-450.json').read_text())['entries']}|set(byid)
for file in ['queries-v1.txt','queries-kolobki-v1.txt']:
    for line in (p/file).read_text().splitlines():
        ident,split,category,text,grade3,grade2=line.split('|')
        rel={x:3 for x in grade3.split(',') if x};rel.update({x:2 for x in grade2.split(',') if x})
        assert set(rel)<=allids
        # Exclusion in c19 forbids the weaker snowflake match.
        if ident=='c19':rel={k:v for k,v in rel.items() if v==3}
        queries.append(dict(id=ident,split=split,category=category,text=text,seed_relevance=rel,no_match=category=='no_match'))
assert len(queries)==145 and len({q['id'] for q in queries})==145
save('queries-frozen.json',{'version':'exploratory-query-seeds-v1','independent_human_gold':False,'pool_judgments_complete':False,'scope':'same AI saw source sheets and wrote queries; seed positives are incomplete; Kolobki requests measured separately','entries':queries})
names=['ai-all-annotations.json','ai-kolobki-annotations.json','queries-frozen.json','evidence/corpus-450.json','evidence/corpus-kolobki.json','evidence/split-v2.json']
save('evidence/retrieval-input-lock.json',{'version':'input-lock-v1','files':{n:hashlib.sha256((p/n).read_bytes()).hexdigest() for n in names}})
print(json.dumps({'queries':len(queries),'annotations':450+len(rows),'frozen_before_retrieval':True}))
