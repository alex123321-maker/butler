[CmdletBinding()]
param([Parameter(Mandatory)][string]$InputRoot,[Parameter(Mandatory)][string]$OutputRoot)
$ErrorActionPreference='Stop'
if(Test-Path -LiteralPath $OutputRoot){throw 'New bridge receipt directory required'}
$inputPath=(Resolve-Path -LiteralPath $InputRoot).Path
$output=(New-Item -ItemType Directory -Path $OutputRoot).FullName
. (Join-Path $PSScriptRoot 'literal-kernel-proof-v1.ps1')
$checks=[Collections.Generic.List[object]]::new()
function CheckBridge([string]$Label,[scriptblock]$Work){
 try{$proof=& $Work;$checks.Add([ordered]@{label=$Label;verdict='pass';proof=$proof})}
 catch{$message=$_.Exception.Message;if(-not $message.StartsWith('literal_v6:')){$message=$_.Exception.GetType().FullName};$checks.Add([ordered]@{label=$Label;verdict='fail';fixed_error=$message})}
}
foreach($fixture in @('kernel-stop','kernel-amend','business')){
 $dir=Join-Path $inputPath $fixture
 $envelope=ConvertFrom-Issue76LiteralJsonV6 ([IO.File]::ReadAllText((Join-Path $dir 'public-input.json')))
 $outcomes=@(ConvertFrom-Issue76LiteralJsonV6 ([IO.File]::ReadAllText((Join-Path $dir 'actual-outcomes.json'))))
 $aliases=ConvertFrom-Issue76LiteralJsonV6 ([IO.File]::ReadAllText((Join-Path $dir 'aliases.json')))
 $tables=@{}
 foreach($file in Get-ChildItem -LiteralPath (Join-Path $dir 'tables') -Filter '*.jsonl' -File){$tables[$file.BaseName]=@([IO.File]::ReadLines($file.FullName)|Where-Object {$_ -ne ''}|ForEach-Object {ConvertFrom-Issue76LiteralJsonV6 $_})}
 $graded=@($envelope.episode.steps|Where-Object evaluate)
 for($i=0;$i -lt $outcomes.Count;$i++){
  $outcome=$outcomes[$i]
  if($null -ne $outcome.kernel_response){
   $step=@($graded|Where-Object {('behavior76:'+$_.id) -ceq $outcome.kernel_response.owner_event_ref})
   CheckBridge ($fixture+'-kernel-'+$i) {Assert-Issue76LiteralV6 ($step.Count -eq 1) 'bridge_original_step_identity';Assert-Issue76LiteralKernelV6 -Outcome $outcome -Step $step[0] -Tables $tables -Aliases $aliases -Selections @($envelope.owner_task_selections|Where-Object {$null -ne $_})}
  }else{
   CheckBridge ($fixture+'-owner-selection-'+$i) {$step=@($graded|Where-Object id -CEQ $outcome.owner_task_selection.step_id);Assert-Issue76LiteralV6 ($step.Count -eq 1) 'bridge_owner_step_identity';Assert-Issue76LiteralTaskSelectionV6 $step[0] @($envelope.owner_task_selections|Where-Object {$null -ne $_}) $outcome.owner_task_selection $aliases}
  }
  CheckBridge ($fixture+'-automatic-'+$i) {Assert-Issue76LiteralAutomaticInputsV6 $outcome $tables}
 }
 if($fixture -ceq 'business'){
  CheckBridge 'business-actual-observation-effect-date-proof' {$v=Assert-Issue76LiteralBusinessEvidenceV6 $tables;Assert-Issue76LiteralV6 ($v.observation_count -eq 3 -and $v.late_confirmed_publications.Count -eq 1) 'bridge_expected_public_three_effects_one_late';$v}
  foreach($ref in @($tables.behavior76_business_observations|ForEach-Object {$_.observation.source_ref}|Select-Object -Unique)){
   CheckBridge 'business-reference-is-data-not-authority' {Resolve-Issue76LiteralSyntheticRefV6 $ref $tables $envelope.episode.id}
  }
 }
}
$inventory=@(Get-ChildItem -LiteralPath $inputPath -File -Recurse|Sort-Object FullName|ForEach-Object {[ordered]@{path=[IO.Path]::GetRelativePath($inputPath,$_.FullName).Replace('\','/');bytes=$_.Length;sha256=(Get-FileHash -LiteralPath $_.FullName).Hash.ToLowerInvariant()}})
$receipt=[ordered]@{version='issue76-literal-actual-public-Go-to-PS-bridge-v1';input_root=$inputPath;status='PUBLIC_PREPARATION_NOT_STUDY_OR_QUALITY_ACCEPTANCE';checks=$checks;PASS=@($checks|Where-Object verdict -CEQ 'pass').Count;FAIL=@($checks|Where-Object verdict -CEQ 'fail').Count;actual_Go_exports_read=$true;checker_Go_PG_Docker_auth_model_executions=0;hidden_or_closed_private_answers_read=$false;no_new_local_automatic_exemption=$true;not_a_full32arm68study=$true;source_pins=@(@{path='literal-kernel-proof-v1.ps1';sha256=(Get-FileHash -LiteralPath (Join-Path $PSScriptRoot 'literal-kernel-proof-v1.ps1')).Hash.ToLowerInvariant()});input_inventory=$inventory}
$path=Join-Path $output 'receipt.json'
[IO.File]::WriteAllText($path,($receipt|ConvertTo-Json -Depth 40),[Text.UTF8Encoding]::new($false))
@{path=$path;sha256=(Get-FileHash -LiteralPath $path).Hash.ToLowerInvariant();PASS=$receipt.PASS;FAIL=$receipt.FAIL}|ConvertTo-Json -Compress
if($receipt.FAIL){exit 1}
