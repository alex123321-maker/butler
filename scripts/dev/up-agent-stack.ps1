[CmdletBinding()]
param(
  [switch]$Build
)

. (Join-Path $PSScriptRoot "common.ps1")

$state = Initialize-ButlerAgentEnvironment
Import-ButlerDotEnv
Assert-CommandAvailable -Name docker

$args = @("up", "-d")
if ($Build) {
  $args += "--build"
}
$args += Get-ButlerAgentComposeServices

Push-Location $state.RepoRoot
try {
  Invoke-ButlerCompose -Arguments $args -RepoRoot $state.RepoRoot
}
finally {
  Pop-Location
}

Write-Host "Agent-side Docker services are running." -ForegroundColor Green
Write-Host "Next step: pwsh -NoProfile -File scripts/dev/run-orchestrator.ps1"
