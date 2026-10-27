$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'local-tail-adapter-v4.ps1')
$utf8=[Text.UTF8Encoding]::new($false);$results=[Collections.Generic.List[object]]::new();$runRoot=Join-Path $PSScriptRoot ('public-response-proof-v4-'+[guid]::NewGuid().ToString('N'));$null=New-Item -ItemType Directory $runRoot
foreach($spec in @(@('test-local-tail-adapter-v4.ps1',@('WriteJson','ReadJson','WriteRows','Digest','Assert','Reseal','Seed')),@('test-local-model-proof-v4.ps1',@('Journal','Attempts','Stops','ModelSeed')),@('test-local-http-proof-v4.ps1',@('SyncNativeProof')))){
 $t=$null;$e=$null;$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $spec[0]),[ref]$t,[ref]$e);if($e.Count){throw 'Public helper parse failure'};foreach($d in $ast.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.FunctionDefinitionAst] -and $_.Name -in $spec[1]}){. ([scriptblock]::Create($d.Extent.Text))}
}
function ReadV4 {Read-Issue76LocalEvidenceV4 -ReportRoot $testRoot -Plan $plan -Pin $pin}
function ResponseSeed([string]$Reason='stream_finish_rejected',[int]$Deltas=2,[switch]$Usage,[switch]$ObservedVersion,[switch]$Automatic,[switch]$LateUnknown){
 ModelSeed -Usage:$Usage -Automatic:$Automatic -LateUnknown:$LateUnknown
 $a=Attempts;$a[0].model_version=$(if($ObservedVersion){'gemini-public'}else{$null});$a[0].events[-1].safe_reason=$Reason
 $chain=@($a[0].events[0]);for($i=0;$i-lt$Deltas;$i++){$chain+=@{type='assistant_delta';text_delta=@{DeltaType='text';Content=('PUBLIC synthetic fragment '+$i);SequenceNo=1}}};$chain+=@($a[0].events[-1]);$a[0].events=$chain;SyncNativeProof $a
 $s=Stops;$s.cause='model_response_rejected';WriteRows 'behavior76_fixture_stops' $s
 $index=ReadJson 'index.json';foreach($entry in $index.episodes[0].steps){$o=ReadJson $entry.outcome_path;$o.state=$o.state.Replace('fixture_local_model_failure','fixture_local_response_failure');WriteJson $entry.outcome_path $o};Reseal
}
function Case([string]$name,[scriptblock]$body){& $body;$results.Add(@{name=$name;status='pass'});Write-Output ('PASS '+$name)}
function Reject([string]$name,[scriptblock]$mutation){Case $name {ResponseSeed;& $mutation;Reseal;$caught=$false;try{$null=ReadV4}catch{$caught=$true};Assert $caught ('Invalid response proof accepted: '+$name)}}
foreach($reason in Get-Issue76ResponseReasonsV4){foreach($deltas in @(0,2)){Case ('allowlisted_'+$reason+'_deltas_'+$deltas) {ResponseSeed -Reason $reason -Deltas $deltas;$r=ReadV4;$f=$r.native_failure_refs[0];Assert ($r.owner_rows.Count-eq96-and$f.cause-ceq'model_response_rejected'-and$f.safe_reason-ceq$reason-and$f.partial_text_delta_events-eq$deltas-and$f.partial_text_quality-ceq'unassessed'-and$f.success-ceq$false-and$f.verdict-ceq'failed') 'Exact response failure/partial observation/denominator changed';Assert ($r.tail_opportunities-eq1-and$r.snapshot_reads.Count-eq2-and$r.other_unavailable_opportunities-eq94-and@($r.owner_rows|Where-Object disposition -CEQ 'not_executed_after_fixture_local_response_failure')[0].verdict-ceq'unavailable') 'Skipped tail promoted/read snapshots'}}}
Case 'known_actual_usage_thinking_version_retained' {ResponseSeed -Usage -ObservedVersion;$f=(ReadV4).native_failure_refs[0];Assert ($f.total_tokens-eq7-and$f.thinking_tokens-eq2-and$f.model_version-ceq'gemini-public') 'Known usage/version lost or recomputed'}
Case 'NULL_usage_with_observed_version_is_not_zero' {ResponseSeed -ObservedVersion;$f=(ReadV4).native_failure_refs[0];Assert ($null-eq$f.total_tokens-and$f.model_version-ceq'gemini-public') 'Independent nullable metadata changed'}
Case 'known_usage_without_thinking_preserves_NULL_dimension' {ResponseSeed -Usage;$a=Attempts;$null=$a[0].events[-1].usage.Remove('ThinkingTokens');$a[0].thinking_tokens=$null;SyncNativeProof $a;Reseal;$f=(ReadV4).native_failure_refs[0];Assert ($f.total_tokens-eq7-and$null-eq$f.thinking_tokens-and$null-eq$f.model_version) 'Missing thinking/version filled'}
Case 'observed_all_zero_usage_remains_measured_zero' {ResponseSeed;$a=Attempts;$a[0].events[-1].usage=@{InputTokens=0;OutputTokens=0;TotalTokens=0};$a[0].input_tokens=0;$a[0].output_tokens=0;$a[0].total_tokens=0;SyncNativeProof $a;Reseal;Assert ((ReadV4).native_failure_refs[0].total_tokens-ceq0) 'Actual reported zero erased'}
Case 'automatic_response_failure_keeps_owner_final' {ResponseSeed -Automatic;$r=ReadV4;Assert ($r.native_failure_refs[0].owner_run-ceq$false-and$r.retained_observation_refs[0].actual_final_run_ids[0]-ceq'public-run') 'Automatic and owner outcomes merged'}
Case 'late_classified_UNKNOWN_not_erased' {ResponseSeed -LateUnknown;$r=ReadV4;Assert ($r.native_failure_refs[0].cause-ceq'model_response_rejected'-and$r.retained_observation_refs[0].actual_effect_ids[0]-ceq'public-effect') 'Late effect lost'}
foreach($kind in @('start','continue','submit')){Case ('native_operation_'+$kind) {ResponseSeed;$a=Attempts;$a[0].kind=$kind;SyncNativeProof $a;Reseal;Assert ((ReadV4).native_failure_refs[0].kind-ceq$kind) 'Operation changed'}}
foreach($reason in @('stream_terminal_missing','stream_envelope_invalid','stream_frame_limit','stream_automatic_function_execution','stream_function_truncated','http_non_sse_response','stream_empty_answer','http_do_failed_without_response','upstream_unauthenticated','stream_finish_rejected_extra','STREAM_FINISH_REJECTED','unknown','')){Reject ('excluded_or_unknown_reason_'+$reason) {$a=Attempts;$a[0].events[-1].safe_reason=$reason;SyncNativeProof $a}}
foreach($code in @('provider_timeout','transport_connection_error','memory_source_stale','execution_budget_tokens','provider_unavailable')){Reject ('wrong_error_code_'+$code) {$a=Attempts;$a[0].events[-1].error_code=$code;SyncNativeProof $a}}
foreach($stage in @('http_round_trip','authentication','')){Reject ('wrong_stage_'+$stage) {$a=Attempts;$a[0].stage=$stage;SyncNativeProof $a}}
foreach($status in @(200,401,500,'0',$null)){Reject ('wrong_HTTP_'+$status) {$a=Attempts;$a[0].events[-1].http_status=$status;SyncNativeProof $a}}
foreach($event in @('assistant_final','tool_call_requested','tool_call_batch_requested','run_completed','run_cancelled','run_timed_out','transport_error','provider_session_bound')){Reject ('extra_or_late_event_'+$event) {$a=Attempts;$a[0].events=@($a[0].events)+@(@{type=$event});SyncNativeProof $a}}
Reject 'thought_delta_not_text' {$a=Attempts;$a[0].events[1].text_delta.DeltaType='thought';SyncNativeProof $a}
Reject 'delta_missing_body' {$a=Attempts;$null=$a[0].events[1].Remove('text_delta');SyncNativeProof $a}
Reject 'delta_extra_usage' {$a=Attempts;$a[0].events[1].usage=@{InputTokens=0;OutputTokens=0;TotalTokens=0};SyncNativeProof $a}
Reject 'delta_case_alias_property' {$a=Attempts;$a[0].events[1].text_delta['content']='PUBLIC alias';SyncNativeProof $a}
Reject 'delta_unknown_property' {$a=Attempts;$a[0].events[1].text_delta.extra='PUBLIC';SyncNativeProof $a}
foreach($value in @($null,'',@{text='PUBLIC'})){Reject ('invalid_delta_content_'+$(if($null-eq$value){'NULL'}elseif($value-is[string]){'empty'}else{'object'})) {$a=Attempts;$a[0].events[1].text_delta.Content=$value;SyncNativeProof $a}}
foreach($seq in @(0,-1,5,'1',$null)){Reject ('invalid_sequence_'+$seq) {$a=Attempts;$a[0].events[1].text_delta.SequenceNo=$seq;SyncNativeProof $a}}
Reject 'sequence_is_invocation_not_incrementing_deltas' {$a=Attempts;$a[0].events[2].text_delta.SequenceNo=2;SyncNativeProof $a}
foreach($field in @('input_tokens','output_tokens','total_tokens','thinking_tokens')){Reject ('fabricated_or_undurable_'+$field) {$a=Attempts;$a[0][$field]=0;SyncNativeProof $a}}
foreach($value in @($null,@{},@{InputTokens=1},@{inputTokens=1;OutputTokens=2;TotalTokens=3},@{InputTokens=-1;OutputTokens=2;TotalTokens=3},@{InputTokens=2;OutputTokens=3;TotalTokens=4},@{InputTokens=2;OutputTokens=3;TotalTokens=7;ThinkingTokens=5},@{InputTokens=2;OutputTokens=3;TotalTokens=7;ThinkingTokens=$null},@{InputTokens='2';OutputTokens=3;TotalTokens=7},@{InputTokens=2.5;OutputTokens=3;TotalTokens=7},@{InputTokens=2;OutputTokens=3;TotalTokens=7;private_detail='DO_NOT_EXPORT'})){
 $label='invalid_usage_'+[guid]::NewGuid().ToString('N').Substring(0,8)
 Reject $label {$a=Attempts;$a[0].events[-1].usage=$value;SyncNativeProof $a}
}
Reject 'known_usage_row_mismatch' {$a=Attempts;$a[0].events[-1].usage=@{InputTokens=2;OutputTokens=3;TotalTokens=7};$a[0].input_tokens=2;$a[0].output_tokens=3;$a[0].total_tokens=8;SyncNativeProof $a}
Reject 'observed_empty_model_version_not_NULL' {$a=Attempts;$a[0].model_version='';SyncNativeProof $a}
Reject 'invalid_observed_model_version' {$a=Attempts;$a[0].model_version='raw-private-version';SyncNativeProof $a}
Reject 'wrong_stop_cause' {$s=Stops;$s.cause='model_empty_answer';WriteRows 'behavior76_fixture_stops' $s}
Reject 'wrong_binding' {$s=Stops;$s.binding.mode='fixture-local-http-v9';WriteRows 'behavior76_fixture_stops' $s}
Reject 'manifest_optin_missing' {$m=ReadJson 'manifest.json';$null=$m.Remove('local_response_failure_mode');WriteJson 'manifest.json' $m}
Reject 'native_journal_proof_mismatch' {$s=Stops;$s.native_evidence.events[1].text_delta.Content='PUBLIC different retained bytes';WriteRows 'behavior76_fixture_stops' $s}
Reject 'foreign_actual_input_witness' {$s=Stops;$s.native_evidence.owner_inputs[0].input_id='foreign';WriteRows 'behavior76_fixture_stops' $s}
Reject 'new_native_after_barrier' {$a=Attempts;$a[0].started_at='2026-10-25T00:00:02Z';Journal $a}
Reject 'tail_wrong_cause' {$i=ReadJson 'index.json';$o=ReadJson $i.episodes[0].steps[-1].outcome_path;$o.state='not_executed_after_fixture_local_model_failure';WriteJson $i.episodes[0].steps[-1].outcome_path $o}
Reject 'tail_fabricates_snapshot' {$i=ReadJson 'index.json';$i.episodes[0].steps[-1].before_snapshot_path=$i.episodes[0].steps[0].before_snapshot_path;WriteJson 'index.json' $i}
Reject 'unsealed_observer_GLOBAL' {$p=$armDir+'/authority-trace.json';$h=ReadJson $p;$h.observer_failed=$true;WriteJson $p $h}
Reject 'nonquiescent_claim_GLOBAL' {$p=$armDir+'/quiescence.json';$q=ReadJson $p;$q.live_claims=1;WriteJson $p $q}
Case 'UTF8_byte_bound_without_writing_oversized_public_artifact' {ResponseSeed -Deltas 1;$a=Attempts;$a[0].events[1].text_delta.Content=('я'*8388609);$caught=$false;try{$null=Assert-Issue76LocalNativeAttemptV4 $a[0] @{} $pin.window $armDir}catch{$caught=$true};Assert $caught 'Existing16MiBUTF8 bound not enforced'}
Case 'normalized_usage_rejects_unsigned_outside_actual_Go_PG_signed64' {$m=[uint64]::MaxValue;$a=@{input_tokens=$m;output_tokens=0;total_tokens=$m;thinking_tokens=$null;model_version=$null};$e=@{usage=@{InputTokens=$m;OutputTokens=0;TotalTokens=$m}};$caught=$false;try{Assert-Issue76ResponseMetadataV4 $a $e}catch{$caught=$true};Assert $caught 'Out-of-range native integer accepted'}
$receipt=@{version='issue76-public-response-proof-v4-selftests';passed=$results.Count;failed=0;native_Go_PG_auth_calls=0;hidden_controls_answers_goldens_mapping_read=$false;criteria_changed=$false;tests=$results.ToArray();public_test_root=$runRoot}
[IO.File]::WriteAllText((Join-Path $runRoot 'selftest-receipt.json'),(ConvertTo-Json $receipt -Depth 40)+"`n",$utf8)
Write-Output ('SELFTEST_PASS '+$results.Count+' '+$runRoot)
