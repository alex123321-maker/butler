$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'local-tail-adapter-v4.ps1')
$utf8=[Text.UTF8Encoding]::new($false);$results=[Collections.Generic.List[object]]::new();$runRoot=Join-Path $PSScriptRoot ('public-response-dimensions-v4-'+[guid]::NewGuid().ToString('N'));$null=New-Item -ItemType Directory $runRoot
foreach($spec in @(@('test-local-tail-adapter-v4.ps1',@('WriteJson','ReadJson','WriteRows','Digest','Assert','Reseal','Seed')),@('test-local-model-proof-v4.ps1',@('Journal','Attempts','Stops','ModelSeed')),@('test-local-http-proof-v4.ps1',@('SyncNativeProof')),@('test-local-response-proof-v4.ps1',@('ResponseSeed')))){
 $t=$null;$e=$null;$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $spec[0]),[ref]$t,[ref]$e);foreach($d in $ast.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.FunctionDefinitionAst] -and $_.Name -in $spec[1]}){. ([scriptblock]::Create($d.Extent.Text))}
}
function ReadV4 {Read-Issue76LocalEvidenceV4 -ReportRoot $testRoot -Plan $plan -Pin $pin}
function Case([string]$name,[scriptblock]$body){& $body;$results.Add(@{name=$name;status='pass'});Write-Output ('PASS '+$name)}
foreach($reason in @('stream_usage_ambiguous','stream_usage_invalid','stream_usage_conflict')){
 Case ('problem_usage_dimension_known_is_GLOBAL_not_erased_'+$reason) {ResponseSeed -Reason $reason -Usage -ObservedVersion;$before=Digest 'behavior76_attempts.jsonl';$caught=$false;try{$null=ReadV4}catch{$caught=$true};Assert ($caught-and(Digest 'behavior76_attempts.jsonl')-ceq$before) 'Inconsistent dimension accepted or erased'}
 Case ('independent_valid_version_with_NULL_problem_usage_retained_'+$reason) {ResponseSeed -Reason $reason -ObservedVersion;$f=(ReadV4).native_failure_refs[0];Assert ($null-eq$f.total_tokens-and$f.model_version-ceq'gemini-public') 'Independent valid version lost'}
}
Case 'problem_model_version_known_is_GLOBAL_not_erased' {ResponseSeed -Reason 'stream_model_version_invalid' -Usage -ObservedVersion;$before=Digest 'behavior76_attempts.jsonl';$caught=$false;try{$null=ReadV4}catch{$caught=$true};Assert ($caught-and(Digest 'behavior76_attempts.jsonl')-ceq$before) 'Conflicting observed version accepted or erased'}
Case 'independent_valid_usage_with_NULL_problem_version_retained' {ResponseSeed -Reason 'stream_model_version_invalid' -Usage;$f=(ReadV4).native_failure_refs[0];Assert ($f.total_tokens-eq7-and$f.thinking_tokens-eq2-and$null-eq$f.model_version) 'Independent valid usage erased'}
Case 'ordinary_rejection_preserves_all_independent_known_metadata' {ResponseSeed -Reason 'stream_part_invalid' -Usage -ObservedVersion;$f=(ReadV4).native_failure_refs[0];Assert ($f.total_tokens-eq7-and$f.model_version-ceq'gemini-public'-and$f.success-ceq$false) 'Valid rejected-result metadata lost'}
$r=@{version='issue76-public-response-metadata-consistency-v4-selftests';passed=$results.Count;failed=0;native_Go_PG_auth_calls=0;hidden_controls_answers_goldens_mapping_read=$false;criteria_changed=$false;tests=$results.ToArray();public_test_root=$runRoot}
[IO.File]::WriteAllText((Join-Path $runRoot 'selftest-receipt.json'),(ConvertTo-Json $r -Depth 30)+"`n",$utf8)
Write-Output ('SELFTEST_PASS '+$results.Count+' '+$runRoot)
