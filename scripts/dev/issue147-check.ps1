[CmdletBinding()]
param([ValidateSet('Build','Rebuild','Test')][string]$Action='Test',
      [string]$BaselineRef='8aa2c0450ee1d200093c691ece6a7670c2143cb5')
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$state=Initialize-ButlerAgentEnvironment
$out=Join-Path $state.CacheRoot 'issue147'
New-Item -ItemType Directory -Force $out | Out-Null
Push-Location $state.RepoRoot
try {
  if ($Action -eq 'Test') {
    & go test -count=1 -json ./apps/orchestrator/internal/orchestrator ./internal/security/content > (Join-Path $out 'unit-final.jsonl')
    Assert-LastExitCode 'request-context unit checks'
    & go test -count=1 -json ./internal/transport/antigravity -run '^TestCurrentRequest' > (Join-Path $out 'wire-final.jsonl')
    Assert-LastExitCode 'native wire check'
    & go vet ./apps/orchestrator/internal/orchestrator ./internal/security/content ./internal/transport/antigravity
    Assert-LastExitCode 'request-context vet'
    exit 0
  }
  $baseline=Join-Path $out ('baseline-'+$BaselineRef)
  if ($Action -eq 'Build') {
    if (Test-Path $baseline) { throw 'Baseline already exists; preserve existing build evidence.' }
    New-Item -ItemType Directory $baseline | Out-Null
    & git archive --format=tar -o (Join-Path $out ('baseline-'+$BaselineRef+'.tar')) $BaselineRef apps internal configs go.mod go.sum migrations testdata
    Assert-LastExitCode 'baseline source export'
    & tar -xf (Join-Path $out ('baseline-'+$BaselineRef+'.tar')) -C $baseline
    Assert-LastExitCode 'baseline extraction'
  }
  foreach($path in @('apps/orchestrator/internal/app/current_request_native_test.go','apps/orchestrator/internal/app/guarded_tab_request_process_test.go')) {
    Copy-Item -LiteralPath (Join-Path $state.RepoRoot $path) -Destination (Join-Path $baseline $path)
  }
  $env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
  New-Item -ItemType Directory -Force (Join-Path $out 'bin') | Out-Null
  & go test -c -trimpath -o (Join-Path $out 'bin/candidate-app.test') ./apps/orchestrator/internal/app
  Assert-LastExitCode 'candidate native driver'
  & go test -c -trimpath -o (Join-Path $out 'bin/broker.test') ./apps/tool-broker/internal/broker
  Assert-LastExitCode 'separate Broker process'
  & go test -c -trimpath -o (Join-Path $out 'bin/orchestrator.test') ./apps/orchestrator/internal/orchestrator
  Assert-LastExitCode 'orchestrator regressions'
  & go test -c -trimpath -o (Join-Path $out 'bin/communication.test') ./apps/orchestrator/internal/communication
  Assert-LastExitCode 'communication regressions'
  Push-Location $baseline
  try {
    & go test -c -trimpath -o (Join-Path $out 'bin/baseline-app.test') ./apps/orchestrator/internal/app
    Assert-LastExitCode 'baseline native driver'
  } finally { Pop-Location }
  Get-ChildItem (Join-Path $out 'bin') -File | Get-FileHash -Algorithm SHA256 | Format-Table -AutoSize
} finally { Pop-Location }
