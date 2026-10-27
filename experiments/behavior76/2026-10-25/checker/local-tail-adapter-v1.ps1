# Public, prospective evidence parser only. It never changes a criterion or raw
# artifact, invokes a provider, reads a mapping, or promotes a semantic result.
$Issue76LocalRequiredTables = @('accepted_inputs','action_authorizations','action_effect_dispatches','action_effect_evidence','action_effects','action_intents','action_mandates','behavior76_aliases','behavior76_object_events','behavior76_objects','behavior76_observations','behavior76_recipient_receipts','behavior76_task_mandates','execution_budget_reservations','execution_budget_scopes','execution_model_attempts','memory_claim_temporal_heads','memory_claim_temporal_revisions','memory_claims','memory_deletions','memory_fact_decision_history','memory_fact_heads','memory_profile','memory_restore_state','memory_run_sources','memory_run_transcripts','memory_scope_grants','memory_source_dependencies','memory_source_revisions','memory_sources','memory_summary_dependencies','memory_summary_sources','memory_temporal_run_pins','memory_temporal_source_pins','messages','owner_goals','run_final_results','runs','session_ownership','session_summaries','sessions','task_authority_bindings','task_control_events','task_control_targets','task_results','tasks','tool_calls','transcript_heads','behavior76_fixture_stops','behavior76_recipient_diagnostics')

function Resolve-Issue76EvidencePath([string]$Root,[string]$Relative) {
 if([string]::IsNullOrWhiteSpace($Relative) -or [IO.Path]::IsPathRooted($Relative) -or $Relative.Contains('\') -or $Relative.Contains(':') -or @($Relative.Split('/') | Where-Object {$_ -in @('','..','.')}).Count){throw 'Invalid nonempty contained evidence path'}
 $full=[IO.Path]::GetFullPath((Join-Path $Root $Relative));$prefix=$Root.TrimEnd([IO.Path]::DirectorySeparatorChar)+[IO.Path]::DirectorySeparatorChar
 if(-not $full.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase)){throw 'Evidence escapes root'}
 $current=$Root
 foreach($part in $Relative.Split('/')){$current=Join-Path $current $part;$item=Get-Item -LiteralPath $current -Force -ErrorAction Stop;if($item.Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Evidence symlink/reparse point denied'}}
 if(-not [IO.File]::Exists($full)){throw 'Evidence file absent'}
 return $full
}
function Get-Issue76Canonical($Value) {
 if($null -eq $Value){return 'null'}
 if($Value -is [Collections.IDictionary]){$o=[ordered]@{};foreach($k in @($Value.Keys|Sort-Object)){$o[$k]=Get-Issue76Canonical $Value[$k]};return ConvertTo-Json -InputObject $o -Compress -Depth 100}
 if($Value -is [array]){return '['+(($Value|ForEach-Object {Get-Issue76Canonical $_}) -join ',')+']'}
 return ConvertTo-Json -InputObject $Value -Compress -Depth 100
}

function Read-Issue76LocalEvidence {
 [CmdletBinding()]
 param([Parameter(Mandatory)][string]$ReportRoot,[Parameter(Mandatory)][object[]]$Plan,[Parameter(Mandatory)][hashtable]$Pin)
 $ErrorActionPreference='Stop'
 $root=(Resolve-Path -LiteralPath $ReportRoot).Path
 if((Get-Item -LiteralPath $root).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Report root reparse point denied'}
 $reads=[Collections.Generic.List[string]]::new();$snapshotReads=[Collections.Generic.List[string]]::new();$hashes=@{}
 function FileHash([string]$r){if(-not $hashes.ContainsKey($r)){$hashes[$r]=(Get-FileHash -LiteralPath (Resolve-Issue76EvidencePath $root $r) -Algorithm SHA256).Hash.ToLowerInvariant()};return $hashes[$r]}
 function JsonFile([string]$r,[switch]$Snapshot){$p=Resolve-Issue76EvidencePath $root $r;$reads.Add($r);if($Snapshot){$snapshotReads.Add($r)};return ConvertFrom-Json ([IO.File]::ReadAllText($p)) -AsHashtable -Depth 100}
 function Table([string]$r){$p=Resolve-Issue76EvidencePath $root $r;$reads.Add($r);return ,@([IO.File]::ReadLines($p)|Where-Object {$_.Trim()}|ForEach-Object {ConvertFrom-Json $_ -AsHashtable -Depth 100})}
 function One($Rows,[string]$Field,[string]$Value){$a=@($Rows|Where-Object {$_[''+$Field] -ceq $Value});if($a.Count -ne 1){throw 'Missing/ambiguous durable identity'};return $a[0]}
 function Require([bool]$OK,[string]$Why){if(-not $OK){throw $Why}}
 foreach($k in @('window','producer','protocol_sha256','stimuli_sha256','manifest_sha256','index_sha256')){Require (-not [string]::IsNullOrWhiteSpace([string]$Pin[$k])) 'Missing independent identity pin'}
 Require ((FileHash 'manifest.json') -ceq $Pin.manifest_sha256 -and (FileHash 'index.json') -ceq $Pin.index_sha256) 'Root identity hash mismatch'
 $manifest=JsonFile 'manifest.json';$index=JsonFile 'index.json'
 Require ($index.version -ceq 'behavior76-checker-index-v2-local-tail') 'Unsupported prospective index version'
 Require ($manifest.window -ceq $Pin.window -and $manifest.source -ceq $Pin.producer -and $manifest.protocol_sha256 -ceq $Pin.protocol_sha256 -and $manifest.stimuli_sha256 -ceq $Pin.stimuli_sha256 -and $index.stimuli_sha256 -ceq $Pin.stimuli_sha256) 'Prospective binding mismatch'
 Require ($manifest.local_unknown_mode -ceq 'whole_fixture_after_exact_classified_effect_unknown' -and $manifest.global_priority -ceq $true) 'Trusted local mode missing'
 $windows=Table 'behavior76_windows.jsonl';$window=One $windows 'id' $Pin.window
 Require ($window.closed -ceq $true) 'Live window cannot be audited as closed'
 $allAttempts=Table 'behavior76_attempts.jsonl';$attempts=@($allAttempts|Where-Object window_id -CEQ $Pin.window|ForEach-Object {@{n=$_.n;slot=$_.slot;run_id=$_.run_id;status=$_.status;input_tokens=$_.input_tokens;output_tokens=$_.output_tokens;total_tokens=$_.total_tokens;thinking_tokens=$_.thinking_tokens;provider_stage=$_.provider_stage}})
 Require ($Plan.Count -eq 48 -and @($Plan|ForEach-Object {$_.steps|Where-Object evaluate}).Count -eq 96) 'Fixed48/96 design changed'
 Require (@($Plan|Group-Object family).Count -eq 12 -and @($Plan|Group-Object episode_id|Where-Object Count -ne 2).Count -eq 0) 'Fixed24/12 design changed'
 Require (@($manifest.schedule).Count -eq 24) 'Original schedule missing'
 $keys=@{};$planPosition=0
 foreach($scheduled in $manifest.schedule){foreach($arm in $scheduled.arm_order){$p=$Plan[$planPosition];Require ($p.episode_id -ceq $scheduled.episode_id -and $p.masked_arm_id -ceq $arm -and @($p.steps|Where-Object evaluate).Count -eq 2) 'Original schedule/step design mismatch';$key=$p.episode_id+'|'+$p.masked_arm_id;Require (-not $keys.ContainsKey($key)) 'Duplicate planned arm';$keys[$key]=$true;$planPosition++}}
 Require ($planPosition -eq 48 -and @($index.episodes).Count -le 48) 'Invalid schedule cardinality'
 $rows=[Collections.Generic.List[object]]::new();$executed=[Collections.Generic.List[object]]::new();$retained=[Collections.Generic.List[object]]::new();$tails=0;$missing=0;$represented=@{}
 for($pi=0;$pi -lt 48;$pi++){
  $p=$Plan[$pi];$armDir='slot-'+([int]([math]::Floor($pi/2)+1)).ToString('00')+'/'+$p.masked_arm_id
  if($pi -ge @($index.episodes).Count){foreach($step in $p.steps|Where-Object evaluate){$rows.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$step.id;disposition='unavailable_unindexed_arm';admission='unavailable';semantic_observation='unavailable';verdict='unavailable'});$missing++};continue}
  $case=$index.episodes[$pi];Require ($case.episode_id -ceq $p.episode_id -and $case.masked_arm_id -ceq $p.masked_arm_id -and $case.tables_dir -ceq $armDir+'/tables') 'Nonprefix or foreign indexed arm'
  $seal=JsonFile ($armDir+'/sealed-files.json');Require ($seal.version -ceq 'behavior76-quiescent-fixture-files-v1') 'Fixture seal absent'
  $listed=@{};foreach($f in $seal.files){$rel=$armDir+'/'+$f.path;Require (-not $listed.ContainsKey($rel)) 'Duplicate sealed file';$listed[$rel]=$true;$full=Resolve-Issue76EvidencePath $root $rel;Require ((Get-Item -LiteralPath $full).Length -eq $f.bytes -and (FileHash $rel) -ceq $f.sha256) 'Sealed fixture bytes changed'}
  $all=@(Get-ChildItem -LiteralPath (Join-Path $root $armDir) -File -Recurse|ForEach-Object {$v=[IO.Path]::GetRelativePath($root,$_.FullName).Replace('\','/');if($v -cne $armDir+'/sealed-files.json'){$v}})
  Require ($all.Count -eq $listed.Count -and @($all|Where-Object {-not $listed.ContainsKey($_)}).Count -eq 0) 'Fixture inventory not exact'
  $tables=@{};foreach($name in $Issue76LocalRequiredTables){$rel=$case.tables_dir+'/'+$name+'.jsonl';Require ($listed.ContainsKey($rel)) 'Required durable table not sealed';$tables[$name]=Table $rel}
  $q=JsonFile ($armDir+'/quiescence.json');Require ($q.broker_reaped -ceq $true -and $q.runtime_stopped -ceq $true -and $q.broker_application_observed_before_stop -ceq $true -and $q.provider_and_recipient_in_flight -eq 0 -and $q.live_leases -eq 0 -and $q.live_claims -eq 0 -and $q.broker_backends -eq 0) 'Quiescence proof missing'
  $anchor=$armDir+'/deletions.log';Require ($listed.ContainsKey($anchor)) 'Exact anchor absent'
  $trace=JsonFile ($armDir+'/authority-trace.json');Require ($trace.observer_failed -ceq $false -and $trace.open_spans -eq 0 -and $trace.dropped_events -eq 0) 'Authority observer not quiescent/available'
  # Reconcile every current UNKNOWN, including outcomes later than the initial
  # immutable stop snapshot. An unclassified later outcome is never excused.
  foreach($unknown in @($tables.action_effects|Where-Object state -CEQ 'effect_unknown')){
   $ui=One $tables.action_intents 'effect_id' $unknown.action_id;$ud=One $tables.behavior76_recipient_diagnostics 'action_id' $unknown.action_id
   Require ($ud.category -cin @('foreign_task','cas_rejected','lost_ack') -and $ud.run_id -ceq $ui.run_id -and $ud.tool_call_id -ceq $ui.tool_call_id) 'Late or unclassified UNKNOWN is global integrity failure'
  }
  $stops=@($tables.behavior76_fixture_stops);$stop=$null
  if($stops.Count){Require ($stops.Count -eq 1) 'Multiple local-stop records';$stop=$stops[0];$b=$stop.binding;Require ($stop.cause -ceq 'effect_unknown' -and $b.window -ceq $Pin.window -and $b.arm -ceq $armDir -and $b.producer -ceq $Pin.producer -and $b.protocol_sha256 -ceq $Pin.protocol_sha256 -and $b.schema -ceq $q.schema -and $b.mode -ceq 'fixture-local-v4' -and $q.fixture_locally_stopped -ceq $true) 'Local-stop immutable binding mismatch'
   Require (@($stop.effects).Count -gt 0) 'Stop has no actual effect'
   foreach($proof in $stop.effects){$e=One $tables.action_effects 'action_id' $proof.effect.action_id;$i=One $tables.action_intents 'effect_id' $e.action_id;$d=One $tables.behavior76_recipient_diagnostics 'action_id' $e.action_id;$run=One $tables.runs 'run_id' $i.run_id;$input=One $tables.accepted_inputs 'run_id' $i.run_id;$auth=One $tables.action_authorizations 'authorization_id' $e.authorization_id
    Require ($e.state -ceq 'effect_unknown' -and $proof.effect.state -ceq 'effect_unknown' -and $d.category -cin @('foreign_task','cas_rejected','lost_ack') -and $d.run_id -ceq $i.run_id -and $d.tool_call_id -ceq $i.tool_call_id) 'Unclassified or foreign UNKNOWN proof'
    Require ((Get-Issue76Canonical $proof.intent_binding) -ceq (Get-Issue76Canonical $i.binding) -and $e.initial_binding.run_id -ceq $i.run_id -and $e.initial_binding.tool_call_id -ceq $i.tool_call_id -and $run.task_id -ceq $i.binding.task_id -and $auth.action_id -ceq $i.action_id) 'Effect/intent/run binding mismatch'
    Require (@($proof.owner_inputs|Where-Object {$_.input_id -ceq $input.input_id -and $_.run_id -ceq $input.run_id -and $_.session_key -ceq $input.session_key -and $_.session_sequence -eq $input.session_sequence}).Count -eq 1) 'Stop input witness absent'
    $dispatch=@($tables.action_effect_dispatches|Where-Object {$_.action_id -ceq $e.action_id -and $_.authorization_id -ceq $e.authorization_id -and $_.execution_action_id -ceq $i.action_id});Require ($dispatch.Count -eq 1 -and @($proof.dispatches|Where-Object {$_.action_id -ceq $e.action_id -and $_.authorization_id -ceq $e.authorization_id -and $_.execution_action_id -ceq $i.action_id}).Count -eq 1) 'Actual dispatch absent'
   }
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
   if($outcome.state -ceq 'not_executed_after_fixture_local_unknown'){
    Require ($null -ne $stop -and $stopStepSeen) 'Tail precedes actual local-stop slot'
    Require ([string]::IsNullOrEmpty([string]$entry.before_snapshot_path) -and [string]::IsNullOrEmpty([string]$entry.after_snapshot_path)) 'Tail fabricates snapshots'
    Require ([string]::IsNullOrEmpty([string]$outcome.owner_input_id) -and [string]::IsNullOrEmpty([string]$outcome.owner_run_id) -and @($outcome.run_ids).Count -eq 0 -and @($outcome.automatic_run_ids).Count -eq 0 -and @($outcome.native_entry_ids).Count -eq 0 -and $null -eq $outcome.event_receipt) 'Tail fabricates executed identity'
    Require ($outcome.failure_phase -ceq 'fixture_local_stop' -and $outcome.fixture_stop_ref -ceq $case.tables_dir+'/behavior76_fixture_stops.jsonl') 'Missing or foreign tail stop-ref'
    Require (@($tables.accepted_inputs|Where-Object event_id -CEQ ('behavior76:'+$step.id)).Count -eq 0 -and @($attempts|Where-Object slot -CEQ ($armDir+'/'+$step.id)).Count -eq 0) 'Unexecuted tail has actual admissions'
    $tailSeen=$true
    if($step.evaluate){$rows.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$step.id;disposition=$outcome.state;admission='unavailable';semantic_observation='unavailable';verdict='unavailable';structured_contract='unavailable';objective_state_effects='unavailable';grounding='unavailable';clarification='unavailable';preference_scope_provenance='unavailable';completion='unavailable';fixture_stop_ref=$outcome.fixture_stop_ref;stop_sha256=(FileHash $outcome.fixture_stop_ref)});$tails++}
    continue
   }
   Require (-not $tailSeen) 'Executed work follows local-stop tail'
   foreach($path in @($entry.before_snapshot_path,$entry.after_snapshot_path)){Require ($path.StartsWith($armDir+'/',[StringComparison]::Ordinal) -and $listed.ContainsKey($path)) 'Executed snapshot absent/foreign/unsealed';$lastAfter=JsonFile $path -Snapshot}
   if($outcome.state -ceq 'fixture_local_unknown'){Require ($null -ne $stop) 'Local disposition without stop proof';$stopStepSeen=$true}
   $executed.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$step.id;original_export=$entry;outcome=$outcome;criteria='unchanged; evaluate original actual observations only';tables_dir=$case.tables_dir})
   if($step.evaluate){$represented[$p.family]=$true;$rows.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$step.id;disposition='actual_recorded_evaluation';admission=$outcome.state;semantic_observation='requires_original_actual_evidence_review';verdict='unassessed';outcome_path=$entry.outcome_path})}
  }
  if($null -ne $lastAfter){Require ($lastAfter.deletion_manifest_sha256 -ceq (FileHash $anchor) -and @($tables.memory_restore_state).Count -eq 1 -and $lastAfter.deletion_watermark -eq $tables.memory_restore_state[0].watermark) 'Final actual anchor/watermark mismatch'}
  $retained.Add(@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;tables_dir=$case.tables_dir;actual_final_run_ids=@($tables.run_final_results|ForEach-Object {$_.run_id});actual_effect_ids=@($tables.action_effects|ForEach-Object {$_.action_id});automatic_run_ids=@($executed|Where-Object {$_.episode_id -ceq $p.episode_id -and $_.masked_arm_id -ceq $p.masked_arm_id}|ForEach-Object {$_.outcome.automatic_run_ids});quiescence_sha256=(FileHash ($armDir+'/quiescence.json'));authority_trace_sha256=(FileHash ($armDir+'/authority-trace.json'));anchor_sha256=(FileHash $anchor)})
 }
 Require ($rows.Count -eq 96 -and @($rows|Group-Object {$_.episode_id+'|'+$_.masked_arm_id+'|'+$_.step_id}|Where-Object Count -ne 1).Count -eq 0) 'Original96 row denominator lost'
 return [ordered]@{version='issue76-local-tail-parser-v1';criteria_changed=$false;thresholds_changed=$false;mapping_read=$false;planned_arms=48;planned_owner_opportunities=96;planned_families=12;represented_families=$represented.Count;recorded_arms=@($index.episodes).Count;tail_opportunities=$tails;other_unavailable_opportunities=$missing;owner_rows=$rows.ToArray();executed_step_views=$executed.ToArray();retained_observation_refs=$retained.ToArray();wrapper_attempt_metadata=$attempts;snapshot_reads=$snapshotReads.ToArray();file_reads=$reads.ToArray();decision='parser evidence availability only; no acceptance/promotion/closure verdict'}
}
