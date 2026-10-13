[CmdletBinding()]
param(
  [ValidateSet('Core', 'Full', 'FullFaults')][string]$Profile = 'Core',
  [string]$HostAlias = 'laba',
  [string]$RemoteRoot = '/srv/storage/butler/agents/durable-tasks',
  [switch]$NoSync,
  [string]$LocalModelLoadOOM = ''
)

# This script never changes the shared lab pointer or its Compose project.
if ($HostAlias -notmatch '^[a-zA-Z0-9][a-zA-Z0-9_.-]*$' -or
    $RemoteRoot -notmatch '^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$') {
  throw 'Use a simple SSH alias and an isolated /srv/storage/butler/agents/<name> root.'
}
if (-not $NoSync) {
  & pwsh -NoProfile -File (Join-Path $PSScriptRoot 'lab.ps1') -Action Sync -HostAlias $HostAlias -RemoteRoot $RemoteRoot
  if ($LASTEXITCODE -ne 0) { throw 'Lab source sync failed.' }
}
$commit = (git -C (Join-Path $PSScriptRoot '../..') rev-parse HEAD).Trim()
if ($commit -notmatch '^[a-f0-9]{40}$') { throw 'Exact source commit required.' }
$labProfile = $Profile.ToLowerInvariant()
if ($Profile -eq 'FullFaults') {
  if ($LocalModelLoadOOM -and ($LocalModelLoadOOM -notmatch '^/srv/storage/butler/agents/[a-zA-Z0-9_/-]+/local-load-oom.json$' -or $LocalModelLoadOOM.Contains('..'))) {
    throw 'Invalid separate sealed local model-load proof for FullFaults.'
  }
  $labProfile = 'full-faults'
}
& ssh -o BatchMode=yes $HostAlias "bash '$RemoteRoot/current/scripts/dev/fault-lab.sh' '$RemoteRoot' '$labProfile' '$commit' '$LocalModelLoadOOM'"
if ($LASTEXITCODE -ne 0) { throw 'Required acceptance evidence failed; see the private report path printed by the lab.' }
