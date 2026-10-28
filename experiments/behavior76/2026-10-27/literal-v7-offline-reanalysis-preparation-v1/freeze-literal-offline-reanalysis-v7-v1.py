"""File/Git-only formal freeze; never invokes scoring, models or databases."""
import hashlib, json, os, subprocess
from pathlib import Path
from datetime import datetime, timezone

ROOT = Path(__file__).resolve().parents[2]
OWN = Path(__file__).resolve().parent
PRODUCER = 'f1eabf538c640adbb92d957ecd672094816bebca'
SEMANTIC = '9fb9ceeb13420af0fb49272816df7273f558e3a4'
STIM = '9bc4872c85fb6b2d5e1a1b84b94b24a6a1e6d64ae749d527cbf5698431ca5d0c'
GOLD = 'f20937430079aa6c259a6917f9f1ab846875b4baff71c58cf4c72e4d1ff49865'
ELF = '7336e3fcfc282f9663a7fb23641146400900a7056e8a57889b803d272c3c411e'
def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1048576), b''): h.update(b)
    return h.hexdigest()
def need(ok, label):
    if not ok: raise RuntimeError('offline_v7_freeze:'+label)
def load(p): return json.loads(Path(p).read_text(encoding='utf-8-sig'))
def pin(p, expected=None):
    p=Path(p); s=sha(p)
    if expected: need(s==expected, 'pin:'+p.name)
    return {'path':p.relative_to(ROOT).as_posix(),'bytes':p.stat().st_size,'sha256':s}
def git(*args): return subprocess.check_output(['git',*args],cwd=ROOT)
def create(p, obj):
    with p.open('x',encoding='utf-8',newline='\n') as f:
        json.dump(obj,f,ensure_ascii=False,indent=2); f.write('\n'); f.flush(); os.fsync(f.fileno())
    p.chmod(0o444)
    return pin(p)

freeze_path=OWN/'formal-freeze-literal-offline-reanalysis-v7-v1.json'
inv_path=OWN/'literal-v7-formal-offline-runtime-inventory-v1.json'
ast_path=OWN/'literal-v7-formal-offline-criterion-witness-v1.json'
out_path=OWN/'closed-literal-v11-r1-offline-v7-private'
for p in (freeze_path,inv_path,ast_path,out_path): need(not p.exists(),'new_output_required:'+p.name)
need(git('rev-parse','HEAD').decode().strip()==PRODUCER,'producer')
need(not git('status','--porcelain').strip(),'worktree_clean')
depspin=pin(OWN/'public-literal-json-token-reanalysis-v7-dependencies-prepared-v1.json','76211a8f9f36bf3890dd98677650a9e2a6effc8b65bd49753ea85227622842ab')
deps=load(ROOT/depspin['path'])
runtime=[pin(OWN/e['path'],e['sha256']) for e in deps['original_eleven_runtime_files_unchanged']]
runtime.append(pin(OWN/deps['new_runtime']['path'],deps['new_runtime']['sha256']))
need(len(runtime)==12,'runtime_count')
allowpin=pin(OWN/'public-literal-json-token-v7-publication-allowlist-v1.json','2daddf50a532b45966ddce1bfa7b26fb2bc5711e02a1ed5b981128ce45feba01')
allow=load(ROOT/allowpin['path'])
for e in allow['files']:
    actual=pin(OWN/e['path'],e['sha256']); need(actual['bytes']==e['bytes'],'public_bytes')

source_invpin=pin(OWN/'literal-v11-formal-source-inventory-v1.json','7002b1bbcdbde4bf9fa8ec6828ad308720b835b7302970be4de0c1e3e49e5b31')
sources=load(ROOT/source_invpin['path'])['source_pins']; source_checks=[]
for e in sources:
    p=ROOT/e['path']; b=p.read_bytes(); committed=git('show',PRODUCER+':'+e['path'])
    need(b==committed,'physical_git_source:'+e['path'])
    original=git('show',SEMANTIC+':'+e['path'])
    if e['path']=='apps/orchestrator/internal/app/behavior76_literal_types_test.go':
        normalized=b.replace(('const behavior76LiteralStimuliSHA = "'+STIM+'"').encode(),b'const behavior76LiteralStimuliSHA = ""').replace(('const behavior76LiteralExpectationsSHA = "'+GOLD+'"').encode(),b'const behavior76LiteralExpectationsSHA = ""')
        need(normalized==original,'two_bindings_only')
    else: need(b==original,'semantic_source_changed:'+e['path'])
    source_checks.append(pin(p))
need(len(source_checks)==269,'source_count')
elfpin=pin(ROOT/'.cache/behavior76/literal-collector-c8411429-r1.test',ELF)
need(elfpin['bytes']==86820998,'elf_bytes')
stim_pin=pin(OWN/'hidden-literal-v11-r1/stimuli.jsonl',STIM)
gold_pin=pin(OWN/'hidden-literal-v11-r1/expectations.jsonl',GOLD)
rawroot=ROOT/'.cache/behavior76/native-literal-v11-r1'
rawinvpin=pin(ROOT/'.cache/behavior76/native-literal-v11-r1-raw-inventory-v1.json','77b6843f538af5e15454817ec3b3a433707ccdb893d935776de789de31b4c127')
raw=load(ROOT/rawinvpin['path']); total=0; names=set()
for e in raw:
    rel=Path(e['path']); need(not rel.is_absolute() and '..' not in rel.parts,'raw_path')
    p=rawroot/rel; actual=pin(p,e['sha256']); need(actual['bytes']==e['bytes'],'raw_bytes')
    need(p.stat().st_file_attributes & 1,'raw_readonly')
    names.add(rel.as_posix()); total+=e['bytes']
need(len(names)==2553 and total==72850851,'raw_counts')
need({p.relative_to(rawroot).as_posix() for p in rawroot.rglob('*') if p.is_file()}==names,'raw_file_set')
prep_pin=pin(OWN/'public-literal-json-token-reanalysis-v7-preparation-receipt-v1.json','c19815f3aef491230762cac1aa61b7afb8239b9390791c0c0230fcc0a20b7a3a')
prep=load(ROOT/prep_pin['path']); need(prep['generic_final_PASS']==45 and prep['existing_public_PASS']==332 and not prep['actual_r1_evaluation_or_communication_or_mapping_started'],'public_gates')
fail_pin=pin(OWN/'public-closed-literal-v11-r1-frozen-checker-receipt-v1.json','b4bfa23b85e40007542dc7f3705d9538c7f64463ddf0a0618d589566519f676c')
need(load(ROOT/fail_pin['path'])['decision']=='FROZEN_V6_STRICT_FAILURE_RETAINED_NO_QUIET_REPAIR','old_failure')
ops_pin=pin(OWN/'public-closed-literal-v11-r1-operational-prevalidation-v1.json','567f2e2a72197afc1454842f48b5b583e305f12454de5bcbc1d0cb80b9c4a85a')
ops=load(ROOT/ops_pin['path'])
need(ops['inclusive']=={'closed_windows':13,'admissions':1134,'measured':1122,'known_full_tokens':9010959,'NULL_usage':12},'inclusive')
need(ops['new_window']=={'admissions':163,'measured':163,'known_full_tokens':1310748,'NULL_usage':0},'new_expense')

# Extract original source ASTs without invoking any predicate or scoring function.
ps="""$ErrorActionPreference='Stop';$r=$args[0];. (Join-Path $r 'local-scoring-composition-v4.ps1');$k=Get-Issue76FrozenKernel (Join-Path $r 'score-mechanical-fresh-v3.ps1');@{source_sha256=$k.source_sha256;witnesses=$k.witnesses;PowerShell=$PSVersionTable.PSVersion.ToString()}|ConvertTo-Json -Depth 20"""
import base64
encoded=base64.b64encode(ps.encode('utf-16-le')).decode()
# Use a separate -Command script with literal owned path; no private data is read.
ps=ps.replace('$r=$args[0]',"$r='"+str(OWN).replace("'","''")+"'")
encoded=base64.b64encode(ps.encode('utf-16-le')).decode()
current=json.loads(subprocess.check_output(['pwsh','-NoProfile','-EncodedCommand',encoded],cwd=ROOT).decode('utf-8-sig'))
need(current['PowerShell']=='7.6.5','ps_version')
priorpin=pin(OWN/'literal-json-token-v7-public-run04/criterion-witness.json','f14cdc8b0138f897d91c320a9b48711dcc311c16ca2a1d6a506ecb1bf11a265a')
prior=load(ROOT/priorpin['path'])
statements=[e for e in current['witnesses'] if e['kind']=='statement']; helpers=[e for e in current['witnesses'] if e['kind']=='function']
need(len(statements)==27 and len(helpers)==12,'AST_counts')
need(statements==prior['original_statement_extents'] and statements==prior['new_statement_extents'] and helpers==prior['helper_function_extents'],'AST_equality')
astpin=create(ast_path,{'version':'issue76-literal-offline-v7-formal-criterion-witness-v1','original27_statements':statements,'original12_helpers':helpers,'unchanged':True,'source_sha256':current['source_sha256'],'prior_public_witness':priorpin,'predicate_execution':False})
invpin=create(inv_path,{'version':'issue76-literal-offline-v7-formal-runtime-source-inventory-v1','runtime_count':12,'runtime':runtime,'source_checks':source_checks,'all_physical_git_bytes_exact':True,'semantic_equivalent_except_original_two_bindings':True,'data_hashes_only_no_content_projection':True})
comm_pin=pin(OWN/'literal-v7-communication-token-boundary-contract-v1.md')
issuer_pin=pin(Path(__file__))
freeze={
 'version':'issue76-literal-offline-reanalysis-v7-formal-freeze-v1',
 'decision':'FORMAL_POSTHOC_SOURCE_SCORER_JSON_BOUNDARY_FREEZE_ONLY_NOT_EVALUATION_AUTHORIZATION',
 'issued_at_utc':datetime.now(timezone.utc).isoformat(),
 'producer':PRODUCER,'semantic_source':SEMANTIC,'worktree_clean':True,
 'runtime_inventory':invpin,'criterion_witness':astpin,'runtime_count':12,
 'measured_ELF':elfpin,'measured_raw':{'root':rawroot.relative_to(ROOT).as_posix(),'inventory':rawinvpin,'files':2553,'bytes':72850851,'all_SHA_readonly_file_set_exact':True},
 'controls':{'stimuli':stim_pin,'expectations':gold_pin,'content_read':False,'case_values_projected':False},
 'plan':{'episodes':16,'arms':32,'owner_opportunities':68,'dimensions':8,'legacy_families':6},
 'original_failure':fail_pin,'original_status':'FROZEN_V6_STRICT_FAIL_NO_GO_IMMUTABLE',
 'public_preparation':prep_pin,'public_allowlist':allowpin,'dependencies':depspin,'closed_operational_proof':ops_pin,
 'inclusive_expense':ops['inclusive'],'new_window_expense':ops['new_window'],
 'original_criteria_statement_extents':27,'original_helper_extents':12,'criteria_rubric_thresholds_stop_UNKNOWN_rules_changed':False,
 'communication_extraction_contract':comm_pin,
 'decoder_scope':'ALL matching nested unqualified and module-qualified ConvertFrom-Json calls under pinned Invoke-Issue76JsonStringTokenScopeV7; not goldens-only. Exact JSON string tokens retained before communication review; no DateTime stringification casts.',
 'future_private_output':out_path.relative_to(ROOT).as_posix(),'output_absent_at_freeze':True,
 'evaluation_state':{'mechanical_reanalysis':'NOT_STARTED','communication':'NOT_STARTED','mapping_read':False,'intentional_mapping_inference':False,'all68_masked_judgments_must_seal_before_mapping':True},
 'next_step_requires_parent_Git_preservation_and_explicit_offline_authorization':True,
 'new_native_authorization':False,'candidate_promotion':False,
 'limitations':['Post hoc representation repair is not original-protocol PASS.','No data resampling, expected-value edits, criterion changes or historical-result rewriting.','Public tests cover their assertions/outcomes, not every dynamic fixture byte or exception trace.','Synthetic-data, statistical and AI-communication-review limitations remain.'],
 'issuer_script':issuer_pin,'operations':{'file_Git_and_AST_only':True,'Go':0,'PG':0,'Docker':0,'auth':0,'provider_model':0,'scoring_or_judgments':0}}
result=create(freeze_path,freeze)
additions=[issuer_pin,comm_pin,astpin,invpin,result]
addpin=create(OWN/'public-literal-offline-v7-freeze-allowlist-additions-v1.json',{'version':'issue76-offline-v7-freeze-public-allowlist-additions-v1','base_allowlist':allowpin,'files':additions,'private_output_not_publishable':out_path.relative_to(ROOT).as_posix(),'no_actual_scoring_or_mapping':True})
print(json.dumps({'formal_freeze':result,'public_allowlist_additions':addpin,'runtime_count':12,'source_count':269,'AST':27,'helpers':12,'scoring_started':False},indent=2))
