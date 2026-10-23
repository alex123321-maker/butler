"""Bounded offline exact retrieval; AI annotations are explicitly not captions."""
import argparse,hashlib,json,os,pathlib,re,resource,signal,time
os.environ.update(HF_HUB_OFFLINE='1',TRANSFORMERS_OFFLINE='1',TOKENIZERS_PARALLELISM='false')
from infer import gpu_pids
BASE=pathlib.Path('/work/experiments/emoji140')
def flatten(x):
    if isinstance(x,dict):return ' '.join(flatten(v) for v in x.values())
    if isinstance(x,list):return ' '.join(flatten(v) for v in x)
    return x if isinstance(x,str) else ''
def tokens(s):return set(re.findall(r'[^\W_]+',s.casefold().replace('ё','е')))
def lexical(query,docs):
    q=tokens(query)
    return [len(q & tokens(d))/max(1,len(q)) for d in docs]
def ranking(scores,ids):return sorted(range(len(ids)),key=lambda i:(-float(scores[i]),ids[i]))
def rrf(a,b,ids):
    values=[0.0]*len(ids)
    for scores in [a,b]:
        for rank,i in enumerate(ranking(scores,ids),1):values[i]+=1/(60+rank)
    return values
def main():
    parser=argparse.ArgumentParser();parser.add_argument('--name',required=True);args=parser.parse_args()
    dest=pathlib.Path('/work/.cache/research')/args.name;dest.mkdir(mode=0o700)
    lock=json.loads((BASE/'evidence/retrieval-input-lock.json').read_text())
    for name,digest in lock['files'].items():
        assert hashlib.sha256((BASE/name).read_bytes()).hexdigest()==digest
    corpus=json.loads((BASE/'evidence/corpus-450.json').read_text())['entries']+json.loads((BASE/'evidence/corpus-kolobki.json').read_text())['entries']
    annotations=json.loads((BASE/'ai-all-annotations.json').read_text())['entries']+json.loads((BASE/'ai-kolobki-annotations.json').read_text())['entries']
    queries=json.loads((BASE/'queries-frozen.json').read_text())['entries']
    byid={r['id']:r for r in annotations};ids=[r['id'] for r in corpus]
    docs=[flatten({'observed':byid[i]['observed'],'inferred':byid[i]['inferred']}) for i in ids]
    metadata=[r['pack']+' '+r['unicode'] for r in corpus]
    if gpu_pids():raise RuntimeError('gpu_busy')
    import numpy as np,torch
    from sentence_transformers import SentenceTransformer
    torch.set_num_threads(2);torch.manual_seed(0);torch.cuda.set_per_process_memory_fraction(.75)
    started=time.monotonic();modelpath=pathlib.Path('/model')
    weight_sha=hashlib.file_digest((modelpath/'model.safetensors').open('rb'),'sha256').hexdigest()
    config={'version':'exact-ai-retrieval-v1','model':'google/embeddinggemma-2','revision':'914f7f89142e33e77833254d9c9b90c3cef7303b','weight_sha256':weight_sha,'precision':'BF16','dimension':768,'max_seq_length':2048,'batch':4,'query_prompt':'SearchQuery','document_prompt':'Document','image_prompt':None,'image_aggregation':'mean normalized individual frame vectors, then L2 normalization; temporal order lost','backgrounds':['light','dark'],'rrf_k':60,'corpus_scope':'original450 and additional52 ranked separately','systems':{'R0':'metadata lexical','R1_AI':'AI annotation lexical','R2_AI':'AI annotation dense','R3_AI':'fixed RRF lexical+dense','R4_light':'direct image mean light','R4_dark':'direct image mean dark'},'queries':len(queries),'documents':len(corpus),'input_lock_sha256':hashlib.sha256((BASE/'evidence/retrieval-input-lock.json').read_bytes()).hexdigest(),'torch':torch.__version__}
    (dest/'config.json').write_text(json.dumps(config,indent=2))
    t=time.monotonic()
    model=SentenceTransformer(str(modelpath),device='cuda:0',local_files_only=True,trust_remote_code=False,model_kwargs={'torch_dtype':torch.bfloat16,'use_safetensors':True},config_kwargs={'audio_config':None})
    model.max_seq_length=2048
    own=gpu_pids()
    if len(own)!=1:raise RuntimeError('gpu_contention')
    timing={'load_seconds':time.monotonic()-t,'batches':[],'images':[]}
    def deadline(*unused):raise TimeoutError('encode_timeout')
    signal.signal(signal.SIGALRM,deadline)
    def encode(values,**kw):
        if gpu_pids()!=own:raise RuntimeError('gpu_contention')
        signal.alarm(120);t=time.monotonic()
        try:
            v=model.encode(values,batch_size=4,normalize_embeddings=True,convert_to_numpy=True,show_progress_bar=False,**kw)
            torch.cuda.synchronize()
        finally:signal.alarm(0)
        assert v.ndim==2 and v.shape[1]==768 and np.isfinite(v).all()
        timing['batches'].append({'items':len(values),'seconds':time.monotonic()-t,'kind':kw.get('prompt_name','image')})
        return v.astype(np.float32)
    def encode_text(values,prompt):return np.concatenate([encode(values[i:i+4],prompt_name=prompt) for i in range(0,len(values),4)])
    qvectors=encode_text([q['text'] for q in queries],'SearchQuery');dvectors=encode_text(docs,'Document')
    np.savez(dest/'text-vectors.npz',queries=qvectors,documents=dvectors)
    repeat=encode([queries[0]['text']],prompt_name='SearchQuery')
    timing['text_repeat_max_abs_delta']=float(np.max(np.abs(qvectors[0]-repeat[0])))
    print(json.dumps({'text_ready':True,'queries':len(queries),'documents':len(docs)}),flush=True)
    image_vectors={}
    for background in ['light','dark']:
        vectors=[]
        for index,row in enumerate(corpus):
            root=pathlib.Path('/work/.cache/research')/('kolobki-01' if row['id'].startswith('kolobki-') else 'inventory-01')
            folder=root/'frames'/row['sha256'];meta=json.loads((folder/'frames.json').read_text())
            values=[{'image':str(folder/(str(j)+'-'+background+'.png'))} for j in range(len(meta['frames']))]
            t=time.monotonic();v=np.concatenate([encode(values[i:i+4]) for i in range(0,len(values),4)])
            mean=v.mean(0);mean/=np.linalg.norm(mean);vectors.append(mean)
            timing['images'].append({'id':row['id'],'background':background,'frames':len(values),'seconds':time.monotonic()-t})
            if (index+1)%25==0:
                np.save(dest/('image-'+background+'-partial.npy'),np.stack(vectors))
                (dest/'progress.json').write_text(json.dumps({'background':background,'completed':index+1,'planned':len(corpus)}))
                print(json.dumps({'background':background,'completed':index+1}),flush=True)
        image_vectors[background]=np.stack(vectors);np.save(dest/('image-'+background+'.npy'),image_vectors[background])
    results=[]
    for qi,q in enumerate(queries):
        eligible=[i for i,x in enumerate(ids) if x.startswith('kolobki-')==(q['split']=='kolobki')]
        localids=[ids[i] for i in eligible]
        scores={'R0':lexical(q['text'],[metadata[i] for i in eligible]),'R1_AI':lexical(q['text'],[docs[i] for i in eligible]),'R2_AI':(dvectors[eligible]@qvectors[qi]).tolist()}
        scores['R3_AI']=rrf(scores['R1_AI'],scores['R2_AI'],localids)
        for bg,v in image_vectors.items():scores['R4_'+bg]=(v[eligible]@qvectors[qi]).tolist()
        ranked={key:[{'id':localids[i],'score':float(s[i])}for i in ranking(s,localids)]for key,s in scores.items()}
        results.append({'query_id':q['id'],'rankings':ranked})
    (dest/'rankings.json').write_text(json.dumps({'version':'exact-rankings-v1','entries':results},ensure_ascii=False))
    timing.update(total_seconds=time.monotonic()-started,gpu_peak_allocated_bytes=torch.cuda.max_memory_allocated(),gpu_peak_reserved_bytes=torch.cuda.max_memory_reserved(),rss_peak_kib=resource.getrusage(resource.RUSAGE_SELF).ru_maxrss,status='complete')
    (dest/'timing.json').write_text(json.dumps(timing,indent=2));print(json.dumps({k:v for k,v in timing.items() if k not in ['batches','images']}),flush=True)
if __name__=='__main__':main()
