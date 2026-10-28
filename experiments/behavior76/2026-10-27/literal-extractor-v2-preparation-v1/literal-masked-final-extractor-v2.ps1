# Additive extraction only. Never executes criteria or converts unavailable data
# into a communication PASS. Loading this file reads no corpus or response.
function Get-Issue76RawFinalRecordV2($Row) {
 $text=$Row.content
 if($text -isnot [string]){return @{run_id=$Row.run_id;result_id=$Row.result_id;raw_final=$null;raw_utf8_sha256=$null;raw_status='nonstring_final_content'}}
 try{$bytes=[Text.UTF8Encoding]::new($false,$true).GetBytes($text)}catch{return @{run_id=$Row.run_id;result_id=$Row.result_id;raw_final=$text;raw_utf8_sha256=$null;raw_status='invalid_utf8_string'}}
 return @{run_id=$Row.run_id;result_id=$Row.result_id;raw_final=$text;raw_utf8_sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant();raw_status='exact_private_raw_final'}
}
function Read-Issue76FinalSyntaxV2([string]$Text) {
 $doc=$null
 try {
  $doc=[Text.Json.JsonDocument]::Parse($Text)
  if($doc.RootElement.ValueKind -ne [Text.Json.JsonValueKind]::Object){return @{status='json_root_not_object';answer=$null;decoded=$null}}
  function Test-Issue76FinalKeysV2($Node,[bool]$Root) {
   if($Node.ValueKind -eq [Text.Json.JsonValueKind]::Object){
    $exact=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $folded=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach($p in $Node.EnumerateObject()){
     if(-not $exact.Add($p.Name)){return $false}
     if($Root -and -not $folded.Add($p.Name)){return $false}
     if(-not (Test-Issue76FinalKeysV2 $p.Value $false)){return $false}
    }
   }elseif($Node.ValueKind -eq [Text.Json.JsonValueKind]::Array){foreach($n in $Node.EnumerateArray()){if(-not (Test-Issue76FinalKeysV2 $n $false)){return $false}}}
   return $true
  }
  if(-not (Test-Issue76FinalKeysV2 $doc.RootElement $true)){return @{status='duplicate_or_root_case_aliased_key';answer=$null;decoded=$null}}
  $answerElement=[Text.Json.JsonElement]::new()
  if(-not $doc.RootElement.TryGetProperty('answer',[ref]$answerElement)){return @{status='exact_answer_key_missing';answer=$null;decoded=$null}}
  if($answerElement.ValueKind -ne [Text.Json.JsonValueKind]::String){return @{status='answer_not_string';answer=$null;decoded=$null}}
  # Caller MUST run inside pinned Invoke-Issue76JsonStringTokenScopeV7.
  # JsonDocument's exact string token is an independent representation witness.
  $decoded=ConvertFrom-Json -InputObject $Text -AsHashtable -Depth 100
  if($decoded -isnot [Collections.IDictionary] -or $decoded.answer -isnot [string] -or $decoded.answer -cne $answerElement.GetString()){return @{status='decoded_string_identity_mismatch';answer=$null;decoded=$null}}
  return @{status='json_object_string_answer';answer=$answerElement.GetString();decoded=$decoded}
 }catch{return @{status='json_invalid';answer=$null;decoded=$null}}
 finally{if($null -ne $doc){$doc.Dispose()}}
}
function Get-Issue76MaskedOwnerFinalV2 {
 [CmdletBinding()]
 param([Parameter(Mandatory)][string]$EpisodeID,[Parameter(Mandatory)][string]$MaskedArmID,[Parameter(Mandatory)][string]$StepID,
  [Parameter(Mandatory)]$OwnerRow,[Parameter(Mandatory)]$Outcome,[object[]]$SemanticRows=@(),[object[]]$MechanicalChecks=@(),[object[]]$FinalRows=@(),[Parameter(Mandatory)][string]$TablesDir)
 $base=[ordered]@{version='issue76-masked-final-extractor-v2';origin=$OwnerRow.origin;communication_availability='UNAVAILABLE';communication_verdict='UNASSESSED_NOT_PASS';availability_reason='existing_mechanical_or_identity_unavailable';decoded_answer=$null;decoded_final=$null;parse_status='not_attempted';raw_finals=@();mechanical_validity_overridden=$false;criteria_changed=$false;model_final=$false}
 if($OwnerRow.origin -ceq 'input_service_kernel'){
  $base.communication_availability='NOT_APPLICABLE_MODEL';$base.availability_reason='separate_existing_kernel_path';$base.parse_status='kernel_not_parsed_as_model';return $base
 }
 $run=[string]$Outcome.owner_run_id
 $selected=@($FinalRows|Where-Object {$run -and $_.run_id -ceq $run})
 $base.raw_finals=@($selected|ForEach-Object {Get-Issue76RawFinalRecordV2 $_})
 if($selected.Count -ne 1){$base.parse_status='selected_final_not_unique';return $base}
 if($base.raw_finals[0].raw_status -cne 'exact_private_raw_final'){$base.parse_status=$base.raw_finals[0].raw_status;return $base}
 $syntax=Read-Issue76FinalSyntaxV2 $base.raw_finals[0].raw_final
 $base.parse_status=$syntax.status
 # This is evidence availability, not a new score or repair of a failed
 # structured contract. The frozen mechanical template remains authoritative.
 if($syntax.status -cne 'json_object_string_answer'){return $base}
 $matchingSem=@($SemanticRows|Where-Object {$_.episode_id -ceq $EpisodeID -and $_.masked_arm_id -ceq $MaskedArmID -and $_.step_id -ceq $StepID})
 $checks=@($MechanicalChecks|Where-Object {$_.episode_id -ceq $EpisodeID -and $_.masked_arm_id -ceq $MaskedArmID})
 $evidence=$TablesDir+'/run_final_results.jsonl'
 $identity=($OwnerRow.episode_id -ceq $EpisodeID -and $OwnerRow.masked_arm_id -ceq $MaskedArmID -and $OwnerRow.step_id -ceq $StepID -and $OwnerRow.origin -ceq 'owner_model_or_actual_denial' -and $OwnerRow.verdict -ceq 'unassessed' -and -not [string]::IsNullOrEmpty([string]$Outcome.owner_input_id) -and $run -cin @($Outcome.run_ids))
 $uniqueSem=($matchingSem.Count -eq 1 -and $matchingSem[0].verdict -ceq 'unassessed' -and $matchingSem[0].answer -is [string] -and $matchingSem[0].answer -ceq $syntax.answer -and $matchingSem[0].evidence_ref -ceq $evidence)
 $proofs=$true
 foreach($name in @(('owner-input-run:'+$StepID),('owner-run-in-slot:'+$StepID))){
  $p=@($checks|Where-Object check -CEQ $name)
  if($p.Count -ne 1 -or $p[0].verdict -cne 'pass'){$proofs=$false}
 }
 $invalid=@($checks|Where-Object {$_.check -cin @(('structured-final:'+$StepID),('one-authoritative-final:'+$StepID),('owner-input-run-correlation:'+$StepID)) -and $_.verdict -cne 'pass'})
 if(-not $identity -or -not $uniqueSem -or -not $proofs -or $invalid.Count){return $base}
 $base.communication_availability='AVAILABLE_FOR_EXISTING_MANUAL_RUBRIC'
 $base.availability_reason='unique_existing_mechanical_valid_final_and_owner_proof'
 $base.decoded_answer=$syntax.answer;$base.decoded_final=$syntax.decoded;$base.model_final=$true
 return $base
}
