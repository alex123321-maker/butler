[CmdletBinding()]
param([switch]$ExecuteReviewedExtraction)
$ErrorActionPreference='Stop'
if(-not $ExecuteReviewedExtraction){@{version='issue76-literal-masked-extraction-v2';status='PREPARED_ONLY_NO_DATA_READ_OR_EXTRACTION';actual_extraction=$false};return}
$repo=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$private=Join-Path $PSScriptRoot 'closed-literal-v11-r1-offline-v7-private'
$out=Join-Path $private 'masked-evidence-v2'
if(Test-Path -LiteralPath $out){throw 'CreateNew extraction-v2 output required'}
foreach($p in @(
 @('literal-json-token-boundary-reanalysis-v7.ps1','57c1bc1a87fb6b284f46f27618ebcd85d8dbc849e0bba4634461b687ef485e80'),
 @('public-literal-r1-offline-v7-mechanical-execution-v1.json','4ab0bec7c4892662de4f30fa4a3495a7794a552cef326bfacd1fe20e8e5ef48e'),
 @('closed-literal-v11-r1-offline-v7-private/posthoc-v7-mechanical-result-v1.json','41825ea0599c7d9adf1a47e314fbcbe4e01209cfcc9c8a5f4d0ab95dbc9d65f4'),
 @('closed-literal-v11-r1-offline-v7-private/masked-extraction-failure-v1.json','94b842bf0f67981c8d0e1ba9748187339dc6ac5e8d8b45fd96302c1b4d1fc8a2'),
 @('hidden-literal-v11-r1/stimuli.jsonl','9bc4872c85fb6b2d5e1a1b84b94b24a6a1e6d64ae749d527cbf5698431ca5d0c'),
 @('hidden-literal-v11-r1/expectations.jsonl','f20937430079aa6c259a6917f9f1ab846875b4baff71c58cf4c72e4d1ff49865')
)){if((Get-FileHash -LiteralPath (Join-Path $PSScriptRoot $p[0])).Hash.ToLowerInvariant() -cne $p[1]){throw 'extraction_v2:original_input_pin_mismatch'}}
. (Join-Path $PSScriptRoot 'literal-json-token-boundary-reanalysis-v7.ps1')
. (Join-Path $PSScriptRoot 'literal-masked-final-extractor-v2.ps1')
Assert-Issue76LiteralV7OriginalRuntimePins
Invoke-Issue76JsonStringTokenScopeV7 -Work {
 function J([string]$p){ConvertFrom-Json ([IO.File]::ReadAllText($p)) -AsHashtable -Depth 100}
 function T([string]$p){@([IO.File]::ReadLines($p)|Where-Object {$_.Trim()}|ForEach-Object {ConvertFrom-Json $_ -AsHashtable -Depth 100})}
 function Snap($s){if($null -eq $s){return $null};@{tasks=@($s.tasks|ForEach-Object {@{id=$_.task_id;goal=$_.goal;status=$_.status;revision=$_.revision;constraints_revision=$_.constraints_revision;constraints=$_.constraints;working=$_.working;execution_policy=$_.execution_policy;completion_criteria=$_.completion_criteria;result_delivery=$_.result_delivery}});objects=$s.objects;profiles=$s.profiles;sources=$s.sources;literal_service=$s.literal_service;deletion_watermark=$s.deletion_watermark;deletion_manifest_sha256=$s.deletion_manifest_sha256}}
 $receipt=J (Join-Path $PSScriptRoot 'public-literal-r1-offline-v7-mechanical-execution-v1.json')
 if($receipt.decision -cne 'POSTHOC_V7_MECHANICAL_EXECUTED_NOT_QUALITY_ACCEPTANCE' -or $receipt.owner_rows -ne 68 -or $receipt.semantic_rows -ne 68){throw 'Existing strict mechanical completion required'}
 $m=(J (Join-Path $private 'posthoc-v7-mechanical-result-v1.json')).result
 $envs=T (Join-Path $PSScriptRoot 'hidden-literal-v11-r1/stimuli.jsonl')
 $golds=T (Join-Path $PSScriptRoot 'hidden-literal-v11-r1/expectations.jsonl')
 $root=Join-Path $repo '.cache/behavior76/native-literal-v11-r1'
 $index=J (Join-Path $root 'index.json')
 $null=New-Item -ItemType Directory -Path $out
 $n=0;$ownerCount=0;$filePins=@()
 foreach($env in $envs){
  $n++;$ep=$env.episode;$gold=@($golds|Where-Object id -CEQ $ep.id)
  if($gold.Count -ne 1){throw 'extraction_v2:unique_existing_golden_required'}
  $projection=[ordered]@{posthoc=$true;episode_number=$n;episode_id=$ep.id;dimension=$env.dimension;polarity=$env.polarity;synthetic_envelope=$env;frozen_expected=$gold[0];arms=@();mapping_read=$false}
  foreach($case in $index.episodes|Where-Object episode_id -CEQ $ep.id){
   $tables=Join-Path $root $case.tables_dir;$finals=T (Join-Path $tables 'run_final_results.jsonl')
   $arm=[ordered]@{masked_arm_id=$case.masked_arm_id;aliases=(J (Join-Path $root $case.aliases_path));steps=@();checks_not_pass=@($m.mechanical_checks|Where-Object {$_.episode_id -ceq $ep.id -and $_.masked_arm_id -ceq $case.masked_arm_id -and $_.verdict -cne 'pass'});source_rows=(T (Join-Path $tables 'memory_run_sources.jsonl'));effect_rows=(T (Join-Path $tables 'action_effects.jsonl'));object_events=(T (Join-Path $tables 'behavior76_object_events.jsonl'));intent_rows=(T (Join-Path $tables 'action_intents.jsonl'));tool_rows=(T (Join-Path $tables 'tool_calls.jsonl'));business_observations=(T (Join-Path $tables 'behavior76_business_observations.jsonl'));tables_dir=$case.tables_dir}
   foreach($step in $ep.steps){
    $view=@($m.availability.executed_step_views|Where-Object {$_.episode_id -ceq $ep.id -and $_.masked_arm_id -ceq $case.masked_arm_id -and $_.step_id -ceq $step.id})
    $row=@($m.owner_rows|Where-Object {$_.episode_id -ceq $ep.id -and $_.masked_arm_id -ceq $case.masked_arm_id -and $_.step_id -ceq $step.id})
    $semantic=@($m.masked_semantic_template|Where-Object {$_.episode_id -ceq $ep.id -and $_.masked_arm_id -ceq $case.masked_arm_id -and $_.step_id -ceq $step.id})
    if($view.Count -gt 1 -or ($step.evaluate -and ($row.Count -ne 1 -or $semantic.Count -ne 1))){throw 'extraction_v2:existing_unique_owner_proof_required'}
    $v=if($view.Count){$view[0]}else{$null};$o=if($v){$v.outcome}elseif($row.Count -eq 1 -and $row[0].outcome_path){J (Join-Path $root $row[0].outcome_path)}else{$null}
    $owner=$null
    if($step.evaluate){
     $ownerCount++
     if($null -eq $o){$owner=@{communication_availability='UNAVAILABLE';decoded_answer=$null;decoded_final=$null;parse_status='no_executed_outcome';raw_finals=@();model_final=$false}}
     else{$owner=Get-Issue76MaskedOwnerFinalV2 -EpisodeID $ep.id -MaskedArmID $case.masked_arm_id -StepID $step.id -OwnerRow $row[0] -Outcome $o -SemanticRows $semantic -MechanicalChecks $m.mechanical_checks -FinalRows $finals -TablesDir $case.tables_dir}
    }
    $autos=@($finals|Where-Object {$_.run_id -cin @($o.automatic_run_ids)}|ForEach-Object {@{run_id=$_.run_id;raw_final_record=(Get-Issue76RawFinalRecordV2 $_);owner_communication_availability='NOT_APPLICABLE_AUTOMATIC_NOT_OWNER'}})
    $arm.steps+=@{step=$step;owner_row=$row;semantic=$semantic;outcome=$o;before=$(if($v -and $v.original_export.before_snapshot_path){Snap (J (Join-Path $root $v.original_export.before_snapshot_path))}else{$null});after=$(if($v -and $v.original_export.after_snapshot_path){Snap (J (Join-Path $root $v.original_export.after_snapshot_path))}else{$null});owner_final_projection=$owner;automatic_finals=$autos;evidence=$(if($v){$v.original_export}else{$null})}
   }
   $projection.arms+=,$arm
  }
  $p=Join-Path $out ('episode-{0:D2}.json' -f $n);$b=[Text.UTF8Encoding]::new($false).GetBytes(($projection|ConvertTo-Json -Depth 100)+"`n")
  $f=[IO.FileStream]::new($p,[IO.FileMode]::CreateNew);try{$f.Write($b);$f.Flush($true)}finally{$f.Dispose()};(Get-Item $p).IsReadOnly=$true
  $filePins+=@{name=[IO.Path]::GetFileName($p);bytes=$b.Length;sha256=(Get-FileHash $p).Hash.ToLowerInvariant()}
 }
 if($n -ne 16 -or $ownerCount -ne 68){throw 'extraction_v2:original_full_denominator_required'}
 $meta=@{version='issue76-masked-extraction-v2-execution';posthoc=$true;episodes=$n;owner_opportunities=$ownerCount;files=$filePins;mechanical_result_sha256=$receipt.private_result_sha256;mechanical_rerun=$false;communication_judgments='NOT_YET_PERFORMED';mapping_read=$false;scope='pinned Invoke-Issue76JsonStringTokenScopeV7';invalid_final_decoded_answer='null; communication UNAVAILABLE';Go_PG_Docker_auth_provider_native=0}
 $p=Join-Path $out 'execution-receipt.json';$b=[Text.UTF8Encoding]::new($false).GetBytes(($meta|ConvertTo-Json -Depth 30)+"`n");$f=[IO.FileStream]::new($p,[IO.FileMode]::CreateNew);try{$f.Write($b);$f.Flush($true)}finally{$f.Dispose()};(Get-Item $p).IsReadOnly=$true
 @{path=$p;sha256=(Get-FileHash $p).Hash.ToLowerInvariant();owner_opportunities=68;mapping_read=$false}|ConvertTo-Json -Compress
}
