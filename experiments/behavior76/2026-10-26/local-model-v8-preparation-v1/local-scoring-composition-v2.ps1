# Prospective local-tail composition. No corpus, golden or mapping is embedded.
# Criterion expressions are executed directly from the exact frozen old source.
function Get-Issue76FrozenKernel([string]$Path) {
 $expected='e481f9c062a3bc4500b9f0b94da8819fbf287bec075490583cad17d54198721d'
 if((Get-FileHash -LiteralPath $Path).Hash.ToLowerInvariant() -cne $expected){throw 'Frozen criterion source changed'}
 $tokens=$null;$errors=$null;$ast=[Management.Automation.Language.Parser]::ParseFile($Path,[ref]$tokens,[ref]$errors)
 if($errors.Count){throw 'Frozen criterion source parse failed'}
 $functions=@($ast.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.FunctionDefinitionAst]})
 $loop=@($ast.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.ForEachStatementAst] -and $_.Variable.Extent.Text -ceq '$slot'})
 if($loop.Count -ne 1 -or $loop[0].Body.Statements.Count -ne 44){throw 'Frozen criterion AST identity changed'}
 $statements=$loop[0].Body.Statements;$blocks=@{};$witnesses=@()
 foreach($n in @(9,10,23)+@(24..43)){
  $text=$statements[$n].Extent.Text;$blocks[$n]=[scriptblock]::Create($text)
  $witnesses+=@{kind='statement';index=$n;start_line=$statements[$n].Extent.StartLineNumber;end_line=$statements[$n].Extent.EndLineNumber;sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($text))).ToLowerInvariant()}
 }
 $inner=$statements[15].Body.Statements
 $snapshots=@($inner|Where-Object {$_.Extent.StartLineNumber -in @(76,77)})
 if($snapshots.Count -ne 2){throw 'Frozen snapshot criterion AST identity changed'}
 foreach($s in $snapshots){$key='snapshot-'+$s.Extent.StartLineNumber;$text=$s.Extent.Text;$blocks[$key]=[scriptblock]::Create($text);$witnesses+=@{kind='statement';index=$key;start_line=$s.Extent.StartLineNumber;end_line=$s.Extent.EndLineNumber;sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($text))).ToLowerInvariant()}}
 foreach($f in $functions){$witnesses+=@{kind='function';name=$f.Name;start_line=$f.Extent.StartLineNumber;end_line=$f.Extent.EndLineNumber;sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($f.Extent.Text))).ToLowerInvariant()}}
 foreach($line in @(205,209)){$statement=@($ast.EndBlock.Statements|Where-Object {$_.Extent.StartLineNumber -eq $line});if($statement.Count -ne 1){throw 'Frozen matrix criterion AST changed'};$text=$statement[0].Extent.Text;$blocks['matrix-'+$line]=[scriptblock]::Create($text);$witnesses+=@{kind='statement';index=('matrix-'+$line);start_line=$line;end_line=$statement[0].Extent.EndLineNumber;sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($text))).ToLowerInvariant()}}
 return @{source_sha256=$expected;functions=$functions;blocks=$blocks;witnesses=$witnesses}
}

function Invoke-Issue76LocalScoringV2 {
 [CmdletBinding()]
 param([Parameter(Mandatory)][string]$ReportRoot,[Parameter(Mandatory)][object[]]$Episodes,[Parameter(Mandatory)][object[]]$Goldens,[Parameter(Mandatory)][hashtable]$Pin,[Parameter(Mandatory)][string]$FrozenScorerPath,[Parameter(Mandatory)][string]$AdapterPath)
 $ErrorActionPreference='Stop'
 if((Get-FileHash -LiteralPath $AdapterPath).Hash.ToLowerInvariant() -cne '897beea0409d4fb526ff434ba03ab51df07b6c919b8c680d81cc97d6176b32fd'){throw 'Frozen local-tail parser changed'}
 . $AdapterPath
 $kernel=Get-Issue76FrozenKernel $FrozenScorerPath
 foreach($definition in $kernel.functions){. ([scriptblock]::Create($definition.Extent.Text))}
 $evidenceRoot=(Resolve-Path -LiteralPath $ReportRoot).Path
 $index=ConvertFrom-Json ([IO.File]::ReadAllText((Join-Path $evidenceRoot 'index.json'))) -AsHashtable -Depth 100
 $manifest=ConvertFrom-Json ([IO.File]::ReadAllText((Join-Path $evidenceRoot 'manifest.json'))) -AsHashtable -Depth 100
 if($Episodes.Count -ne 24 -or $Goldens.Count -ne 24 -or @($Episodes|Group-Object id|Where-Object Count -ne 1).Count -or @($Goldens|Group-Object id|Where-Object Count -ne 1).Count){throw 'Original 24 episode/golden identity required'}
 $plan=@();foreach($scheduled in $manifest.schedule){$original=Row $Episodes 'id' $scheduled.episode_id;if($null -eq $original){throw 'Foreign scheduled episode'};foreach($arm in $scheduled.arm_order){$plan+=@{episode_id=$original.id;masked_arm_id=$arm;family=$original.family;steps=$original.steps}}}
 $availability=Read-Issue76LocalEvidenceV2 -ReportRoot $evidenceRoot -Plan $plan -Pin $Pin
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
   if($outcome.state -cin @('not_executed_after_fixture_local_unknown','not_executed_after_fixture_local_model_failure')){
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
  $originalEpisode=$episode;$episode=@{steps=@($originalEpisode.steps|Where-Object {$actualStepIDs.ContainsKey($_.id)})}
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
 # Availability has exactly96 original owner rows, independently of whether a
 # usable owner final, an actual admission denial or a tail exists.
 $script:episodeID='matrix';$script:armID='all';& $blocks['matrix-205'];& $blocks['matrix-209']
 $ownerRows=@();foreach($row in $availability.owner_rows){
  $ownerRows+=$row
  if($row.verdict -ceq 'unavailable'){
   $semantic.Add(@{episode_id=$row.episode_id;masked_arm_id=$row.masked_arm_id;step_id=$row.step_id;verdict='unavailable';disposition=$row.disposition;evidence_ref=$row.fixture_stop_ref;source_context_audit='unavailable';actual_effect_correlation_audit='unavailable'})
   $script:episodeID=$row.episode_id;$script:armID=$row.masked_arm_id;Missing ('original-owner-opportunity:'+$row.step_id) ([string]$row.fixture_stop_ref)
  }
  elseif(@($semantic|Where-Object {$_.episode_id -ceq $row.episode_id -and $_.masked_arm_id -ceq $row.masked_arm_id -and $_.step_id -ceq $row.step_id}).Count -eq 0){
   $denial=@($checks|Where-Object {$_.episode_id -ceq $row.episode_id -and $_.masked_arm_id -ceq $row.masked_arm_id -and $_.check -ceq ('expected-admission-denial:'+$row.step_id) -and $_.verdict -ceq 'pass'})
   $zero=@($checks|Where-Object {$_.episode_id -ceq $row.episode_id -and $_.masked_arm_id -ceq $row.masked_arm_id -and $_.check -ceq ('zero-native-on-denial:'+$row.step_id) -and $_.verdict -ceq 'pass'})
   $semantic.Add(@{episode_id=$row.episode_id;masked_arm_id=$row.masked_arm_id;step_id=$row.step_id;verdict=$(if($denial.Count -eq 1 -and $zero.Count -eq 1){'not_applicable_actual_expected_denial'}else{'unavailable'});disposition=$row.disposition;evidence_ref=$row.outcome_path;source_context_audit='required';actual_effect_correlation_audit='required'})
  }
 }
 # Explicit failed native outcomes are availability facts from the new trusted
 # stop protocol; all old criterion expressions above execute unchanged.
 foreach($failure in @($availability.native_failure_refs)){
  $script:episodeID=$failure.episode_id;$script:armID=$failure.masked_arm_id
  Check ('native-empty-answer:'+$failure.n) 'provider_result' $false 'completed native response without usable text/function' 'usable native outcome' $failure.evidence_ref
  $ownerRow=@($ownerRows|Where-Object {$_.episode_id -ceq $failure.episode_id -and $_.masked_arm_id -ceq $failure.masked_arm_id -and $_.step_id -ceq $failure.step_id})
  if($ownerRow.Count -ne 1){throw 'Original owner row for native failure absent'}
  $ownerRow[0]['native_failure_refs']=@($availability.native_failure_refs|Where-Object {$_.episode_id -ceq $failure.episode_id -and $_.masked_arm_id -ceq $failure.masked_arm_id -and $_.step_id -ceq $failure.step_id})
  if($failure.owner_run -and $failure.actual_final_for_failed_run -eq 0){
   $ownerRow[0]['native_response']='failed';$ownerRow[0]['success']=$false
   $entry=@($semantic|Where-Object {$_.episode_id -ceq $failure.episode_id -and $_.masked_arm_id -ceq $failure.masked_arm_id -and $_.step_id -ceq $failure.step_id})
   if($entry.Count -ne 1){throw 'Original semantic opportunity for failed response absent'}
   $entry[0]['verdict']='failed';$entry[0]['failure_class']='model_empty_answer';$entry[0]['communication_judgment']='unavailable_no_usable_owner_answer';$entry[0]['evidence_ref']=$failure.evidence_ref
  }
 }
 if($semantic.Count -ne 96 -or @($semantic|Group-Object {$_.episode_id+'|'+$_.masked_arm_id+'|'+$_.step_id}|Where-Object Count -ne 1).Count){throw 'Original96 semantic opportunity denominator lost'}
 return [ordered]@{version='issue76-local-scoring-composition-v2';criteria_source_sha256=$kernel.source_sha256;criterion_extents=$kernel.witnesses;criteria_changed=$false;thresholds_changed=$false;planned_arms=48;planned_owner_opportunities=96;planned_families=12;owner_rows=$ownerRows;mechanical_checks=$checks.ToArray();masked_semantic_template=$semantic.ToArray();matrix=$matrix.ToArray();critical_unknown_refs=$unknownRefs.ToArray();critical_unknown_is_success=$false;native_failure_refs=@($availability.native_failure_refs);model_failure_is_success=$false;availability=$availability;communication_judgments='unassessed';source_context_audit='required';promotion='not_assessed';issue_closure='not_assessed'}
}

# Real entrypoint: only independently pinned stimulus/golden bytes are parsed.
# Future authorship fills literal Pin hashes/paths; no evaluator branch changes.
function Invoke-Issue76LocalScoringFilesV2 {
 [CmdletBinding()]
 param([Parameter(Mandatory)][string]$ReportRoot,[Parameter(Mandatory)][string]$StimuliPath,[Parameter(Mandatory)][string]$GoldensPath,[Parameter(Mandatory)][hashtable]$Pin,[Parameter(Mandatory)][string]$FrozenScorerPath,[Parameter(Mandatory)][string]$AdapterPath)
 if([string]::IsNullOrEmpty([string]$Pin.expectations_sha256) -or (Get-FileHash -LiteralPath $StimuliPath).Hash.ToLowerInvariant() -cne $Pin.stimuli_sha256 -or (Get-FileHash -LiteralPath $GoldensPath).Hash.ToLowerInvariant() -cne $Pin.expectations_sha256){throw 'Independent stimulus/golden byte commitment mismatch'}
 $manifest=ConvertFrom-Json ([IO.File]::ReadAllText((Join-Path $ReportRoot 'manifest.json'))) -AsHashtable -Depth 100
 if($manifest.expectations_sha256 -cne $Pin.expectations_sha256){throw 'Manifest golden commitment mismatch'}
 $episodes=@([IO.File]::ReadLines($StimuliPath)|Where-Object {$_.Trim()}|ForEach-Object {ConvertFrom-Json $_ -AsHashtable -Depth 100})
 $goldens=@([IO.File]::ReadLines($GoldensPath)|Where-Object {$_.Trim()}|ForEach-Object {ConvertFrom-Json $_ -AsHashtable -Depth 100})
 Invoke-Issue76LocalScoringV2 -ReportRoot $ReportRoot -Episodes $episodes -Goldens $goldens -Pin $Pin -FrozenScorerPath $FrozenScorerPath -AdapterPath $AdapterPath
}
