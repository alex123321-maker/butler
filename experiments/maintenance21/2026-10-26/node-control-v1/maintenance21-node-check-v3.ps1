$ErrorActionPreference='Stop'
. ./scripts/dev/common.ps1
$state=Initialize-ButlerAgentEnvironment
$env:GOMAXPROCS='2'
$env:GOTOOLCHAIN='local'
$env:BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/maintenance21_test?sslmode=disable'
& (Join-Path $state.GOMODCACHE 'golang.org/toolchain@v0.0.1-go1.27.2.windows-amd64/bin/go.exe') test -v -count=1 -run '^TestMaintenanceNodeControl' ./internal/maintenance *> .cache/maintenance21-node-package-v3.log
$packageExit=$LASTEXITCODE
& (Join-Path $state.GOMODCACHE 'golang.org/toolchain@v0.0.1-go1.27.2.windows-amd64/bin/go.exe') test -v -count=1 ./apps/maintenance-controller *> .cache/maintenance21-node-process-v3.log
$processExit=$LASTEXITCODE
& (Join-Path $state.GOMODCACHE 'golang.org/toolchain@v0.0.1-go1.27.2.windows-amd64/bin/go.exe') vet ./internal/maintenance ./apps/maintenance-controller *> .cache/maintenance21-node-vet-v3.log
$vetExit=$LASTEXITCODE
@{package_exit=$packageExit;process_exit=$processExit;vet_exit=$vetExit;native_calls=0;physical_operations=0;live_update=$false}|ConvertTo-Json|Set-Content .cache/maintenance21-node-check-v3-exit.json
if($packageExit-or$processExit-or$vetExit){throw 'Node boundary checks failed; retain all logs'}
