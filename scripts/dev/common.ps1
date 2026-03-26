Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-ButlerRepoRoot {
  $scriptDir = Split-Path -Parent $PSScriptRoot
  return (Resolve-Path (Join-Path $scriptDir "..")).Path
}

function Get-ButlerCacheRoot {
  param(
    [string]$RepoRoot = (Get-ButlerRepoRoot)
  )

  return Join-Path $RepoRoot ".cache"
}

function Initialize-ButlerAgentEnvironment {
  param(
    [string]$RepoRoot = (Get-ButlerRepoRoot)
  )

  $cacheRoot = Get-ButlerCacheRoot -RepoRoot $RepoRoot
  $cacheDirs = @(
    $cacheRoot,
    (Join-Path $cacheRoot "go-build"),
    (Join-Path $cacheRoot "go-mod"),
    (Join-Path $cacheRoot "npm"),
    (Join-Path $cacheRoot "ms-playwright"),
    (Join-Path $cacheRoot "docker")
  )

  foreach ($dir in $cacheDirs) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
  }

  $env:GOCACHE = Join-Path $cacheRoot "go-build"
  $env:GOMODCACHE = Join-Path $cacheRoot "go-mod"
  $env:NPM_CONFIG_CACHE = Join-Path $cacheRoot "npm"
  $env:PLAYWRIGHT_BROWSERS_PATH = Join-Path $cacheRoot "ms-playwright"
  $env:DOCKER_CONFIG = Join-Path $cacheRoot "docker"
  $env:COMPOSE_PROJECT_NAME = "butler"

  return @{
    RepoRoot = $RepoRoot
    CacheRoot = $cacheRoot
    GOCACHE = $env:GOCACHE
    GOMODCACHE = $env:GOMODCACHE
    NPM_CACHE = $env:NPM_CONFIG_CACHE
    PLAYWRIGHT_BROWSERS_PATH = $env:PLAYWRIGHT_BROWSERS_PATH
    DOCKER_CONFIG = $env:DOCKER_CONFIG
  }
}

function Import-ButlerDotEnv {
  param(
    [string]$Path = (Join-Path (Get-ButlerRepoRoot) ".env")
  )

  if (-not (Test-Path $Path)) {
    return
  }

  foreach ($line in Get-Content $Path) {
    $trimmed = $line.Trim()
    if ($trimmed.Length -eq 0 -or $trimmed.StartsWith("#")) {
      continue
    }

    $parts = $trimmed -split "=", 2
    if ($parts.Count -ne 2) {
      continue
    }

    $name = $parts[0].Trim()
    $value = $parts[1]
    if ($value.Length -ge 2) {
      if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
        $value = $value.Substring(1, $value.Length - 2)
      }
    }

    [Environment]::SetEnvironmentVariable($name, $value, "Process")
  }
}

function Assert-CommandAvailable {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name
  )

  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "Required command not found: $Name"
  }
}

function Assert-LastExitCode {
  param(
    [string]$Action = "Native command"
  )

  if ($LASTEXITCODE -ne 0) {
    throw "$Action failed with exit code $LASTEXITCODE."
  }
}

function Get-ButlerComposeArgs {
  param(
    [string]$RepoRoot = (Get-ButlerRepoRoot)
  )

  return @(
    "-f", (Join-Path $RepoRoot "deploy/docker-compose.yml"),
    "-f", (Join-Path $RepoRoot "deploy/docker-compose.dev.yml")
  )
}

function Get-ButlerAgentComposeServices {
  return @(
    "postgres",
    "redis",
    "ollama",
    "migrator",
    "tool-http",
    "tool-webfetch",
    "tool-browser",
    "tool-browser-local",
    "tool-doctor",
    "tool-broker"
  )
}

function Invoke-ButlerCompose {
  param(
    [Parameter(Mandatory = $true)]
    [string[]]$Arguments,
    [string]$RepoRoot = (Get-ButlerRepoRoot)
  )

  & docker compose @(Get-ButlerComposeArgs -RepoRoot $RepoRoot) @Arguments
  Assert-LastExitCode -Action "docker compose"
}
