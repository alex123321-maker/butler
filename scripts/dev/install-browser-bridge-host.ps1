[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [string]$ExtensionId,

  [ValidateSet("Chrome", "Chromium", "Edge")]
  [string]$Browser = "Chrome",

  [string]$OutputDir,

  [switch]$SkipBuild
)

. (Join-Path $PSScriptRoot "common.ps1")

$state = Initialize-ButlerAgentEnvironment
Assert-CommandAvailable -Name go

$repoRoot = $state.RepoRoot
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
  $OutputDir = Join-Path $repoRoot ".cache\native-host"
}

$manifestDir = Join-Path $OutputDir "manifest"
$binaryDir = Join-Path $OutputDir "bin"
New-Item -ItemType Directory -Force -Path $manifestDir | Out-Null
New-Item -ItemType Directory -Force -Path $binaryDir | Out-Null

$binaryPath = Join-Path $binaryDir "browser-bridge.exe"
if (-not $SkipBuild) {
  Push-Location $repoRoot
  try {
    go build -o $binaryPath ./apps/browser-bridge
    Assert-LastExitCode -Action "go build browser-bridge"
  }
  finally {
    Pop-Location
  }
}

if (-not (Test-Path $binaryPath)) {
  throw "browser-bridge binary not found at $binaryPath"
}

$manifestPath = Join-Path $manifestDir "com.butler.browser_bridge.json"
$manifest = @{
  name = "com.butler.browser_bridge"
  description = "Butler browser bridge native messaging host"
  path = (Resolve-Path $binaryPath).Path
  type = "stdio"
  allowed_origins = @(
    "chrome-extension://$ExtensionId/"
  )
}

$manifest | ConvertTo-Json -Depth 4 | Set-Content -Encoding UTF8 -Path $manifestPath

$registryPath = switch ($Browser) {
  "Chrome" { "HKCU:\Software\Google\Chrome\NativeMessagingHosts\com.butler.browser_bridge" }
  "Chromium" { "HKCU:\Software\Chromium\NativeMessagingHosts\com.butler.browser_bridge" }
  "Edge" { "HKCU:\Software\Microsoft\Edge\NativeMessagingHosts\com.butler.browser_bridge" }
}

New-Item -Path $registryPath -Force | Out-Null
Set-ItemProperty -Path $registryPath -Name "(default)" -Value (Resolve-Path $manifestPath).Path

Write-Host "Installed Butler browser bridge native host." -ForegroundColor Green
Write-Host ("Browser:      {0}" -f $Browser)
Write-Host ("Extension ID: {0}" -f $ExtensionId)
Write-Host ("Binary:       {0}" -f (Resolve-Path $binaryPath).Path)
Write-Host ("Manifest:     {0}" -f (Resolve-Path $manifestPath).Path)
Write-Host ("Registry:     {0}" -f $registryPath)
