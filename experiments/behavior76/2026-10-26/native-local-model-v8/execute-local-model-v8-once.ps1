param([Parameter(Mandatory)][ValidatePattern('^[a-f0-9]{64}$')][string]$CertificateSha256)
$ErrorActionPreference='Stop'
$repo='D:\CodexData\home\worktrees\issue20-local\butler'
Set-Location $repo
$certificate=Join-Path $repo '.cache/issue76-independent/final-native-admission-local-model-v8-v1.json'
if((Get-FileHash -LiteralPath $certificate).Hash.ToLowerInvariant() -cne $CertificateSha256){throw 'Exact independent certificate required'}
$cert=Get-Content -LiteralPath $certificate -Raw|ConvertFrom-Json -Depth 50
if(-not ([string]$cert.decision).StartsWith('ADMIT_EXACTLY_ONE_')){throw 'Certificate does not admit one collection'}
$now=[DateTimeOffset]::UtcNow
$issued=[DateTimeOffset]$cert.issued_utc
$latest=[DateTimeOffset]$cert.latest_start_utc
if($now -lt $issued -or $now -gt $latest){throw 'Independent admission is not current'}
$head=(& git rev-parse HEAD).Trim()
if($LASTEXITCODE -ne 0 -or $head -cne $cert.producer){throw 'Exact admitted producer required'}
& git diff --quiet HEAD
if($LASTEXITCODE -ne 0 -or @(git ls-files --others --exclude-standard).Count){throw 'Clean admitted worktree required'}
$binary=Join-Path $repo '.cache/behavior76/collector.test'
$elf=(Get-FileHash -LiteralPath $binary).Hash.ToLowerInvariant()
if($elf -cne $cert.collector_elf_sha256){throw 'Exact admitted ELF required'}
$output='.cache/behavior76/native-local-model-v8'
if($cert.output -cne $output -or (Test-Path -LiteralPath (Join-Path $repo $output))){throw 'New exact output identity required'}
$nonce=[IO.File]::ReadAllText((Join-Path $repo '.cache/behavior76/host-marker')).Trim()
if($nonce -notmatch '^[a-f0-9]{32}$'){throw 'Existing host/source marker unavailable'}
(Get-Item -LiteralPath $binary).IsReadOnly=$true
$marker=Join-Path $repo '.cache/issue76-independent/local-model-v8-admission-consumed-v1.json'
$body=[ordered]@{version='issue76-local-model-v8-one-use-launch-v1';certificate_sha256=$CertificateSha256;producer=$head;elf_sha256=$elf;output=$output;admission_consumed_utc=$now;new_attempts_allowed=1;rebuild_after_admission=$false;old_windows_reopened=$false}
$bytes=[Text.UTF8Encoding]::new($false).GetBytes(($body|ConvertTo-Json -Depth 10)+"`n")
$file=[IO.File]::Open($marker,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)
try{$file.Write($bytes);$file.Flush($true)}finally{$file.Dispose()}
$source="git:$head;binary-sha256:$elf"
& wsl -d docker-desktop --exec /bin/sh /mnt/host/wsl/butler-effort76/src20/scripts/dev/behavior-local-host.sh daynative $nonce $source $output
if($LASTEXITCODE -ne 0){throw 'One-use native collection failed; preserve original window, nullable spend and evidence. No retry.'}
