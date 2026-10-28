"""File/Git-only formal freeze. No tests, provider, auth, database or hidden data."""
from pathlib import Path
import subprocess, hashlib, json, re, datetime

ROOT=Path(__file__).resolve().parents[2]
OWN=ROOT/'.cache/issue76-independent'
HEAD='9fb9ceeb13420af0fb49272816df7273f558e3a4'
PUB='experiments/behavior76/2026-10-27'
CHECK=PUB+'/literal-checker-preparation-v2'
PLAN=PUB+'/literal-plan-components-v1'
LAUNCH=PUB+'/literal-launcher-preparation-v1'
def git(*args): return subprocess.check_output(['git','-C',str(ROOT),*args])
def sha(b): return hashlib.sha256(b).hexdigest()
def read(p): return (ROOT/p).read_bytes()
def js(p): return json.loads(read(p))
def need(v,s):
 if not v: raise RuntimeError(s)
def pin(p): return {'path':p,'sha256':sha(read(p)),'bytes':len(read(p))}
def text(p): return read(p).decode('utf-8',errors='replace').replace('\x00','')
def create(name,value):
 p=OWN/name
 with p.open('xb') as f: f.write((json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode('utf-8'))
 return pin(str(p.relative_to(ROOT)).replace('\\','/'))
need(git('rev-parse','HEAD').decode().strip()==HEAD,'producer mismatch')
need(not git('status','--porcelain').strip(),'producer not clean')
need(not list(OWN.glob('hidden-literal-v11*')),'hidden authorship precedes freeze')
old=js('.cache/issue76-independent/semantic-freeze-local-response-v10-r3-v2.json')
deps=js(CHECK+'/public-literal-checker-v6-dependencies-prepared-v2.json')
need(sha(read(CHECK+'/public-literal-checker-v6-dependencies-prepared-v2.json'))=='3680094390c73866e746c23c0d712ef9b34e9e4c0c6ad11c266ed788b33a8c02','deps mismatch')
need(len(deps['unchanged_old7runtime'])==7 and len(deps['new_additive_runtime4'])==4,'runtime count mismatch')
for p in deps['unchanged_old7runtime']+deps['new_additive_runtime4']:
 need(sha(read(CHECK+'/'+p['path']))==p['sha256']==sha(read('.cache/issue76-independent/'+p['path'])),'runtime differs: '+p['path'])
extents=js(CHECK+'/literal-v6-original-criterion-extents-prepared-v1.json')
need(len(extents['statement_extents'])==27 and extents['per_case_types']==25 and extents['old_global_matrix_types']==2 and not extents['changed'],'criterion witness mismatch')
need(extents['statement_extents']==[e for e in old['criterion_extents'] if e['kind']=='statement'],'old27 AST witness changed')
need(extents['original_source_sha256']==sha(read(CHECK+'/score-mechanical-fresh-v3.ps1')),'criterion source changed')
need(sha(read('.cache/issue76-independent/public-literal-v11-launcher-static-review-v3.json'))=='328e28c968f4cc9a4377c23398ebda1fd6c4e3031d9265218437b71f0999acaa','review mismatch')
review=js('.cache/issue76-independent/public-literal-v11-launcher-static-review-v3.json')
for e in review['source_pins']: need(sha(read(e['path']))==e['sha256'],'reviewed source changed: '+e['path'])
all_tracked=git('ls-files','-z').decode().split('\x00')
all_tracked=[p for p in all_tracked if p]
source=set(e['path'] for e in old['source_pins'])
source.update(p for p in all_tracked if p.startswith('apps/orchestrator/internal/app/behavior76') and p.endswith('.go'))
prefixes=['apps/orchestrator/internal/input/','apps/orchestrator/internal/task/','internal/owneridentity/','internal/ingress/','internal/ownership/','internal/actionauth/','apps/orchestrator/internal/orchestrator/','apps/tool-broker/internal/','internal/memory/catalog/']
source.update(p for p in all_tracked if any(p.startswith(q) for q in prefixes) and p.endswith('.go') and not p.endswith('_test.go'))
source.update(['.gitattributes','docs/testing/issue76-literal-supplement-protocol.md','docs/testing/issue76-literal-supplement-proposal.md','scripts/dev/behavior76-literal-launch.ps1','scripts/dev/behavior76-literal-launcher-core.ps1','scripts/dev/behavior76-literal-host.sh','scripts/dev/behavior76-literal-admission.schema.json','scripts/dev/test-behavior76-literal-launcher.ps1','scripts/dev/test-behavior76-literal-host-bind.ps1'])
published=[p for p in all_tracked if p.startswith(PUB+'/')]
paths=sorted(source|set(published))
# One raw Git batch compares physical bytes, avoiding text normalization.
batch=git('cat-file','--batch',input=None) if False else subprocess.run(['git','-C',str(ROOT),'cat-file','--batch'],input=('\n'.join(HEAD+':'+p for p in paths)+'\n').encode(),stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True).stdout
pos=0
for path in paths:
 end=batch.index(b'\n',pos); header=batch[pos:end].split(); need(len(header)==3 and header[1]==b'blob','missing Git blob: '+path)
 count=int(header[2]); start=end+1; data=batch[start:start+count]; need(data==read(path),'physical Git byte mismatch: '+path); need(batch[start+count:start+count+1]==b'\n','Git batch framing');pos=start+count+1
need(pos==len(batch),'unexpected Git blob tail')
for folder in ['literal-checker-preparation-v2','literal-input-observation-v1','literal-launcher-preparation-v1','literal-plan-components-v1']:
 for e in js(PUB+'/'+folder+'/selected-evidence-inventory.json'):
  p=PUB+'/'+folder+'/'+e['path'];need(sha(read(p))==e['sha256'] and len(read(p))==e['bytes'],'published selected inventory mismatch: '+p)
for suffix,want in [('literal-plan-composition-v7',9),('literal-legacy-public-v8',61),('literal-actual-proof-v9',1)]:
 r=js(PLAN+'/'+suffix+'-exit.json'); need(r['test_exit']==0 and r['native_calls']==0 and not r['hidden_controls_bound'],'public exit mismatch')
 log=text(PLAN+'/'+suffix+'.log');need(len(re.findall(r'^--- PASS: Test',log,re.M))==want and not re.search(r'^--- (FAIL|SKIP): Test|WARNING: DATA RACE',log,re.M),'public gate count mismatch: '+suffix)
need(js(PLAN+'/literal-plan-composition-v7-exit.json')['vet_exit']==0,'vet missing')
need('actual literal composition32 arms/68 owner opportunities' in text(PLAN+'/literal-plan-composition-v7.log'),'actual32/68 proof missing')
r=js(LAUNCH+'/literal-admission-time-v10-exit.json');need(r['test_exit']==r['vet_exit']==r['native_calls']==0,'final time guard gate failed')
need(len(re.findall(r'^--- PASS: Test',text(LAUNCH+'/literal-admission-time-v10.log'),re.M))==2,'time guard2 proof missing')
for e in js(LAUNCH+'/literal-admission-time-v10-source-after.json'):
 p=e['path'].replace('\\','/'); p=p[p.index('/apps/')+1:] if '/apps/' in p else p
 need(sha(read(p))==e['sha256'],'time guard source pins changed')
neutral=js(CHECK+'/literal-v6-public-neutral-run05-receipt.json');need(neutral['PASS']==22 and neutral['FAIL']==0,'neutral checker gate mismatch')
bridge=js(PLAN+'/independent-actual-bridge-receipt.json');need(bridge['PASS']==12 and bridge['FAIL']==0 and bridge['checker_Go_PG_Docker_auth_model_executions']==0,'bridge mismatch')
need(sha(read(PLAN+'/independent-actual-bridge-receipt.json'))=='938f18c0b63ef1a6d4c7a786184a3f3097c866b8a7946302070ad3dfc0e442bb','independent bridge not byte-exact')
launch_receipts=[js(p) for p in published if p.startswith(LAUNCH+'/literal-launcher-public-') and p.endswith('-receipt.json')]
need(any(r['pass']==18 and r['fail']==0 for r in launch_receipts),'18 purePS proof missing')
host_receipts=[js(p) for p in published if p.startswith(LAUNCH+'/literal-host-public-') and p.endswith('/receipt.json')]
need(any(r['pass']==9 and r['fail']==0 and not r['actual_ELF_entered'] for r in host_receipts),'9 host-pin proof missing')
types=text('apps/orchestrator/internal/app/behavior76_literal_types_test.go')
for name in ['behavior76LiteralStimuliSHA','behavior76LiteralExpectationsSHA']:
 need(re.search(r'^const '+name+r' = ""$',types,re.M),'new controls already bound')
need(sha(read('apps/orchestrator/internal/prompt/task_continuity.go'))==old['candidate_sha256'],'candidate changed')
old_protocol=git('cat-file','blob',old['producer']+':docs/testing/issue76-local-response-protocol.md')
need(sha(old_protocol)==old['protocol_sha256'],'prior protocol witness mismatch')
new_protocol=read('docs/testing/issue76-local-response-protocol.md')
need(sha(new_protocol)=='a5e5e9f12020867bc9db10e479c874594518701b75611008490656588efd0354','current protocol differs from reviewed historical append')
need(new_protocol.startswith(old_protocol),'old stop protocol body changed')
need(new_protocol[len(old_protocol):].decode().splitlines()==['','## Closed v10-r3 result, 2026-10-26','','The separately frozen r3 collection completed operationally. Its unchanged','quality gate rejects the candidate. See [results and scope limits](issue76-r3-results-2026-10-26.md)','for the independently sealed96 judgments, retained negative outcomes and full','expense. Completion of this bounded pilot does not close the entire issue\'s','literal coverage gaps, activate a candidate or amend any earlier closed window.'],'unexpected protocol appendix')
observer=js('.cache/behavior76/pgx-observer-build-v1/pins.json')
need(observer['version']=='behavior76-pgx-observer-build-v1' and observer['derived_sha256']=='9fdbb58e77cebad5ff489cec5481b1aac2d5e111e44157605b796f7a7d8078fc','observer build contract changed')
need(sha(read('go.mod'))==observer['original_go_mod_sha256'] and sha(read('go.sum'))==observer['original_go_sum_sha256'],'root module changed')
need(sha(read('.cache/behavior76/pgx-observer-build-v1/pgx/conn.go'))==observer['derived_sha256'],'observer copied conn mismatch')
need(sha(read('.cache/behavior76/pgx-observer-build-v1/module-files.json'))==observer['module_files_sha256'],'observer module inventory mismatch')
need(not git('status','--porcelain').strip() and git('rev-parse','HEAD').decode().strip()==HEAD,'source changed during verification')
inventory=create('literal-v11-formal-source-inventory-v1.json',{'version':'issue76-literal-v11-formal-source-inventory-v1','producer':HEAD,'all_physical_git_blobs_exact':True,'source_pins':[dict(pin(p),git_blob_bytes_exact=True) for p in sorted(source)],'public_evidence_pins':[dict(pin(p),git_blob_bytes_exact=True) for p in published]})
witness=create('literal-v11-formal-criterion-witness-v1.json',dict(extents,status='FORMAL_WITNESS_OLD27_UNCHANGED_NEW32_68_GLOBAL_MATRIX_SEPARATE',producer=HEAD))
runtime=[dict(e,published_path=CHECK+'/'+e['path']) for e in deps['unchanged_old7runtime']+deps['new_additive_runtime4']]
receipt={'version':'issue76-literal-v11-formal-source-scorer-freeze-v1','decision':'FORMAL_SOURCE_AND_SCORER_FREEZE_BEFORE_NEW_AUTHORSHIP_NOT_NATIVE_ADMISSION','issued_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00','Z'),'producer':HEAD,'semantic_source':HEAD,'worktree_clean':True,'source_inventory':inventory,'criterion_witness':witness,'runtime_dependencies':runtime,'runtime_dependency_receipt':pin(CHECK+'/public-literal-checker-v6-dependencies-prepared-v2.json'),'original_criterion_AST_extents':27,'original_per_case_extent_types':25,'old_global_matrix_extents_preserved_not_invoked_for_newplan':2,'criteria_changed':False,'thresholds_changed':False,'candidate':pin('apps/orchestrator/internal/prompt/task_continuity.go'),'protocol':pin('docs/testing/issue76-literal-supplement-protocol.md'),'fixture_stop_protocol':pin('docs/testing/issue76-local-response-protocol.md'),'prepared_admission_schema':pin('scripts/dev/behavior76-literal-admission.schema.json'),'plan':{'episodes':16,'arms':32,'owner_opportunities':68,'dimensions':8,'legacy_families':6,'feedback_owner_turns_per_episode':3,'other_owner_turns_per_episode':2},'bounds':{'native_entries':768,'entries_per_owner':8,'entries_per_run':4,'model_seconds':120,'arm_seconds':1200,'window_seconds':14400,'authority_milliseconds':750,'PG_business':9,'PG_authority':1,'latest_start_UTC_hour':20,'owning_UTC_day_fit':True},'history':{'closed_windows':12,'admissions':971,'measured':959,'known_full_tokens':7700211,'NULL_usage':12,'old_failures_and_UNKNOWN_retained':True,'old_r2_cached5b56_not_restored':True,'closed_r3_remains_NO_GO':True,'no_new_live_DB_audit':True},'gates':{'legacy_public_top_PASS':61,'legacy_FAIL':0,'legacy_SKIP':0,'new_public_plan_top_PASS':9,'actual_public_composition_arms':32,'actual_public_composition_owner_opportunities':68,'actual_public_bridge_checks_PASS':12,'pure_file_neutral_PASS':22,'purePS_launcher_checks_PASS':18,'actual_host_pin_only_checks_PASS':9,'final_Go_time_guard_top_PASS':2,'vet_exit':0,'actual_new_hidden_setup_or_composition':False},'stable_observer_build':observer,'allowed_postfreeze_source_bindings':{'file':'apps/orchestrator/internal/app/behavior76_literal_types_test.go','exact_replacements':[{'before':'const behavior76LiteralStimuliSHA = ""','after_pattern':'const behavior76LiteralStimuliSHA = "[0-9a-f]{64}"'},{'before':'const behavior76LiteralExpectationsSHA = ""','after_pattern':'const behavior76LiteralExpectationsSHA = "[0-9a-f]{64}"'}],'count':2,'other_semantic_changes':False,'nonsemantic_preservation_results_artifacts_allowed':True},'authorship':{'status':'NOT_STARTED','directory_glob_absent':'hidden-literal-v11*','must_wait_for_parent_Git_preservation_and_separate_instruction':True,'fresh_independent_16_episode_controls_required':True,'old_controls_or_closed_answers_maps_reused':False},'issuer_operations':{'file_Git_only':True,'Go':0,'PG':0,'Docker':0,'auth':0,'native_model':0},'remaining_limits':['This is not a new native certificate, task acceptance, quality PASS or candidate promotion.','Actual future fresh bound setup/composition and immutable unique ELF are still required before separate finite one-use admission.','Expected restrictive classifier/automatic catalog failures remain ordinary actual faults, without authority regrant or new local exemption.','Old7 denotes checker PS/CSharp runtime dependencies, not production services.']}
formal=create('semantic-freeze-literal-v11-v1.json',receipt)
print(json.dumps({'formal_receipt':formal,'source_inventory':inventory,'criterion_witness':witness,'source_pins':len(source),'public_evidence_pins':len(published),'hidden_authorship_started':False,'native_admission':False},ensure_ascii=False))
