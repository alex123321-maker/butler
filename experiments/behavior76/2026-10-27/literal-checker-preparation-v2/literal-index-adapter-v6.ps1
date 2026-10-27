# PREPARED additive reader: old files remain pinned and unchanged.
foreach($p in @(@('local-tail-adapter-v4.ps1','4e6bdc53519a1d9dce02a10e2c5a8887becbed8205e791c16c11d13d233c344a'),@('local-model-proof-reader-v4.ps1','2241e8d023b6f00e28dbfc4599a74d5d2b571bf1bb2667faf0de5c1c7f85fd39'))){if((Get-FileHash -LiteralPath (Join-Path $PSScriptRoot $p[0])).Hash.ToLowerInvariant() -cne $p[1]){throw 'literal_v6:frozen_dependency_changed'}}
. (Join-Path $PSScriptRoot 'local-tail-adapter-v4.ps1')
. (Join-Path $PSScriptRoot 'literal-kernel-proof-v1.ps1')
$Issue76LiteralRequiredTables=@($Issue76LocalRequiredTables)+@('behavior76_business_deadlines','behavior76_business_deadline_events','behavior76_business_observations')
function New-Issue76LiteralPlanV6 {
 param([Parameter(Mandatory)][object[]]$Envelopes,[Parameter(Mandatory)]$Manifest)
 Assert-Issue76LiteralV6 ($Envelopes.Count -eq 16 -and @($Manifest.schedule).Count -eq 16) 'fixed16episode_plan_required'
 $families=@{ambiguous_object='clarification';accepted_preference_conflict='preference_lifecycle';repeated_feedback='feedback_candidate';status_initiative='status_continuation';format_amendment='amendment';deadline_amendment='amendment';recipient_amendment='amendment';scoped_action_stop='scoped_stop_negation'}
 $byID=@{};$pairs=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
 foreach($e in $Envelopes){
  Assert-Issue76LiteralKeysV6 $e @('version','dimension','polarity','episode') @('business_deadlines','business_deadline_changes','owner_task_selections')
  Assert-Issue76LiteralV6 ($e.version -ceq 'behavior76-literal-supplement-v11' -and $families.ContainsKey([string]$e.dimension) -and $e.polarity -cin @('positive','negative') -and $pairs.Add($e.dimension+'|'+$e.polarity)) 'literal_dimension_pair'
  $ep=$e.episode
  Assert-Issue76LiteralV6 (-not [string]::IsNullOrWhiteSpace([string]$ep.id) -and -not $byID.ContainsKey([string]$ep.id) -and $ep.family -ceq $families[$e.dimension] -and $ep.provenance.kind -ceq 'synthetic') 'literal_episode_identity'
  $graded=@($ep.steps|Where-Object evaluate);$wanted=if($e.dimension -ceq 'repeated_feedback'){3}else{2}
  Assert-Issue76LiteralV6 ($graded.Count -eq $wanted) 'literal_per_episode_owner_denominator'
  $stepIDs=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
  foreach($s in $ep.steps){
   Assert-Issue76LiteralV6 ($stepIDs.Add([string]$s.id) -and $s.event.type -cne 'owner_control') 'literal_step_identity_or_typed_control'
   if($s.event.type -ceq 'owner_message'){
    Assert-Issue76LiteralV6 ($s.evaluate -ceq $true -and -not [string]::IsNullOrEmpty([string]$s.event.text)) 'literal_owner_exchange_not_real'
    foreach($k in @('intent','kind','task_key','instruction','resource_key','new_body','provenance','source_key','operation','execution_policy')){Assert-Issue76LiteralV6 ([string]::IsNullOrEmpty([string]$s.event[$k])) 'literal_precomputed_intent_or_other_event_field'}
    foreach($k in @('target_task_key','goal','constraints','summary')){Assert-Issue76LiteralV6 ($null -eq $s.event[$k]) 'literal_precomputed_target_or_mutation'}
   }
   if($s.evaluate){Assert-Issue76LiteralV6 ($s.event.type -ceq 'owner_message') 'literal_non_owner_graded_step'}
  }
  $selectionIDs=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
  foreach($selection in @($e.owner_task_selections|Where-Object {$null -ne $_})){
   Assert-Issue76LiteralKeysV6 $selection @('step_id','task_key','provenance')
   $s=Get-Issue76LiteralOneV6 $graded 'id' $selection.step_id
   Assert-Issue76LiteralV6 ($selectionIDs.Add([string]$selection.step_id) -and $selection.provenance -ceq 'explicit_owner_ui' -and $s.run_task_key -ceq $selection.task_key -and @($ep.initial.tasks|Where-Object key -CEQ $selection.task_key).Count -eq 1) 'literal_ui_selection_not_declared'
  }
  foreach($s in $graded){Assert-Issue76LiteralV6 ((-not [string]::IsNullOrEmpty([string]$s.run_task_key)) -eq $selectionIDs.Contains([string]$s.id)) 'literal_selection_not_one_to_one'}
  $resources=@($ep.initial.resources|ForEach-Object {$_.key});$declaredDates=@{}
  foreach($d in @($e.business_deadlines|Where-Object {$null -ne $_})){
   Assert-Issue76LiteralKeysV6 $d @('resource_key','deadline_at')
   Assert-Issue76LiteralV6 ($d.resource_key -cin $resources -and -not $declaredDates.ContainsKey([string]$d.resource_key) -and ([datetimeoffset]$d.deadline_at).Offset -eq [timespan]::Zero) 'literal_business_date_declaration'
   $declaredDates[$d.resource_key]=$true
  }
  $changes=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
  foreach($d in @($e.business_deadline_changes|Where-Object {$null -ne $_})){
   Assert-Issue76LiteralKeysV6 $d @('before_step_id','resource_key','deadline_at','provenance')
   Assert-Issue76LiteralV6 ($declaredDates.ContainsKey([string]$d.resource_key) -and $stepIDs.Contains([string]$d.before_step_id) -and $changes.Add($d.before_step_id+'|'+$d.resource_key) -and $d.provenance -ceq 'synthetic_service_configuration' -and ([datetimeoffset]$d.deadline_at).Offset -eq [timespan]::Zero) 'literal_business_change_declaration'
  }
  if($e.dimension -ceq 'deadline_amendment'){Assert-Issue76LiteralV6 ($declaredDates.Count -ge 1 -and $changes.Count -ge 1) 'literal_business_dimension_has_no_real_observation_contract'}
  $byID[$ep.id]=$e
 }
 Assert-Issue76LiteralV6 ($pairs.Count -eq 16) 'literal_missing_positive_negative_pair'
 $plan=[Collections.Generic.List[object]]::new();$seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
 foreach($schedule in $Manifest.schedule){
  Assert-Issue76LiteralV6 ($byID.ContainsKey([string]$schedule.episode_id) -and $seen.Add([string]$schedule.episode_id) -and @($schedule.arm_order).Count -eq 2 -and $schedule.arm_order[0] -cne $schedule.arm_order[1]) 'literal_schedule_identity'
  $e=$byID[$schedule.episode_id]
  foreach($arm in $schedule.arm_order){$plan.Add(@{episode_id=$e.episode.id;masked_arm_id=$arm;family=$e.episode.family;dimension=$e.dimension;polarity=$e.polarity;steps=$e.episode.steps;owner_task_selections=$e.owner_task_selections})}
 }
 return ,$plan.ToArray()
}
function Assert-Issue76LiteralPlanBindingV6($Plan,$Manifest,$Index) {
 Assert-Issue76LiteralV6 ($Plan.Count -eq 32 -and @($Plan|ForEach-Object {$_.steps|Where-Object evaluate}).Count -eq 68 -and @($Plan|Group-Object dimension).Count -eq 8 -and @($Plan|Group-Object family).Count -eq 6) 'literal_fixed32_68_8_6_plan'
 foreach($root in @($Manifest,$Index)){
  $v=$root.literal_plan;Assert-Issue76LiteralKeysV6 $v @('version','episodes','arms','owner_opportunities','dimensions','planned_opportunities')
  Assert-Issue76LiteralV6 ($v.version -ceq 'behavior76-literal-supplement-v11' -and $v.episodes -eq 16 -and $v.arms -eq 32 -and $v.owner_opportunities -eq 68 -and $v.dimensions -eq 8) 'literal_plan_envelope'
  $tuples=@($Plan|ForEach-Object {$p=$_;foreach($s in $p.steps|Where-Object evaluate){@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$s.id}}})
  Assert-Issue76LiteralV6 (@($v.planned_opportunities).Count -eq 68) 'literal_all68_planned_before_collection'
  for($i=0;$i -lt 68;$i++){
   Assert-Issue76LiteralKeysV6 $v.planned_opportunities[$i] @('episode_id','masked_arm_id','step_id')
   foreach($k in @('episode_id','masked_arm_id','step_id')){Assert-Issue76LiteralV6 ($v.planned_opportunities[$i][$k] -ceq $tuples[$i][$k]) 'literal_planned_tuple_order_or_identity'}
  }
 }
 Assert-Issue76LiteralV6 ($Manifest.planned_arms -eq 32 -and $Manifest.planned_owner_opportunities -eq 68 -and $Manifest.planned_dimensions -eq 8 -and $Manifest.planned_families -eq 6 -and @($Index.episodes).Count -le 32) 'literal_manifest_totals'
}
function Read-Issue76LiteralEvidenceV6 {
 [CmdletBinding()]
 param([Parameter(Mandatory)][string]$ReportRoot,[Parameter(Mandatory)][object[]]$Plan,[Parameter(Mandatory)][hashtable]$Pin)
 $ErrorActionPreference='Stop'
 $root=(Resolve-Path -LiteralPath $ReportRoot).Path
 if((Get-Item -LiteralPath $root).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Report root reparse point denied'}
 $reads=[Collections.Generic.List[string]]::new();$snapshotReads=[Collections.Generic.List[string]]::new();$hashes=@{}
 function FileHash([string]$r){if(-not $hashes.ContainsKey($r)){$hashes[$r]=(Get-FileHash -LiteralPath (Resolve-Issue76EvidencePath $root $r) -Algorithm SHA256).Hash.ToLowerInvariant()};return $hashes[$r]}
 function JsonFile([string]$r,[switch]$Snapshot){$p=Resolve-Issue76EvidencePath $root $r;$reads.Add($r);if($Snapshot){$snapshotReads.Add($r)};return ConvertFrom-Issue76LiteralJsonV6 ([IO.File]::ReadAllText($p))}
 function Table([string]$r){$p=Resolve-Issue76EvidencePath $root $r;$reads.Add($r);return ,@([IO.File]::ReadLines($p)|Where-Object {$_.Trim()}|ForEach-Object {ConvertFrom-Issue76LiteralJsonV6 $_})}
 function One($Rows,[string]$Field,[string]$Value){$a=@($Rows|Where-Object {$_[''+$Field] -ceq $Value});if($a.Count -ne 1){throw 'Missing/ambiguous durable identity'};return $a[0]}
 function Require([bool]$OK,[string]$Why){if(-not $OK){throw $Why}}
 foreach($k in @('window','producer','protocol_sha256','stimuli_sha256','manifest_sha256','index_sha256','attempts_sha256','windows_sha256')){Require (-not [string]::IsNullOrWhiteSpace([string]$Pin[$k])) 'Missing independent identity pin'}
 Require ((FileHash 'manifest.json') -ceq $Pin.manifest_sha256 -and (FileHash 'index.json') -ceq $Pin.index_sha256) 'Root identity hash mismatch'
 $manifest=JsonFile 'manifest.json';$index=JsonFile 'index.json'
 Require ($index.version -ceq 'behavior76-checker-index-v6-literal') 'Unsupported prospective index version'
 Require ($manifest.window -ceq $Pin.window -and $manifest.source -ceq $Pin.producer -and $manifest.protocol_sha256 -ceq $Pin.protocol_sha256 -and $manifest.stimuli_sha256 -ceq $Pin.stimuli_sha256 -and $index.stimuli_sha256 -ceq $Pin.stimuli_sha256) 'Prospective binding mismatch'
 Require ($manifest.local_unknown_mode -ceq 'whole_fixture_after_exact_classified_effect_unknown' -and $manifest.global_priority -ceq $true) 'Trusted local mode missing'
 Require ($manifest.local_model_failure_mode -ceq 'whole_fixture_after_exact_empty_answer_v1') 'Explicit prospective model-failure mode absent'
 Require ($manifest.local_http_failure_mode -ceq 'whole_fixture_after_exact_http_do_no_response_v1') 'Explicit prospective HTTP-failure mode absent'
 Require ($manifest.local_response_failure_mode -ceq 'whole_fixture_after_exact_response_rejection_v1') 'Explicit prospective response-failure mode absent'
 Require ((FileHash 'behavior76_attempts.jsonl') -ceq $Pin.attempts_sha256 -and (FileHash 'behavior76_windows.jsonl') -ceq $Pin.windows_sha256) 'Native root journal seal mismatch'
 $windows=Table 'behavior76_windows.jsonl';$window=One $windows 'id' $Pin.window
 Require ($window.closed -ceq $true) 'Live window cannot be audited as closed'
 $allAttempts=Table 'behavior76_attempts.jsonl';$attempts=@($allAttempts|Where-Object window_id -CEQ $Pin.window|ForEach-Object {@{n=$_.n;slot=$_.slot;run_id=$_.run_id;status=$_.status;input_tokens=$_.input_tokens;output_tokens=$_.output_tokens;total_tokens=$_.total_tokens;thinking_tokens=$_.thinking_tokens;provider_stage=$_.provider_stage}})
 Require ((Test-Issue76Integer $window.admitted) -and $window.admitted -eq $attempts.Count -and $window.admitted -le 768) 'Actual native window counter mismatch'
 $orderedAttempts=@($attempts|Sort-Object n);for($ai=0;$ai -lt $orderedAttempts.Count;$ai++){Require ((Test-Issue76Integer $orderedAttempts[$ai].n) -and $orderedAttempts[$ai].n -eq ($ai+1)) 'Missing or duplicate native reservation identity'}
 Assert-Issue76LiteralPlanBindingV6 $Plan $manifest $index
 $keys=@{};foreach($p in $Plan){$keys[$p.episode_id+'|'+$p.masked_arm_id]=$true}
 $rows=[Collections.Generic.List[object]]::new();$executed=[Collections.Generic.List[object]]::new();$retained=[Collections.Generic.List[object]]::new();$nativeFailures=[Collections.Generic.List[object]]::new();$tails=0;$missing=0;$represented=@{}
 for($pi=0;$pi -lt 32;$pi++){
  $p=$Plan[$pi];$armDir='slot-'+([int]([math]::Floor($pi/2)+1)).ToString('00')+'/'+$p.masked_arm_id
  if($pi -ge @($index.episodes).Count){foreach($step in $p.steps|Where-Object evaluate){$rows.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$step.id;disposition='unavailable_unindexed_arm';admission='unavailable';semantic_observation='unavailable';verdict='unavailable'});$missing++};continue}
  $case=$index.episodes[$pi];Require ($case.episode_id -ceq $p.episode_id -and $case.masked_arm_id -ceq $p.masked_arm_id -and $case.tables_dir -ceq $armDir+'/tables' -and $case.dimension -ceq $p.dimension -and $case.polarity -ceq $p.polarity) 'Nonprefix or foreign indexed arm'
  $seal=JsonFile ($armDir+'/sealed-files.json');Require ($seal.version -ceq 'behavior76-quiescent-fixture-files-v1') 'Fixture seal absent'
  $listed=@{};foreach($f in $seal.files){$rel=$armDir+'/'+$f.path;Require (-not $listed.ContainsKey($rel)) 'Duplicate sealed file';$listed[$rel]=$true;$full=Resolve-Issue76EvidencePath $root $rel;Require ((Get-Item -LiteralPath $full).Length -eq $f.bytes -and (FileHash $rel) -ceq $f.sha256) 'Sealed fixture bytes changed'}
  $all=@(Get-ChildItem -LiteralPath (Join-Path $root $armDir) -File -Recurse|ForEach-Object {$v=[IO.Path]::GetRelativePath($root,$_.FullName).Replace('\','/');if($v -cne $armDir+'/sealed-files.json'){$v}})
  Require ($all.Count -eq $listed.Count -and @($all|Where-Object {-not $listed.ContainsKey($_)}).Count -eq 0) 'Fixture inventory not exact'
  $tables=@{};foreach($name in $Issue76LiteralRequiredTables){$rel=$case.tables_dir+'/'+$name+'.jsonl';Require ($listed.ContainsKey($rel)) 'Required durable table not sealed';$tables[$name]=Table $rel}
  $business=Assert-Issue76LiteralBusinessEvidenceV6 $tables
  $q=JsonFile ($armDir+'/quiescence.json');Require ($q.broker_reaped -ceq $true -and $q.runtime_stopped -ceq $true -and $q.broker_application_observed_before_stop -ceq $true -and $q.provider_and_recipient_in_flight -eq 0 -and $q.live_leases -eq 0 -and $q.live_claims -eq 0 -and $q.broker_backends -eq 0) 'Quiescence proof missing'
  Require (-not [string]::IsNullOrWhiteSpace([string]$q.schema)) 'Actual quiescence schema absent'
  $null=Get-Issue76ProofTime $q.observed_at
  $anchor=$armDir+'/deletions.log';Require ($listed.ContainsKey($anchor)) 'Exact anchor absent'
  $trace=JsonFile ($armDir+'/authority-trace.json');Require ($trace.observer_failed -ceq $false -and $trace.open_spans -eq 0 -and $trace.dropped_events -eq 0) 'Authority observer not quiescent/available'
  # Reconcile every current UNKNOWN, including outcomes later than the initial
  # immutable stop snapshot. An unclassified later outcome is never excused.
  foreach($unknown in @($tables.action_effects|Where-Object state -CEQ 'effect_unknown')){
   $ui=One $tables.action_intents 'effect_id' $unknown.action_id;$ud=One $tables.behavior76_recipient_diagnostics 'action_id' $unknown.action_id
   Require ($ud.category -cin @('foreign_task','cas_rejected','lost_ack') -and $ud.run_id -ceq $ui.run_id -and $ud.tool_call_id -ceq $ui.tool_call_id) 'Late or unclassified UNKNOWN is global integrity failure'
  }
  $armAttempts=@($allAttempts|Where-Object {$_.window_id -ceq $Pin.window -and $_.slot -is [string] -and $_.slot.StartsWith($armDir+'/',[StringComparison]::Ordinal)})
  $stops=@($tables.behavior76_fixture_stops);$stop=$null
  if($stops.Count){Require ($stops.Count -eq 1) 'Multiple local-stop records';$stop=$stops[0];$b=$stop.binding;Require ($stop.cause -cin @('effect_unknown','model_empty_answer','model_http_no_response','model_response_rejected') -and $b.window -ceq $Pin.window -and $b.arm -ceq $armDir -and $b.producer -ceq $Pin.producer -and $b.protocol_sha256 -ceq $Pin.protocol_sha256 -and $b.schema -ceq $q.schema -and $b.mode -ceq 'fixture-local-response-v10' -and $q.fixture_locally_stopped -ceq $true) 'Local-stop immutable binding mismatch'
   if($stop.cause -ceq 'effect_unknown'){
   Require (@($stop.effects).Count -gt 0 -and $null -eq $stop.native_evidence) 'Stop has no actual effect or mixes cause proofs'
   foreach($proof in $stop.effects){$e=One $tables.action_effects 'action_id' $proof.effect.action_id;$i=One $tables.action_intents 'effect_id' $e.action_id;$d=One $tables.behavior76_recipient_diagnostics 'action_id' $e.action_id;$run=One $tables.runs 'run_id' $i.run_id;$input=One $tables.accepted_inputs 'run_id' $i.run_id;$auth=One $tables.action_authorizations 'authorization_id' $e.authorization_id
    Require ($e.state -ceq 'effect_unknown' -and $proof.effect.state -ceq 'effect_unknown' -and $d.category -cin @('foreign_task','cas_rejected','lost_ack') -and $d.run_id -ceq $i.run_id -and $d.tool_call_id -ceq $i.tool_call_id) 'Unclassified or foreign UNKNOWN proof'
    Require ((Get-Issue76Canonical $proof.intent_binding) -ceq (Get-Issue76Canonical $i.binding) -and $e.initial_binding.run_id -ceq $i.run_id -and $e.initial_binding.tool_call_id -ceq $i.tool_call_id -and $run.task_id -ceq $i.binding.task_id -and $auth.action_id -ceq $i.action_id) 'Effect/intent/run binding mismatch'
    Require (@($proof.owner_inputs|Where-Object {$_.input_id -ceq $input.input_id -and $_.run_id -ceq $input.run_id -and $_.session_key -ceq $input.session_key -and $_.session_sequence -eq $input.session_sequence}).Count -eq 1) 'Stop input witness absent'
    $dispatch=@($tables.action_effect_dispatches|Where-Object {$_.action_id -ceq $e.action_id -and $_.authorization_id -ceq $e.authorization_id -and $_.execution_action_id -ceq $i.action_id});Require ($dispatch.Count -eq 1 -and @($proof.dispatches|Where-Object {$_.action_id -ceq $e.action_id -and $_.authorization_id -ceq $e.authorization_id -and $_.execution_action_id -ceq $i.action_id}).Count -eq 1) 'Actual dispatch absent'
   }
   }else{$null=Assert-Issue76ModelStopV4 $stop $tables $armAttempts $Pin.window $armDir}
  }
  $failedEntries=@()
  foreach($a in $armAttempts){
   if($a.status -ceq 'transport_error'){$v=Assert-Issue76LocalNativeAttemptV4 $a $tables $Pin.window $armDir;Require ($null -ne $stop) 'Model failure lacks durable arm barrier';$failedEntries+=@{attempt=$a;input=$v.input;cause=$v.cause}}
   else{Require ($a.status -cin @('completed','tool_handoff')) 'Other native failure remains GLOBAL';Require (@($a.events|Where-Object {$_.type -cin @('transport_error','entry_failed','run_cancelled','run_timed_out')}).Count -eq 0) 'Failed native event hidden by status'}
   if($null -ne $stop){Require ((Get-Issue76ProofTime $a.started_at) -le (Get-Issue76ProofTime $stop.observed_at)) 'Same-arm native admission after local barrier'}
  }
  Require ((@($tables.action_effects|Where-Object state -CEQ 'effect_unknown').Count -eq 0) -or $null -ne $stop) 'UNKNOWN lacks durable local barrier'
  if($null -ne $stop){Require ([datetimeoffset]$q.observed_at -ge [datetimeoffset]$stop.observed_at) 'Quiescence predates local barrier'}
  Require (@($case.steps).Count -le @($p.steps).Count) 'Foreign original step count'
  $stopStepSeen=$false;$tailSeen=$false;$lastAfter=$null
  for($si=0;$si -lt @($p.steps).Count;$si++){
   $step=$p.steps[$si]
   if($si -ge @($case.steps).Count){if($step.evaluate){$rows.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$step.id;disposition='unavailable_unrecorded_step';admission='unavailable';semantic_observation='unavailable';verdict='unavailable'});$missing++};continue}
   $entry=$case.steps[$si];Require ($entry.step_id -ceq $step.id) 'Original step order changed';$outcome=JsonFile $entry.outcome_path
   Require ($entry.outcome_path.StartsWith($armDir+'/',[StringComparison]::Ordinal) -and $listed.ContainsKey($entry.outcome_path)) 'Foreign/unsealed outcome'
   if($outcome.state -cin @('not_executed_after_fixture_local_unknown','not_executed_after_fixture_local_model_failure','not_executed_after_fixture_local_http_failure','not_executed_after_fixture_local_response_failure')){
    Require ($null -ne $stop -and $stopStepSeen) 'Tail precedes actual local-stop slot'
    $wantedTail=switch -CaseSensitive ($stop.cause){'model_empty_answer'{'not_executed_after_fixture_local_model_failure'} 'model_http_no_response'{'not_executed_after_fixture_local_http_failure'} 'model_response_rejected'{'not_executed_after_fixture_local_response_failure'} default{'not_executed_after_fixture_local_unknown'}}
    Require ($outcome.state -ceq $wantedTail) 'Tail cause does not match immutable stop'
    Require ([string]::IsNullOrEmpty([string]$entry.before_snapshot_path) -and [string]::IsNullOrEmpty([string]$entry.after_snapshot_path)) 'Tail fabricates snapshots'
    Require ([string]::IsNullOrEmpty([string]$outcome.owner_input_id) -and [string]::IsNullOrEmpty([string]$outcome.owner_run_id) -and @($outcome.run_ids).Count -eq 0 -and @($outcome.automatic_run_ids).Count -eq 0 -and @($outcome.native_entry_ids).Count -eq 0 -and $null -eq $outcome.event_receipt) 'Tail fabricates executed identity'
    Require ($outcome.failure_phase -ceq 'fixture_local_stop' -and $outcome.fixture_stop_ref -ceq $case.tables_dir+'/behavior76_fixture_stops.jsonl') 'Missing or foreign tail stop-ref'
    Require (@($tables.accepted_inputs|Where-Object event_id -CEQ ('behavior76:'+$step.id)).Count -eq 0 -and @($attempts|Where-Object slot -CEQ ($armDir+'/'+$step.id)).Count -eq 0) 'Unexecuted tail has actual admissions'
    $tailSeen=$true
    if($step.evaluate){$rows.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$step.id;disposition=$outcome.state;admission='unavailable';semantic_observation='unavailable';verdict='unavailable';structured_contract='unavailable';objective_state_effects='unavailable';grounding='unavailable';clarification='unavailable';preference_scope_provenance='unavailable';completion='unavailable';fixture_stop_ref=$outcome.fixture_stop_ref;stop_sha256=(FileHash $outcome.fixture_stop_ref)});$tails++}
    continue
   }
   Require (-not $stopStepSeen) 'Original step executed after local barrier'
   Require (-not $tailSeen) 'Executed work follows local-stop tail'
   foreach($path in @($entry.before_snapshot_path,$entry.after_snapshot_path)){Require ($path.StartsWith($armDir+'/',[StringComparison]::Ordinal) -and $listed.ContainsKey($path)) 'Executed snapshot absent/foreign/unsealed';$lastAfter=JsonFile $path -Snapshot}
   if($outcome.state -ceq 'fixture_local_unknown'){Require ($null -ne $stop) 'Local disposition without stop proof';$stopStepSeen=$true}
   if($outcome.state -ceq 'fixture_local_unknown'){Require ($stop.cause -ceq 'effect_unknown') 'Wrong local cause disposition'}
   if($outcome.state -cin @('fixture_local_model_failure','fixture_local_http_failure','fixture_local_response_failure')){
    $wantedCause=switch -CaseSensitive ($outcome.state){'fixture_local_response_failure'{'model_response_rejected'} 'fixture_local_http_failure'{'model_http_no_response'} default{'model_empty_answer'}}
    Require ($null -ne $stop -and $stop.cause -ceq $wantedCause -and $stop.native_evidence.n -in @($outcome.native_entry_ids) -and $stop.native_evidence.run_id -cin @($outcome.run_ids) -and $stop.native_evidence.slot -ceq $armDir+'/'+$step.id) 'Model failure disposition lacks exact original native slot'
    Require ($outcome.fixture_stop_ref -ceq $case.tables_dir+'/behavior76_fixture_stops.jsonl') 'Model failure stop reference absent'
    $stopStepSeen=$true
   }
   $kernel=$null
   if($step.evaluate){
    $aliases=JsonFile $case.aliases_path
    $null=Assert-Issue76LiteralTaskSelectionV6 $step $p.owner_task_selections $outcome.owner_task_selection $aliases
    if($null -ne $outcome.kernel_response){$kernel=Assert-Issue76LiteralKernelV6 -Outcome $outcome -Step $step -Tables $tables -Aliases $aliases -Selections $p.owner_task_selections}
    $automatic=Assert-Issue76LiteralAutomaticInputsV6 $outcome $tables
    foreach($failure in $automatic.non_success){
     if($failure.status -cin @('queued','running')){throw 'literal_v6:automatic_cohort_not_quiescent'}
     if($failure.error_code -ceq 'owner_control' -and $failure.status -ceq 'cancelled'){continue}
     # An ordinary durable approval/interaction wait is not a Core error or a
     # new local failure exemption. Retain it as not-success, never resume it.
     if($failure.status -ceq 'waiting_for_user' -and [string]::IsNullOrEmpty([string]$failure.error_code) -and $failure.actual_checkpoint -cin @('awaiting_approval','awaiting_interaction') -and $failure.actual_run_state -ceq 'awaiting_approval'){continue}
     $local=@($failedEntries|Where-Object {$_.input.input_id -ceq $failure.input_id})
     if($local.Count -eq 0){throw 'literal_v6:automatic_non_success_outside_original_local_union_is_global'}
    }
   }
   $executed.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$step.id;original_export=$entry;outcome=$outcome;criteria='unchanged; evaluate original actual observations only';tables_dir=$case.tables_dir})
   if($null -ne $kernel){$executed[-1]['kernel_proof']=$kernel}

   if($step.evaluate){$represented[$p.dimension]=$true;$rows.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$step.id;dimension=$p.dimension;polarity=$p.polarity;origin=$(if($null -ne $kernel){'input_service_kernel'}else{'owner_model_or_actual_denial'});kernel_proof=$kernel;disposition='actual_recorded_evaluation';admission=$outcome.state;semantic_observation='requires_original_actual_evidence_review';verdict='unassessed';outcome_path=$entry.outcome_path})}
  }
  if($null -ne $stop){Require $stopStepSeen 'Durable barrier has no original stopping outcome'}
  foreach($failure in $failedEntries){
   $a=$failure.attempt;$views=@($executed|Where-Object {$_.episode_id -ceq $p.episode_id -and $_.masked_arm_id -ceq $p.masked_arm_id -and $a.n -in @($_.outcome.native_entry_ids) -and $a.run_id -cin @($_.outcome.run_ids) -and $a.slot -ceq $armDir+'/'+$_.step_id})
   Require ($views.Count -eq 1) 'Native failure lacks unique actual step/run references'
   $isOwner=$views[0].outcome.owner_run_id -ceq $a.run_id
   Require (($failure.input.input_id -ceq $views[0].outcome.owner_input_id) -eq $isOwner) 'Owner versus automatic input identity mismatch'
   if(-not $isOwner){Require ($a.run_id -cin @($views[0].outcome.automatic_run_ids)) 'Foreign automatic failure identity'}
   $nativeFailures.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$views[0].step_id;window_id=$a.window_id;n=$a.n;run_id=$a.run_id;input_id=$failure.input.input_id;kind=$a.kind;cause=$failure.cause;owner_run=$isOwner;actual_final_for_failed_run=@($tables.run_final_results|Where-Object run_id -CEQ $a.run_id).Count;error_code=$a.events[-1].error_code;safe_reason=$a.events[-1].safe_reason;partial_text_delta_events=$(if($failure.cause -ceq 'model_response_rejected'){@($a.events).Count-2}else{0});partial_text_quality='unassessed';http_status=0;input_tokens=$a.input_tokens;output_tokens=$a.output_tokens;total_tokens=$a.total_tokens;thinking_tokens=$a.thinking_tokens;model_version=$a.model_version;verdict='failed';success=$false;critical_unknown_waived=$false;evidence_ref='behavior76_attempts.jsonl';fixture_stop_ref=$case.tables_dir+'/behavior76_fixture_stops.jsonl'})
  }
  if($null -ne $lastAfter){Require ($lastAfter.deletion_manifest_sha256 -ceq (FileHash $anchor) -and @($tables.memory_restore_state).Count -eq 1 -and $lastAfter.deletion_watermark -eq $tables.memory_restore_state[0].watermark) 'Final actual anchor/watermark mismatch'}
  $retained.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;tables_dir=$case.tables_dir;business_observation_proof=$business;actual_final_run_ids=@($tables.run_final_results|ForEach-Object {$_.run_id});actual_effect_ids=@($tables.action_effects|ForEach-Object {$_.action_id});automatic_run_ids=@($executed|Where-Object {$_.episode_id -ceq $p.episode_id -and $_.masked_arm_id -ceq $p.masked_arm_id}|ForEach-Object {$_.outcome.automatic_run_ids});quiescence_sha256=(FileHash ($armDir+'/quiescence.json'));authority_trace_sha256=(FileHash ($armDir+'/authority-trace.json'));anchor_sha256=(FileHash $anchor)})
 }
 Require ($rows.Count -eq 68 -and @($rows|Group-Object {$_.episode_id+'|'+$_.masked_arm_id+'|'+$_.step_id}|Where-Object Count -ne 1).Count -eq 0) 'Original68 literal row denominator lost'
 return [ordered]@{version='issue76-literal-index-adapter-v6';criteria_changed=$false;thresholds_changed=$false;mapping_read=$false;planned_episodes=16;planned_arms=32;planned_owner_opportunities=68;planned_dimensions=8;planned_families=6;represented_dimensions=$represented.Count;recorded_arms=@($index.episodes).Count;tail_opportunities=$tails;other_unavailable_opportunities=$missing;owner_rows=$rows.ToArray();executed_step_views=$executed.ToArray();retained_observation_refs=$retained.ToArray();native_failure_refs=$nativeFailures.ToArray();model_failure_is_success=$false;wrapper_attempt_metadata=$attempts;snapshot_reads=$snapshotReads.ToArray();file_reads=$reads.ToArray();decision='parser evidence availability only; no acceptance/promotion/closure verdict'}
}
