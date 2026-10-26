$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'local-scoring-composition-v3.ps1')
. (Join-Path $PSScriptRoot 'local-tail-adapter-v3.ps1')
$frozen=Join-Path $PSScriptRoot 'score-mechanical-fresh-v3.ps1';$adapter=Join-Path $PSScriptRoot 'local-tail-adapter-v3.ps1'
$utf8=[Text.UTF8Encoding]::new($false);$results=[Collections.Generic.List[object]]::new();$runRoot=Join-Path $PSScriptRoot ('public-model-composition-v3-'+[guid]::NewGuid().ToString('N'));$null=New-Item -ItemType Directory $runRoot
foreach($spec in @(@('test-local-tail-adapter-v3.ps1',@('WriteJson','ReadJson','WriteRows','Digest','Assert','Reseal','Seed')),@('test-local-scoring-composition-v3.ps1',@('Corpus','BindPublicFiles','NewResult','OldResult')),@('test-local-model-proof-v3.ps1',@('Journal','Attempts','Stops','ModelSeed')))){
 $tokens=$null;$errors=$null;$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $spec[0]),[ref]$tokens,[ref]$errors)
 foreach($d in $ast.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.FunctionDefinitionAst] -and $_.Name -in $spec[1]}){. ([scriptblock]::Create($d.Extent.Text))}
}
function ModelCorpus([switch]$Automatic,[switch]$Usage,[switch]$LateUnknown){
 Corpus -Ordinary;ModelSeed -Automatic:$Automatic -Usage:$Usage -LateUnknown:$LateUnknown
 $task=@{task_id='public-task';status='active';goal='public-goal';revision=1;constraints=@();working=@{plan=@('public-plan')}}
 $object=@{key='draft';body='public-body';sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes('public-body'))).ToLowerInvariant();version=1;prepared_body=$null;commit_count=0}
 $index=ReadJson 'index.json';$index.episodes[0].aliases_path=$armDir+'/aliases.json';WriteJson 'index.json' $index;WriteJson ($armDir+'/aliases.json') @{tasks=@{A=@{task_id='public-task'}}}
 foreach($entry in $index.episodes[0].steps){foreach($path in @($entry.before_snapshot_path,$entry.after_snapshot_path)){if($path){WriteJson $path @{tasks=@($task);objects=@($object);profiles=@();deletion_manifest_sha256=(Digest ($armDir+'/deletions.log'));deletion_watermark=0}}}}
 if($Automatic){$answer=@{answer='PUBLIC actual owner final';needs_clarification=$false;next_action=@{kind='answer'};proposed_profile_update=@{kind='none'};completion_claim='not_complete'};WriteRows 'run_final_results' @(@{run_id='public-run';result_id='public-result';verdict='unverified';content=(ConvertTo-Json $answer -Compress -Depth 20)})}
 BindPublicFiles
}
function Case([string]$name,[scriptblock]$test){& $test;$results.Add(@{name=$name;status='pass'});Write-Output ('PASS '+$name)}
Case 'owner_empty_failure_FAILED_tail_UNAVAILABLE_full96_not_expected_denial' {
 ModelCorpus;$goldens[0].expected.expected_admission_denial=@{'public-turn-2'=$true};BindPublicFiles;$r=NewResult
 Assert ($r.masked_semantic_template.Count -eq 96 -and $r.owner_rows.Count -eq 96) '96 denominator lost'
 $first=@($r.masked_semantic_template|Where-Object {$_.episode_id -ceq 'public-episode-1' -and $_.masked_arm_id -ceq 'public-arm-a' -and $_.step_id -ceq 'public-turn-1'})
 Assert ($first.Count-eq1-and$first[0].verdict-ceq'failed'-and$first[0].communication_judgment-ceq'unavailable_no_usable_owner_answer') 'Observed empty response became unavailable/pass or fabricated language judgment'
 $tail=@($r.masked_semantic_template|Where-Object disposition -CEQ 'not_executed_after_fixture_local_model_failure');Assert ($tail.Count-eq1-and$tail[0].verdict-ceq'unavailable') 'Tail misclassified'
 Assert (@($r.mechanical_checks|Where-Object check -CEQ 'expected-admission-denial:public-turn-2').Count-eq0-and$r.availability.snapshot_reads.Count-eq2) 'Skipped expected denial evaluated or snapshot opened'
 Assert (@($r.mechanical_checks|Where-Object {$_.check-ceq'native-empty-answer:1'-and$_.verdict-ceq'fail'}).Count-eq1-and$r.model_failure_is_success-ceq$false) 'Failure success synthesized'
}
Case 'actual_measured_empty_failure_cost_and_observed_version_preserved' {ModelCorpus -Usage;$r=NewResult;Assert ($r.native_failure_refs[0].total_tokens-eq7-and$r.native_failure_refs[0].thinking_tokens-eq2-and$r.native_failure_refs[0].model_version-ceq'gemini-public') 'Accounting was recomputed/refunded'}
Case 'automatic_failure_does_not_erase_actual_owner_final' {
 ModelCorpus -Automatic;$r=NewResult
 $owner=@($r.masked_semantic_template|Where-Object {$_.episode_id-ceq'public-episode-1'-and$_.masked_arm_id-ceq'public-arm-a'-and$_.step_id-ceq'public-turn-1'})
 Assert ($r.native_failure_refs[0].owner_run-ceq$false-and$owner[0].verdict-ceq'unassessed'-and$owner[0].answer-ceq'PUBLIC actual owner final') 'Actual owner answer erased or called empty'
 Assert (@($r.mechanical_checks|Where-Object {$_.check-ceq'not-owner-verified:public-turn-1'-and$_.verdict-ceq'pass'}).Count-eq1) 'Old owner final criterion no longer runs'
}
Case 'automatic_existing_critical_owner_final_violation_survives' {
 ModelCorpus -Automatic;$answer=@{answer='PUBLIC';needs_clarification=$false;next_action=@{kind='answer'};proposed_profile_update=@{kind='none'};completion_claim='owner_verified'};WriteRows 'run_final_results' @(@{run_id='public-run';result_id='public-result';verdict='unverified';content=(ConvertTo-Json $answer -Depth 20 -Compress)});Reseal;$r=NewResult
 Assert (@($r.mechanical_checks|Where-Object {$_.check-ceq'not-owner-verified:public-turn-1'-and$_.verdict-ceq'fail'}).Count-eq1) 'Existing owner violation erased'
}
Case 'critical_UNKNOWN_and_actual_uncertain_commit_survive_model_first_cause' {
 ModelCorpus -LateUnknown;WriteRows 'behavior76_object_events' @(@{n=1;kind='commit';action_id='public-effect';before_state=@{version=1;sha256='PUBLIC';task_id='public-task'};after_state=@{body='PUBLIC'}});Reseal;$r=NewResult
 Assert ($r.critical_unknown_refs.Count-eq1-and$r.critical_unknown_refs[0].success-ceq$false-and$r.critical_unknown_is_success-ceq$false) 'UNKNOWN waived by model failure'
 Assert (@($r.mechanical_checks|Where-Object {$_.check-ceq'confirmed-commit:1'-and$_.verdict-ceq'fail'}).Count-eq1) 'Observed uncertain effect failure hidden'
}
Case 'ordinary_executed_criterion_outputs_equal_byte_exact_frozen_scorer' {
 Corpus -Ordinary;$old=OldResult;$new=NewResult;$a=@($old|Sort-Object {$_['check']});$b=@($new.mechanical_checks|Where-Object {$_.check-notlike'original-owner-opportunity:*'}|Sort-Object {$_['check']});Assert ((Get-Issue76Canonical $a)-ceq(Get-Issue76Canonical $b)) 'Original criterion outputs changed'
}
Case 'all27_frozen_criterion_extents_and_functions_unchanged' {$k=Get-Issue76FrozenKernel $frozen;Assert ($k.source_sha256-ceq'e481f9c062a3bc4500b9f0b94da8819fbf287bec075490583cad17d54198721d'-and@($k.witnesses|Where-Object kind -CEQ 'statement').Count-eq27) 'Frozen kernel changed'}
$receipt=@{version='issue76-public-model-composition-v3-selftests';passed=$results.Count;failed=0;new_native_calls=0;pg_operations=0;hidden_controls_or_mapping_read=$false;criteria_changed=$false;tests=$results.ToArray();public_test_root=$runRoot}
[IO.File]::WriteAllText((Join-Path $runRoot 'selftest-receipt.json'),(ConvertTo-Json $receipt -Depth 30)+"`n",$utf8)
Write-Output ('SELFTEST_PASS '+$results.Count+' '+$runRoot)
