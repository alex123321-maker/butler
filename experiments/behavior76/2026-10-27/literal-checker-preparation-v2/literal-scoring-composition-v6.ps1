# Additive literal orchestration; all old criterion AST text remains original.
if((Get-FileHash -LiteralPath (Join-Path $PSScriptRoot 'local-scoring-composition-v4.ps1')).Hash.ToLowerInvariant() -cne '35b2734bb3ccc3c381de2d796a0d1d1a09779615c5ada2e26d67be37a081ee25'){throw 'literal_v6:frozen_composer_changed'}
. (Join-Path $PSScriptRoot 'local-scoring-composition-v4.ps1')
. (Join-Path $PSScriptRoot 'literal-index-adapter-v6.ps1')
function Invoke-Issue76LiteralScoringV6 {
 [CmdletBinding()]
 param([Parameter(Mandatory)][string]$ReportRoot,[Parameter(Mandatory)][object[]]$Envelopes,[Parameter(Mandatory)][object[]]$Goldens,[Parameter(Mandatory)][hashtable]$Pin,[Parameter(Mandatory)][string]$FrozenScorerPath,[Parameter(Mandatory)][string]$AdapterPath)
 $ErrorActionPreference='Stop'
 if((Get-FileHash -LiteralPath $AdapterPath).Hash.ToLowerInvariant() -cne $Pin.literal_adapter_sha256){throw 'Frozen local-tail parser changed'}
 . $AdapterPath
 $kernel=Get-Issue76FrozenKernel $FrozenScorerPath
 $Episodes=@($Envelopes|ForEach-Object {$_.episode})
 foreach($definition in $kernel.functions){. ([scriptblock]::Create($definition.Extent.Text))}
 $evidenceRoot=(Resolve-Path -LiteralPath $ReportRoot).Path
 $index=ConvertFrom-Json ([IO.File]::ReadAllText((Join-Path $evidenceRoot 'index.json'))) -AsHashtable -Depth 100
 $manifest=ConvertFrom-Json ([IO.File]::ReadAllText((Join-Path $evidenceRoot 'manifest.json'))) -AsHashtable -Depth 100
 $plan=New-Issue76LiteralPlanV6 -Envelopes $Envelopes -Manifest $manifest
 if($Goldens.Count -ne 16 -or @($Goldens|Group-Object id|Where-Object Count -ne 1).Count){throw 'literal_v6:independent16goldens_required'}
 $availability=Read-Issue76LiteralEvidenceV6 -ReportRoot $evidenceRoot -Plan $plan -Pin $Pin
 $checks=[Collections.Generic.List[object]]::new();$semantic=[Collections.Generic.List[object]]::new();$matrix=[Collections.Generic.List[object]]::new();$unknownRefs=[Collections.Generic.List[object]]::new()
 $blocks=$kernel.blocks
 foreach($slot in $index.episodes){
  $script:episodeID=[string]$slot.episode_id;$script:armID=[string]$slot.masked_arm_id
  $episode=Row $Episodes 'id' $episodeID;$gold=Row $Goldens 'id' $episodeID
  if($null -eq $episode -or $null -eq $gold){throw 'Foreign episode/golden'}
  # The old defaults add empty containers only; do not mutate caller goldens.
  $expected=ConvertFrom-Json (ConvertTo-Json -InputObject $gold.expected -Depth 100) -AsHashtable -Depth 100
  & $blocks[9];& $blocks[10]
  if($slot.aliases_path -cne ($slot.tables_dir -replace '/tables$','/aliases.json')){throw 'Foreign arm aliases'}
  $null=Resolve-Issue76EvidencePath $evidenceRoot $slot.aliases_path
  $aliases=ReadJson $slot.aliases_path
  if($null -eq $aliases -or -not $aliases.ContainsKey('tasks')){throw 'Actual task aliases required'}
  $steps=@{};$snapshotPairs=@{};$actualStepIDs=@{};$incomplete=$false
  foreach($stepExport in $slot.steps){
   $steps[$stepExport.step_id]=$stepExport;$outcome=ReadJson $stepExport.outcome_path
   if($outcome.state -cin @('not_executed_after_fixture_local_unknown','not_executed_after_fixture_local_model_failure','not_executed_after_fixture_local_http_failure','not_executed_after_fixture_local_response_failure')){
    # The adapter has validated the barrier/identity and absence of admissions.
    # This branch precedes any old ReadJson/SafePath call for snapshots.
    $snapshotPairs[$stepExport.step_id]=@{before=$null;after=$null;outcome=$outcome;export=$stepExport};$incomplete=$true;continue
   }
   $before=ReadJson $stepExport.before_snapshot_path;$after=ReadJson $stepExport.after_snapshot_path
   $snapshotPairs[$stepExport.step_id]=@{before=$before;after=$after;outcome=$outcome;export=$stepExport};$actualStepIDs[$stepExport.step_id]=$true
   & $blocks['snapshot-76'];& $blocks['snapshot-77']
  }
  foreach($step in $episode.steps){if(-not $steps.ContainsKey($step.id)){Missing "step:$($step.id)" $slot.tables_dir;$incomplete=$true}}
  $first=$snapshotPairs[$episode.steps[0].id];$last=$snapshotPairs[$episode.steps[-1].id];$initial=$first.before;$final=$last.after
  $tables=@{};& $blocks[23]
  $boundaryAvailable=$null -ne $initial -and $null -ne $final
  if($boundaryAvailable){foreach($n in 24..31){& $blocks[$n]}; & $blocks[33]}
  else{Missing 'episode-boundary-snapshots' $slot.tables_dir;Missing 'original-final-state-criteria' $slot.tables_dir}
  # These original blocks score actual retained finals/mutations, including
  # already-admitted automatic work after the first stop snapshot.
  $globalStart=$checks.Count;& $blocks[32];foreach($n in 34..37){& $blocks[$n]}
  if($incomplete){for($n=$globalStart;$n -lt $checks.Count;$n++){if($checks[$n].check -in @('no-task-controls','no-owner-verification') -and $checks[$n].verdict -ceq 'pass'){$checks[$n].observed_verdict='pass';$checks[$n].verdict='unavailable';$checks[$n].availability_reason='Original future episode observations absent'}}}
  # The frozen owner criterion loop is unchanged. Its traversal contains ONLY
  # real evaluated steps. Skipped expectations never reach denial/pass logic.
  $originalEpisode=$episode;$episode=@{steps=@($originalEpisode.steps|Where-Object {$actualStepIDs.ContainsKey($_.id) -and $null -eq $snapshotPairs[$_.id].outcome.kernel_response})}
  & $blocks[38];$episode=$originalEpisode
  if($expected.require_post_event_read){if($actualStepIDs.ContainsKey($expected.require_post_event_read.step)){& $blocks[39]}else{Missing 'post-event-independent-read' $slot.tables_dir}}
  if($expected.no_commit_before){if($actualStepIDs.ContainsKey($expected.no_commit_before.step)){& $blocks[40]}else{foreach($key in $expected.no_commit_before.resources){Missing "no-premature-commit:$key" $slot.tables_dir}}}
  if($expected.post_event_profile_summary){if($boundaryAvailable){& $blocks[41]}else{Missing 'corrected-profile-current' $slot.tables_dir}}
  if($expected.ContainsKey('native_entries_after_stop')){
   $nativeCheckStart=$checks.Count;& $blocks[42]
   if($incomplete){for($n=$nativeCheckStart;$n -lt $checks.Count;$n++){if($checks[$n].verdict -ceq 'pass' -or $checks[$n].actual -le $checks[$n].expected){$checks[$n].observed_verdict=$checks[$n].verdict;$checks[$n].verdict='unavailable';$checks[$n].availability_reason='Original future Stop/after-Stop observations absent'}}}
  }
  & $blocks[43]
  foreach($effect in $tables.action_effects|Where-Object state -CEQ 'effect_unknown'){$unknownRefs.Add(@{episode_id=$episodeID;masked_arm_id=$armID;action_id=$effect.action_id;evidence=$slot.tables_dir+'/action_effects.jsonl';critical_unknown=$true;success=$false})}
 }
 # Availability has exactly68 original owner rows, independently of whether a
 # usable owner final, an actual admission denial or a tail exists.
 $script:episodeID='matrix';$script:armID='all'
 Check 'literal-matrix-32-arms' 'matrix' ($index.episodes.Count -eq 32) $index.episodes.Count 32 'index'
 Check 'literal-full68-opportunities' 'matrix' ($availability.owner_rows.Count -eq 68) $availability.owner_rows.Count 68 'literal_plan'
 $ownerRows=@();foreach($row in $availability.owner_rows){
  $ownerRows+=$row
  if($row.origin -ceq 'input_service_kernel'){
   $kernelSemantic=New-Issue76LiteralKernelSemanticRowV6 $row
   $kernelGolden=Row $Goldens 'id' $row.episode_id
   $kernelSemantic['expected_boundary']=$kernelGolden.boundary
   $kernelSemantic['rubric']=$kernelGolden.expected.communication
   $semantic.Add($kernelSemantic)
   if($kernelGolden.expected.clarification -is [Collections.IDictionary] -and $kernelGolden.expected.clarification.ContainsKey($row.step_id)){
    $script:episodeID=$row.episode_id;$script:armID=$row.masked_arm_id
    $asked=$row.kernel_proof.kind -ceq 'task_target_clarification'
    Check ('literal-kernel-clarification:'+$row.step_id) 'kernel_clarification' ($asked -ceq $kernelGolden.expected.clarification[$row.step_id]) $asked $kernelGolden.expected.clarification[$row.step_id] $row.outcome_path
   }
  } elseif($row.verdict -ceq 'unavailable'){
   $semantic.Add(@{episode_id=$row.episode_id;masked_arm_id=$row.masked_arm_id;step_id=$row.step_id;verdict='unavailable';disposition=$row.disposition;evidence_ref=$row.fixture_stop_ref;source_context_audit='unavailable';actual_effect_correlation_audit='unavailable'})
   $script:episodeID=$row.episode_id;$script:armID=$row.masked_arm_id;Missing ('original-owner-opportunity:'+$row.step_id) ([string]$row.fixture_stop_ref)
  }
  elseif(@($semantic|Where-Object {$_.episode_id -ceq $row.episode_id -and $_.masked_arm_id -ceq $row.masked_arm_id -and $_.step_id -ceq $row.step_id}).Count -eq 0){
   $denial=@($checks|Where-Object {$_.episode_id -ceq $row.episode_id -and $_.masked_arm_id -ceq $row.masked_arm_id -and $_.check -ceq ('expected-admission-denial:'+$row.step_id) -and $_.verdict -ceq 'pass'})
   $zero=@($checks|Where-Object {$_.episode_id -ceq $row.episode_id -and $_.masked_arm_id -ceq $row.masked_arm_id -and $_.check -ceq ('zero-native-on-denial:'+$row.step_id) -and $_.verdict -ceq 'pass'})
   $semantic.Add(@{episode_id=$row.episode_id;masked_arm_id=$row.masked_arm_id;step_id=$row.step_id;verdict=$(if($denial.Count -eq 1 -and $zero.Count -eq 1){'not_applicable_actual_expected_denial'}else{'unavailable'});disposition=$row.disposition;evidence_ref=$row.outcome_path;source_context_audit='required';actual_effect_correlation_audit='required'})
  }
 }
 foreach($arm in $availability.retained_observation_refs){
  $script:episodeID=$arm.episode_id;$script:armID=$arm.masked_arm_id
  foreach($late in $arm.business_observation_proof.late_confirmed_publications){
   Check ('literal-late-confirmed-publication:'+$late.action_id) 'business_deadline_behavior' $false $late 'no confirmed publication after observed service deadline' ($arm.tables_dir+'/behavior76_business_observations.jsonl')
  }
 }
 # Explicit failed native outcomes are availability facts from the new trusted
 # stop protocol; all old criterion expressions above execute unchanged.
 foreach($failure in @($availability.native_failure_refs)){
  $script:episodeID=$failure.episode_id;$script:armID=$failure.masked_arm_id
  $failureLabel=switch -CaseSensitive ($failure.cause){'model_response_rejected'{'native-response-rejected:'} 'model_http_no_response'{'native-http-no-response:'} default{'native-empty-answer:'}}
  Check ($failureLabel+$failure.n) 'provider_result' $false $failure.cause 'usable native outcome' $failure.evidence_ref
  $ownerRow=@($ownerRows|Where-Object {$_.episode_id -ceq $failure.episode_id -and $_.masked_arm_id -ceq $failure.masked_arm_id -and $_.step_id -ceq $failure.step_id})
  if($ownerRow.Count -ne 1){throw 'Original owner row for native failure absent'}
  $ownerRow[0]['native_failure_refs']=@($availability.native_failure_refs|Where-Object {$_.episode_id -ceq $failure.episode_id -and $_.masked_arm_id -ceq $failure.masked_arm_id -and $_.step_id -ceq $failure.step_id})
  if($failure.owner_run -and $failure.actual_final_for_failed_run -eq 0){
   $ownerRow[0]['native_response']='failed';$ownerRow[0]['success']=$false
   $entry=@($semantic|Where-Object {$_.episode_id -ceq $failure.episode_id -and $_.masked_arm_id -ceq $failure.masked_arm_id -and $_.step_id -ceq $failure.step_id})
   if($entry.Count -ne 1){throw 'Original semantic opportunity for failed response absent'}
   $entry[0]['verdict']='failed';$entry[0]['partial_text_delta_events']=$failure.partial_text_delta_events;$entry[0]['partial_text_quality']='unassessed';$entry[0]['failure_class']=$failure.cause;$entry[0]['communication_judgment']='unavailable_no_usable_owner_answer';$entry[0]['evidence_ref']=$failure.evidence_ref
  }
 }
 if($semantic.Count -ne 68 -or @($semantic|Group-Object {$_.episode_id+'|'+$_.masked_arm_id+'|'+$_.step_id}|Where-Object Count -ne 1).Count){throw 'Original68 literal semantic opportunity denominator lost'}
 return [ordered]@{version='issue76-literal-scoring-composition-v6';criteria_source_sha256=$kernel.source_sha256;criterion_extents=$kernel.witnesses;criteria_changed=$false;thresholds_changed=$false;planned_episodes=16;planned_arms=32;planned_owner_opportunities=68;planned_dimensions=8;planned_families=6;old_per_case_extent_types_reused=25;old_global_matrix_extents_retained_not_applied=2;new_literal_cardinality_checks_additive=$true;owner_rows=$ownerRows;mechanical_checks=$checks.ToArray();masked_semantic_template=$semantic.ToArray();matrix=$matrix.ToArray();critical_unknown_refs=$unknownRefs.ToArray();critical_unknown_is_success=$false;native_failure_refs=@($availability.native_failure_refs);model_failure_is_success=$false;availability=$availability;communication_judgments='unassessed';source_context_audit='required';promotion='not_assessed';issue_closure='not_assessed'}
}

# Real entrypoint: only independently pinned stimulus/golden bytes are parsed.
# Future authorship fills literal Pin hashes/paths; no evaluator branch changes.
function Invoke-Issue76LiteralScoringFilesV6 {
 [CmdletBinding()]
 param([Parameter(Mandatory)][string]$ReportRoot,[Parameter(Mandatory)][string]$StimuliPath,[Parameter(Mandatory)][string]$GoldensPath,[Parameter(Mandatory)][hashtable]$Pin,[Parameter(Mandatory)][string]$FrozenScorerPath,[Parameter(Mandatory)][string]$AdapterPath)
 if([string]::IsNullOrEmpty([string]$Pin.expectations_sha256) -or (Get-FileHash -LiteralPath $StimuliPath).Hash.ToLowerInvariant() -cne $Pin.stimuli_sha256 -or (Get-FileHash -LiteralPath $GoldensPath).Hash.ToLowerInvariant() -cne $Pin.expectations_sha256){throw 'Independent stimulus/golden byte commitment mismatch'}
 $manifest=ConvertFrom-Json ([IO.File]::ReadAllText((Join-Path $ReportRoot 'manifest.json'))) -AsHashtable -Depth 100
 if($manifest.expectations_sha256 -cne $Pin.expectations_sha256){throw 'Manifest golden commitment mismatch'}
 $envelopes=@([IO.File]::ReadLines($StimuliPath)|Where-Object {$_.Trim()}|ForEach-Object {ConvertFrom-Issue76LiteralJsonV6 $_})
 $goldens=@([IO.File]::ReadLines($GoldensPath)|Where-Object {$_.Trim()}|ForEach-Object {ConvertFrom-Issue76LiteralJsonV6 $_})
 Invoke-Issue76LiteralScoringV6 -ReportRoot $ReportRoot -Envelopes $envelopes -Goldens $goldens -Pin $Pin -FrozenScorerPath $FrozenScorerPath -AdapterPath $AdapterPath
}
