$ErrorActionPreference='Stop'
$repo=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$private=Join-Path $PSScriptRoot 'closed-literal-v11-r1-offline-v7-private'
$out=Join-Path $private 'masked-evidence'
if(Test-Path -LiteralPath $out){throw 'CreateNew extraction output required'}
. (Join-Path $PSScriptRoot 'literal-json-token-boundary-reanalysis-v7.ps1')
Assert-Issue76LiteralV7OriginalRuntimePins
$null=New-Item -ItemType Directory -Path $out
Invoke-Issue76JsonStringTokenScopeV7 -Work {
 function J([string]$p){ConvertFrom-Json ([IO.File]::ReadAllText($p)) -AsHashtable -Depth 100}
 function T([string]$p){@([IO.File]::ReadLines($p)|Where-Object {$_.Trim()}|ForEach-Object {ConvertFrom-Json $_ -AsHashtable -Depth 100})}
 function Snap($s){
  if($null -eq $s){return $null}
  @{tasks=@($s.tasks|ForEach-Object {@{id=$_.task_id;goal=$_.goal;status=$_.status;revision=$_.revision;constraints_revision=$_.constraints_revision;constraints=$_.constraints;working=$_.working;execution_policy=$_.execution_policy;completion_criteria=$_.completion_criteria;result_delivery=$_.result_delivery}});objects=$s.objects;profiles=$s.profiles;sources=$s.sources;literal_service=$s.literal_service;deletion_watermark=$s.deletion_watermark;deletion_manifest_sha256=$s.deletion_manifest_sha256}
 }
 $receipt=J (Join-Path $PSScriptRoot 'public-literal-r1-offline-v7-mechanical-execution-v1.json')
 if($receipt.decision -cne 'POSTHOC_V7_MECHANICAL_EXECUTED_NOT_QUALITY_ACCEPTANCE' -or $receipt.owner_rows -ne 68 -or $receipt.semantic_rows -ne 68){throw 'Strict mechanical completion required'}
 $rp=Join-Path $private 'posthoc-v7-mechanical-result-v1.json'
 if((Get-FileHash -LiteralPath $rp).Hash.ToLowerInvariant() -cne $receipt.private_result_sha256){throw 'Mechanical result pin mismatch'}
 $m=(J $rp).result
 $envs=T (Join-Path $PSScriptRoot 'hidden-literal-v11-r1/stimuli.jsonl')
 $golds=T (Join-Path $PSScriptRoot 'hidden-literal-v11-r1/expectations.jsonl')
 $root=Join-Path $repo '.cache/behavior76/native-literal-v11-r1'
 $index=J (Join-Path $root 'index.json')
 $n=0
 foreach($env in $envs){
  $n++;$ep=$env.episode;$gold=@($golds|Where-Object id -CEQ $ep.id)[0]
  $projection=[ordered]@{episode_number=$n;episode_id=$ep.id;dimension=$env.dimension;polarity=$env.polarity;synthetic_envelope=$env;frozen_expected=$gold;arms=@()}
  foreach($case in $index.episodes|Where-Object episode_id -CEQ $ep.id){
   $tables=Join-Path $root $case.tables_dir
   $finals=T (Join-Path $tables 'run_final_results.jsonl')
   $aliasPath=$case.aliases_path
   $arm=[ordered]@{masked_arm_id=$case.masked_arm_id;aliases=(J (Join-Path $root $aliasPath));steps=@();checks_not_pass=@($m.mechanical_checks|Where-Object {$_.episode_id -ceq $ep.id -and $_.masked_arm_id -ceq $case.masked_arm_id -and $_.verdict -cne 'pass'});source_rows=(T (Join-Path $tables 'memory_run_sources.jsonl'));effect_rows=(T (Join-Path $tables 'action_effects.jsonl'));object_events=(T (Join-Path $tables 'behavior76_object_events.jsonl'));intent_rows=(T (Join-Path $tables 'action_intents.jsonl'));tool_rows=(T (Join-Path $tables 'tool_calls.jsonl'));business_observations=(T (Join-Path $tables 'behavior76_business_observations.jsonl'));tables_dir=$case.tables_dir}
   foreach($step in $ep.steps){
    $view=@($m.availability.executed_step_views|Where-Object {$_.episode_id -ceq $ep.id -and $_.masked_arm_id -ceq $case.masked_arm_id -and $_.step_id -ceq $step.id})
    $row=@($m.owner_rows|Where-Object {$_.episode_id -ceq $ep.id -and $_.masked_arm_id -ceq $case.masked_arm_id -and $_.step_id -ceq $step.id})
    $semantic=@($m.masked_semantic_template|Where-Object {$_.episode_id -ceq $ep.id -and $_.masked_arm_id -ceq $case.masked_arm_id -and $_.step_id -ceq $step.id})
    $v=if($view.Count){$view[0]}else{$null};$o=if($v){$v.outcome}else{$null}
    $f=@($finals|Where-Object run_id -CEQ $o.owner_run_id)
    $autos=@($finals|Where-Object {$_.run_id -cin @($o.automatic_run_ids)}|ForEach-Object {@{run_id=$_.run_id;content=(ConvertFrom-Json $_.content -AsHashtable -Depth 100)}})
    $arm.steps+=@{step=$step;owner_row=$row;semantic=$semantic;outcome=$o;before=$(if($v -and $v.original_export.before_snapshot_path){Snap (J (Join-Path $root $v.original_export.before_snapshot_path))}else{$null});after=$(if($v -and $v.original_export.after_snapshot_path){Snap (J (Join-Path $root $v.original_export.after_snapshot_path))}else{$null});owner_final=$(if($f.Count -eq 1){ConvertFrom-Json $f[0].content -AsHashtable -Depth 100}else{$null});automatic_finals=$autos;evidence=$(if($v){$v.original_export}else{$null})}
   }
   $projection.arms+=,$arm
  }
  $p=Join-Path $out ('episode-{0:D2}.json' -f $n)
  $bytes=[Text.UTF8Encoding]::new($false).GetBytes(($projection|ConvertTo-Json -Depth 100)+"`n")
  $f=[IO.FileStream]::new($p,[IO.FileMode]::CreateNew);try{$f.Write($bytes);$f.Flush($true)}finally{$f.Dispose()};(Get-Item -LiteralPath $p).IsReadOnly=$true
 }
 @{episodes=$n;owner_rows=$m.owner_rows.Count;semantic_rows=$m.masked_semantic_template.Count;mapping_read=$false;extraction_scope='pinned Invoke-Issue76JsonStringTokenScopeV7; no model request or trusted instruction projection';posthoc=$true}|ConvertTo-Json -Compress
}
