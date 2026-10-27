$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'local-scoring-composition-v4.ps1')
. (Join-Path $PSScriptRoot 'local-tail-adapter-v4.ps1')
. (Join-Path $PSScriptRoot 'authority-stream-scoring-v2d.ps1')
$frozen=Join-Path $PSScriptRoot 'score-mechanical-fresh-v3.ps1';$adapter=Join-Path $PSScriptRoot 'local-tail-adapter-v4.ps1';$composer=Join-Path $PSScriptRoot 'local-scoring-composition-v4.ps1'
$utf8=[Text.UTF8Encoding]::new($false);$results=[Collections.Generic.List[object]]::new();$runRoot=Join-Path $PSScriptRoot ('public-http-composition-v4-'+[guid]::NewGuid().ToString('N'));$null=New-Item -ItemType Directory $runRoot
foreach($spec in @(@('test-local-tail-adapter-v4.ps1',@('WriteJson','ReadJson','WriteRows','Digest','Assert','Reseal','Seed')),@('test-local-scoring-composition-v4.ps1',@('Corpus','BindPublicFiles','NewResult','OldResult')),@('test-local-model-proof-v4.ps1',@('Journal','Attempts','Stops','ModelSeed')),@('test-local-model-composition-v4.ps1',@('ModelCorpus')),@('test-local-http-proof-v4.ps1',@('SyncNativeProof')),@('authority-stream-scoring-v2d-selftests.ps1',@('SetPublicStream','StreamResult')))){
 $t=$null;$e=$null;$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $spec[0]),[ref]$t,[ref]$e);if($e.Count){throw 'Public test AST invalid'}
 foreach($d in $ast.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.FunctionDefinitionAst] -and $_.Name -in $spec[1]}){. ([scriptblock]::Create($d.Extent.Text))}
}
function HttpCorpus([switch]$Automatic,[switch]$LateUnknown){
 ModelCorpus -Automatic:$Automatic -LateUnknown:$LateUnknown
 $a=Attempts;$a[0].stage='http_round_trip';$a[0].model_version=$null;$a[0].events[1].error_code='transport_connection_error';$a[0].events[1].safe_reason='http_do_failed_without_response';SyncNativeProof $a
 $s=Stops;$s.cause='model_http_no_response';WriteRows 'behavior76_fixture_stops' $s
 $i=ReadJson 'index.json';foreach($entry in $i.episodes[0].steps){$o=ReadJson $entry.outcome_path;$o.state=$o.state.Replace('fixture_local_model_failure','fixture_local_http_failure');WriteJson $entry.outcome_path $o};BindPublicFiles
}
function Case([string]$name,[scriptblock]$body){& $body;$results.Add(@{name=$name;status='pass'});Write-Output ('PASS '+$name)}
Case 'owner_HTTP_FAILED_tail_UNAVAILABLE_96_no_expected_denial' {
 HttpCorpus;$goldens[0].expected.expected_admission_denial=@{'public-turn-2'=$true};BindPublicFiles;$r=NewResult
 $row=@($r.masked_semantic_template|Where-Object {$_.episode_id-ceq'public-episode-1'-and$_.masked_arm_id-ceq'public-arm-a'-and$_.step_id-ceq'public-turn-1'})
 Assert ($r.owner_rows.Count-eq96-and$r.masked_semantic_template.Count-eq96-and$row[0].verdict-ceq'failed'-and$row[0].failure_class-ceq'model_http_no_response'-and$row[0].communication_judgment-ceq'unavailable_no_usable_owner_answer') 'HTTP outcome reclassified as success/language evidence'
 Assert (@($r.masked_semantic_template|Where-Object {$_.disposition-ceq'not_executed_after_fixture_local_http_failure'-and$_.verdict-ceq'unavailable'}).Count-eq1-and$r.availability.snapshot_reads.Count-eq2) 'HTTP tail read snapshots/lost denominator'
 Assert (@($r.mechanical_checks|Where-Object {$_.check-ceq'native-http-no-response:1'-and$_.verdict-ceq'fail'}).Count-eq1-and@($r.mechanical_checks|Where-Object check -CEQ 'expected-admission-denial:public-turn-2').Count-eq0) 'HTTP failure or unexecuted denial became pass'
 Assert ($null-eq$r.native_failure_refs[0].total_tokens-and$null-eq$r.native_failure_refs[0].model_version) 'NULL cost/version backfilled'
}
Case 'automatic_HTTP_does_not_erase_completed_owner_final' {HttpCorpus -Automatic;$r=NewResult;Assert ($r.native_failure_refs[0].owner_run-ceq$false-and@($r.masked_semantic_template|Where-Object {$_.answer-ceq'PUBLIC actual owner final'-and$_.verdict-ceq'unassessed'}).Count-eq1) 'Automatic HTTP failure erased owner final'}
Case 'actual_owner_verified_violation_retained' {HttpCorpus -Automatic;$answer=@{answer='PUBLIC';needs_clarification=$false;next_action=@{kind='answer'};proposed_profile_update=@{kind='none'};completion_claim='owner_verified'};WriteRows 'run_final_results' @(@{run_id='public-run';result_id='public-result';verdict='unverified';content=(ConvertTo-Json $answer -Depth 20 -Compress)});Reseal;$r=NewResult;Assert (@($r.mechanical_checks|Where-Object {$_.check-ceq'not-owner-verified:public-turn-1'-and$_.verdict-ceq'fail'}).Count-eq1) 'Existing violation hidden'}
Case 'critical_late_UNKNOWN_and_uncertain_commit_survive' {HttpCorpus -LateUnknown;WriteRows 'behavior76_object_events' @(@{n=1;kind='commit';action_id='public-effect';before_state=@{version=1;sha256='PUBLIC';task_id='public-task'};after_state=@{body='PUBLIC'}});Reseal;$r=NewResult;Assert ($r.critical_unknown_refs.Count-eq1-and$r.critical_unknown_is_success-ceq$false-and@($r.mechanical_checks|Where-Object {$_.check-ceq'confirmed-commit:1'-and$_.verdict-ceq'fail'}).Count-eq1) 'Critical UNKNOWN waived'}
Case 'ordinary_actual_criteria_equal_frozen_original_outputs' {Corpus -Ordinary;$old=OldResult;$r=NewResult;$a=@($old|Sort-Object {$_['check']});$b=@($r.mechanical_checks|Where-Object {$_.check-notlike'original-owner-opportunity:*'}|Sort-Object {$_['check']});Assert ((Get-Issue76Canonical $a)-ceq(Get-Issue76Canonical $b)) 'Original criteria changed'}
Case 'frozen_27_AST_extents_unchanged' {$k=Get-Issue76FrozenKernel $frozen;Assert ($k.source_sha256-ceq'e481f9c062a3bc4500b9f0b94da8819fbf287bec075490583cad17d54198721d'-and@($k.witnesses|Where-Object kind -CEQ 'statement').Count-eq27) 'Frozen expression identity changed'}
Case 'stream_wrapper_validates_HTTP_failure_without_criteria_change' {HttpCorpus;SetPublicStream;$old=NewResult;$r=StreamResult;Assert ((Get-Issue76Canonical $old.mechanical_checks)-ceq(Get-Issue76Canonical $r.mechanical_checks)-and$r.authority_stream_prevalidation.traces.Count-eq1-and$r.masked_semantic_template.Count-eq96) 'Streaming HTTP composition changed output'}
Case 'stream_wrapper_corrupt_bytes_rejected_before_scoring' {HttpCorpus;SetPublicStream;$p=Join-Path $testRoot ($armDir+'/authority-segment-000001.jsonl.gz');[IO.File]::AppendAllText($p,'tampered',$utf8);$caught=$false;try{$null=StreamResult}catch{$caught=$true};Assert $caught 'Corrupt stream accepted'}
$receipt=@{version='issue76-public-http-composition-v4-selftests';passed=$results.Count;failed=0;native_calls=0;PG_calls=0;hidden_controls_answers_mapping_read=$false;criteria_changed=$false;tests=$results.ToArray();public_test_root=$runRoot}
[IO.File]::WriteAllText((Join-Path $runRoot 'selftest-receipt.json'),(ConvertTo-Json $receipt -Depth 30)+"`n",$utf8)
Write-Output ('SELFTEST_PASS '+$results.Count+' '+$runRoot)
