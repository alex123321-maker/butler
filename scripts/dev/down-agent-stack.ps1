[CmdletBinding()]
param(
  [switch]$RemoveVolumes
)

. (Join-Path $PSScriptRoot "common.ps1")

$state = Initialize-ButlerAgentEnvironment
Assert-CommandAvailable -Name docker

Push-Location $state.RepoRoot
try {
  if ($RemoveVolumes) {
    Invoke-ButlerCompose -Arguments @("down", "-v") -RepoRoot $state.RepoRoot
  }
  else {
    Invoke-ButlerCompose -Arguments (@("stop") + (Get-ButlerAgentComposeServices)) -RepoRoot $state.RepoRoot
  }
}
finally {
  Pop-Location
}

Write-Host "Agent-side Docker services were stopped." -ForegroundColor Green
