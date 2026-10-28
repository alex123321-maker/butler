$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'local-tail-adapter-v4.ps1')
$utf8=[Text.UTF8Encoding]::new($false);$results=[Collections.Generic.List[object]]::new()
$runRoot=Join-Path $PSScriptRoot ('public-http-proof-v4-'+[guid]::NewGuid().ToString('N'));$null=New-Item -ItemType Directory $runRoot
foreach($spec in @(@('test-local-tail-adapter-v4.ps1',@('WriteJson','ReadJson','WriteRows','Digest','Assert','Reseal','Seed')),@('test-local-model-proof-v4.ps1',@('Journal','Attempts','Stops','ModelSeed')))){
 $tok=$null;$err=$null;$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $spec[0]),[ref]$tok,[ref]$err);if($err.Count){throw 'Public test source parse failed'}
 foreach($d in $ast.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.FunctionDefinitionAst] -and $_.Name -in $spec[1]}){. ([scriptblock]::Create($d.Extent.Text))}
}
function ReadV4 {Read-Issue76LocalEvidenceV4 -ReportRoot $testRoot -Plan $plan -Pin $pin}
function SyncNativeProof($a){Journal $a;$s=Stops;$projection=Get-Issue76NativeProjectionV4 $a[0];foreach($k in $projection.Keys){$s.native_evidence[$k]=$projection[$k]};WriteRows 'behavior76_fixture_stops' $s}
function HttpSeed([switch]$Automatic,[switch]$LateUnknown){
 ModelSeed -Automatic:$Automatic -LateUnknown:$LateUnknown
 $a=Attempts;$a[0].stage='http_round_trip';$a[0].model_version=$null;$a[0].events[1].error_code='transport_connection_error';$a[0].events[1].safe_reason='http_do_failed_without_response';SyncNativeProof $a
 $s=Stops;$s.cause='model_http_no_response';WriteRows 'behavior76_fixture_stops' $s
 $i=ReadJson 'index.json';foreach($entry in $i.episodes[0].steps){$o=ReadJson $entry.outcome_path;$o.state=$o.state.Replace('fixture_local_model_failure','fixture_local_http_failure');WriteJson $entry.outcome_path $o};Reseal
}
function Case([string]$name,[scriptblock]$body){& $body;$results.Add(@{name=$name;status='pass'});Write-Output ('PASS '+$name)}
function Reject([string]$name,[scriptblock]$mutate){Case $name {HttpSeed;& $mutate;Reseal;$caught=$false;try{$null=ReadV4}catch{$caught=$true};Assert $caught ('HTTP local proof accepted invalid case '+$name)}}
Case 'exact_http_failure_FAILED_unknown_cost_tail_UNAVAILABLE_full96' {HttpSeed;$r=ReadV4;$f=$r.native_failure_refs[0];Assert ($r.owner_rows.Count-eq96-and$r.native_failure_refs.Count-eq1-and$f.cause-ceq'model_http_no_response'-and$f.verdict-ceq'failed'-and$f.success-ceq$false-and$null-eq$f.total_tokens-and$null-eq$f.model_version) 'HTTP failure became success/known expense';Assert ($r.tail_opportunities-eq1-and$r.other_unavailable_opportunities-eq94-and$r.snapshot_reads.Count-eq2-and@($r.owner_rows|Where-Object disposition -CEQ 'not_executed_after_fixture_local_http_failure')[0].verdict-ceq'unavailable') '96 denominator/tail semantics changed'}
Case 'automatic_failure_retains_actual_owner_final' {HttpSeed -Automatic;$r=ReadV4;Assert ($r.native_failure_refs[0].owner_run-ceq$false-and$r.retained_observation_refs[0].actual_final_run_ids[0]-ceq'public-run') 'Automatic failure erased owner final'}
Case 'classified_late_UNKNOWN_never_erased' {HttpSeed -LateUnknown;$r=ReadV4;Assert ($r.native_failure_refs[0].cause-ceq'model_http_no_response'-and$r.retained_observation_refs[0].actual_effect_ids[0]-ceq'public-effect') 'Later effect vanished'}
foreach($kind in @('start','continue','submit')){Case ('operation_'+$kind) {HttpSeed;$a=Attempts;$a[0].kind=$kind;SyncNativeProof $a;Reseal;Assert ((ReadV4).native_failure_refs[0].kind-ceq$kind) 'Operation changed'}}
foreach($field in @('input_tokens','output_tokens','total_tokens','thinking_tokens')){Reject ('observed_or_fabricated_usage_'+$field) {$a=Attempts;$a[0][$field]=0;SyncNativeProof $a}}
Reject 'observed_model_version_not_HTTP_no_response' {$a=Attempts;$a[0].model_version='gemini-public';SyncNativeProof $a}
foreach($value in @($null,@{},@{InputTokens=0;OutputTokens=0;TotalTokens=0})) {Reject ('event_usage_property_'+$(if($null-eq$value){'NULL'}elseif($value.Count){'counts'}else{'empty'})) {$a=Attempts;$a[0].events[1].usage=$value;SyncNativeProof $a}}
foreach($reason in @('','HTTP_DO_FAILED_WITHOUT_RESPONSE','stream_empty_answer','http_timeout','context_cancelled','memory_source_stale','authority_failure','generic_connection_error')){Reject ('other_reason_'+$reason) {$a=Attempts;$a[0].events[1].safe_reason=$reason;SyncNativeProof $a}}
foreach($stage in @('response_stream','authentication','budget','')){Reject ('other_stage_'+$stage) {$a=Attempts;$a[0].stage=$stage;SyncNativeProof $a}}
foreach($code in @('provider_timeout','provider_protocol_error','memory_source_stale','provider_unavailable')){Reject ('other_error_'+$code) {$a=Attempts;$a[0].events[1].error_code=$code;SyncNativeProof $a}}
foreach($status in @(200,401,429,500,'0')){Reject ('HTTP_not_exact_integer_zero_'+$status) {$a=Attempts;$a[0].events[1].http_status=$status;SyncNativeProof $a}}
foreach($event in @('assistant_delta','assistant_final','tool_call_requested','run_completed','run_cancelled','run_timed_out')){Reject ('event_extra_'+$event) {$a=Attempts;$a[0].events=@($a[0].events[0],@{type=$event},$a[0].events[1]);SyncNativeProof $a}}
Reject 'cause_empty_cannot_substitute_HTTP_proof' {$s=Stops;$s.cause='model_empty_answer';WriteRows 'behavior76_fixture_stops' $s}
Reject 'cause_UNKNOWN_cannot_substitute_HTTP_proof' {$s=Stops;$s.cause='effect_unknown';WriteRows 'behavior76_fixture_stops' $s}
Reject 'wrong_binding_mode' {$s=Stops;$s.binding.mode='fixture-local-model-v8';WriteRows 'behavior76_fixture_stops' $s}
Reject 'old_index_version' {$i=ReadJson 'index.json';$i.version='behavior76-checker-index-v3-local-model-tail';WriteJson 'index.json' $i}
Reject 'missing_manifest_optin' {$m=ReadJson 'manifest.json';$null=$m.Remove('local_http_failure_mode');WriteJson 'manifest.json' $m}
Reject 'generic_manifest_optin' {$m=ReadJson 'manifest.json';$m.local_http_failure_mode='all_http_errors';WriteJson 'manifest.json' $m}
Reject 'wrong_HTTP_tail_cause' {$i=ReadJson 'index.json';$o=ReadJson $i.episodes[0].steps[-1].outcome_path;$o.state='not_executed_after_fixture_local_model_failure';WriteJson $i.episodes[0].steps[-1].outcome_path $o}
Reject 'tail_cannot_be_expected_denial_or_executed' {$i=ReadJson 'index.json';$o=ReadJson $i.episodes[0].steps[-1].outcome_path;$o.state='admission_denied';WriteJson $i.episodes[0].steps[-1].outcome_path $o}
Reject 'post_barrier_reservation' {$a=Attempts;$a[0].started_at='2026-10-25T00:00:02Z';Journal $a}
Reject 'unsealed_observer' {$q=ReadJson ($armDir+'/authority-trace.json');$q.observer_failed=$true;WriteJson ($armDir+'/authority-trace.json') $q}
Reject 'quiescence_incomplete' {$q=ReadJson ($armDir+'/quiescence.json');$q.broker_reaped=$false;WriteJson ($armDir+'/quiescence.json') $q}
$receipt=@{version='issue76-public-http-proof-v4-selftests';passed=$results.Count;failed=0;native_calls=0;PG_calls=0;hidden_controls_answers_mapping_read=$false;criteria_changed=$false;tests=$results.ToArray();public_test_root=$runRoot}
[IO.File]::WriteAllText((Join-Path $runRoot 'selftest-receipt.json'),(ConvertTo-Json $receipt -Depth 30)+"`n",$utf8)
Write-Output ('SELFTEST_PASS '+$results.Count+' '+$runRoot)
