[CmdletBinding()]
param(
  [switch]$InstallWeb,
  [switch]$InstallToolBrowser,
  [switch]$InstallAllNodeDeps,
  [switch]$DownloadGoModules,
  [switch]$InstallAllDependencies,
  [switch]$ShowEnv
)

. (Join-Path $PSScriptRoot "common.ps1")

$state = Initialize-ButlerAgentEnvironment
Assert-CommandAvailable -Name go

if ($InstallAllDependencies) {
  $InstallAllNodeDeps = $true
  $DownloadGoModules = $true
}

if ($InstallAllNodeDeps) {
  $InstallWeb = $true
  $InstallToolBrowser = $true
}

if ($InstallWeb -or $InstallToolBrowser) {
  Assert-CommandAvailable -Name npm
}

if (-not (Test-Path (Join-Path $state.RepoRoot ".env"))) {
  Write-Host "No .env file found. Start from .env.codex-windows.example or .env.example." -ForegroundColor Yellow
}

if ($DownloadGoModules) {
  Push-Location $state.RepoRoot
  try {
    go mod download
    Assert-LastExitCode -Action "go mod download"
  }
  finally {
    Pop-Location
  }
}

if ($InstallWeb) {
  Push-Location (Join-Path $state.RepoRoot "web")
  try {
    npm ci
    Assert-LastExitCode -Action "npm ci (web)"
  }
  finally {
    Pop-Location
  }
}

if ($InstallToolBrowser) {
  Push-Location (Join-Path $state.RepoRoot "apps/tool-browser")
  try {
    npm ci
    Assert-LastExitCode -Action "npm ci (apps/tool-browser)"
  }
  finally {
    Pop-Location
  }
}

Write-Host "Repo-local agent cache directories are ready." -ForegroundColor Green

if ($ShowEnv) {
  Write-Host ("GOCACHE={0}" -f $state.GOCACHE)
  Write-Host ("GOMODCACHE={0}" -f $state.GOMODCACHE)
  Write-Host ("NPM_CONFIG_CACHE={0}" -f $state.NPM_CACHE)
  Write-Host ("PLAYWRIGHT_BROWSERS_PATH={0}" -f $state.PLAYWRIGHT_BROWSERS_PATH)
  Write-Host ("DOCKER_CONFIG={0}" -f $state.DOCKER_CONFIG)
}
