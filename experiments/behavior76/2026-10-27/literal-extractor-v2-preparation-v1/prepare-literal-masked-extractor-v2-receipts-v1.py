"""Prepare public file-only extractor review receipts; never reads response text."""
from pathlib import Path
import hashlib,json,os
from datetime import datetime,timezone
R=Path(__file__).resolve().parent
def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def load(n): return json.loads((R/n).read_text(encoding='utf-8-sig'))
def pin(n):
 p=R/n
 return {'path':n,'bytes':p.stat().st_size,'sha256':sha(p)}
def need(v,m):
 if not v: raise RuntimeError(m)
def create(n,j):
 p=R/n
 with p.open('x',encoding='utf-8',newline='\n') as f:
  json.dump(j,f,ensure_ascii=False,indent=2);f.write('\n');f.flush();os.fsync(f.fileno())
 p.chmod(0o444)
 return pin(n)
deps=load('public-literal-json-token-reanalysis-v7-dependencies-prepared-v1.json')
runtime=deps['original_eleven_runtime_files_unchanged']+[deps['new_runtime']]
for e in runtime: need(sha(R/e['path'])==e['sha256'],'old runtime changed')
for n,s in {
 'closed-literal-v11-r1-offline-v7-private/posthoc-v7-mechanical-result-v1.json':'41825ea0599c7d9adf1a47e314fbcbe4e01209cfcc9c8a5f4d0ab95dbc9d65f4',
 'closed-literal-v11-r1-offline-v7-private/masked-extraction-failure-v1.json':'94b842bf0f67981c8d0e1ba9748187339dc6ac5e8d8b45fd96302c1b4d1fc8a2',
 'public-literal-r1-offline-v7-mechanical-execution-v1.json':'4ab0bec7c4892662de4f30fa4a3495a7794a552cef326bfacd1fe20e8e5ef48e',
}.items(): need(sha(R/n)==s,'original result/failure changed')
run01=load('literal-masked-final-v2-public-run01/receipt.json')
run02=load('literal-masked-final-v2-public-run02/receipt.json')
need(run01['PASS']==41 and run01['FAIL']==0 and run02['PASS']==45 and run02['FAIL']==0,'public tests')
need(sha(R/'literal-masked-final-v2-public-run01/test-source-as-run.ps1.txt')==run01['test_sha256'],'old public source snapshot')
need(sha(R/'literal-masked-final-extractor-v2.ps1')==run02['helper_sha256'],'helper test pin')
need(sha(R/'extract-literal-r1-offline-v7-masked-evidence-v2.ps1')==run02['driver_sha256'],'driver test pin')
need(not (R/'closed-literal-v11-r1-offline-v7-private/masked-evidence-v2').exists(),'actual extraction must not start')
source=[pin(n) for n in ['literal-masked-final-extractor-v2.ps1','extract-literal-r1-offline-v7-masked-evidence-v2.ps1','test-literal-masked-final-extractor-v2.ps1']]
docs=[pin(n) for n in ['public-literal-masked-extractor-v2-contract-prepared-v1.md','public-literal-masked-extractor-v2-minimal-delta-v1.md','public-literal-offline-v7-extraction-v1-safe-diagnosis-v1.json']]
receipt=create('public-literal-masked-extractor-v2-preparation-receipt-v1.json',{
 'version':'issue76-literal-masked-extractor-v2-preparation-receipt-v1',
 'decision':'PREPARED_FOR_PARENT_REVIEW_NO_ACTUAL_V2_EXTRACTION_OR_JUDGMENTS',
 'issued_at_utc':datetime.now(timezone.utc).isoformat(),
 'sources':source,'documents':docs,'old12_runtime_unchanged':runtime,
 'formal_v7_freeze_sha256':'a8342dd7a8b4e5c1de391cb1bbaa3e27267de865e8c8f85893e9fdfaad288378',
 'old27_statement_and12_helper_ASTs_exact':True,
 'public_tests':[pin('literal-masked-final-v2-public-run01/receipt.json'),pin('literal-masked-final-v2-public-run02/receipt.json')],
 'latest_public_PASS':45,'latest_public_FAIL':0,
 'original_mechanical_result_unchanged':'41825ea0599c7d9adf1a47e314fbcbe4e01209cfcc9c8a5f4d0ab95dbc9d65f4',
 'original_extractor_v1_failure_retained':'94b842bf0f67981c8d0e1ba9748187339dc6ac5e8d8b45fd96302c1b4d1fc8a2',
 'original_v1_extractor_source':pin('extract-literal-r1-offline-v7-masked-evidence-v1.ps1'),
 'raw_final_policy':'Exact private raw plus strict UTF8SHA; no repair/truncation/first-object extraction; invalid or mechanically unavailable means decoded_answer null and communication UNAVAILABLE.',
 'availability_is_not_PASS':True,
 'unique_existing_mechanical_proof_required':True,
 'kernel_and_automatic_are_not_owner_model_finals':True,
 'decoder_scope':'Same pinned V7 scope before all nested JSON parsing/rendering, including rubric/case/outcome. Not golden-only.',
 'planned_owner_denominator':68,
 'criteria_rubric_thresholds_data_expectedvalues_expense_mapping_changed':False,
 'actual_v2_extraction':'NOT_STARTED','actual_communication_review':'NOT_STARTED','mapping_read':False,
 'mechanical_rerun':False,'new_native_or_promotion':False,
 'original_v6_status':'STRICT_FAIL_NO_GO_IMMUTABLE',
 'requires_parent_review_and_explicit_authorization':True,
 'operations':{'file_only':True,'Go':0,'PG':0,'Docker':0,'auth':0,'provider_model':0},
 'limitations':['This is a post hoc extraction plumbing addition, not original-protocol PASS.','Generic availability tests are not actual quality judgments.','The strict frozen mechanical result remains independently authoritative.']})
names=[e['path'] for e in source+docs]+[
 'public-literal-masked-extractor-v2-preparation-receipt-v1.json',
 'literal-masked-final-v2-public-run01/receipt.json',
 'literal-masked-final-v2-public-run01/test-source-as-run.ps1.txt',
 'literal-masked-final-v2-public-run02/receipt.json',
 'extract-literal-r1-offline-v7-masked-evidence-v1.ps1',
 Path(__file__).name]
allow=create('public-literal-masked-extractor-v2-publication-allowlist-v1.json',{
 'version':'issue76-literal-masked-extractor-v2-publication-allowlist-v1',
 'status':'PUBLIC_PREPARATION_ONLY_NOT_ACTUAL_REVIEW',
 'files':[pin(n) for n in names],
 'existing_frozen12_dependencies':'public-literal-json-token-reanalysis-v7-dependencies-prepared-v1.json',
 'forbidden':['actual raw finals/private extraction failure/private projection','corpus/goldens/mapping/assessments','auth/settings/credentials/dumps'],
 'preserve_relative_layout':True})
(R/'extract-literal-r1-offline-v7-masked-evidence-v1.ps1').chmod(0o444)
print(json.dumps({'preparation':receipt,'public_allowlist':allow,'PASS':45,'actual_extraction':False,'mechanical_rerun':False},indent=2))
