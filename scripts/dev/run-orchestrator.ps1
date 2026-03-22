[CmdletBinding()]
param()

. (Join-Path $PSScriptRoot "common.ps1")

$state = Initialize-ButlerAgentEnvironment
Import-ButlerDotEnv
Assert-CommandAvailable -Name go

if (-not $env:BUTLER_POSTGRES_URL) {
  $env:BUTLER_POSTGRES_URL = "postgres://butler:butler-dev-password@localhost:5432/butler?sslmode=disable"
}
if (-not $env:BUTLER_REDIS_URL) {
  $env:BUTLER_REDIS_URL = "redis://localhost:6379/0"
}
if (-not $env:BUTLER_TOOL_BROKER_ADDR) {
  $env:BUTLER_TOOL_BROKER_ADDR = "127.0.0.1:10090"
}

Push-Location $state.RepoRoot
try {
  go run ./apps/orchestrator
  Assert-LastExitCode -Action "go run ./apps/orchestrator"
}
finally {
  Pop-Location
}
