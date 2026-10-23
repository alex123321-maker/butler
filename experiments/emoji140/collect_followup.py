"""Export safe measured evidence after a completed run, retaining private hashes."""
import datetime,hashlib,importlib.metadata,json,pathlib,statistics,sys
p=pathlib.Path('/work/experiments/emoji140');run=pathlib.Path(sys.argv[1])
timing=json.loads((run/'timing.json').read_text());assert timing['status']=='complete'
def sha(path):
    with path.open('rb') as f:return hashlib.file_digest(f,'sha256').hexdigest()
def save(name,value):
    with (p/'evidence'/name).open('x') as f:json.dump(value,f,ensure_ascii=False,indent=2)
save('retrieval-config.json',json.loads((run/'config.json').read_text()))
save('retrieval-timing.json',timing)
names=['prepare_followup.py','freeze_followup.py','prepare_kolobki.py','freeze_retrieval.py','grade_kolobki.py','retrieve.py','score_retrieval.py','summarize_retrieval.py','build_review_pool.py','ai-rest-labels.txt','ai-kolobki-labels.txt','queries-v1.txt','queries-kolobki-v1.txt','kolobki-qrels.json']
private={f.name:{'sha256':sha(f),'bytes':f.stat().st_size} for f in run.iterdir() if f.is_file()}
save('followup-provenance.json',{'version':'followup-provenance-v1','run':run.name,'recorded_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'files':{name:sha(p/name) for name in names},'private_derived_artifacts':private,'packages':{name:importlib.metadata.version(name) for name in ['torch','transformers','sentence-transformers','Pillow','numpy']},'runtime_image':'sha256:1f8334477430a5d885a9defcbe79ac21367591dbe85f8f9ede4d4b50e45ff502'})
inventory=json.loads((p/'evidence/kolobki-inventory.json').read_text());prep=json.loads((p/'evidence/kolobki-preprocessing.json').read_text())
downloads=sorted(r['download_ms'] for r in inventory['entries']);start=datetime.datetime.fromisoformat(inventory['started_at'].replace('Z','+00:00'));finish=datetime.datetime.fromisoformat(inventory['finished_at'].replace('Z','+00:00'))
save('kolobki-summary.json',{'items':len(inventory['entries']),'downloaded':inventory['downloaded'],'failed':inventory['failed_downloads'],'bytes':sum(r['bytes'] for r in inventory['entries']),'import_seconds':(finish-start).total_seconds(),'download_p50_ms':statistics.median(downloads),'download_p95_ms':downloads[49],'decode_success':sum(r['status']=='decoded' for r in prep),'decode_child_seconds':sum(r['decode_seconds'] for r in prep),'frames':sum(len(r['frames']) for r in prep),'frame_protocol':'8 ordered224px frames; no continuous playback judgment','rater':'single AI'})
print(json.dumps({'safe_evidence_exported':True,'private_artifact_bytes':sum(x['bytes'] for x in private.values())}))
