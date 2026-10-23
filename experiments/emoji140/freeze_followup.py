"""Build explicit AI labels and freeze group-safe retrieval partitions."""
import hashlib,json,pathlib
p=pathlib.Path(__file__).parent
def write(name,value):
    path=p/name
    with path.open('x') as f:json.dump(value,f,ensure_ascii=False,indent=2)
corpus=json.loads((p/'evidence/corpus-450.json').read_text())['entries']
byid={r['id']:r for r in corpus}
styles={'G':'глянцевый цветной мультфильм','C':'цветной рисунок карандашными штрихами','K':'цветной мультяшный кот с контуром','O':'чёрно-белый плоский значок','B':'синий глянцевый объёмный символ'}
rows=[]
for line in (p/'ai-rest-labels.txt').read_text().splitlines():
    n,subjects,actions,expression,ocr,style,meanings=line.split('|')
    ident='rest-'+n
    row={'id':ident,'sha256':byid[ident]['sha256'],'custom_emoji_id':byid[ident]['custom_emoji_id'],
      'observed':{'subjects':subjects.split(';'),'actions':actions.split(';'),'expression':expression or None,'text':ocr.strip() or None,'style':styles[style],'temporal_sequence':actions.split(';')},
      'inferred':{'possible_meanings':meanings.split(';'),'intensity':None,'ambiguity':style not in ['B','O']},
      'review_notes':'AI contact-sheet observation; continuous animation and independent human review not performed'}
    rows.append(row)
assert len(rows)==307 and len({r['id']for r in rows})==307
write('ai-rest-annotations.json',{'version':'ai-rest-v1','rater':'Codex primary assistant; AI, not human','evidence':'26 numbered sheets; eight ordered frames; prior to retrieval rankings','model_outputs_seen_for_these_items':False,'entries':rows})
allrows=[]
for name in ['ai-annotations.json','ai-food-inventory.json']:
    allrows+=json.loads((p/name).read_text())['entries']
allrows+=rows
assert len(allrows)==450 and set(r['id']for r in allrows)==set(byid)
write('ai-all-annotations.json',{'version':'ai-all-v1','rater':'Codex single AI rater','not_human_gold':True,'scope':'exploratory retrieval; same AI created annotations and query judgments','entries':allrows})
parent={x:x for x in byid}
def find(x):
    while parent[x]!=x:x=parent[x]
    return x
def union(a,b):
    a,b=find(a),find(b);parent[max(a,b)]=min(a,b)
groups=['r009 r167 r214 p052','r013 r129','r042 r277','r056 r098','r032 r262','r064 r158','r093 r144 r248 r304','r123 r197','r020 r269','r102 r201','r148 r291','r033 r206 r227 r139 r255 r239 p057 p060','r254 p051','r301 p054','r300 p055 p058','r181 p053 p056 p059']
review=[]
for group in groups:
    ids=[{'r':'rest-','p':'pilot-'}[s[0]]+s[1:]for s in group.split()]
    for ident in ids[1:]:union(ids[0],ident)
    review.append({'ids':ids,'reason':'conservative AI visual grouping of related line/filled icon designs; over-grouping is possible'})
hashid={r['sha256']:r['id']for r in corpus}
for pair in json.loads((p/'evidence/near-duplicates-reviewed.json').read_text())['pairs']:
    ids=[hashid[pair['a']],hashid[pair['b']]];union(*ids);review.append({'ids':ids,'reason':pair['review']})
members={}
for ident in byid:members.setdefault(find(ident),[]).append(ident)
dev={g for g,ids in members.items()if any(x.startswith('pilot-')for x in ids)}
hold={g for g,ids in members.items()if any(x.startswith('food-')for x in ids)}
assert not dev&hold
cal=set();count=0
eligible=sorted(set(members)-dev-hold,key=lambda g:hashlib.sha256(('emoji140-followup-v1:'+g).encode()).hexdigest())
for g in eligible:
    if count+len(members[g])<=90:cal.add(g);count+=len(members[g])
assert count==90
for row in corpus:
    g=find(row['id']);row['duplicate_group']=g;row['split']='development' if g in dev else 'calibration' if g in cal else 'test'
write('evidence/split-v2.json',{'version':'group-safe-exploratory-v2','seed':'emoji140-followup-v1','scope':'AI evaluation; not an independently rated locked test','deviation':'prior 60 pilot items plus conservative near designs remain development; test can be below300','held_out_pack':'Strawberry_hot_by_fStikBot','counts':{s:sum(r['split']==s for r in corpus)for s in ['development','calibration','test']},'reviewed_groups':review,'near_duplicate_recall':'unknown; whole corpus visually reviewed, conservative icon groups added','entries':corpus})
print(json.dumps({s:sum(r['split']==s for r in corpus)for s in ['development','calibration','test']}))
