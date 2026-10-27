$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'local-tail-adapter-v2.ps1')
$testRoot=Join-Path $PSScriptRoot ('public-adapter-v2-test-'+[guid]::NewGuid().ToString('N'))
$null=New-Item -ItemType Directory -Path $testRoot
$results=[Collections.Generic.List[object]]::new()
$utf8=[Text.UTF8Encoding]::new($false)
function WriteJson($path,$value){$full=Join-Path $testRoot $path;$null=[IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($full));[IO.File]::WriteAllText($full,(ConvertTo-Json -InputObject $value -Depth 100),$utf8)}
function ReadJson($path){ConvertFrom-Json ([IO.File]::ReadAllText((Join-Path $testRoot $path))) -AsHashtable -Depth 100}
function WriteRows($table,$rows){$raw=(@($rows|ForEach-Object {ConvertTo-Json -InputObject $_ -Depth 100 -Compress}) -join "`n");[IO.File]::WriteAllText((Join-Path $testRoot ($script:armDir+'/tables/'+$table+'.jsonl')),$raw,$utf8)}
function Digest($path){(Get-FileHash -LiteralPath (Join-Path $testRoot $path)).Hash.ToLowerInvariant()}
function Assert($yes,$message){if(-not $yes){throw $message}}
function Reseal {
 $caseRoot=Join-Path $testRoot $script:armDir;$items=@(Get-ChildItem -LiteralPath $caseRoot -File -Recurse|Where-Object Name -CNE 'sealed-files.json'|ForEach-Object {@{path=[IO.Path]::GetRelativePath($caseRoot,$_.FullName).Replace('\','/');bytes=$_.Length;sha256=(Get-FileHash -LiteralPath $_.FullName).Hash.ToLowerInvariant()}})
 WriteJson ($script:armDir+'/sealed-files.json') @{version='behavior76-quiescent-fixture-files-v1';files=$items}
 $script:pin.manifest_sha256=Digest 'manifest.json';$script:pin.index_sha256=Digest 'index.json'; $script:pin.attempts_sha256=Digest 'behavior76_attempts.jsonl'; $script:pin.windows_sha256=Digest 'behavior76_windows.jsonl'
}
function Seed([switch]$Ordinary){
 $script:armDir='slot-01/public-arm-a';$script:plan=@();$schedule=@()
 for($n=1;$n -le 24;$n++){
  $id='public-episode-'+$n;$schedule+=@{episode_id=$id;arm_order=@('public-arm-a','public-arm-b')}
  foreach($arm in @('public-arm-a','public-arm-b')){$script:plan+=@{episode_id=$id;masked_arm_id=$arm;family=('public-family-'+[int][math]::Ceiling($n/2));steps=@(@{id='public-turn-1';evaluate=$true},@{id='public-typed-event';evaluate=$false},@{id='public-turn-2';evaluate=$true;expected_denial=$true})}}
 }
 $script:pin=@{window='public-window';producer=('a'*40);protocol_sha256=('b'*64);stimuli_sha256=('c'*64)}
 WriteJson 'manifest.json' @{window=$pin.window;source=$pin.producer;protocol_sha256=$pin.protocol_sha256;stimuli_sha256=$pin.stimuli_sha256;local_unknown_mode='whole_fixture_after_exact_classified_effect_unknown';local_model_failure_mode='whole_fixture_after_exact_empty_answer_v1';global_priority=$true;schedule=$schedule}
 $null=[IO.Directory]::CreateDirectory((Join-Path $testRoot ($armDir+'/tables')))
 foreach($name in $Issue76LocalRequiredTables){WriteRows $name @()}
 [IO.File]::WriteAllText((Join-Path $testRoot 'behavior76_windows.jsonl'),(ConvertTo-Json -InputObject @{id=$pin.window;closed=$true;admitted=0} -Compress),$utf8)
 [IO.File]::WriteAllText((Join-Path $testRoot 'behavior76_attempts.jsonl'),'', $utf8)
 $input=@{input_id='public-input';run_id='public-run';session_key='public-session';session_sequence=1;status='completed';event_id='behavior76:public-turn-1'}
 $binding=@{run_id='public-run';tool_call_id='public-tool';task_id='public-task'}
 $effect=@{action_id='public-effect';authorization_id='public-auth';state='effect_unknown';initial_binding=$binding}
 $intent=@{action_id='public-intent';effect_id='public-effect';run_id='public-run';tool_call_id='public-tool';binding=$binding}
 $dispatch=@{action_id='public-effect';authorization_id='public-auth';execution_action_id='public-intent'}
 WriteRows 'accepted_inputs' @($input)
 WriteRows 'runs' @(@{run_id='public-run';task_id='public-task'},@{run_id='public-auto';task_id='public-task'})
 WriteRows 'run_final_results' @(@{run_id='public-run'},@{run_id='public-auto'})
 WriteRows 'action_intents' @($intent)
 WriteRows 'action_authorizations' @(@{authorization_id='public-auth';action_id='public-intent'})
 WriteRows 'action_effect_dispatches' @($dispatch)
 $stop=@{cause='effect_unknown';observed_at='2026-10-25T00:00:00Z';binding=@{window=$pin.window;arm=$armDir;producer=$pin.producer;protocol_sha256=$pin.protocol_sha256;schema='public-schema';mode='fixture-local-model-v8'};effects=@(@{effect=$effect;intent_binding=$binding;owner_inputs=@($input);dispatches=@($dispatch)})}
 if($Ordinary){$effect.state='effect_confirmed'}else{WriteRows 'behavior76_fixture_stops' @($stop);WriteRows 'behavior76_recipient_diagnostics' @(@{action_id='public-effect';run_id='public-run';tool_call_id='public-tool';category='lost_ack'})}
 WriteRows 'action_effects' @($effect)
 WriteJson ($armDir+'/quiescence.json') @{schema='public-schema';observed_at='2026-10-25T00:00:01Z';fixture_locally_stopped=(-not $Ordinary);broker_reaped=$true;runtime_stopped=$true;broker_application_observed_before_stop=$true;provider_and_recipient_in_flight=0;live_leases=0;live_claims=0;broker_backends=0}
 [IO.File]::WriteAllText((Join-Path $testRoot ($armDir+'/deletions.log')),'',$utf8)
 WriteJson ($armDir+'/authority-trace.json') @{observer_failed=$false;open_spans=0;dropped_events=0}
 WriteRows 'memory_restore_state' @(@{singleton=$true;watermark=0})
 $steps=@();$n=0
 foreach($step in $script:plan[0].steps){$n++;$base=$armDir+'/step-'+$n;$entry=@{step_id=$step.id;before_snapshot_path='';after_snapshot_path='';outcome_path=$base+'/outcome.json'}
  $outcome=@{state='not_executed_after_fixture_local_unknown';owner_input_id='';owner_run_id='';run_ids=@();automatic_run_ids=@();native_entry_ids=@();failure_phase='fixture_local_stop';fixture_stop_ref=$armDir+'/tables/behavior76_fixture_stops.jsonl';event_receipt=$null}
  if($Ordinary -or $n -eq 1){$entry.before_snapshot_path=$base+'/before.json';$entry.after_snapshot_path=$base+'/after.json';WriteJson $entry.before_snapshot_path @{deletion_watermark=0;deletion_manifest_sha256=(Digest ($armDir+'/deletions.log'))};WriteJson $entry.after_snapshot_path @{deletion_watermark=0;deletion_manifest_sha256=(Digest ($armDir+'/deletions.log'))};$outcome=@{state=$(if($Ordinary){'completed'}else{'fixture_local_unknown'});owner_input_id='public-input';owner_run_id='public-run';run_ids=@('public-run','public-auto');automatic_run_ids=@('public-auto');native_entry_ids=@();event_receipt=$null}}
  WriteJson $entry.outcome_path $outcome;$steps+=$entry
 }
 WriteJson 'index.json' @{version='behavior76-checker-index-v3-local-model-tail';stimuli_sha256=$pin.stimuli_sha256;episodes=@(@{episode_id='public-episode-1';masked_arm_id='public-arm-a';tables_dir=$armDir+'/tables';steps=$steps})}
 Reseal
}
function ReadAdapter {Read-Issue76LocalEvidenceV2 -ReportRoot $testRoot -Plan $script:plan -Pin $script:pin}
function TestCase($name,[scriptblock]$body){& $body;$results.Add(@{name=$name;status='pass'});Write-Output ('PASS '+$name)}
function Negative($name,[scriptblock]$mutate,[switch]$NoReseal){TestCase $name {Seed;& $mutate;if(-not $NoReseal){Reseal};$caught=$false;try{$null=ReadAdapter}catch{$caught=$true};Assert $caught ('Expected rejection: '+$name)}}

TestCase 'valid_tail_unavailable_no_snapshot_open_96_rows_and_receipts_retained' {
 Seed;$r=ReadAdapter;Assert ($r.owner_rows.Count -eq 96 -and $r.tail_opportunities -eq 1 -and $r.other_unavailable_opportunities -eq 94) 'Fixed denominator/missingness wrong'
 $tail=@($r.owner_rows|Where-Object disposition -CEQ 'not_executed_after_fixture_local_unknown');Assert ($tail.Count -eq 1 -and $tail[0].verdict -ceq 'unavailable' -and $tail[0].completion -ceq 'unavailable') 'Expected-denial tail promoted'
 Assert ($r.snapshot_reads.Count -eq 2 -and @($r.snapshot_reads|Where-Object {$_ -like '*/step-2/*' -or $_ -like '*/step-3/*' -or [string]::IsNullOrEmpty($_)}).Count -eq 0) 'Tail snapshot opened'
 Assert ($r.retained_observation_refs[0].actual_final_run_ids.Count -eq 2 -and $r.retained_observation_refs[0].actual_effect_ids[0] -ceq 'public-effect' -and $r.retained_observation_refs[0].automatic_run_ids[0] -ceq 'public-auto') 'Actual automatic/final/effect dropped'
 Assert ($r.executed_step_views.Count -eq 1 -and $r.executed_step_views[0].outcome.state -ceq 'fixture_local_unknown' -and $r.owner_rows[0].verdict -ceq 'unassessed') 'Stopping slot incorrectly promoted'
}
TestCase 'ordinary_original_step_refs_and_observations_unchanged' {Seed -Ordinary;$index=ReadJson 'index.json';$r=ReadAdapter;Assert ($r.executed_step_views.Count -eq 3 -and $r.snapshot_reads.Count -eq 6 -and $r.tail_opportunities -eq 0) 'Ordinary path changed';for($i=0;$i -lt 3;$i++){Assert ((Get-Issue76Canonical $index.episodes[0].steps[$i]) -ceq (Get-Issue76Canonical $r.executed_step_views[$i].original_export)) 'Original references mutated'}}
Negative 'fabricated_tail_snapshot' {$x=ReadJson 'index.json';$x.episodes[0].steps[2].before_snapshot_path=$armDir+'/step-1/before.json';WriteJson 'index.json' $x}
Negative 'fabricated_tail_run' {$p=$armDir+'/step-3/outcome.json';$x=ReadJson $p;$x.run_ids=@('invented');WriteJson $p $x}
Negative 'fabricated_tail_native' {$p=$armDir+'/step-3/outcome.json';$x=ReadJson $p;$x.native_entry_ids=@(1);WriteJson $p $x}
Negative 'fabricated_tail_event' {$p=$armDir+'/step-3/outcome.json';$x=ReadJson $p;$x.event_receipt=@{applied=$true};WriteJson $p $x}
Negative 'foreign_tail_stop_ref' {$p=$armDir+'/step-3/outcome.json';$x=ReadJson $p;$x.fixture_stop_ref='other/tables/stops.jsonl';WriteJson $p $x}
Negative 'missing_actual_stop' {WriteRows 'behavior76_fixture_stops' @()}
foreach($field in @('window','arm','producer','protocol_sha256','schema')){Negative ('stop_binding_'+$field) {$x=Get-Content -LiteralPath (Join-Path $testRoot ($armDir+'/tables/behavior76_fixture_stops.jsonl'))|ConvertFrom-Json -AsHashtable;$x.binding[$field]='wrong';WriteRows 'behavior76_fixture_stops' @($x)}}
Negative 'tail_before_stopping_slot' {$p=$armDir+'/step-1/outcome.json';$x=ReadJson $p;$x.state='completed';WriteJson $p $x}
Negative 'actual_tail_input' {WriteRows 'accepted_inputs' @(@{input_id='public-input';run_id='public-run';session_key='public-session';session_sequence=1;status='completed';event_id='behavior76:public-turn-1'},@{input_id='late';event_id='behavior76:public-turn-2'})}
Negative 'actual_tail_native_admission' {[IO.File]::WriteAllText((Join-Path $testRoot 'behavior76_attempts.jsonl'),'{"window_id":"public-window","n":1,"slot":"slot-01/public-arm-a/public-turn-2","run_id":"late"}',$utf8)}
Negative 'missing_quiescence' {[IO.File]::Delete((Join-Path $testRoot ($armDir+'/quiescence.json')))}
Negative 'live_claim' {$p=$armDir+'/quiescence.json';$x=ReadJson $p;$x.live_claims=1;WriteJson $p $x}
Negative 'quiescence_predates_barrier' {$p=$armDir+'/quiescence.json';$x=ReadJson $p;$x.observed_at='2026-10-24T00:00:00Z';WriteJson $p $x}
Negative 'late_unclassified_unknown' {$p=$armDir+'/tables/action_effects.jsonl';$e=Get-Content -LiteralPath (Join-Path $testRoot $p)|ConvertFrom-Json -AsHashtable;WriteRows 'action_effects' @($e,@{action_id='late-unknown';state='effect_unknown'})}
Negative 'diagnostic_identity_corruption' {WriteRows 'behavior76_recipient_diagnostics' @(@{action_id='public-effect';run_id='foreign';tool_call_id='public-tool';category='lost_ack'})}
Negative 'unknown_diagnostic_category' {WriteRows 'behavior76_recipient_diagnostics' @(@{action_id='public-effect';run_id='public-run';tool_call_id='public-tool';category='unclassified'})}
Negative 'path_traversal' {$x=ReadJson 'index.json';$x.episodes[0].steps[0].before_snapshot_path='../outside.json';WriteJson 'index.json' $x}
Negative 'sealed_byte_tamper' {[IO.File]::AppendAllText((Join-Path $testRoot ($armDir+'/step-1/before.json')),' ',$utf8)} -NoReseal
Negative 'missing_required_table' {[IO.File]::Delete((Join-Path $testRoot ($armDir+'/tables/memory_run_sources.jsonl')))}
Negative 'missing_anchor' {[IO.File]::Delete((Join-Path $testRoot ($armDir+'/deletions.log')))}
Negative 'anchor_final_snapshot_mismatch' {[IO.File]::WriteAllText((Join-Path $testRoot ($armDir+'/deletions.log')),'changed',$utf8)}
Negative 'watermark_final_snapshot_mismatch' {WriteRows 'memory_restore_state' @(@{singleton=$true;watermark=7})}
Negative 'authority_trace_open_span' {$p=$armDir+'/authority-trace.json';$x=ReadJson $p;$x.open_spans=1;WriteJson $p $x}
Negative 'authority_trace_observer_failure' {$p=$armDir+'/authority-trace.json';$x=ReadJson $p;$x.observer_failed=$true;WriteJson $p $x}
Negative 'root_hash_mismatch' {[IO.File]::AppendAllText((Join-Path $testRoot 'index.json'),' ',$utf8)} -NoReseal
TestCase 'old_frozen_scorers_unchanged' {Assert ((Get-FileHash (Join-Path $PSScriptRoot 'score-mechanical-v2.ps1')).Hash.ToLowerInvariant() -ceq 'c4d06cbc395d660eb9a2401b5d8f55bbc8e05db411d82e90b06f2e49dfb1c869') 'Old v2 scorer changed';Assert ((Get-FileHash (Join-Path $PSScriptRoot 'score-mechanical-fresh-v3.ps1')).Hash.ToLowerInvariant() -ceq 'e481f9c062a3bc4500b9f0b94da8819fbf287bec075490583cad17d54198721d') 'Old v3 scorer changed'}
$receipt=@{version='issue76-public-local-tail-parser-selftest-v2';passed=$results.Count;failed=0;provider_calls=0;hidden_controls_read=$false;mapping_read=$false;criteria_changed=$false;tests=$results.ToArray();public_test_root=$testRoot}
$receipt|ConvertTo-Json -Depth 20|Set-Content -LiteralPath (Join-Path $testRoot 'selftest-receipt.json')
Write-Output ('SELFTEST_PASS '+$results.Count+' '+$testRoot)
