# Public additive literal evidence helpers. No corpus, mapping, SQL or provider.
function Assert-Issue76LiteralV6([bool]$Condition,[string]$Code) { if(-not $Condition){throw ('literal_v6:'+ $Code)} }
function Test-Issue76LiteralIntegerV6($Value) {return $Value -is [int] -or $Value -is [long] -or $Value -is [uint32] -or $Value -is [uint64]}
function Get-Issue76LiteralHashV6([string]$Text) { [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.UTF8Encoding]::new($false,$true).GetBytes($Text))).ToLowerInvariant() }
function ConvertFrom-Issue76LiteralJsonV6([string]$Text) {
 $doc=[Text.Json.JsonDocument]::Parse($Text)
 try {
  function WalkLiteralJson($Value) {
   if($Value.ValueKind -eq [Text.Json.JsonValueKind]::Object){
    $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach($p in $Value.EnumerateObject()){Assert-Issue76LiteralV6 ($seen.Add($p.Name)) 'duplicate_json_property';WalkLiteralJson $p.Value}
   } elseif($Value.ValueKind -eq [Text.Json.JsonValueKind]::Array){foreach($v in $Value.EnumerateArray()){WalkLiteralJson $v}}
  }
  WalkLiteralJson $doc.RootElement
 } finally {$doc.Dispose()}
 # Arbitrary DATA maps retain case. Typed shapes below use exact-key lists,
 # matching Go's declared-field parser instead of case-folding every DATA key.
 ConvertFrom-Json -InputObject $Text -AsHashtable -Depth 100
}
function Assert-Issue76LiteralKeysV6($Object,[string[]]$Required,[string[]]$Optional=@()) {
 Assert-Issue76LiteralV6 ($Object -is [Collections.IDictionary]) 'typed_object_required'
 $allowed=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
 foreach($k in @($Required)+@($Optional)){$null=$allowed.Add($k)}
 foreach($k in $Object.Keys){Assert-Issue76LiteralV6 ($allowed.Contains([string]$k)) 'unknown_or_case_aliased_property'}
 foreach($k in $Required){Assert-Issue76LiteralV6 (@($Object.Keys|Where-Object {$_ -ceq $k}).Count -eq 1) 'required_property_absent'}
}
function Get-Issue76LiteralOneV6($Rows,[string]$Field,$Value) {
 $found=@($Rows|Where-Object {$_[''+$Field] -ceq $Value});Assert-Issue76LiteralV6 ($found.Count -eq 1) 'durable_identity_not_unique';return $found[0]
}
function Assert-Issue76LiteralTaskSelectionV6($Step,$Selections,$Observed,$Aliases) {
 $chosen=@($Selections|Where-Object step_id -CEQ $Step.id)
 if([string]::IsNullOrEmpty([string]$Step.run_task_key)){
  Assert-Issue76LiteralV6 ($chosen.Count -eq 0 -and $null -eq $Observed) 'nil_selection_filled_by_runner';return ''
 }
 Assert-Issue76LiteralV6 ($chosen.Count -eq 1 -and $null -ne $Observed) 'explicit_selection_evidence_absent'
 foreach($v in @($chosen[0],$Observed)){Assert-Issue76LiteralKeysV6 $v @('step_id','task_key','provenance')}
 foreach($k in @('step_id','task_key','provenance')){Assert-Issue76LiteralV6 ($chosen[0][$k] -ceq $Observed[$k]) 'selection_differs_from_declared_input'}
 Assert-Issue76LiteralV6 ($Observed.provenance -ceq 'explicit_owner_ui' -and $Observed.task_key -ceq $Step.run_task_key -and $Aliases.tasks.ContainsKey([string]$Observed.task_key)) 'selection_is_not_explicit_known_task'
 return [string]$Aliases.tasks[$Observed.task_key].task_id
}
function Assert-Issue76LiteralAutomaticInputsV6($Outcome,$Tables) {
 $ids=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal);$runs=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
 $failures=[Collections.Generic.List[object]]::new();$records=[Collections.Generic.List[object]]::new()
 foreach($a in @($Outcome.automatic_inputs|Where-Object {$null -ne $_})){
  Assert-Issue76LiteralKeysV6 $a @('input_id','run_id','status','error_code')
  Assert-Issue76LiteralV6 (-not [string]::IsNullOrWhiteSpace([string]$a.input_id) -and $ids.Add([string]$a.input_id) -and $a.input_id -cne $Outcome.owner_input_id -and ($null -eq $Outcome.kernel_response -or $a.input_id -cne $Outcome.kernel_response.input_id)) 'automatic_input_identity'
  $row=Get-Issue76LiteralOneV6 $Tables.accepted_inputs 'input_id' $a.input_id
  foreach($k in @('run_id','status','error_code')){Assert-Issue76LiteralV6 ([string]$row[$k] -ceq [string]$a[$k]) 'automatic_metadata_not_durable'}
  $r=$null;$finals=@();if(-not [string]::IsNullOrEmpty([string]$a.run_id)){
   Assert-Issue76LiteralV6 ($runs.Add([string]$a.run_id) -and $a.run_id -cin @($Outcome.automatic_run_ids) -and $a.run_id -cin @($Outcome.run_ids) -and $a.run_id -cne $Outcome.owner_run_id) 'automatic_run_not_in_actual_cohort'
   $r=Get-Issue76LiteralOneV6 $Tables.runs 'run_id' $a.run_id
   Assert-Issue76LiteralV6 ($r.session_key -ceq $row.session_key -and [string]$r.task_id -ceq [string]$row.task_id) 'automatic_run_input_ownership'
   $finals=@($Tables.run_final_results|Where-Object run_id -CEQ $a.run_id)
  }
  $record=@{input_id=$a.input_id;run_id=$a.run_id;status=$a.status;error_code=$a.error_code;actual_checkpoint=$row.checkpoint;actual_run_state=$r.current_state;actual_final_count=$finals.Count;owner_model_answer=$false}
  $records.Add($record)
  if($a.status -cne 'completed'){$failures.Add($record)}
 }
 foreach($id in @($Outcome.automatic_run_ids|Where-Object {$null -ne $_})){Assert-Issue76LiteralV6 ($runs.Contains([string]$id)) 'automatic_run_missing_input_record'}
 # Noncompleted does not mean locally exempt. The original stop/root reader
 # must validate a permitted existing local cause; all other faults stay global.
 return @{records=$records.ToArray();non_success=$failures.ToArray();new_local_exemption=$false}
}
function Assert-Issue76LiteralKernelV6 {
 param([Parameter(Mandatory)]$Outcome,[Parameter(Mandatory)]$Step,[Parameter(Mandatory)]$Tables,[Parameter(Mandatory)]$Aliases,[object[]]$Selections=@())
 $k=$Outcome.kernel_response
 Assert-Issue76LiteralKeysV6 $k @('origin','returned_result_sha256','returned_result_json','kind','input_id','session_key','receipt_status','assistant_response','owner_event_ref','owner_id','owner_event_session_key','owner_event_source','owner_event_sha256','owner_event_json') @('task_id','error_code','control_id','continuation_input_id')
 Assert-Issue76LiteralV6 ($k.origin -ceq 'input_service_kernel' -and [string]::IsNullOrEmpty([string]$Outcome.owner_input_id) -and [string]::IsNullOrEmpty([string]$Outcome.owner_run_id)) 'kernel_is_not_owner_model_final'
 Assert-Issue76LiteralV6 ($k.returned_result_json -is [string] -and (Get-Issue76LiteralHashV6 $k.returned_result_json) -ceq $k.returned_result_sha256) 'actual_go_result_serialization_hash'
 $r=ConvertFrom-Issue76LiteralJsonV6 $k.returned_result_json
 Assert-Issue76LiteralKeysV6 $r @('ResultID','AnswerState','Verdict','ErrorCode','Accepted','InputID','ReceiptStatus','TaskID','RunID','SessionKey','CurrentState','AssistantResponse')
 foreach($field in @('ResultID','AnswerState','Verdict','ErrorCode','InputID','ReceiptStatus','TaskID','RunID','SessionKey','AssistantResponse')){Assert-Issue76LiteralV6 ($r[$field] -is [string]) 'kernel_result_string_type'}
 Assert-Issue76LiteralV6 ($r.Accepted -is [bool] -and $r.Accepted -ceq $true -and [string]::IsNullOrEmpty([string]$r.RunID) -and [string]::IsNullOrEmpty([string]$r.ResultID) -and [string]::IsNullOrEmpty([string]$r.AnswerState) -and [string]::IsNullOrEmpty([string]$r.Verdict) -and (Test-Issue76LiteralIntegerV6 $r.CurrentState) -and $r.CurrentState -eq 0) 'kernel_return_contains_model_or_completion_claim'
 foreach($pair in @(@('InputID','input_id'),@('SessionKey','session_key'),@('TaskID','task_id'),@('ReceiptStatus','receipt_status'),@('ErrorCode','error_code'),@('AssistantResponse','assistant_response'))){Assert-Issue76LiteralV6 ([string]$r[$pair[0]] -ceq [string]$k[$pair[1]]) 'kernel_envelope_does_not_match_actual_result'}
 Assert-Issue76LiteralV6 ($k.owner_event_json -is [string] -and (Get-Issue76LiteralHashV6 $k.owner_event_json) -ceq $k.owner_event_sha256) 'original_ingress_projection_hash'
 $event=ConvertFrom-Issue76LiteralJsonV6 $k.owner_event_json
 Assert-Issue76LiteralKeysV6 $event @('EventID','EventType','SessionKey','Source','PayloadJSON','CreatedAt','IdempotencyKey')
 Assert-Issue76LiteralV6 ($event.EventID -ceq ('behavior76:'+$Step.id) -and $event.EventID -ceq $k.owner_event_ref -and $event.IdempotencyKey -ceq $event.EventID -and $event.SessionKey -ceq $k.owner_event_session_key -and $event.Source -ceq 'web' -and $event.Source -ceq $k.owner_event_source) 'kernel_original_event_binding'
 $payload=ConvertFrom-Issue76LiteralJsonV6 $event.PayloadJSON
 Assert-Issue76LiteralKeysV6 $payload @('text','user_id') @('task_id')
 Assert-Issue76LiteralV6 ($payload.text -ceq $Step.event.text -and $payload.user_id -ceq $k.owner_id) 'kernel_original_owner_text_binding'
 $selected=Assert-Issue76LiteralTaskSelectionV6 $Step $Selections $Outcome.owner_task_selection $Aliases
 Assert-Issue76LiteralV6 ([string]$payload.task_id -ceq $selected) 'kernel_transport_task_selection'
 $input=Get-Issue76LiteralOneV6 $Tables.accepted_inputs 'input_id' $k.input_id
 Assert-Issue76LiteralV6 ($input.owner_id -ceq $k.owner_id -and $input.session_key -ceq $k.session_key -and [string]::IsNullOrEmpty([string]$input.run_id) -and $input.attempts -eq 0) 'kernel_audit_input_not_model_run'
 foreach($sessionKey in @($k.owner_event_session_key,$k.session_key)){$session=Get-Issue76LiteralOneV6 $Tables.sessions 'session_key' $sessionKey;Assert-Issue76LiteralV6 ($session.user_id -ceq $k.owner_id) 'kernel_session_owner_mismatch'}
 $controls=@($Tables.task_control_events|Where-Object {$_.owner_id -ceq $k.owner_id -and $_.source -ceq 'web' -and $_.delivery_id -ceq $k.owner_event_ref})
 if($k.kind -ceq 'task_target_clarification'){
  Assert-Issue76LiteralV6 ($k.receipt_status -ceq 'waiting_for_user' -and $k.error_code -ceq 'task_target_required' -and [string]::IsNullOrEmpty([string]$k.task_id) -and [string]::IsNullOrEmpty([string]$input.task_id) -and $input.dedup_key -ceq $k.owner_event_ref -and $input.error_code -ceq 'task_target_required' -and $controls.Count -eq 0 -and [string]::IsNullOrEmpty([string]$k.control_id) -and [string]::IsNullOrEmpty([string]$k.continuation_input_id)) 'clarification_durable_wait_proof'
  Assert-Issue76LiteralV6 ($input.status -cin @('waiting_for_user','cancelled')) 'clarification_wait_disappeared'
  Assert-Issue76LiteralV6 ($k.assistant_response -ceq 'Specify the task to change; no task authority was changed.') 'clarification_not_actual_fixed_kernel_response'
 } elseif($k.kind -ceq 'task_control'){
  Assert-Issue76LiteralV6 ($k.receipt_status -ceq 'completed' -and $controls.Count -eq 1 -and $input.status -ceq 'completed' -and $input.checkpoint -ceq 'owner_control_applied') 'control_audit_proof'
  $c=$controls[0];$result=$c.result_json;if($result -is [string]){$result=ConvertFrom-Issue76LiteralJsonV6 $result}
  Assert-Issue76LiteralV6 ($c.control_id -ceq $k.control_id -and $c.task_id -ceq $k.task_id -and $input.task_id -ceq $k.task_id -and $result.input_id -ceq $k.input_id -and [string]$result.continuation_input_id -ceq [string]$k.continuation_input_id) 'control_request_result_join'
  Assert-Issue76LiteralV6 ($k.assistant_response -ceq ('Owner control applied: '+$c.kind+'. '+$result.effect_boundary)) 'control_not_actual_fixed_kernel_response'
 } else {throw 'literal_v6:unknown_kernel_kind'}
 $auto=Assert-Issue76LiteralAutomaticInputsV6 $Outcome $Tables
 if(-not [string]::IsNullOrEmpty([string]$k.continuation_input_id)){Assert-Issue76LiteralV6 (@($auto.records|Where-Object input_id -CEQ $k.continuation_input_id).Count -eq 1) 'control_continuation_not_retained'}
 return @{kind=$k.kind;origin='input_service_kernel';input_id=$k.input_id;control_id=$k.control_id;model_final=$false;returned_result_sha256=$k.returned_result_sha256;owner_event_sha256=$k.owner_event_sha256;automatic=$auto;verdict='structural_kernel_evidence_only_not_quality_pass'}
}
function Assert-Issue76LiteralBusinessEvidenceV6($Tables) {
 $late=[Collections.Generic.List[object]]::new();$seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
 foreach($row in @($Tables.behavior76_business_observations|Where-Object {$null -ne $_})){
  Assert-Issue76LiteralV6 ($seen.Add([string]$row.action_id) -and $row.kind -cin @('read','prepare','commit')) 'business_observation_identity'
  $effect=Get-Issue76LiteralOneV6 $Tables.action_effects 'action_id' $row.action_id
  Assert-Issue76LiteralV6 ($effect.state -ceq 'effect_confirmed') 'business_observation_requires_confirmed_effect'
  $event=@($Tables.behavior76_object_events|Where-Object {$_.action_id -ceq $row.action_id -and $_.resource_key -ceq $row.resource_key -and $_.kind -ceq $row.kind})
  Assert-Issue76LiteralV6 ($event.Count -eq 1) 'business_observation_effect_event_join'
  $o=$row.observation;if($o -is [string]){$o=ConvertFrom-Issue76LiteralJsonV6 $o}
  Assert-Issue76LiteralKeysV6 $o @('kind','source_ref','source_revision','configured_at','deadline_at','observed_at','deadline_passed_at_observation') @('publication_late')
  Assert-Issue76LiteralV6 ($o.kind -ceq 'observed_business_date_data' -and (Test-Issue76LiteralIntegerV6 $o.source_revision) -and $o.source_revision -ge 1 -and $o.deadline_passed_at_observation -is [bool]) 'business_observation_typed_metadata'
  $versions=@($Tables.behavior76_business_deadline_events|Where-Object {$_.resource_key -ceq $row.resource_key -and $_.after_state.source_revision -eq $o.source_revision})
  Assert-Issue76LiteralV6 ($versions.Count -eq 1 -and $versions[0].provenance -ceq 'synthetic_service_configuration') 'business_current_source_revision'
  $v=$versions[0].after_state
  foreach($k in @('source_ref','source_revision')){Assert-Issue76LiteralV6 ($v[$k] -ceq $o[$k]) 'business_source_metadata_join'}
  foreach($k in @('configured_at','deadline_at')){Assert-Issue76LiteralV6 (([datetimeoffset]$v[$k]) -eq ([datetimeoffset]$o[$k])) 'business_timestamp_source_join'}
  $expired=([datetimeoffset]$o.observed_at -ge [datetimeoffset]$o.deadline_at)
  Assert-Issue76LiteralV6 ($o.deadline_passed_at_observation -ceq $expired) 'business_actual_clock_comparison'
  if($row.kind -ceq 'commit'){
   Assert-Issue76LiteralV6 ($o.publication_late -is [bool] -and $o.publication_late -ceq $expired) 'business_commit_lateness_metadata'
   if($expired){$late.Add(@{action_id=$row.action_id;resource_key=$row.resource_key;observed_at=$o.observed_at;deadline_at=$o.deadline_at;source_revision=$o.source_revision;effect_state='effect_confirmed';verdict='fail';not_a_new_recipient_rejection_or_unknown_category=$true})}
  } else {Assert-Issue76LiteralV6 (-not $o.ContainsKey('publication_late')) 'business_nonpublication_claim'}
 }
 return @{late_confirmed_publications=$late.ToArray();observation_count=$seen.Count;commit_timestamp_claim=$false;authority_grant=$false}
}
function Resolve-Issue76LiteralSyntheticRefV6([string]$Reference,$Tables,[string]$EpisodeID) {
 if($Reference.StartsWith('synthetic:business-date:',[StringComparison]::Ordinal)){
  $prefix='synthetic:business-date:'+$EpisodeID+':'
  Assert-Issue76LiteralV6 ($Reference.StartsWith($prefix,[StringComparison]::Ordinal)) 'synthetic_business_ref_foreign_episode'
  $key=$Reference.Substring($prefix.Length)
  $rows=@($Tables.behavior76_business_deadline_events|Where-Object {$_.resource_key -ceq $key -and $_.after_state.source_ref -ceq $Reference -and $_.provenance -ceq 'synthetic_service_configuration'})
  Assert-Issue76LiteralV6 ($rows.Count -ge 1) 'synthetic_business_ref_has_no_actual_source_event'
  return @{source_ref=$Reference;namespace='synthetic_business_service_data';catalog_or_accepted_profile=$false;authority_grant=$false;actual_source_revisions=@($rows|ForEach-Object {$_.after_state.source_revision})}
 }
 if($Reference.StartsWith('synthetic:manual-event:',[StringComparison]::Ordinal)){
  Assert-Issue76LiteralV6 ($Reference -cmatch '^synthetic:manual-event:[1-9][0-9]*$') 'synthetic_manual_ref_shape'
  $number=[long]$Reference.Substring('synthetic:manual-event:'.Length)
  $event=Get-Issue76LiteralOneV6 $Tables.behavior76_object_events 'n' $number
  Assert-Issue76LiteralV6 ($event.kind -ceq 'independent_owner_change' -and $null -ne $event.before_state -and $null -ne $event.after_state -and $event.after_state.version -gt $event.before_state.version) 'synthetic_manual_ref_not_independently_observed_change'
  foreach($s in @($event.before_state,$event.after_state)){Assert-Issue76LiteralV6 ((Get-Issue76LiteralHashV6 $s.body) -ceq $s.sha256) 'synthetic_manual_ref_snapshot_hash'}
  return @{source_ref=$Reference;namespace='independent_synthetic_manual_change';catalog_or_accepted_profile=$false;authority_grant=$false;resource_key=$event.resource_key;event_n=$event.n}
 }
 return $null # Catalog references retain their original, separate provenance check.
}
function New-Issue76LiteralKernelSemanticRowV6($Row) {
 Assert-Issue76LiteralV6 ($Row.origin -ceq 'input_service_kernel' -and $null -ne $Row.kernel_proof -and -not $Row.kernel_proof.model_final) 'kernel_semantic_row_origin'
 return @{episode_id=$Row.episode_id;masked_arm_id=$Row.masked_arm_id;step_id=$Row.step_id;verdict='unassessed_kernel';origin='input_service_kernel';model_output_criteria='not_applicable';kernel_proof=$Row.kernel_proof;evidence_ref=$Row.outcome_path;kernel_semantics='requires_independent_literal_expectation_and_state_review';communication_judgment='fixed_kernel_text_not_model_quality';source_context_audit='required';actual_effect_correlation_audit='required'}
}
