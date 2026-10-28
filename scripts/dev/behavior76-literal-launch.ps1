[CmdletBinding()]
param(
 [Parameter(Mandatory)][string]$CertificatePath,
 [Parameter(Mandatory)][string]$CertificateSHA256,
 [switch]$EnableNative,
 [switch]$OwnResourceLaneStopped
)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'behavior76-literal-launcher-core.ps1')
$repoRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$schemaPath=Join-Path $PSScriptRoot 'behavior76-literal-admission.schema.json'
if($CertificatePath -cnotmatch '^\.cache/issue76-independent/[A-Za-z0-9_./-]+[.]json$'){throw 'Independent local certificate reference required'}
$certificateFile=Resolve-B76RepoPath $repoRoot $CertificatePath
$certificate=Read-B76Certificate $certificateFile $schemaPath $CertificateSHA256
Push-Location $repoRoot
try{
 $head=(& git rev-parse HEAD).Trim()
 if($LASTEXITCODE -ne 0 -or $head -cne $certificate.source.producer_commit){throw 'Exact certificate producer required'}
 & git diff --quiet HEAD
 if($LASTEXITCODE -ne 0 -or @(& git ls-files --others --exclude-standard).Count -ne 0){throw 'Clean committed producer required'}
 foreach($field in @('semantic_commit','bound_code_commit','preservation_commit')){
  & git merge-base --is-ancestor $certificate.source[$field] $head
  if($LASTEXITCODE -ne 0){throw 'Certificate source ancestry mismatch'}
 }
 $paths=@{}
 foreach($entry in $certificate.artifacts.GetEnumerator()){$paths[$entry.Key]=Assert-B76Pin $repoRoot $entry.Value}
 if($certificate.artifacts.launcher.path -cne 'scripts/dev/behavior76-literal-launch.ps1' -or $certificate.artifacts.host_wrapper.path -cne 'scripts/dev/behavior76-literal-host.sh'){throw 'Fixed literal launcher/wrapper required'}
 if($certificate.artifacts.launcher_core.path -cne 'scripts/dev/behavior76-literal-launcher-core.ps1' -or $certificate.artifacts.admission_schema.path -cne 'scripts/dev/behavior76-literal-admission.schema.json'){throw 'Fixed literal validation core/schema required'}
 if($certificate.artifacts.prepared_unique_elf.path -cnotmatch '^\.cache/behavior76/literal-collector-[A-Za-z0-9_-]+[.]test$'){throw 'Prepared uniquely named literal executable required'}
 if(-not (Get-Item -LiteralPath $paths.prepared_unique_elf).IsReadOnly){throw 'Prepared literal executable must already be read-only'}
 if($certificate.artifacts.stimuli.path -cnotmatch '^\.cache/issue76-independent/hidden-literal-v11[A-Za-z0-9_-]*/stimuli[.]jsonl$' -or $certificate.artifacts.expectations.path -cnotmatch '^\.cache/issue76-independent/hidden-literal-v11[A-Za-z0-9_-]*/expectations[.]jsonl$'){throw 'Separate fresh literal corpus references required'}
 if($certificate.one_use.output_path -cnotmatch '^\.cache/behavior76/native-literal-v11-[A-Za-z0-9_-]+$' -or $certificate.one_use.consumed_marker_path -cnotmatch '^\.cache/issue76-independent/[A-Za-z0-9_./-]+[.]consumed[.]json$'){throw 'Separate new literal output and marker references required'}
 if($certificate.plan.paired_steps -ne 2*$certificate.plan.original_steps){throw 'Full original paired step denominator required'}
 $bindingText=[IO.File]::ReadAllText((Join-Path $repoRoot 'apps/orchestrator/internal/app/behavior76_literal_types_test.go'))
 foreach($binding in @(@('behavior76LiteralStimuliSHA','stimuli'),@('behavior76LiteralExpectationsSHA','expectations'))){
  $match=[regex]::Match($bindingText,('(?m)^const '+$binding[0]+' = "([0-9a-f]{64})"\r?$'))
  if(-not $match.Success -or $match.Groups[1].Value -cne $certificate.artifacts[$binding[1]].sha256){throw 'Actual literal source binding mismatch'}
 }
 $issued=Get-B76Utc $certificate.issued_utc
 $notBefore=Get-B76Utc $certificate.not_before_utc
 $latest=Get-B76Utc $certificate.latest_start_utc
 $now=[DateTimeOffset]::UtcNow
 $day=$now.UtcDateTime.ToString('yyyy-MM-dd',[Globalization.CultureInfo]::InvariantCulture)
 $dayLimit=[DateTimeOffset]::ParseExact(($certificate.owner_utc_day+'T20:00:00Z'),"yyyy-MM-dd'T'HH:mm:ss'Z'",[Globalization.CultureInfo]::InvariantCulture,[Globalization.DateTimeStyles]::AssumeUniversal)
 if($certificate.owner_utc_day -cne $day -or $issued -gt $now -or $notBefore -lt $issued -or $latest -lt $notBefore -or $latest -gt $issued.AddMinutes(30) -or $latest -gt $dayLimit -or $now -lt $notBefore -or $now -gt $latest){throw 'Finite same-day admission time unavailable or uncertain'}
 $output=Resolve-B76RepoPath $repoRoot $certificate.one_use.output_path -Absent
 $marker=Resolve-B76RepoPath $repoRoot $certificate.one_use.consumed_marker_path -Absent
 if(-not $EnableNative){Write-Output 'HOST_VALIDATED_ONLY; no marker, process, auth or model entry';return}
 if(-not $OwnResourceLaneStopped){throw 'Parent-owned overlapping native/heavy work must be stopped before use'}
 $source='git:'+$head+';binary-sha256:'+$certificate.artifacts.prepared_unique_elf.sha256
 $consumed=[ordered]@{version='issue76-literal-v11-consumed-v1';certificate_sha256=$CertificateSHA256;consumed_utc=$now.UtcDateTime.ToString("yyyy-MM-dd'T'HH:mm:ss.fffffff'Z'");nonce=[guid]::NewGuid().ToString('N');source=$source;executable=$certificate.artifacts.prepared_unique_elf.path;output=$certificate.one_use.output_path;entrypoint='TestBehavior76NativeLiteralSupplement';retry_or_resume=$false}
 $bytes=[Text.UTF8Encoding]::new($false).GetBytes(($consumed|ConvertTo-Json -Compress))
 # Consumption is irreversible even if the following checks/process fail.
 Write-B76ConsumedMarker $marker $bytes
 if((Get-FileHash -LiteralPath $certificateFile -Algorithm SHA256).Hash.ToLowerInvariant() -cne $CertificateSHA256){throw 'Consumed certificate changed; do not retry'}
 foreach($name in @('prepared_unique_elf','launcher','launcher_core','admission_schema','host_wrapper','stimuli')){[void](Assert-B76Pin $repoRoot $certificate.artifacts[$name])}
 if([DateTimeOffset]::UtcNow -gt $latest -or (& git rev-parse HEAD).Trim() -cne $head){throw 'Consumed admission expired or producer changed; do not retry'}
 & git diff --quiet HEAD
 if($LASTEXITCODE -ne 0 -or @(& git ls-files --others --exclude-standard).Count -ne 0){throw 'Consumed producer changed; do not retry'}
 # A fresh Windows-written proof links the hardcoded Linux bind to this exact
 # checked repository. The first Linux command is constant inline code, not a
 # script loaded from that still-unverified bind. It verifies mounted bytes
 # before executing the pinned host wrapper, which repeats the ELF check.
 $hostProofRelative='.cache/behavior76/literal-host-proof-'+$consumed.nonce+'.json'
 $hostProofPath=Resolve-B76RepoPath $repoRoot $hostProofRelative -Absent
 Write-B76ConsumedMarker $hostProofPath $bytes
 $hostProofSHA=(Get-FileHash -LiteralPath $hostProofPath -Algorithm SHA256).Hash.ToLowerInvariant()
 $linuxGate=@'
set -eu
root=/mnt/host/wsl/butler-effort76/src20
check_pin(){ actual=$(sha256sum "$root/$1"); test "${actual%% *}" = "$2"; }
check_pin "$1" "$2"
shift 2
wrapper=$1
check_pin "$1" "$2"
shift 2
check_pin "$1" "$2"
shift 2
check_pin "$1" "$2"
shift 2
check_pin "$1" "$2"
shift 2
elf=$1
digest=$2
check_pin "$elf" "$digest"
producer=$3
stimuli=$4
stimuli_digest=$5
output=$6
latest_epoch=$7
not_before_utc=$8
latest_start_utc=$9
check_pin "$stimuli" "$stimuli_digest"
case "$latest_epoch" in ''|*[!0-9]*) exit 2;; esac
test "$(date -u +%s)" -lt "$latest_epoch"
exec /bin/sh "$root/$wrapper" "$elf" "$digest" "$producer" "$stimuli" "$output" "$latest_epoch" "$not_before_utc" "$latest_start_utc"
'@
 & wsl -d docker-desktop --exec /bin/sh -c $linuxGate literal-mounted-entry $hostProofRelative $hostProofSHA $certificate.artifacts.host_wrapper.path $certificate.artifacts.host_wrapper.sha256 $certificate.artifacts.launcher.path $certificate.artifacts.launcher.sha256 $certificate.artifacts.launcher_core.path $certificate.artifacts.launcher_core.sha256 $certificate.artifacts.admission_schema.path $certificate.artifacts.admission_schema.sha256 $certificate.artifacts.prepared_unique_elf.path $certificate.artifacts.prepared_unique_elf.sha256 $source $certificate.artifacts.stimuli.path $certificate.artifacts.stimuli.sha256 $certificate.one_use.output_path $latest.ToUnixTimeSeconds() $certificate.not_before_utc $certificate.latest_start_utc
 if($LASTEXITCODE -ne 0){throw 'One-use literal process failed; preserve marker/output and never retry'}
 Write-Output 'ONE_USE_PROCESS_EXIT_0; collection and quality require independent closed-evidence verification'
}finally{Pop-Location}
