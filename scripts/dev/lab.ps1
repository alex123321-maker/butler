[CmdletBinding()]
param(
  [ValidateSet('Sync', 'Up', 'Test', 'Integration', 'Race', 'Web', 'Status')][string]$Action = 'Status',
  [string]$HostAlias = 'laba',
  [string]$RemoteRoot = '/srv/storage/butler',
  [string[]]$Packages = @('./apps/orchestrator/...')
)

. (Join-Path $PSScriptRoot 'common.ps1')
$state = Initialize-ButlerAgentEnvironment
Assert-CommandAvailable -Name ssh
Assert-CommandAvailable -Name scp
if ($HostAlias -notmatch '^[a-zA-Z0-9][a-zA-Z0-9_.-]*$' -or
    $RemoteRoot -notmatch '^/(srv|opt|home)/[a-zA-Z0-9_./-]+$' -or
    $RemoteRoot.Contains('..')) {
  throw 'HostAlias and RemoteRoot must be simple SSH alias and absolute workspace path.'
}
foreach ($package in $Packages) {
  if ($package -notmatch '^\./[a-zA-Z0-9_./-]+$') { throw "Invalid Go package: $package" }
}

if ($Action -eq 'Sync') {
  Assert-CommandAvailable -Name tar
  $snapshot = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
  $archive = Join-Path $state.CacheRoot 'lab-source.tar.gz'
  $fileList = Join-Path $state.CacheRoot 'lab-source-files.txt'
  Push-Location $state.RepoRoot
  try {
    $files = @(git ls-files --cached --others --exclude-standard | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf })
    Assert-LastExitCode -Action 'git ls-files'
    [IO.File]::WriteAllText($fileList, (($files -join "`n") + "`n"), [Text.UTF8Encoding]::new($false))
    & tar -czf $archive -T $fileList
    Assert-LastExitCode -Action 'source archive'
  } finally { Pop-Location }
  & scp $archive "${HostAlias}:${RemoteRoot}/source.tar.gz"
  Assert-LastExitCode -Action 'source upload'
  & ssh -o BatchMode=yes $HostAlias "set -eu; mkdir -p '$RemoteRoot/snapshots/$snapshot'; tar -xzf '$RemoteRoot/source.tar.gz' -C '$RemoteRoot/snapshots/$snapshot'; ln -sfn '$RemoteRoot/snapshots/$snapshot' '$RemoteRoot/current'; printf '%s\n' '$snapshot'"
  Assert-LastExitCode -Action 'source snapshot'
  exit 0
}

$packageArgs = $Packages -join ' '
& ssh -o BatchMode=yes $HostAlias "bash '$RemoteRoot/current/scripts/dev/lab.sh' '$Action' '$RemoteRoot' $packageArgs"
Assert-LastExitCode -Action "lab $Action"
