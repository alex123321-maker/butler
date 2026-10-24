"""Known-positive diagnostics and separate exhaustive single-AI Kolobki qrels."""
import json,math,pathlib,random,statistics,sys
def success(ranking,rel,k=5):return int(any(rel.get(r['id'],0)>=2 for r in ranking[:k]))
def mrr(ranking,rel,k=10):return next((1/(i+1) for i,r in enumerate(ranking[:k]) if rel.get(r['id'],0)>=2),0.0)
def ndcg(ranking,rel,k=10):
    def dcg(values):return sum((2**g-1)/math.log2(i+2) for i,g in enumerate(values[:k]))
    ideal=dcg(sorted(rel.values(),reverse=True))
    return dcg([rel.get(r['id'],0) for r in ranking])/ideal if ideal else None
def bootstrap(values,seed=140):
    if not values:return None
    rng=random.Random(seed);n=len(values)
    means=sorted(sum(values[rng.randrange(n)] for _ in range(n))/n for _ in range(2000))
    return {'mean':statistics.mean(values),'query_bootstrap_95':[means[49],means[1949]],'n':n,'resamples':2000}
def wilson(k,n):
    if not n:return None
    z=1.959963984540054;p=k/n;den=1+z*z/n;mid=(p+z*z/(2*n))/den;half=z*math.sqrt(p*(1-p)/n+z*z/(4*n*n))/den
    return {'numerator':k,'denominator':n,'rate':p,'wilson_95':[max(0,mid-half),min(1,mid+half)]}
def select_threshold(rows):
    # Only the30 original calibration queries. Permit <=1/10 false acceptance,
    # then maximize positive coverage; ties favor fewer false accepts, larger cutoff.
    candidates=[math.nextafter(max(x['score'] for x in rows),math.inf)]+[x['score'] for x in rows]
    scored=[]
    for t in candidates:
        fa=sum(x['no_match'] and x['score']>=t for x in rows)
        coverage=sum(not x['no_match'] and x['score']>=t for x in rows)
        if fa<=1:scored.append((coverage,-fa,t))
    return max(scored)[2]
def main():
    p=pathlib.Path(__file__).parent;run=pathlib.Path(sys.argv[1])
    qs=json.loads((p/'queries-frozen.json').read_text())['entries']
    rankings={r['query_id']:r['rankings'] for r in json.loads((run/'rankings.json').read_text())['entries']}
    full={r['query_id']:r['grades'] for r in json.loads((p/'kolobki-qrels.json').read_text())['entries']}
    systems=list(next(iter(rankings.values())));summary={'version':'retrieval-diagnostics-v1','limitations':['single AI rater and same query/annotation author','original450 qrels are known-positive seeds, not a judged top10 pool','original metrics are seed-hit lower bounds, not complete relevance or recall','Kolobki all52 qrels are AI contact-sheet judgments, not independent human gold','bootstrap assumes query independence despite shared visual families; intervals are exploratory'],'cohorts':{},'paired_seed_success_delta_to_R0':{},'abstention':{},'errors':{}}
    for split in ['calibration','test','kolobki']:
        positive=[q for q in qs if q['split']==split and not q['no_match']]
        summary['cohorts'][split]={}
        for s in systems:
            values=[success(rankings[q['id']][s],full.get(q['id'],q['seed_relevance'])) for q in positive]
            score={'success_at5':wilson(sum(values),len(values)),'mrr_at10':bootstrap([mrr(rankings[q['id']][s],full.get(q['id'],q['seed_relevance'])) for q in positive])}
            if split=='kolobki':score['ndcg_at10']=bootstrap([ndcg(rankings[q['id']][s],full[q['id']]) for q in positive])
            score['categories']={cat:wilson(sum(success(rankings[q['id']][s],full.get(q['id'],q['seed_relevance'])) for q in positive if q['category']==cat),sum(q['category']==cat for q in positive)) for cat in sorted({q['category'] for q in positive})}
            summary['cohorts'][split][s]=score
            if split!='calibration':
                delta=[success(rankings[q['id']][s],full.get(q['id'],q['seed_relevance']))-success(rankings[q['id']]['R0'],full.get(q['id'],q['seed_relevance'])) for q in positive]
                summary['paired_seed_success_delta_to_R0'][split+'/'+s]=bootstrap(delta)
            summary['errors'][split+'/'+s]=[{'query_id':q['id'],'query':q['text'],'expected_seed':q['seed_relevance'],'top5':rankings[q['id']][s][:5]} for q in positive if not success(rankings[q['id']][s],full.get(q['id'],q['seed_relevance']))]
    summary['paired_comparisons']={}
    for split in ['test','kolobki']:
        positive=[q for q in qs if q['split']==split and not q['no_match']]
        for a,b in [('R2_AI','R1_AI'),('R3_AI','R2_AI'),('R4_light','R2_AI'),('R4_dark','R4_light')]:
            getrel=lambda q:full.get(q['id'],q['seed_relevance'])
            value={'success_at5_difference':bootstrap([success(rankings[q['id']][a],getrel(q))-success(rankings[q['id']][b],getrel(q)) for q in positive])}
            if split=='kolobki':value['ndcg_at10_difference']=bootstrap([ndcg(rankings[q['id']][a],getrel(q))-ndcg(rankings[q['id']][b],getrel(q)) for q in positive])
            summary['paired_comparisons'][split+'/'+a+'-'+b]=value
    for s in systems:
        cal=[{'score':rankings[q['id']][s][0]['score'],'no_match':q['no_match']} for q in qs if q['split']=='calibration']
        threshold=select_threshold(cal);out={'threshold':threshold,'selection':'calibration only; <=1/10 false acceptance, maximal positive coverage, fewer false accepts, higher threshold; no margin tuning','cohorts':{}}
        for split in ['calibration','test','kolobki']:
            subset=[q for q in qs if q['split']==split];negative=[q for q in subset if q['no_match']];positive=[q for q in subset if not q['no_match']]
            accepted=lambda q:rankings[q['id']][s][0]['score']>=threshold
            out['cohorts'][split]={'false_accept':wilson(sum(accepted(q) for q in negative),len(negative)),'positive_coverage':wilson(sum(accepted(q) for q in positive),len(positive)),'accepted_top1_known_correct':wilson(sum(accepted(q) and success(rankings[q['id']][s],full.get(q['id'],q['seed_relevance']),1) for q in positive),sum(accepted(q) for q in positive))}
        if s=='R3_AI':out['transfer_warning']='RRF score distribution depends on candidate count; threshold transfer450 to52 is not qualified'
        summary['abstention'][s]=out
    timing=json.loads((run/'timing.json').read_text());summary['runtime']={k:v for k,v in timing.items() if k not in ['batches','images']}
    summary['runtime']['image_calls']=len(timing['images']);summary['runtime']['encoded_frames']=sum(x['frames'] for x in timing['images'])
    summary['runtime']['encode_batches']=len(timing['batches'])
    summary['runtime']['text_input_counts']={kind:sum(x['items'] for x in timing['batches'] if x['kind']==kind) for kind in ['SearchQuery','Document']}
    for bg in ['light','dark']:
        values=sorted(x['seconds'] for x in timing['images'] if x['background']==bg)
        summary['runtime'][bg]={'items':len(values),'p50_seconds':statistics.median(values),'p95_seconds':values[math.ceil(.95*len(values))-1],'total_seconds':sum(values)}
    target=p/'evidence/retrieval-results.json'
    with target.open('x') as f:json.dump(summary,f,ensure_ascii=False,indent=2)
    # Public top10 only; media and all502 vectors remain private derived artifacts.
    with (p/'evidence/retrieval-top10.json').open('x') as f:json.dump({q:{s:rows[:10]for s,rows in systems.items()}for q,systems in rankings.items()},f,ensure_ascii=False)
    for split,values in summary['cohorts'].items():print(split,{s:v['success_at5']['numerator'] for s,v in values.items()})
if __name__=='__main__':main()
