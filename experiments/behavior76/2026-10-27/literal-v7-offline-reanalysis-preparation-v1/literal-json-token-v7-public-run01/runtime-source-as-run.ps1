# ADDITIVE/PREPARED offline representation boundary. All original eleven files
# remain byte-exact. Loading this file never reads controls or runs evaluation.
$Issue76LiteralV7OriginalRuntimePins=[ordered]@{
 'local-model-proof-reader-v4.ps1'='2241e8d023b6f00e28dbfc4599a74d5d2b571bf1bb2667faf0de5c1c7f85fd39'
 'local-tail-adapter-v4.ps1'='4e6bdc53519a1d9dce02a10e2c5a8887becbed8205e791c16c11d13d233c344a'
 'local-scoring-composition-v4.ps1'='35b2734bb3ccc3c381de2d796a0d1d1a09779615c5ada2e26d67be37a081ee25'
 'authority-stream-scoring-v2d.ps1'='d8ef5be5485d2ae515f435bd5e3ef87a261a089c58e422350b41e117956745ab'
 'authority-stream-reader-v2a.cs'='836e02f4d40d0c52449f32f8560102a8ed5e4f06ff0b95a8f5dd0a5128a74641'
 'authority-stream-reader-v2a.ps1'='b03c60b3fbcdd4ee8613545738951ecbfc9f7f204409aaea20de70a25d0332c5'
 'score-mechanical-fresh-v3.ps1'='e481f9c062a3bc4500b9f0b94da8819fbf287bec075490583cad17d54198721d'
 'literal-kernel-proof-v1.ps1'='b66fc09932f010176f470a7b31499fabe39d70ebce1cf1ab0f0cedc40a7919d5'
 'literal-index-adapter-v6.ps1'='509344a7d02e68f75751e8828ec4e50b25588fcb487e66d2af22c4bce141ef27'
 'literal-scoring-composition-v6.ps1'='f9d8a4d555300b66e3b055906d28888bdc5fe8ec886a5ec63556f23ae0d67912'
 'authority-stream-scoring-v6-literal.ps1'='e306bdbe342b1f9975426d519dd6a6a08633cfe7d94d40185f94b97df0f72779'
}
function Assert-Issue76LiteralV7OriginalRuntimePins {
 foreach($entry in $Issue76LiteralV7OriginalRuntimePins.GetEnumerator()){
  if((Get-FileHash -LiteralPath (Join-Path $PSScriptRoot $entry.Key) -Algorithm SHA256).Hash.ToLowerInvariant() -cne $entry.Value){throw 'literal_reanalysis_v7:original_runtime_changed'}
 }
}
function Invoke-Issue76JsonStringTokenScopeV7 {
 [CmdletBinding()]
 param([Parameter(Mandatory)][scriptblock]$Work)
 $actual=Get-Command ConvertFrom-Json -ErrorAction Stop
 $qualified=Get-Command Microsoft.PowerShell.Utility\ConvertFrom-Json -CommandType Cmdlet -ErrorAction Stop
 if($actual.CommandType -ne 'Cmdlet' -or $actual.ModuleName -cne 'Microsoft.PowerShell.Utility' -or $actual.ImplementingType -ne $qualified.ImplementingType -or -not $qualified.Parameters.ContainsKey('DateKind') -or -not $qualified.Parameters['DateKind'].ParameterType.IsEnum -or 'String' -cnotin [Enum]::GetNames($qualified.Parameters['DateKind'].ParameterType)){
  throw 'literal_reanalysis_v7:type_preserving_installed_cmdlet_unavailable'
 }
 # A local copy is inherited by unchanged nested reader/functions/scriptblocks;
 # the caller's defaults and the eleven source files are never mutated.
 $inherited=$PSDefaultParameterValues
 if($null -ne $inherited -and $inherited.ContainsKey('Disabled') -and $inherited['Disabled']){throw 'literal_reanalysis_v7:parameter_defaults_disabled'}
 $local:PSDefaultParameterValues=@{}
 if($null -ne $inherited){foreach($key in $inherited.Keys){$local:PSDefaultParameterValues[$key]=$inherited[$key]}}
 $local:PSDefaultParameterValues['ConvertFrom-Json:DateKind']='String'
 $local:PSDefaultParameterValues['Microsoft.PowerShell.Utility\ConvertFrom-Json:DateKind']='String'
 # Capability witness is public and independent of the closed corpus. It checks
 # actual nested command binding before any future private read is permitted.
 $sample='2041-02-03T04:05:06.007Z'
 $probe=ConvertFrom-Json -InputObject ('{"value":"'+$sample+'"}') -AsHashtable
 if($probe.value -isnot [string] -or $probe.value -cne $sample){throw 'literal_reanalysis_v7:JSON_string_token_identity_unavailable'}
 & $Work
}
function Invoke-Issue76LiteralOfflineReanalysisFilesV7 {
 [CmdletBinding()]
 param(
  [Parameter(Mandatory)][string]$ReportRoot,
  [Parameter(Mandatory)][string]$StimuliPath,
  [Parameter(Mandatory)][string]$GoldensPath,
  [Parameter(Mandatory)][hashtable]$Pin,
  [Parameter(Mandatory)][string]$OriginalFrozenFailureReceiptPath,
  [Parameter(Mandatory)][string]$OriginalFrozenFailureReceiptSHA256,
  [switch]$ExecuteReviewedOfflineReanalysis
 )
 if(-not $ExecuteReviewedOfflineReanalysis){return @{version='issue76-literal-offline-reanalysis-v7';status='PREPARED_ONLY_NO_EVALUATION';scoring_executed=$false}}
 Assert-Issue76LiteralV7OriginalRuntimePins
 if($OriginalFrozenFailureReceiptSHA256 -cnotmatch '^[0-9a-f]{64}$' -or (Get-FileHash -LiteralPath $OriginalFrozenFailureReceiptPath -Algorithm SHA256).Hash.ToLowerInvariant() -cne $OriginalFrozenFailureReceiptSHA256){throw 'literal_reanalysis_v7:original_failed_execution_receipt_required'}
 . (Join-Path $PSScriptRoot 'authority-stream-scoring-v6-literal.ps1')
 $originalArguments=@{ReportRoot=$ReportRoot;StimuliPath=$StimuliPath;GoldensPath=$GoldensPath;Pin=$Pin;FrozenScorerPath=(Join-Path $PSScriptRoot 'score-mechanical-fresh-v3.ps1');AdapterPath=(Join-Path $PSScriptRoot 'literal-index-adapter-v6.ps1');CompositionPath=(Join-Path $PSScriptRoot 'literal-scoring-composition-v6.ps1')}
 $originalResult=Invoke-Issue76JsonStringTokenScopeV7 -Work {
  $failure=ConvertFrom-Json -InputObject ([IO.File]::ReadAllText($OriginalFrozenFailureReceiptPath)) -AsHashtable
  if($failure.decision -cne 'FROZEN_V6_STRICT_FAILURE_RETAINED_NO_QUIET_REPAIR' -or $failure.window -cne $Pin.window -or $failure.mapping_read -cne $false){throw 'literal_reanalysis_v7:original_failure_identity_mismatch'}
  Invoke-Issue76LiteralStreamScoringFilesV6 @originalArguments
 }
 # The old result version and predicates remain untouched within this NEW
 # envelope. A later result can never overwrite/relabel the frozen v6 failure.
 return [ordered]@{
  version='issue76-literal-offline-reanalysis-v7'
  representation='JSON string tokens retained using installed ConvertFrom-Json DateKind String in a local parameter-binding scope'
  original_execution_status='FROZEN_V6_STRICT_FAILURE_RETAINED'
  original_failure_receipt_sha256=$OriginalFrozenFailureReceiptSHA256
  posthoc_representation_reanalysis=$true
  criteria_changed=$false
  thresholds_changed=$false
  controls_or_measured_data_changed=$false
  new_native_authorization=$false
  mapping_disclosure_authorized=$false
  result=$originalResult
 }
}
