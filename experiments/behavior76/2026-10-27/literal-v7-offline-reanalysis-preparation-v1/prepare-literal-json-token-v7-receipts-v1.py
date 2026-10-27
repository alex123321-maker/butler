from pathlib import Path
import json,hashlib,collections,re,datetime,subprocess
ROOT=Path(__file__).resolve().parents[2];OWN=ROOT/'.cache/issue76-independent';REG=OWN/'literal-v7-existing-public-regressions-run01';GEN=OWN/'literal-json-token-v7-public-run04'
def H(b):return hashlib.sha256(b).hexdigest()
def js(p):return json.loads(p.read_bytes())
def pin(p):return {'path':str(p.relative_to(OWN)).replace('\\','/'),'sha256':H(p.read_bytes()),'bytes':p.stat().st_size}
def need(v,m):
 if not v:raise RuntimeError(m)
def create(name,obj):
 p=OWN/name
 with p.open('xb') as f:f.write((json.dumps(obj,ensure_ascii=False,indent=2)+'\n').encode())
 return pin(p)
original=js(OWN/'public-local-response-checker-v4-receipt-v1.json');reg=js(REG/'receipt.json');generic=js(GEN/'receipt.json');deps=js(OWN/'public-literal-checker-v6-dependencies-prepared-v2.json')
need(reg['passed_cases']==332 and reg['failed_script_groups']==0 and generic['PASS']==45 and generic['FAIL']==0,'public regression failed')
runtime=deps['unchanged_old7runtime']+deps['new_additive_runtime4']
for e in runtime:need(H((OWN/e['path']).read_bytes())==e['sha256'],'original runtime changed')
def names(raw):
 t=raw.decode('utf-8-sig',errors='replace').replace('\x00','')
 result=re.findall(r'^PASS (.+)$',t,re.M)
 if result:return result
 try:
  o=json.loads(t[t.index('{'):t.rindex('}')+1]);return [v['name'] if isinstance(v,dict) else v for v in o['tests']]
 except Exception:return []
comparison=[]
for previous in original['public_tests']:
 current=next(x for x in reg['results'] if x['script']==previous['script'])
 before=names((OWN/previous['log']).read_bytes());after=names((ROOT/current['log']).read_bytes())
 before=[n.rstrip('\r') for n in before];after=[n.rstrip('\r') for n in after]
 normalization=None
 if previous['script']=='test-local-response-proof-v4.ps1':
  pattern=r'^invalid_usage_[0-9a-f]{8}$'
  need(sum(bool(re.fullmatch(pattern,n)) for n in before)==sum(bool(re.fullmatch(pattern,n)) for n in after)==11,'documented random-label multiplicity differs')
  normalize=lambda items:[re.sub(pattern,'invalid_usage_<GUID>',n) for n in items]
  before=normalize(before);after=normalize(after)
  normalization={'field':'public test output label only','exact_pattern':pattern,'normalized_label':'invalid_usage_<GUID>','old_instances':11,'new_instances':11,'multiset_preserves_all_instances':True,'source_line':40,'not_dynamic_fixture_byte_equivalence':True}
 need(len(before)==len(after)==previous['expected_cases'] and collections.Counter(before)==collections.Counter(after),'old/new public case-name multiset differs: '+previous['script'])
 comparison.append({'script':previous['script'],'source_sha256':previous['script_sha256'],'old_case_count':len(before),'new_case_count':len(after),'same_case_name_multiset_and_asserted_pass_outcomes':True,'label_only_normalization':normalization,'old_log_sha256':H((OWN/previous['log']).read_bytes()),'new_log_sha256':current['log_sha256']})
old_lit=js(OWN/'literal-v6-public-neutral-run05/receipt.json');new_lit=js(REG/'literal22-public-files/receipt.json')
need(old_lit['cases']==new_lit['cases'] and new_lit['PASS']==22 and new_lit['FAIL']==0,'literal public assertions differ')
old_bridge=js(OWN/'literal-actual-bridge-run01/receipt.json');new_bridge=js(REG/'actual-public-bridge-files/receipt.json')
need(old_bridge['checks']==new_bridge['checks'] and new_bridge['PASS']==12 and new_bridge['FAIL']==0,'actual public bridge payloads differ')
need(H((ROOT/'.cache/behavior76/native-literal-v11-r1-raw-inventory-v1.json').read_bytes())=='77b6843f538af5e15454817ec3b3a433707ccdb893d935776de789de31b4c127','original raw inventory changed')
need(H((OWN/'public-closed-literal-v11-r1-frozen-checker-receipt-v1.json').read_bytes())=='b4bfa23b85e40007542dc7f3705d9538c7f64463ddf0a0618d589566519f676c','original failed execution changed')
commit=js(OWN/'public-commitment-literal-v11-r1.json')
for kind in ['stimuli','expectations']:need(H((ROOT/commit[kind+'_path']).read_bytes())==commit[kind+'_sha256'],'original control bytes changed')
cmp=create('public-literal-json-token-v7-public-regression-comparison-v1.json',{'version':'issue76-json-token-v7-public-outcome-comparison-v1','original_public_groups':comparison,'old_public_cases':298,'new_public_cases':298,'literal22_case_outcomes_exact':True,'actual_saved_public_bridge12_check_payloads_exact':True,'comparison_scope':'Same unchanged test sources, case-name multisets and original tests positive/denial assertions. Does not assert identity of every intermediate object or complete exception trace.','original_public_and_scoped_results_both_passing':True,'actual_r1_scoring_or_mapping':False})
test_files={x['script'] for x in original['public_tests']}|{'test-literal-checker-v6.ps1','check-literal-actual-bridge-v1.ps1','test-literal-json-token-boundary-v7.ps1','score-mechanical-v2.ps1'}
new_runtime=pin(OWN/'literal-json-token-boundary-reanalysis-v7.ps1')
dependency=create('public-literal-json-token-reanalysis-v7-dependencies-prepared-v1.json',{'version':'issue76-literal-json-token-reanalysis-v7-dependencies-prepared-v1','status':'PREPARED_FOR_PARENT_REVIEW_NOT_FORMAL_REANALYSIS_FREEZE','entrypoint':'Invoke-Issue76LiteralOfflineReanalysisFilesV7','default':'PREPARED_ONLY_NO_EVALUATION','execute_switch':'ExecuteReviewedOfflineReanalysis','new_runtime':new_runtime,'original_eleven_runtime_files_unchanged':runtime,'runtime_file_count':12,'installed_capability':{'PowerShell':'7.6.5','module':'Microsoft.PowerShell.Utility','cmdlet':'ConvertFrom-Json','DateKind':'String','no_dependency_upgrade':True},'scope':'ALL matching unqualified and module-qualified nested ConvertFrom-Json calls, not just expectations; locally cloned default table, no caller/global mutation.','old_predicate_statement_count':27,'old_helper_function_count':12,'old_per_case_types':25,'old_global_matrix_types':2,'original_criterion_source_sha256':'e481f9c062a3bc4500b9f0b94da8819fbf287bec075490583cad17d54198721d','criterion_witness':pin(GEN/'criterion-witness.json'),'public_test_source_dependencies':[pin(OWN/p) for p in sorted(test_files)],'prepared_tools':[pin(OWN/p) for p in ['run-scoped-public-json-v7.ps1','run-existing-public-json-v7-v1.py','prepare-literal-json-token-v7-receipts-v1.py']],'contract':pin(OWN/'public-literal-json-token-reanalysis-v7-contract-prepared-v1.md'),'minimal_exact_delta':pin(OWN/'public-literal-json-token-reanalysis-v7-minimal-delta-v1.md'),'old11_files_patched':0,'criteria_or_thresholds_changed':False,'native_authorization':False})
receipt=create('public-literal-json-token-reanalysis-v7-preparation-receipt-v1.json',{'version':'issue76-literal-json-token-reanalysis-v7-preparation-receipt-v1','decision':'CONCRETE_BOUNDARY_FIX_READY_FOR_PARENT_REVIEW_ONLY','issued_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'new_runtime':new_runtime,'dependencies':dependency,'contract':pin(OWN/'public-literal-json-token-reanalysis-v7-contract-prepared-v1.md'),'minimal_delta':pin(OWN/'public-literal-json-token-reanalysis-v7-minimal-delta-v1.md'),'generic_public_history':[pin(OWN/('literal-json-token-v7-public-run0'+str(i)+'/receipt.json')) for i in range(1,5)],'generic_final_PASS':45,'generic_final_FAIL':0,'generic_test_history_note':'Run01 two negative callback probes strengthened in run02 with actual not-entered witnesses; unrelated defaults and exact frozen ReadJson tests added subsequently. Prior source-as-run snapshots retained. No runtime or old predicate change.','existing_public_regressions':pin(REG/'receipt.json'),'existing_public_PASS':332,'existing_public_failed_groups':0,'public_baseline_comparison':cmp,'scoped_public_bridge':pin(REG/'actual-public-bridge-files/receipt.json'),'original27_and12_helper_ASTs_exact':True,'old11_runtime_bytes_unchanged':True,'JSON_token_strings_preserved_before_conversion':True,'all_decoder_calls_in_scope_affected_not_only_goldens':True,'DateTime_stringification_casts_used':False,'default_scope_leakage_on_success_or_throw':False,'private_case_value_used_in_public_examples':False,'original_frozen_v6_failure_retained':'b4bfa23b85e40007542dc7f3705d9538c7f64463ddf0a0618d589566519f676c','original_v6_qualification':'STRICT_FAIL_NO_GO_UNCHANGED','new_boundary_is_posthoc_and_requires_separate_review_freeze':True,'actual_r1_evaluation_or_communication_or_mapping_started':False,'corpus_goldens_raw_expense_criteria_rubric_thresholds_stop_rules_unchanged':True,'actual_r1_plan':'16episodes/32arms/68owner; remains unassessed','historical_r3_candidate_NO_GO_retained':True,'Go_PG_Docker_provider_auth_native_calls':0,'new_native_admission':False,'formal_reanalysis_freeze_issued':False,'limitations':['Public regression equality covers original case assertions and declared outcomes, not every intermediate exception trace.','Scoped DateKind String deliberately changes representation of any matching ISO JSON string in any nested decoder; do not claim gold-loader-only scope.','The failed frozen prospective execution is never rewritten as an original-protocol pass.','No actual r1 result may be generated until parent reviews the concrete pack and separately freezes/authorizes offline reanalysis.','No candidate promotion, new stop exemption, new provider call, data resampling or private expected-value cast is permitted.']})
print(json.dumps({'preparation_receipt':receipt,'dependencies':dependency,'public_comparison':cmp,'new_runtime':new_runtime,'generic_PASS':45,'public_regression_PASS':332,'actual_r1_reanalysis_run':False}))
