$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'local-tail-adapter-v4.ps1')
$utf8=[Text.UTF8Encoding]::new($false);$results=[Collections.Generic.List[object]]::new()
$runRoot=Join-Path $PSScriptRoot ('public-model-proof-v4-'+[guid]::NewGuid().ToString('N'));$null=New-Item -ItemType Directory $runRoot
$tokens=$null;$errors=$null;$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'test-local-tail-adapter-v4.ps1'),[ref]$tokens,[ref]$errors)
foreach($d in $ast.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.FunctionDefinitionAst] -and $_.Name -in @('WriteJson','ReadJson','WriteRows','Digest','Assert','Reseal','Seed')}){. ([scriptblock]::Create($d.Extent.Text))}
function Journal($rows){[IO.File]::WriteAllText((Join-Path $testRoot 'behavior76_attempts.jsonl'),((@($rows|ForEach-Object {ConvertTo-Json $_ -Depth 80 -Compress}) -join "`n")+"`n"),$utf8);[IO.File]::WriteAllText((Join-Path $testRoot 'behavior76_windows.jsonl'),(ConvertTo-Json @{id=$pin.window;closed=$true;admitted=@($rows).Count} -Compress),$utf8)}
function Attempts {return ,@([IO.File]::ReadLines((Join-Path $testRoot 'behavior76_attempts.jsonl'))|Where-Object {$_.Trim()}|ForEach-Object {ConvertFrom-Json $_ -AsHashtable -Depth 80})}
function Stops {return @(Get-Content -LiteralPath (Join-Path $testRoot ($armDir+'/tables/behavior76_fixture_stops.jsonl'))|Where-Object {$_.Trim()}|ForEach-Object {ConvertFrom-Json $_ -AsHashtable -Depth 80})}
function ReadV2 {Read-Issue76LocalEvidenceV4 -ReportRoot $testRoot -Plan $plan -Pin $pin}
function ModelSeed([switch]$Automatic,[switch]$Usage,[switch]$LateUnknown){
 $script:testRoot=Join-Path $runRoot ([guid]::NewGuid().ToString('N'));$null=New-Item -ItemType Directory $script:testRoot;Seed
 $runID=if($Automatic){'public-auto'}else{'public-run'}
 $input=@{input_id=$(if($Automatic){'public-auto-input'}else{'public-input'});run_id=$runID;session_key=$(if($Automatic){'public-auto-session'}else{'public-session'});session_sequence=1;status='blocked';event_id=$(if($Automatic){'public-auto-event'}else{'behavior76:public-turn-1'})}
 $inputs=@($input);$runs=@(@{run_id=$runID;task_id='public-task';session_key=$input.session_key;status='failed'})
 if($Automatic){$inputs+=@{input_id='public-input';run_id='public-run';session_key='public-session';session_sequence=1;status='completed';event_id='behavior76:public-turn-1'};$runs+=@{run_id='public-run';task_id='public-task';session_key='public-session';status='completed'}}
 WriteRows 'accepted_inputs' $inputs;WriteRows 'runs' $runs;WriteRows 'tasks' @(@{task_id='public-task';status='active';goal='PUBLIC';revision=1})
 if($Automatic){WriteRows 'run_final_results' @(@{run_id='public-run';result_id='public-owner-final';verdict='unverified';content='PUBLIC_SYNTHETIC_OWNER_FINAL'})}else{WriteRows 'run_final_results' @()}
 if(-not $LateUnknown){foreach($name in @('action_effects','action_intents','action_authorizations','action_effect_dispatches','behavior76_recipient_diagnostics')){WriteRows $name @()}}
 $error=@{type='transport_error';error_code='provider_protocol_error';safe_reason='stream_empty_answer';http_status=0}
 $a=@{window_id=$pin.window;n=1;slot=$armDir+'/public-turn-1';run_id=$runID;kind='submit';status='transport_error';stage='response_stream';milliseconds=100;input_tokens=$null;output_tokens=$null;total_tokens=$null;thinking_tokens=$null;model_version='gemini-public';events=@(@{type='run_started'},$error);started_at='2026-10-25T00:00:00Z'}
 if($Usage){$error.usage=@{InputTokens=2;OutputTokens=3;TotalTokens=7;ThinkingTokens=2};$a.input_tokens=2;$a.output_tokens=3;$a.total_tokens=7;$a.thinking_tokens=2}
 Journal @($a)
 $e=Get-Issue76NativeProjectionV4 $a;$proof=@{};foreach($k in $e.Keys){$proof[$k]=$e[$k]};$w=[ordered]@{};foreach($k in @('input_id','run_id','session_key','session_sequence')){$w[$k]=$input[$k]};$proof.owner_inputs=@($w);$proof.native_admission_cutoff_n=1
 $binding=@{window=$pin.window;arm=$armDir;producer=$pin.producer;protocol_sha256=$pin.protocol_sha256;schema='public-schema';mode='fixture-local-response-v10'}
 WriteRows 'behavior76_fixture_stops' @(@{cause='model_empty_answer';observed_at='2026-10-25T00:00:01Z';binding=$binding;effects=@();native_evidence=$proof})
 $index=ReadJson 'index.json'
 for($i=0;$i -lt $index.episodes[0].steps.Count;$i++){$entry=$index.episodes[0].steps[$i];$o=ReadJson $entry.outcome_path;if($i-eq0){$o.state='fixture_local_model_failure';$o.owner_input_id='public-input';$o.owner_run_id='public-run';$o.run_ids=@('public-run');$o.automatic_run_ids=@();if($Automatic){$o.run_ids+=@('public-auto');$o.automatic_run_ids=@('public-auto')};$o.native_entry_ids=@(1);$o.fixture_stop_ref=$armDir+'/tables/behavior76_fixture_stops.jsonl'}else{$o.state='not_executed_after_fixture_local_model_failure'};WriteJson $entry.outcome_path $o}
 WriteJson 'index.json' $index;Reseal
}
function Case([string]$name,[scriptblock]$body){& $body;$results.Add(@{name=$name;status='pass'});Write-Output ('PASS '+$name)}
function Reject([string]$name,[scriptblock]$mutation){Case $name {ModelSeed;& $mutation;Reseal;$caught=$false;try{$null=ReadV2}catch{$caught=$true};Assert $caught ('Accepted invalid proof: '+$name)}}
Case 'NULL_expense_observed_version_failed_native_96_rows_no_tail_snapshots' {ModelSeed;$r=ReadV2;Assert ($r.owner_rows.Count-eq96-and$r.native_failure_refs.Count-eq1-and$r.native_failure_refs[0].success-ceq$false-and$null-eq$r.native_failure_refs[0].total_tokens-and$r.native_failure_refs[0].model_version-ceq'gemini-public') 'Native failure/cost changed';Assert ($r.snapshot_reads.Count-eq2-and$r.tail_opportunities-eq1-and$r.other_unavailable_opportunities-eq94) 'Missingness changed';Assert (@($r.owner_rows|Where-Object disposition -CEQ 'not_executed_after_fixture_local_model_failure')[0].verdict-ceq'unavailable') 'Tail became success'}
Case 'complete_actual_usage_retained_not_refunded' {ModelSeed -Usage;$r=ReadV2;Assert ($r.native_failure_refs[0].total_tokens-eq7-and$r.native_failure_refs[0].thinking_tokens-eq2) 'Measured failure usage changed'}
Case 'automatic_empty_failure_retains_real_owner_final' {ModelSeed -Automatic;$r=ReadV2;Assert ($r.native_failure_refs[0].owner_run-ceq$false-and$r.retained_observation_refs[0].actual_final_run_ids[0]-ceq'public-run') 'Automatic and owner confused'}
Case 'classified_late_UNKNOWN_not_erased_by_model_first_cause' {ModelSeed -LateUnknown;$r=ReadV2;Assert ($r.native_failure_refs.Count-eq1-and$r.retained_observation_refs[0].actual_effect_ids[0]-ceq'public-effect') 'Late effect erased'}
foreach($kind in @('start','continue')){Case ('eligible_operation_'+$kind) {ModelSeed;$a=Attempts;$a[0].kind=$kind;Journal $a;$s=Stops;$s.native_evidence.kind=$kind;WriteRows 'behavior76_fixture_stops' $s;Reseal;$r=ReadV2;Assert ($r.native_failure_refs[0].kind-ceq$kind) 'Operation type changed'}}
foreach($field in @('window_id','slot','run_id','status','stage','model_version')){Reject ('proof_journal_mismatch_'+$field) {$s=Stops;$s.native_evidence[$field]='wrong';WriteRows 'behavior76_fixture_stops' $s}}
foreach($field in @('input_tokens','output_tokens','total_tokens','thinking_tokens')){Reject ('unobserved_expense_not_zero_'+$field) {$s=Stops;$s.native_evidence[$field]=0;WriteRows 'behavior76_fixture_stops' $s}}
foreach($field in @('input_id','run_id','session_key','session_sequence')){Reject ('foreign_input_witness_'+$field) {$s=Stops;$s.native_evidence.owner_inputs[0][$field]='wrong';WriteRows 'behavior76_fixture_stops' $s}}
Reject 'cutoff_below_actual_reservation' {$s=Stops;$s.native_evidence.native_admission_cutoff_n=0;WriteRows 'behavior76_fixture_stops' $s}
Reject 'cutoff_string_is_not_integer' {$s=Stops;$s.native_evidence.native_admission_cutoff_n='1';WriteRows 'behavior76_fixture_stops' $s}
Reject 'case_aliased_extra_proof_property' {$s=Stops;$s.native_evidence['N']=1;WriteRows 'behavior76_fixture_stops' $s}
Reject 'missing_typed_native_proof' {$s=Stops;$s.native_evidence=$null;WriteRows 'behavior76_fixture_stops' $s}
Reject 'model_cause_with_effect_snapshot' {$s=Stops;$s.effects=@(@{state='effect_unknown'});WriteRows 'behavior76_fixture_stops' $s}
Reject 'wrong_reason_in_actual_journal_and_proof' {$a=Attempts;$a[0].events[1].safe_reason='stream_json_invalid';Journal $a;$s=Stops;$s.native_evidence.events=$a[0].events;WriteRows 'behavior76_fixture_stops' $s}
Reject 'string_HTTP_zero_not_accepted' {$a=Attempts;$a[0].events[1].http_status='0';Journal $a;$s=Stops;$s.native_evidence.events=$a[0].events;WriteRows 'behavior76_fixture_stops' $s}
Reject 'explicit_NULL_usage_object_invalid' {$a=Attempts;$a[0].events[1].usage=$null;Journal $a;$s=Stops;$s.native_evidence.events=$a[0].events;WriteRows 'behavior76_fixture_stops' $s}
Reject 'partial_text_delta_invalidates_local_exception' {$a=Attempts;$a[0].events=@($a[0].events[0],@{type='assistant_delta';text_delta='PUBLIC'},$a[0].events[1]);Journal $a;$s=Stops;$s.native_evidence.events=$a[0].events;WriteRows 'behavior76_fixture_stops' $s}
foreach($event in @('tool_call_requested','assistant_final','run_cancelled','run_timed_out','run_completed')){Reject ('forbidden_stream_event_'+$event) {$a=Attempts;$a[0].events+=@{type=$event};Journal $a;$s=Stops;$s.native_evidence.events=$a[0].events;WriteRows 'behavior76_fixture_stops' $s}}
Reject 'additional_arbitrary_error_property' {$a=Attempts;$a[0].events[1].provider_text='PRIVATE_NOT_EXPORTABLE';Journal $a;$s=Stops;$s.native_evidence.events=$a[0].events;WriteRows 'behavior76_fixture_stops' $s}
Reject 'native_started_after_durable_barrier' {$a=Attempts;$a[0].started_at='2026-10-25T00:00:02Z';Journal $a}
Reject 'later_same_arm_reservation_even_if_empty' {$a=Attempts;$b=ConvertFrom-Json (ConvertTo-Json $a[0] -Depth 60) -AsHashtable -Depth 60;$b.n=2;$a+=@($b);Journal $a}
foreach($status in @('entry_failed','stream_closed','cancelled','unknown','executing')){Reject ('other_failure_remains_GLOBAL_'+$status) {$a=Attempts;$b=ConvertFrom-Json (ConvertTo-Json $a[0] -Depth 60) -AsHashtable -Depth 60;$b.n=2;$b.status=$status;$a+=@($b);Journal $a;$s=Stops;$s.native_evidence.native_admission_cutoff_n=2;WriteRows 'behavior76_fixture_stops' $s}}
Reject 'tail_wrong_cause_label' {$p=$armDir+'/step-3/outcome.json';$o=ReadJson $p;$o.state='not_executed_after_fixture_local_unknown';WriteJson $p $o}
Reject 'original_event_executed_after_barrier' {$p=$armDir+'/step-2/outcome.json';$o=ReadJson $p;$o.state='not_evaluated';WriteJson $p $o;$i=ReadJson 'index.json';$i.episodes[0].steps[1].before_snapshot_path=$armDir+'/step-1/before.json';$i.episodes[0].steps[1].after_snapshot_path=$armDir+'/step-1/after.json';WriteJson 'index.json' $i}
Reject 'stopping_native_number_missing_from_outcome' {$p=$armDir+'/step-1/outcome.json';$o=ReadJson $p;$o.native_entry_ids=@();WriteJson $p $o}
Reject 'default_zero_quiescence_not_proof' {$p=$armDir+'/quiescence.json';$q=ReadJson $p;$q.schema='';$q.observed_at='0001-01-01T00:00:00Z';WriteJson $p $q}
Case 'root_journal_hash_tamper_fails_before_projection' {ModelSeed;[IO.File]::AppendAllText((Join-Path $testRoot 'behavior76_attempts.jsonl'),' ',$utf8);$caught=$false;try{$null=ReadV2}catch{$caught=$true};Assert $caught 'Unsealed journal admitted'}
$receipt=@{version='issue76-public-model-proof-v4-selftests';passed=$results.Count;failed=0;new_native_calls=0;pg_operations=0;hidden_controls_or_mapping_read=$false;criteria_changed=$false;tests=$results.ToArray();public_test_root=$runRoot}
[IO.File]::WriteAllText((Join-Path $runRoot 'selftest-receipt.json'),(ConvertTo-Json $receipt -Depth 30)+"`n",$utf8)
Write-Output ('SELFTEST_PASS '+$results.Count+' '+$runRoot)
