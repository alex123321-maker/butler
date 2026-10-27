# Public synthetic offline PS tests only. Three independent workers maximum;
# no Go, WSL, database, auth, provider, old corpus, response or mapping access.
$ErrorActionPreference='Stop';$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$runtime=@('local-model-proof-reader-v4.ps1','local-tail-adapter-v4.ps1','local-scoring-composition-v4.ps1','authority-stream-scoring-v2d.ps1','authority-stream-reader-v2a.cs','authority-stream-reader-v2a.ps1','score-mechanical-fresh-v3.ps1')
function Hash($p){(Get-FileHash -LiteralPath $p -Algorithm SHA256).Hash.ToLowerInvariant()}
$pins=@($runtime|ForEach-Object {@{path=$_;sha256=(Hash (Join-Path $PSScriptRoot $_))}})
$cases=@(
 @('test-local-tail-adapter-v4.ps1','local-tail-adapter-v4-final-v3.log',32),
 @('test-local-model-proof-v4.ps1','local-model-proof-v4-final-v3.log',47),
 @('test-local-scoring-composition-v4.ps1','local-scoring-composition-v4-final-v3.log',12),
 @('test-local-model-proof-v4-union.ps1','local-model-proof-v4-union-final-v3.log',2),
 @('test-local-model-composition-v4.ps1','local-model-composition-v4-final-v3.log',7),
 @('authority-stream-scoring-v2d-selftests.ps1','authority-stream-scoring-v2d-final-v3.log',6),
 @('test-local-http-proof-v4.ps1','local-http-proof-v4-final-v3.log',52),
 @('test-local-http-composition-v4.ps1','local-http-composition-v4-final-v3.log',8),
 @('test-local-response-proof-v4.ps1','local-response-proof-v4-final-v2.log',113),
 @('test-local-response-composition-v4.ps1','local-response-composition-v4-final-v2.log',10)
)
$waiting=[Collections.Generic.Queue[object]]::new();foreach($c in $cases){$waiting.Enqueue($c)}
$active=[Collections.Generic.List[object]]::new();$results=[Collections.Generic.List[object]]::new()
while($waiting.Count-or$active.Count){
 while($waiting.Count-and$active.Count-lt3){$c=$waiting.Dequeue();$log=Join-Path $PSScriptRoot $c[1];if(Test-Path $log){throw 'Final log must be new'};$info=[Diagnostics.ProcessStartInfo]::new((Get-Process -Id $PID).Path);$info.WorkingDirectory=$repo;$info.UseShellExecute=$false;$info.CreateNoWindow=$true;$info.RedirectStandardOutput=$true;$info.RedirectStandardError=$true;foreach($a in @('-NoProfile','-File',(Join-Path $PSScriptRoot $c[0]))){$info.ArgumentList.Add($a)};$p=[Diagnostics.Process]::Start($info);$active.Add(@{process=$p;stdout=$p.StandardOutput.ReadToEndAsync();stderr=$p.StandardError.ReadToEndAsync();case=$c;started=[datetimeoffset]::UtcNow})}
 foreach($job in @($active.ToArray())){if(-not$job.process.HasExited){continue};$p=$job.process;$p.WaitForExit();$out=$job.stdout.GetAwaiter().GetResult();$err=$job.stderr.GetAwaiter().GetResult();$text=$out;if($err){$text+="`n"+$err};$path=Join-Path $PSScriptRoot $job.case[1];$f=[IO.File]::Open($path,[IO.FileMode]::CreateNew);try{$b=[Text.UTF8Encoding]::new($false).GetBytes($text);$f.Write($b)}finally{$f.Dispose()};(Get-Item $path).IsReadOnly=$true;$ok=$p.ExitCode-eq0;if($ok){if($job.case[0]-like'authority-stream-*'){try{$r=ConvertFrom-Json $out -AsHashtable;$ok=$r.passed-eq$job.case[2]-and$r.failed-eq0}catch{$ok=$false}}else{$ok=$out-match('(?m)^SELFTEST_PASS '+$job.case[2]+'(?: |\r?$)')}};$results.Add(@{script=$job.case[0];script_sha256=(Hash (Join-Path $PSScriptRoot $job.case[0]));log=$job.case[1];log_sha256=(Hash $path);expected_cases=$job.case[2];passed=$(if($ok){$job.case[2]}else{0});status=$(if($ok){'pass'}else{'FAIL'});exit_code=$p.ExitCode;elapsed_seconds=([datetimeoffset]::UtcNow-$job.started).TotalSeconds});Write-Output ($results[-1]|ConvertTo-Json -Compress);$p.Dispose();$null=$active.Remove($job)}
 if($active.Count){Start-Sleep -Milliseconds 100}
}
foreach($pin in $pins){if((Hash (Join-Path $PSScriptRoot $pin.path))-cne$pin.sha256){throw 'Runtime changed during final public tests'}}
$failed=@($results|Where-Object status -CNE 'pass').Count
$receipt=@{version='issue76-final-public-response-checker-v4-run-v1';runtime_files=$pins;runtime_unchanged_during_all_tests=$true;public_tests=$results.ToArray();passed=($results|Measure-Object passed -Sum).Sum;failed_groups=$failed;maximum_parallel_test_processes=3;Go_PG_auth_native_calls=0;hidden_controls_answers_goldens_mapping_read=$false;source_freeze_or_new_authorship_or_native_admission=$false}
$path=Join-Path $PSScriptRoot 'public-local-response-checker-v4-final-test-run-v1.json';$f=[IO.File]::Open($path,[IO.FileMode]::CreateNew);try{$b=[Text.UTF8Encoding]::new($false).GetBytes((ConvertTo-Json $receipt -Depth 40)+"`n");$f.Write($b)}finally{$f.Dispose()};(Get-Item $path).IsReadOnly=$true
@{receipt=$path;sha256=(Hash $path);passed=$receipt.passed;failed_groups=$failed}|ConvertTo-Json
if($failed){exit 1}
