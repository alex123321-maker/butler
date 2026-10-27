"""Reuse existing public-only tests under a scoped JSON representation setting."""
from pathlib import Path
import subprocess,hashlib,json,concurrent.futures,time,re,datetime,os
ROOT=Path(__file__).resolve().parents[2];OWN=ROOT/'.cache/issue76-independent';OUT=OWN/'literal-v7-existing-public-regressions-run01';OUT.mkdir(exist_ok=False)
def H(b):return hashlib.sha256(b).hexdigest()
old=json.loads((OWN/'public-local-response-checker-v4-receipt-v1.json').read_bytes())
tasks=[]
for e in old['public_tests']:
 p=OWN/e['script']
 if H(p.read_bytes())!=e['script_sha256']:raise RuntimeError('Existing public test source differs')
 tasks.append({'script':e['script'],'expected':e['expected_cases'],'original_expected_pass':e['passed'],'source_sha256':e['script_sha256'],'args':[],'group':'existing_original_public298'})
tasks.append({'script':'test-literal-checker-v6.ps1','expected':22,'original_expected_pass':22,'source_sha256':H((OWN/'test-literal-checker-v6.ps1').read_bytes()),'args':['-PublicOutputRoot',str(OUT/'literal22-public-files')],'group':'existing_literal22'})
tasks.append({'script':'check-literal-actual-bridge-v1.ps1','expected':12,'original_expected_pass':12,'source_sha256':H((OWN/'check-literal-actual-bridge-v1.ps1').read_bytes()),'args':['-PublicOutputRoot',str(OUT/'actual-public-bridge-files'),'-PublicInputRoot',str(ROOT/'.cache/behavior76/literal-actual-proofs-v9')],'group':'retained_actual_public_Go_to_PS12'})
def work(t):
 log=OUT/(t['script']+'.log');start=time.monotonic()
 cmd=['pwsh','-NoProfile','-File',str(OWN/'run-scoped-public-json-v7.ps1'),'-PublicScript',str(OWN/t['script']),*t['args']]
 with log.open('xb') as f:done=subprocess.run(cmd,cwd=ROOT,stdout=f,stderr=subprocess.STDOUT)
 raw=log.read_bytes();text=raw.decode('utf-8',errors='replace')
 patterns=[r'SELFTEST_PASS\s+(\d+)',r'"PASS"\s*:\s*(\d+)',r'"passed"\s*:\s*(\d+)']
 observed=[]
 for pattern in patterns:observed += [int(x) for x in re.findall(pattern,text)]
 ok=done.returncode==0 and t['expected'] in observed
 result={**{k:v for k,v in t.items() if k!='args'},'exit':done.returncode,'observed_summary_counts':observed,'pass':ok,'elapsed_seconds':round(time.monotonic()-start,3),'log':str(log.relative_to(ROOT)).replace('\\','/'),'log_sha256':H(raw),'log_bytes':len(raw),'scope':'ALL nested ConvertFrom-Json command invocations receive DateKind String default; no original file edited.'}
 with (OUT/(t['script']+'-exit.json')).open('x',encoding='utf-8') as f:json.dump(result,f,indent=2)
 print(json.dumps({'completed_public_script':t['script'],'pass':ok,'expected_cases':t['expected'],'exit':done.returncode}),flush=True)
 return result
results=[]
with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
 for future in concurrent.futures.as_completed([pool.submit(work,t) for t in tasks]):results.append(future.result())
results.sort(key=lambda r:r['script'])
receipt={'version':'issue76-json-token-v7-existing-public-regressions-v1','status':'PREPARED_ONLY_NO_ACTUAL_R1_REANALYSIS','issued_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'groups':{'original_public_expected_cases':sum(t['expected'] for t in tasks if t['group']=='existing_original_public298'),'literal_public_expected_cases':22,'actual_saved_public_bridge_expected_checks':12},'passed_cases':sum(r['expected'] for r in results if r['pass']),'failed_script_groups':sum(not r['pass'] for r in results),'results':results,'original_frozen_execution_FAIL_retained':True,'old11_runtime_and27_predicates_not_edited':True,'actual_closed_r1_corpus_answers_goldens_mapping_not_opened':True,'Go_PG_Docker_provider_auth_native_calls':0,'only_new_public_GUID_or_requested_CreateNew_output_paths_written':True,'note':'Existing original public scripts and their negative source/clock/authority/stop/UNKNOWN/proof cases executed unchanged; no private study rows or judgments evaluated.'}
p=OUT/'receipt.json'
with p.open('x',encoding='utf-8') as f:json.dump(receipt,f,indent=2)
print(json.dumps({'receipt':str(p.relative_to(ROOT)).replace('\\','/'),'sha256':H(p.read_bytes()),'passed_cases':receipt['passed_cases'],'failed_script_groups':receipt['failed_script_groups'],'actual_r1_reanalysis_run':False}),flush=True)
raise SystemExit(0 if receipt['failed_script_groups']==0 else 1)
