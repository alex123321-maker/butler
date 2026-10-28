$ErrorActionPreference='Stop'
. ./scripts/dev/common.ps1
$state=Initialize-ButlerAgentEnvironment
$env:GOMAXPROCS='2'
$env:GOTOOLCHAIN='local'
$env:BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/maintenance21_test?sslmode=disable'
& go test -v -count=1 -run '^TestMaintenanceNodeControl' ./internal/maintenance *> .cache/maintenance21-node-package-v1.log
$packageExit=$LASTEXITCODE
& go test -v -count=1 ./apps/maintenance-controller *> .cache/maintenance21-node-process-v1.log
$processExit=$LASTEXITCODE
& go vet ./internal/maintenance ./apps/maintenance-controller *> .cache/maintenance21-node-vet-v1.log
$vetExit=$LASTEXITCODE
@{package_exit=$packageExit;process_exit=$processExit;vet_exit=$vetExit;native_calls=0;physical_operations=0;live_update=$false}|ConvertTo-Json|Set-Content .cache/maintenance21-node-check-v1-exit.json
if($packageExit-or$processExit-or$vetExit){throw 'Node boundary checks failed; retain all logs'}
