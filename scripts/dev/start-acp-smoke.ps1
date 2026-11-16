param([switch]$InitializeNewAnchor)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$state = Initialize-ButlerAgentEnvironment
$stand = Join-Path $state.CacheRoot 'acp-stand'
$bin = Join-Path $stand 'bin'
New-Item -ItemType Directory -Force $bin | Out-Null
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
Push-Location $state.RepoRoot
try {
  & docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges --mount type=volume,src=butler-acp-200-app_memory-anchor,dst=/profile --entrypoint /bin/sh butler-acp-runtime:1.5.9 -c 'test -r /profile/deletions.log'
  if ($LASTEXITCODE -ne 0) {
    if (-not $InitializeNewAnchor) { throw 'Fresh disposable stand requires explicit -InitializeNewAnchor; a missing existing anchor is never repaired automatically.' }
    $database = & docker ps -a --filter name=butler-acp-200-app-postgres-1 --format '{{.Names}}'
    if ($database) { throw 'Refusing anchor initialization beside an existing database; inspect the stand rather than resetting history.' }
    & docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges --mount type=volume,src=butler-acp-200-app_memory-anchor,dst=/profile --entrypoint /bin/sh butler-acp-runtime:1.5.9 -c 'set -C; umask 077; printf "butler-memory-deletions-v1\n" > /profile/deletions.log'
    if ($LASTEXITCODE -ne 0) { throw 'Fresh anchor initialization failed; no existing file was overwritten.' }
  }
  $env:GOOS = 'linux'
  $env:GOARCH = 'amd64'
  $env:CGO_ENABLED = '0'
  foreach ($service in @('orchestrator','tool-broker','tool-doctor','migrator')) {
    & go build -o (Join-Path $bin $service) "./apps/$service"
    if ($LASTEXITCODE -ne 0) { throw "Failed to build ACP acceptance service $service." }
  }
  $env:BUTLER_ACP_BUILD_DIR = $stand.Replace('\','/')
  $env:BUTLER_ACP_SOURCE_DIR = $state.RepoRoot.Replace('\','/')
  $env:BUTLER_ACP_TEST_BUDGET = '{"models":{"antigravity-acp:gemini-3.8-flash":{"basis":"subscription_agent_no_overage"}},"model_millis":30000,"model_input_bytes":2097152,"agent_output_bytes":65536}'
  & docker compose -p butler-acp-200-app -f deploy/acp/compose.smoke.yml up -d
  if ($LASTEXITCODE -ne 0) { throw 'Isolated ACP Butler stack did not start.' }
} finally {
  Pop-Location
  $env:GOOS = $previousGOOS
  $env:GOARCH = $previousGOARCH
  $env:CGO_ENABLED = $previousCGO
}
