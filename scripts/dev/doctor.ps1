[CmdletBinding()]
param()

. (Join-Path $PSScriptRoot "common.ps1")

$state = Initialize-ButlerAgentEnvironment
$checks = @(
  @{ Name = "go"; Required = $true },
  @{ Name = "npm"; Required = $true },
  @{ Name = "docker"; Required = $true },
  @{ Name = "git"; Required = $false }
)

$failed = $false
foreach ($check in $checks) {
  $command = Get-Command $check.Name -ErrorAction SilentlyContinue
  if ($null -eq $command) {
    $color = if ($check.Required) { "Red" } else { "Yellow" }
    Write-Host ("missing: {0}" -f $check.Name) -ForegroundColor $color
    if ($check.Required) {
      $failed = $true
    }
    continue
  }

  Write-Host ("ok: {0} -> {1}" -f $check.Name, $command.Source) -ForegroundColor Green
}

if (-not $failed) {
  & docker info --format "{{.ServerVersion}}" *> $null
  if ($LASTEXITCODE -ne 0) {
    Write-Host "docker: daemon is not reachable" -ForegroundColor Red
    $failed = $true
  }
  else {
    Write-Host "docker: daemon is reachable" -ForegroundColor Green
  }
}

$envFiles = @(
  (Join-Path $state.RepoRoot ".env"),
  (Join-Path $state.RepoRoot ".env.codex-windows.example"),
  (Join-Path $state.RepoRoot ".env.example")
)

$foundEnv = $false
foreach ($envFile in $envFiles) {
  if (Test-Path $envFile) {
    Write-Host ("env: found {0}" -f $envFile) -ForegroundColor Green
    $foundEnv = $true
    break
  }
}

if (-not $foundEnv) {
  Write-Host "env: no .env template found" -ForegroundColor Red
  $failed = $true
}

Write-Host ("cache: {0}" -f $state.CacheRoot) -ForegroundColor Green
Write-Host ("GOCACHE: {0}" -f $state.GOCACHE)
Write-Host ("GOMODCACHE: {0}" -f $state.GOMODCACHE)
Write-Host ("NPM cache: {0}" -f $state.NPM_CACHE)
Write-Host ("Playwright browsers: {0}" -f $state.PLAYWRIGHT_BROWSERS_PATH)
Write-Host ("Docker config: {0}" -f $state.DOCKER_CONFIG)

if (-not $failed) {
  Write-Host "Codex Desktop host workflow looks ready." -ForegroundColor Green
  exit 0
}

Write-Host "Codex Desktop host workflow is missing required tools." -ForegroundColor Red
exit 1
