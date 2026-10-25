[CmdletBinding()]
param(
 [ValidateSet('Offline','Preflight','Composition','Boundary','Native')][string]$Action='Offline',
 [switch]$EnableNative,
 [string]$OutputDirectory=''
)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$state=Initialize-ButlerAgentEnvironment
if($Action -eq 'Native' -and -not $EnableNative){throw 'EnableNative required for actual provider inference'}
Push-Location $state.RepoRoot
try {
 $before=(& git rev-parse HEAD).Trim()
 if($LASTEXITCODE -ne 0){throw 'Source commit unavailable'}
 $nonce=[guid]::NewGuid().ToString('N')
 $cache=Join-Path $state.RepoRoot '.cache/behavior76'
 [IO.Directory]::CreateDirectory($cache)|Out-Null
 [IO.File]::WriteAllText((Join-Path $cache 'host-marker'),$nonce,[Text.UTF8Encoding]::new($false))
 $hostScript='/mnt/host/wsl/butler-effort76/src20/scripts/dev/behavior-local-host.sh'
 & wsl -d docker-desktop --exec /bin/sh $hostScript build $nonce
 if($LASTEXITCODE -ne 0){throw 'Local behavioral collector build failed'}
 $digest=(Get-FileHash (Join-Path $cache 'collector.test') -Algorithm SHA256).Hash.ToLowerInvariant()
 if($Action -eq 'Native'){
  & git diff --quiet HEAD
  if($LASTEXITCODE -ne 0 -or @(git ls-files --others --exclude-standard).Count -or (& git rev-parse HEAD).Trim() -ne $before){throw 'Committed clean prospective source required'}
  if($OutputDirectory -notmatch '^\.cache/behavior76/[A-Za-z0-9_-]+$'){throw 'New output directory required under .cache/behavior76'}
 }
 $source="git:$before;binary-sha256:$digest"
 & wsl -d docker-desktop --exec /bin/sh $hostScript $Action.ToLowerInvariant() $nonce $source $OutputDirectory
 if($LASTEXITCODE -ne 0){throw 'Local behavioral acceptance failed; preserve original logs/window'}
}finally{Pop-Location}
