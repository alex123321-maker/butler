"""Aggregate planned denominators without mixing prompt versions or hiding failures."""
import json,pathlib,re,statistics
root=pathlib.Path(__file__).parent/"evidence"
groups={"nf4_v1_multi":["nf4-feasibility-01","nf4-pilot-rest-01"],"nf4_ru_v2_multi":["nf4-ru-v2-01"],"nf4_ru_v3_multi":["nf4-ru-v3-01"],"nf4_ru_v3_single":["nf4-ru-v3-single-01"]}
def q(values,which):
 if not values:return None
 a=sorted(values);k=(len(a)-1)*which;i=int(k)
 return a[i]+(a[min(i+1,len(a)-1)]-a[i])*(k-i)
report={};full={}
for group,dirs in groups.items():
 windows=[];rows=[];planned=0;loads=[]
 for name in dirs:
  path=root/name
  if not (path/"config.json").exists():continue
  config=json.loads((path/"config.json").read_text());planned+=config["planned"]
  attempts=[json.loads(p.read_text())for p in sorted(path.glob("pilot-*.json"))]
  rows.extend(attempts);load=json.loads((path/"load.json").read_text())if (path/"load.json").exists()else None
  if load:loads.append(load)
  windows.append({"name":name,"config":config,"load":load,"attempts":attempts})
 full[group]=windows
 lat=[r["total_seconds"]for r in rows if "total_seconds"in r]
 first=[r["first_token_seconds"]for r in rows if r.get("first_token_seconds")is not None]
 def observed(r):return r.get("parsed",{}).get("observed",{})
 def inferred(r):return r.get("parsed",{}).get("inferred",{})
 report[group]={"planned":planned,"attempt_records":len(rows),"missing_planned":planned-len(rows),"schema_valid":sum(r.get("schema_valid",False)for r in rows),"truncated":sum(r.get("truncated",False)for r in rows),"failed":sum(r["status"]=="failed"for r in rows),"empty_observed_among_parsed":sum("parsed"in r and not any(observed(r).values())for r in rows),"not_parsed":sum("parsed"not in r for r in rows),"with_style":sum(bool(observed(r).get("style"))for r in rows),"with_text":sum(bool(observed(r).get("text"))for r in rows),"with_meanings":sum(bool(inferred(r).get("possible_meanings"))for r in rows),"contains_cyrillic_proxy_not_language_grade":sum(bool(re.search("[А-Яа-яЁё]",json.dumps(r.get("parsed",{}),ensure_ascii=False)))for r in rows),"latency_seconds":{"p50":q(lat,.5),"p95":q(lat,.95),"mean":statistics.mean(lat)if lat else None,"denominator":len(lat)},"first_token_seconds":{"p50":q(first,.5),"p95":q(first,.95),"denominator":len(first)},"input_tokens":sum(r.get("input_tokens",0)for r in rows),"output_tokens":sum(r.get("output_tokens",0)for r in rows),"peak_gpu_allocated_mib":max([r.get("peak_gpu_allocated_bytes",0)/1048576 for r in rows]+[0]),"peak_gpu_reserved_mib":max([r.get("peak_gpu_reserved_bytes",0)/1048576 for r in rows]+[0]),"peak_process_rss_mib":max([r.get("rss_peak_kib",0)/1024 for r in rows]+[0]),"load_seconds":[x["load_seconds"]for x in loads],"quality_grade":"not independently evaluated"}
report["notes"]=["Language/style/text/meaning counts measure populated fields, not correctness.","Cyrillic presence does not prove Russian language compliance.","All examples are development; v2 was revised after examining v1.","No captioner, embedder or precision equivalence is established."]
(root/"caption-metrics.json").write_text(json.dumps(report,indent=2))
(root/"caption-results.json").write_text(json.dumps(full,ensure_ascii=False,indent=2))
print(json.dumps(report,indent=2))
