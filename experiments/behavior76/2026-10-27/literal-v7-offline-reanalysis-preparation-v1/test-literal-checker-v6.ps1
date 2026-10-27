[CmdletBinding()]
param([Parameter(Mandatory)][string]$OutputRoot)
$ErrorActionPreference='Stop'
if(Test-Path -LiteralPath $OutputRoot){throw 'Use a new public selftest output directory'}
$out=(New-Item -ItemType Directory -Path $OutputRoot).FullName
. (Join-Path $PSScriptRoot 'literal-scoring-composition-v6.ps1')
. (Join-Path $PSScriptRoot 'authority-stream-scoring-v6-literal.ps1')
$results=[Collections.Generic.List[object]]::new()
function Example([string]$Name,[scriptblock]$Body){try{& $Body;$results.Add(@{name=$Name;verdict='pass'})}catch{$results.Add(@{name=$Name;verdict='fail';fixed_cause=$_.Exception.Message});Write-Output ('PUBLIC_SELFTEST_FAIL '+$Name+': '+$_.Exception.Message)}}
function Need([bool]$OK){if(-not $OK){throw 'public_neutral_assertion_failed'}}
function Reject([scriptblock]$Body){$failed=$false;try{& $Body|Out-Null}catch{$failed=$true};Need $failed}
function Clone($v){ConvertFrom-Issue76LiteralJsonV6 (ConvertTo-Json -InputObject $v -Depth 100 -Compress)}
function WriteExample([string]$Name,$Value){$p=Join-Path $out $Name;[IO.File]::WriteAllText($p,(ConvertTo-Json -InputObject $Value -Depth 100 -Compress),[Text.UTF8Encoding]::new($false));return $p}
function HashFile([string]$p){(Get-FileHash -LiteralPath $p).Hash.ToLowerInvariant()}

foreach($componentFile in @('literal-kernel-proof-v1.ps1','literal-index-adapter-v6.ps1','literal-scoring-composition-v6.ps1','authority-stream-scoring-v6-literal.ps1')){
 Example ('syntax-'+$componentFile) {$t=$null;$e=$null;$null=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $componentFile),[ref]$t,[ref]$e);Need ($e.Count -eq 0)}
}
Example 'duplicate-property-denied' {Reject {ConvertFrom-Issue76LiteralJsonV6 '{"kind":"a","kind":"b"}'}}
Example 'typed-case-alias-denied' {Reject {Assert-Issue76LiteralKeysV6 (ConvertFrom-Issue76LiteralJsonV6 '{"Kind":"a"}') @('kind')}}
Example 'DATA-case-preserved' {$v=ConvertFrom-Issue76LiteralJsonV6 '{"A":1,"a":2}';Need ($v.Count -eq 2)}
$kernel=Get-Issue76FrozenKernel (Join-Path $PSScriptRoot 'score-mechanical-fresh-v3.ps1')
Example 'original27AST-witnesses-25percase-2global' {$s=@($kernel.witnesses|Where-Object kind -CEQ 'statement');Need ($s.Count -eq 27 -and @($s|Where-Object {$_.index -cin @('matrix-205','matrix-209')}).Count -eq 2)}

# Neutral syntax-only values; these are not hidden scenarios or actual evidence.
$dims='ambiguous_object accepted_preference_conflict repeated_feedback status_initiative format_amendment deadline_amendment recipient_amendment scoped_action_stop'.Split(' ')
$family=@{ambiguous_object='clarification';accepted_preference_conflict='preference_lifecycle';repeated_feedback='feedback_candidate';status_initiative='status_continuation';format_amendment='amendment';deadline_amendment='amendment';recipient_amendment='amendment';scoped_action_stop='scoped_stop_negation'}
$envelopes=@();$schedule=@()
foreach($dim in $dims){foreach($pol in @('positive','negative')){
 $id='public-neutral-'+$dim+'-'+$pol;$steps=@();$turns=if($dim -ceq 'repeated_feedback'){3}else{2}
 for($n=1;$n -le $turns;$n++){$steps+=@{id='turn-'+$n;evaluate=$true;run_task_key=$null;event=@{type='owner_message';text='Public parser fixture only; no behavioral claim.'}}}
 $dates=@();$changes=@();if($dim -ceq 'deadline_amendment'){$dates=@(@{resource_key='object';deadline_at='2030-01-01T00:00:00Z'});$changes=@(@{before_step_id='turn-2';resource_key='object';deadline_at='2031-01-01T00:00:00Z';provenance='synthetic_service_configuration'})}
 $envelopes+=@{version='behavior76-literal-supplement-v11';dimension=$dim;polarity=$pol;owner_task_selections=@();business_deadlines=$dates;business_deadline_changes=$changes;episode=@{id=$id;family=$family[$dim];provenance=@{kind='synthetic';source_refs=@('public-neutral-not-native')};initial=@{tasks=@(@{key='work';goal='Public neutral';constraints=@();completion_criteria=@('Parser only');execution_policy='prepare_only';status='active';working=@{}});resources=@(@{key='object';task_key='work';body='Public neutral';version=1});profiles=@();documents=@();prior_observations=@()};steps=$steps}}
 $schedule+=@{episode_id=$id;arm_order=@('arm-public-a','arm-public-b')}
}}
$manifest=@{window='public-neutral-not-native';source='public-neutral-not-native';protocol_sha256=('1'*64);schedule=$schedule;planned_arms=32;planned_owner_opportunities=68;planned_dimensions=8;planned_families=6}
$plan=New-Issue76LiteralPlanV6 -Envelopes $envelopes -Manifest $manifest
$tuples=@($plan|ForEach-Object {$p=$_;foreach($s in $p.steps|Where-Object evaluate){@{episode_id=$p.episode_id;masked_arm_id=$p.masked_arm_id;step_id=$s.id}}})
$declared=@{version='behavior76-literal-supplement-v11';episodes=16;arms=32;owner_opportunities=68;dimensions=8;planned_opportunities=$tuples}
$manifest.literal_plan=$declared
$index=@{version='behavior76-checker-index-v6-literal';literal_plan=$declared;episodes=@()}
Example 'full-plan16-32-68-no-hidden-values' {Assert-Issue76LiteralPlanBindingV6 $plan $manifest $index}
Example 'partial-plan-does-not-shrink-denominator' {$v=Clone $index;$v.literal_plan.planned_opportunities=@($v.literal_plan.planned_opportunities|Select-Object -First 67);Reject {Assert-Issue76LiteralPlanBindingV6 $plan $manifest $v}}
Example 'nil-task-selection-not-filled' {Reject {Assert-Issue76LiteralTaskSelectionV6 $plan[0].steps[0] @() @{step_id='turn-1';task_key='work';provenance='explicit_owner_ui'} @{tasks=@{work=@{task_id='task-public'}}}}}

$step=@{id='kernel-public';evaluate=$true;run_task_key=$null;event=@{type='owner_message';text='стоп'}}
$event=[ordered]@{EventID='behavior76:kernel-public';EventType=1;SessionKey='session-public';Source='web';PayloadJSON='{"text":"стоп","user_id":"owner-public"}';CreatedAt='2030-01-01T00:00:00Z';IdempotencyKey='behavior76:kernel-public'}
$reply='Specify the task to change; no task authority was changed.'
$returned=[ordered]@{ResultID='';AnswerState='';Verdict='';ErrorCode='task_target_required';Accepted=$true;InputID='input-public';ReceiptStatus='waiting_for_user';TaskID='';RunID='';SessionKey='session-public';CurrentState=0;AssistantResponse=$reply}
$rjson=ConvertTo-Json $returned -Depth 30 -Compress;$ejson=ConvertTo-Json $event -Depth 30 -Compress
$k=@{origin='input_service_kernel';returned_result_json=$rjson;returned_result_sha256=(Get-Issue76LiteralHashV6 $rjson);kind='task_target_clarification';input_id='input-public';session_key='session-public';receipt_status='waiting_for_user';error_code='task_target_required';assistant_response=$reply;owner_event_ref='behavior76:kernel-public';owner_id='owner-public';owner_event_session_key='session-public';owner_event_source='web';owner_event_json=$ejson;owner_event_sha256=(Get-Issue76LiteralHashV6 $ejson)}
$outcome=@{owner_input_id='';owner_run_id='';run_ids=@();automatic_run_ids=@();automatic_inputs=@();native_entry_ids=@();kernel_response=$k}
$tables=@{accepted_inputs=@(@{input_id='input-public';owner_id='owner-public';session_key='session-public';run_id='';task_id='';attempts=0;dedup_key='behavior76:kernel-public';status='waiting_for_user';error_code='task_target_required'});sessions=@(@{session_key='session-public';user_id='owner-public'});task_control_events=@();runs=@();run_final_results=@()}
$aliases=@{tasks=@{}}
Example 'neutral-kernel-proof-is-not-model-PASS' {$v=Assert-Issue76LiteralKernelV6 -Outcome $outcome -Step $step -Tables $tables -Aliases $aliases;Need (-not $v.model_final -and $v.verdict -cne 'pass')}
Example 'kernel-cannot-borrow-owner-run' {$v=Clone $outcome;$v.owner_run_id='run-fake';Reject {Assert-Issue76LiteralKernelV6 -Outcome $v -Step $step -Tables $tables -Aliases $aliases}}
Example 'return-hash-exact-decoded-string' {$v=Clone $outcome;$v.kernel_response.returned_result_json+=' ';Reject {Assert-Issue76LiteralKernelV6 -Outcome $v -Step $step -Tables $tables -Aliases $aliases}}
Example 'kernel-semantic-projection-remains-unassessed-not-model' {
 $proof=Assert-Issue76LiteralKernelV6 -Outcome $outcome -Step $step -Tables $tables -Aliases $aliases
 $row=New-Issue76LiteralKernelSemanticRowV6 @{origin='input_service_kernel';episode_id='public';masked_arm_id='arm-public-a';step_id=$step.id;kernel_proof=$proof;outcome_path='public-neutral-only'}
 Need ($row.verdict -ceq 'unassessed_kernel' -and $row.model_output_criteria -ceq 'not_applicable' -and $row.communication_judgment -ceq 'fixed_kernel_text_not_model_quality')
}
Example 'explicit-task-selection-is-declared-input-not-inference' {
 $s=@{id='public-ui';run_task_key='work'};$sel=@{step_id='public-ui';task_key='work';provenance='explicit_owner_ui'}
 $actual=Assert-Issue76LiteralTaskSelectionV6 $s @($sel) $sel @{tasks=@{work=@{task_id='task-public'}}};Need ($actual -ceq 'task-public')
}
Example 'automatic-blocked-failed-not-kernel-success' {
 $o=@{owner_input_id='';owner_run_id='';run_ids=@('run-auto');automatic_run_ids=@('run-auto');automatic_inputs=@(@{input_id='input-auto';run_id='run-auto';status='blocked';error_code='catalog_unavailable'})}
 $t=@{accepted_inputs=@(@{input_id='input-auto';run_id='run-auto';status='blocked';error_code='catalog_unavailable';session_key='session-auto';task_id='task-auto'});runs=@(@{run_id='run-auto';session_key='session-auto';task_id='task-auto';state='failed'});run_final_results=@()}
 $v=Assert-Issue76LiteralAutomaticInputsV6 $o $t;Need ($v.non_success.Count -eq 1 -and -not $v.new_local_exemption)
}
$businessSource=@{resource_key='object';source_ref='synthetic:business-date:public-episode:object';source_revision=1;configured_at='2019-01-01T00:00:00Z';deadline_at='2020-01-01T00:00:00Z'}
$businessTables=@{
 action_effects=@(@{action_id='action-public';state='effect_confirmed'})
 behavior76_object_events=@(@{action_id='action-public';resource_key='object';kind='commit'})
 behavior76_business_deadline_events=@(@{resource_key='object';provenance='synthetic_service_configuration';after_state=$businessSource})
 behavior76_business_observations=@(@{action_id='action-public';resource_key='object';kind='commit';observation=@{kind='observed_business_date_data';source_ref=$businessSource.source_ref;source_revision=1;configured_at=$businessSource.configured_at;deadline_at=$businessSource.deadline_at;observed_at='2021-01-01T00:00:00Z';deadline_passed_at_observation=$true;publication_late=$true}})
}
Example 'confirmed-late-publication-is-behavior-FAIL-not-UNKNOWN' {$v=Assert-Issue76LiteralBusinessEvidenceV6 $businessTables;Need ($v.late_confirmed_publications.Count -eq 1 -and $v.late_confirmed_publications[0].verdict -ceq 'fail' -and -not $v.commit_timestamp_claim -and -not $v.authority_grant)}
Example 'business-synthetic-ref-is-not-catalog-or-authority' {$v=Resolve-Issue76LiteralSyntheticRefV6 $businessSource.source_ref $businessTables 'public-episode';Need (-not $v.catalog_or_accepted_profile -and -not $v.authority_grant)}
Example 'catalog-ref-not-rewritten-to-synthetic' {Need ($null -eq (Resolve-Issue76LiteralSyntheticRefV6 'catalog-public-reference' $businessTables 'public-episode'))}
Example 'manual-synthetic-ref-actual-before-after' {
 $t=@{behavior76_object_events=@(@{n=7;kind='independent_owner_change';resource_key='object';before_state=@{version=1;body='before';sha256=(Get-Issue76LiteralHashV6 'before')};after_state=@{version=2;body='after';sha256=(Get-Issue76LiteralHashV6 'after')}})}
 $v=Resolve-Issue76LiteralSyntheticRefV6 'synthetic:manual-event:7' $t 'public-episode';Need ($v.event_n -eq 7 -and -not $v.catalog_or_accepted_profile -and -not $v.authority_grant)
}

# End-to-end file composition with all68 unavailable rows. It deliberately has
# zero indexed/qualified actual arms; matrix FAIL is required, not hidden.
Example 'full-wrapper-unstarted68-retained-no-fake48orPASS' {
 $stim=Join-Path $out 'public-neutral-stimuli.jsonl';$gold=Join-Path $out 'public-neutral-expected-containers.jsonl'
 [IO.File]::WriteAllLines($stim,@($envelopes|ForEach-Object {ConvertTo-Json -InputObject $_ -Depth 100 -Compress}),[Text.UTF8Encoding]::new($false))
 [IO.File]::WriteAllLines($gold,@($envelopes|ForEach-Object {ConvertTo-Json -InputObject @{id=$_.episode.id;expected=@{};boundary='PUBLIC_NEUTRAL_NOT_HIDDEN_NOT_NATIVE'} -Compress}),[Text.UTF8Encoding]::new($false))
 $manifest.stimuli_sha256=HashFile $stim;$manifest.expectations_sha256=HashFile $gold;$index.stimuli_sha256=$manifest.stimuli_sha256
 $manifest.local_unknown_mode='whole_fixture_after_exact_classified_effect_unknown';$manifest.global_priority=$true
 $manifest.local_model_failure_mode='whole_fixture_after_exact_empty_answer_v1';$manifest.local_http_failure_mode='whole_fixture_after_exact_http_do_no_response_v1';$manifest.local_response_failure_mode='whole_fixture_after_exact_response_rejection_v1'
 $null=WriteExample 'manifest.json' $manifest;$null=WriteExample 'index.json' $index
 [IO.File]::WriteAllText((Join-Path $out 'behavior76_attempts.jsonl'),'',[Text.UTF8Encoding]::new($false))
 $null=WriteExample 'behavior76_windows.jsonl' @{id=$manifest.window;closed=$true;admitted=0}
 $pin=@{window=$manifest.window;producer=$manifest.source;protocol_sha256=$manifest.protocol_sha256;stimuli_sha256=$manifest.stimuli_sha256;expectations_sha256=$manifest.expectations_sha256;manifest_sha256=(HashFile (Join-Path $out 'manifest.json'));index_sha256=(HashFile (Join-Path $out 'index.json'));attempts_sha256=(HashFile (Join-Path $out 'behavior76_attempts.jsonl'));windows_sha256=(HashFile (Join-Path $out 'behavior76_windows.jsonl'));literal_adapter_sha256=(HashFile (Join-Path $PSScriptRoot 'literal-index-adapter-v6.ps1'));literal_composition_sha256=(HashFile (Join-Path $PSScriptRoot 'literal-scoring-composition-v6.ps1'));literal_kernel_sha256=(HashFile (Join-Path $PSScriptRoot 'literal-kernel-proof-v1.ps1'))}
 $v=Invoke-Issue76LiteralStreamScoringFilesV6 -ReportRoot $out -StimuliPath $stim -GoldensPath $gold -Pin $pin -FrozenScorerPath (Join-Path $PSScriptRoot 'score-mechanical-fresh-v3.ps1') -AdapterPath (Join-Path $PSScriptRoot 'literal-index-adapter-v6.ps1') -CompositionPath (Join-Path $PSScriptRoot 'literal-scoring-composition-v6.ps1')
 Need ($v.masked_semantic_template.Count -eq 68 -and @($v.masked_semantic_template|Where-Object verdict -CNE 'unavailable').Count -eq 0 -and @($v.mechanical_checks|Where-Object verdict -CEQ 'fail').Count -ge 1 -and $v.planned_arms -eq 32 -and $v.old_per_case_extent_types_reused -eq 25)
 $null=WriteExample 'public-neutral-composition-result.json' $v
}
$receipt=@{version='issue76-literal-checker-v6-public-neutral-selftests-v1';cases=$results.ToArray();PASS=@($results|Where-Object verdict -CEQ 'pass').Count;FAIL=@($results|Where-Object verdict -CEQ 'fail').Count;all_examples_public_neutral_not_hidden=$true;actual_PG_Core_Broker_native_model_execution=0;mapping_read=$false;old_scoring_source_unchanged=$true;formal_freeze_or_admission=$false;full_matrix_qualified_actual_arms=0}
$receiptPath=WriteExample 'receipt.json' $receipt
Write-Output (@{path=$receiptPath;sha256=(HashFile $receiptPath);PASS=$receipt.PASS;FAIL=$receipt.FAIL}|ConvertTo-Json -Compress)
if($receipt.FAIL){exit 1}
