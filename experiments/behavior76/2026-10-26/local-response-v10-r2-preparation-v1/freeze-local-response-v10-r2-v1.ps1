[CmdletBinding()]
param(
 [switch]$IssueFormalFreeze,
 [string]$Producer='',
 [string]$SemanticSource='',
 [string]$PreparationDirectory='',
 [string]$ParentReceiptSHA256='',
 [string]$PublicLogName='',
 [string]$VetLogName=''
)
$ErrorActionPreference='Stop'
# Merely loading/running this prepared script cannot issue a certificate.
# Invoke the explicit switch only after the parent supplies the exact clean
# committed producer and completed final existing gate receipts.
if(-not $IssueFormalFreeze){Write-Output 'PREPARED_ONLY: no formal freeze, authorship, auth, PG or native action';return}
foreach($pin in @($Producer,$SemanticSource)){if($pin -cnotmatch '^[0-9a-f]{40}$'){throw 'Explicit full clean source pins required'}}
if($ParentReceiptSHA256 -cnotmatch '^[0-9a-f]{64}$' -or $PreparationDirectory -cnotmatch '^experiments/behavior76/2026-10-26/[A-Za-z0-9_-]+$' -or $PublicLogName -cnotmatch '^[A-Za-z0-9_.-]+\.log$' -or $VetLogName -cnotmatch '^[A-Za-z0-9_.-]+\.log$'){throw 'Explicit bounded public preparation inputs required'}
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
function Digest([string]$p){(Get-FileHash -LiteralPath $p -Algorithm SHA256).Hash.ToLowerInvariant()}
function JSON([string]$p){ConvertFrom-Json ([IO.File]::ReadAllText($p)) -AsHashtable -Depth 80}
function BlobHash([string]$path,[string]$commit=$Producer){
 $info=[Diagnostics.ProcessStartInfo]::new('git');$info.WorkingDirectory=$repo;$info.UseShellExecute=$false;$info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true
 foreach($a in @('cat-file','blob',($commit+':'+$path))){$info.ArgumentList.Add($a)}
 $p=[Diagnostics.Process]::Start($info)
 try{$hash=[Security.Cryptography.SHA256]::HashData($p.StandardOutput.BaseStream);$errorText=$p.StandardError.ReadToEnd();$p.WaitForExit();if($p.ExitCode){throw ('Git source blob unavailable: '+$path)};return [Convert]::ToHexString($hash).ToLowerInvariant()}finally{$p.Dispose()}
}
if((git -C $repo rev-parse HEAD).Trim() -cne $Producer -or (git -C $repo status --porcelain)){throw 'Exact clean committed semantic producer required'}
$future=Join-Path $PSScriptRoot 'hidden-local-response-v10-r2';if(Test-Path -LiteralPath $future){throw 'Fresh authorship must not precede formal source/scorer freeze'}
$receiptPath=$PreparationDirectory+'/public-preparation-receipt.json';$receiptFile=Join-Path $repo $receiptPath
if((Digest $receiptFile) -cne $ParentReceiptSHA256 -or (BlobHash $receiptPath) -cne $ParentReceiptSHA256){throw 'Parent final public receipt not exact committed bytes'}
$parent=JSON $receiptFile
if($parent.hidden_authorship -cne $false -or $parent.new_native_admission -cne $false -or $parent.native_calls -ne 0 -or $parent.auth_access -ne 0 -or $parent.vet_exit -ne 0 -or $parent.checker_pass -ne 298 -or $parent.checker_fail -ne 0 -or $parent.public_exit -ne 0 -or $parent.public_fail -ne 0 -or $parent.public_skip -ne 0 -or $parent.public_top_level_pass -ne 59 -or $parent.criteria_changed -cne $false -or $parent.thresholds_changed -cne $false){throw 'Final existing gates/unbound/auth-zero status not established'}
foreach($item in @(@('prior_windows',10),@('prior_admissions',723),@('prior_measured',713),@('prior_known_full_tokens',5626388),@('prior_null',10))){if($parent[$item[0]] -ne $item[1]){throw 'Closed history contract mismatch'}}
$dependencyPath=Join-Path $PSScriptRoot 'public-local-response-v10-r2-dependencies-v1.json'
if((Digest $dependencyPath) -cne '878e8cbbb91d3cfba6f50ac79a02406cd56b83dc31fc543a9ecd26c0d773276d'){throw 'Independent dependency commitment changed'}
$dependencies=JSON (Join-Path $PSScriptRoot 'public-local-response-v10-r2-dependencies-v1.json')
$checker=JSON (Join-Path $PSScriptRoot 'public-local-response-checker-v4-receipt-v1.json')
if($checker.passed -ne 298 -or $checker.failed -ne 0 -or $checker.criteria_changed -cne $false -or $checker.thresholds_changed -cne $false -or $checker.frozen_statement_extents -ne 27 -or $checker.runtime_files.Count -ne 7){throw 'Frozen independent checker changed'}
$witnesses=[Collections.Generic.List[object]]::new();$published=@{}
foreach($entry in $parent.preparation_files){
 if($entry.path -cnotmatch '^[A-Za-z0-9_.-]+$' -or $published.ContainsKey($entry.path)){throw 'Invalid duplicate public preparation path'}
 $path=$PreparationDirectory+'/'+$entry.path;$actual=Digest (Join-Path $repo $path)
 if($actual -cne $entry.sha256 -or $actual -cne (BlobHash $path)){throw ('Published evidence differs from committed bytes: '+$path)}
 $published[$entry.path]=$actual;$witnesses.Add(@{path=$path;sha256=$actual;git_blob_bytes_exact=$true})
}
foreach($entry in $dependencies.required_publication_files){
 if((Digest (Join-Path $PSScriptRoot $entry.path)) -cne $entry.sha256 -or -not $published.ContainsKey($entry.path) -or $published[$entry.path] -cne $entry.sha256){throw ('Checker/publication dependency differs: '+$entry.path)}
}
foreach($entry in $dependencies.external_committed_selftest_witness_files){if((Digest (Join-Path $PSScriptRoot $entry.path)) -cne $entry.sha256 -or (Digest (Join-Path $repo $entry.published_path)) -cne $entry.sha256 -or (BlobHash $entry.published_path) -cne $entry.sha256){throw 'Committed older selftest witness differs'}}
$review=JSON (Join-Path $PSScriptRoot 'public-local-response-v10-r2-source-review-v1.json')
if($review.decision -cne 'NO_CONCRETE_STATIC_BLOCKER_FOUND_PENDING_PARENT_ORDINARY_GATES'){throw 'Independent source blocker unresolved'}
foreach($entry in $review.source_files){if((Digest (Join-Path $repo $entry.path)) -cne $entry.sha256){throw ('Source changed after latest independent review: '+$entry.path)}}
$sourcePaths=@(git -C $repo ls-files 'apps/orchestrator/internal/app/behavior76*.go')+@('apps/orchestrator/internal/app/behavior76_claim_window_test.go','apps/orchestrator/internal/app/behavior76_idle_claim_diagnostic_test.go','apps/orchestrator/internal/orchestrator/service_state.go','apps/orchestrator/internal/orchestrator/service_execute.go','apps/orchestrator/internal/orchestrator/cancellation_cause.go','apps/orchestrator/internal/orchestrator/terminal_write.go','apps/orchestrator/internal/orchestrator/terminal_write_test.go','apps/orchestrator/internal/orchestrator/budget.go','apps/orchestrator/internal/orchestrator/communication_failure.go','apps/orchestrator/internal/orchestrator/recovery.go','apps/orchestrator/internal/orchestrator/terminal_watch_test.go','apps/orchestrator/internal/orchestrator/working_memory.go','apps/orchestrator/internal/task/authority.go','apps/orchestrator/internal/task/store.go','apps/orchestrator/internal/prompt/task_continuity.go','apps/orchestrator/README.md','internal/transport/antigravity/provider.go','internal/transport/antigravity/protocol.go','internal/transport/antigravity/stream_diagnostics.go','internal/transport/antigravity/http_failure_diagnostic_test.go','internal/transport/antigravity/README.md','internal/transport/errors.go','docs/testing/issue76-local-response-protocol.md','docs/testing/issue76-local-http-protocol.md','docs/testing/issue76-local-model-protocol.md','docs/testing/issue76-day-window-admission.md','docs/testing/issue76-fixture-stream-protocol.md','docs/architecture/model-transport-contract.md','docs/architecture/run-lifecycle-spec.md','scripts/dev/behavior-local-run.sh','scripts/dev/behavior-local-host.sh','scripts/dev/behavior-local-acceptance.ps1','scripts/dev/behavior76-pgx-build.go','go.mod','go.sum')
$sourcePins=@();foreach($path in @($sourcePaths|Sort-Object -Unique)){$hash=Digest (Join-Path $repo $path);if($hash -cne (BlobHash $path) -or $hash -cne (BlobHash $path $SemanticSource)){throw ('Source/semantic/producer blob mismatch: '+$path)};$sourcePins+=@{path=$path;sha256=$hash;git_blob_bytes_exact=$true;semantic_source_bytes_exact=$true}}
$publicPath=$PreparationDirectory+'/'+$PublicLogName;$vetPath=$PreparationDirectory+'/'+$VetLogName
if(-not $published.ContainsKey($PublicLogName) -or -not $published.ContainsKey($VetLogName)){throw 'Final gate logs not in committed preparation inventory'}
$public=[IO.File]::ReadAllText((Join-Path $repo $publicPath));$pass=[regex]::Matches($public,'(?m)^--- PASS: Test').Count
if($pass -ne $parent.public_top_level_pass -or $public -match '(?m)^--- (FAIL|SKIP): Test|WARNING: DATA RACE' -or $public -notmatch '(?m)^PASS\r?$'){throw 'Completed final Public PASS/no skips/no race not established'}
foreach($test in $checker.public_tests){$log=Join-Path $repo ($PreparationDirectory+'/'+$test.log);$text=[IO.File]::ReadAllText($log);if($test.script -like 'authority-stream-*'){$r=ConvertFrom-Json $text -AsHashtable;if($r.passed -ne $test.passed -or $r.failed -ne 0){throw 'Stream selftest result mismatch'}}elseif($text -notmatch ('(?m)^SELFTEST_PASS '+$test.passed+'(?: |\r?$)')){throw 'Independent selftest PASS count absent'}}
$elf=Digest (Join-Path $repo '.cache/behavior76/collector.test');if($elf -cne $parent.collector_sha256){throw 'Actual tested unbound collector ELF mismatch'}
$candidate=Digest (Join-Path $repo 'apps/orchestrator/internal/prompt/task_continuity.go');if($candidate -cne 'f9a880ca92a51fb16c68cd859be7830bbc696a69496853537f64c49bb61acaeb'){throw 'Frozen candidate changed'}
. (Join-Path $PSScriptRoot 'local-scoring-composition-v4.ps1')
$kernel=Get-Issue76FrozenKernel (Join-Path $PSScriptRoot 'score-mechanical-fresh-v3.ps1')
if(@($kernel.witnesses|Where-Object kind -CEQ 'statement').Count -ne 27){throw 'Frozen criterion AST identity changed'}
$closedDir='experiments/behavior76/2026-10-26/native-local-model-v8'
foreach($closed in @(@('public-closed-local-model-v8-boundary-v1.json','03ed283c6c5af8756145c5e073fc3cc15e59828a78630c518c3d842be209a076'),@('raw-inventory.json','c85f02ef8e3555f1043c4698f48f1bfa0e3af6670fb269fd7a20c3d83c4f2d00'))){$p=$closedDir+'/'+$closed[0];if((Digest (Join-Path $repo $p)) -cne $closed[1] -or (BlobHash $p) -cne $closed[1]){throw 'Immutable closed-v8 boundary/inventory differs'}}
$closedV9='experiments/behavior76/2026-10-26/native-local-http-v9'
foreach($closed in @(@('public-closed-local-http-v9-boundary-v1.json','2fdefd63659e5ec8ce9d4f768d0bb2a45bfd2857c21e91be6beea51058bf9422'),@('raw-inventory.json','045adc74308ea3faf3024e2b8070cd97aa9ea1470e0bce025354d88b72dfb879'))){$p=$closedV9+'/'+$closed[0];if((Digest (Join-Path $repo $p)) -cne $closed[1] -or (BlobHash $p) -cne $closed[1]){throw 'Immutable closed-v9 boundary/inventory differs'}}
# The old failed pre-native corpus is immutable and cannot be silently reused.
$daySource=[IO.File]::ReadAllText((Join-Path $repo 'apps/orchestrator/internal/app/behavior76_day_admission_test.go'))
foreach($literal in @('const behavior76DayStimuliSHA = "UNBOUND_FRESH_V10_R2_STIMULI"','const behavior76DayExpectationsSHA = "UNBOUND_FRESH_V10_R2_EXPECTATIONS"')){if([regex]::Matches($daySource,[regex]::Escape($literal)).Count-ne1){throw 'Fresh r2 unbound literal required before freeze'}}
$wrapper=[IO.File]::ReadAllText((Join-Path $repo 'scripts/dev/behavior-local-run.sh'))
if([regex]::Matches($wrapper,'hidden-local-response-v10-r2/stimuli\.jsonl').Count-ne3-or$wrapper-match'(?<![a-zA-Z0-9-])hidden-local-response-v10/stimuli\.jsonl'){throw 'Fresh fixed r2 wrapper paths required'}
$failedPackage='experiments/behavior76/2026-10-26/local-response-v10-pre-native-failure-v1/package-receipt-v1.json'
if((Digest (Join-Path $repo $failedPackage))-cne'579a3b8599ed299036d6e920c1b2ad46329c4143a2a93c214dc0ec327a25ba6e'-or(BlobHash $failedPackage)-cne'579a3b8599ed299036d6e920c1b2ad46329c4143a2a93c214dc0ec327a25ba6e'){throw 'Closed failed pre-native package changed'}
$doc='docs/testing/issue76-local-response-protocol.md'
$certificate=[ordered]@{
 version='issue76-independent-formal-semantic-source-scorer-freeze-local-response-v10-r2-v1';decision='FORMAL_SOURCE_AND_SCORER_FREEZE_BEFORE_NEW_AUTHORSHIP_NOT_NATIVE_ADMISSION';issued_at_utc=[datetime]::UtcNow.ToString('O');producer=$Producer;semantic_source=$SemanticSource;worktree_clean=$true
 public_evidence=@{receipt_path=$receiptPath;receipt_sha256=$ParentReceiptSHA256;final_public_race_top_level_PASS=$pass;FAIL=0;SKIP=0;vet_exit_recorded=0;public_log_path=$publicPath;public_log_sha256=$published[$PublicLogName];vet_log_path=$vetPath;vet_log_sha256=$published[$VetLogName];independent_offline_PASS=298;tested_unbound_ELF_sha256=$elf}
 source_pins=$sourcePins;published_evidence_git_blob_witnesses=$witnesses.ToArray();runtime_checker_files=$checker.runtime_files;selftest_dependency_files=$dependencies.selftest_dependency_files;checker_receipt_sha256=(Digest (Join-Path $PSScriptRoot 'public-local-response-checker-v4-receipt-v1.json'));checker_dependencies_sha256=(Digest (Join-Path $PSScriptRoot 'public-local-response-v10-r2-dependencies-v1.json'));independent_source_review_sha256=(Digest (Join-Path $PSScriptRoot 'public-local-response-v10-r2-source-review-v1.json'));independent_actual_PG_terminal_regression=@{PASS=1;subcases_PASS=2;receipt_sha256='3fba854649a67e0074495f9efee8a0a537c2ae2b66483d654c8c31161600f043';legacy_causal_receipt_sha256='eb5d7d4d593a1d9718ffa57199efffcf65d64de00a5932f6f165c1ba3796b6e2';historical_native_outcomes_reclassified=$false};criterion_source_sha256=$kernel.source_sha256;criterion_extents=$kernel.witnesses;criteria_changed=$false;thresholds_changed=$false;protocol_path=$doc;protocol_sha256=(Digest (Join-Path $repo $doc));candidate_sha256=$candidate
 history=@{closed_windows=10;admissions=723;measured=713;known_provider_total_tokens=5626388;null_usage=10;closed_v8_certificate_sha256='03ed283c6c5af8756145c5e073fc3cc15e59828a78630c518c3d842be209a076';closed_v8_raw_files=1253;closed_v8_raw_inventory_sha256='c85f02ef8e3555f1043c4698f48f1bfa0e3af6670fb269fd7a20c3d83c4f2d00';closed_v9_certificate_sha256='2fdefd63659e5ec8ce9d4f768d0bb2a45bfd2857c21e91be6beea51058bf9422';closed_v9_raw_files=383;closed_v9_raw_inventory_sha256='045adc74308ea3faf3024e2b8070cd97aa9ea1470e0bce025354d88b72dfb879';old_failures_and_unknowns_retained=$true;prior_authority_latency_failure_remains_unresolved=$true;no_new_live_database_audit_by_checker=$true}
 bounds='4h owning-UTC-day-fit;768entries;8/owner-slot;4/run;120smodel;20mfixture;750msauthority;9+1';stop_semantics='Old exact completed empty-answer first, typed confirmed HTTP-no-response, or exact13 response-format/finish safe-reason failures may stop original whole isolated arm; classified effect UNKNOWN unchanged/critical; known usage/version retained; all other faults GLOBAL'
 authorship=@{directory_absent_at_this_freeze=$true;path='hidden-local-response-v10-r2';status='NOT_STARTED';next_step='Only after this formal receipt independently author substantively fresh24 synthetic episodes/12families/2owner opportunities; then literal bindings and existing setup/composition';old_controls_reused=$false};operations_during_this_freeze=@{Go=0;PG=0;auth=0;native_model=0};independent_prior_public_PG_diagnostic_and_regression=$true;quality_or_promotion_or_closure=$false;new_native_admission=$false
}
$path=Join-Path $PSScriptRoot 'semantic-freeze-local-response-v10-r2-v1.json';$f=[IO.File]::Open($path,[IO.FileMode]::CreateNew);try{$b=[Text.UTF8Encoding]::new($false).GetBytes((ConvertTo-Json $certificate -Depth 60)+"`n");$f.Write($b)}finally{$f.Dispose()};(Get-Item $path).IsReadOnly=$true
@{path=$path;sha256=(Digest $path);source_files=$sourcePins.Count;published_files=$witnesses.Count;public_PASS=$pass;checker_PASS=298;authorship='NOT_STARTED';native_admission=$false}|ConvertTo-Json
