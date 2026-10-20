[CmdletBinding()]
param()

. (Join-Path $PSScriptRoot 'common.ps1')
$state = Initialize-ButlerAgentEnvironment
Assert-CommandAvailable -Name docker
$runID = (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '-' + [guid]::NewGuid().ToString('N').Substring(0,8)
$name = "butler-issue17-$runID".ToLowerInvariant()
$report = Join-Path $state.CacheRoot "issue17/$runID"
New-Item -ItemType Directory -Force -Path $report | Out-Null
$networkCreated = $false
$databaseCreated = $false
$binariesCreated = $false
Push-Location $state.RepoRoot
try {
  function Write-SourceManifest([string]$path) {
    $paths = @(git ls-files --cached --others --exclude-standard | Sort-Object -Unique)
    $paths | ForEach-Object {
      $hash = Get-FileHash -LiteralPath (Join-Path $state.RepoRoot $_) -Algorithm SHA256
      "$($hash.Hash.ToLowerInvariant())  $_"
    } | Set-Content -LiteralPath $path
  }
  git rev-parse HEAD | Set-Content (Join-Path $report 'source-commit.txt')
  $sourceDiff = @(git diff --binary HEAD)
  Set-Content (Join-Path $report 'source-diff.patch') -Value ($sourceDiff -join "`n")
  Write-SourceManifest (Join-Path $report 'source-before.sha256')
  Write-Host "Local issue17 report: $report"
  & docker version --format '{{.Server.Os}} {{.Server.Version}}' | Set-Content (Join-Path $report 'docker-version.txt')
  Assert-LastExitCode -Action 'Local Docker availability'
  & docker run --rm --memory 2g --cpus 4 -v "$($state.RepoRoot):/src:ro" -v "$($state.CacheRoot):/src/.cache" -w /src golang:1.27.2 bash scripts/dev/issue17-tests.sh prepare
  Assert-LastExitCode -Action 'Prepare pinned Go dependencies locally'
  & docker build -f scripts/dev/issue17-browser.Dockerfile -t butler-issue17-browser:1.52.0 . > (Join-Path $report 'browser-build.log') 2>&1
  Assert-LastExitCode -Action 'Build isolated Chromium fixture image'
  & docker image inspect golang:1.27.2 pgvector/pgvector:pg17 butler-issue17-browser:1.52.0 --format '{{.RepoTags}} {{.Id}}' | Set-Content (Join-Path $report 'images.txt')
  Assert-LastExitCode -Action 'Record exact local image identities'
  & docker network create --internal "$name-net" | Out-Null
  Assert-LastExitCode -Action 'Create isolated issue17 network'
  $networkCreated = $true
  & docker volume create "$name-binaries" | Out-Null
  Assert-LastExitCode -Action 'Create isolated executable fixture volume'
  $binariesCreated = $true
  & docker run --rm --network none -v "${name}-binaries:/proof" golang:1.27.2 chown 10001:10001 /proof
  Assert-LastExitCode -Action 'Provision private non-root executable volume'
  & docker run -d --name $name --network "$name-net" --network-alias issue17-postgres --memory 512m --cpus 1 --pids-limit 96 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=butler_issue17_test pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=48 -c max_wal_size=64MB | Out-Null
  Assert-LastExitCode -Action 'Create disposable issue17 PostgreSQL'
  $databaseCreated = $true
  $ready = $false
  for ($attempt=0; $attempt -lt 60; $attempt++) {
    & docker exec $name pg_isready -h 127.0.0.1 -U postgres -d butler_issue17_test *> $null
    if ($LASTEXITCODE -eq 0) { $ready=$true; break }
    Start-Sleep -Milliseconds 500
  }
  if (-not $ready) { throw 'Disposable issue17 PostgreSQL is not ready' }
  & docker exec $name psql -v ON_ERROR_STOP=1 -U postgres -d butler_issue17_test -c 'CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public' | Out-Null
  Assert-LastExitCode -Action 'Initialize pgvector before independent test schemas'
  $dsn = 'postgres://postgres@issue17-postgres:5432/butler_issue17_test?sslmode=disable'
  # The production Unix transport requires a private operator-provisioned parent.
  # Keep it on this disposable container's tmpfs, owned by the actual test UID.
  & docker run --rm --user 10001:10001 --network "$name-net" --memory 3g --cpus 4 --pids-limit 256 --tmpfs /run/butler-workspace:rw,nosuid,nodev,noexec,uid=10001,gid=10001,mode=0700 -v "$($state.RepoRoot):/src:ro" -v "$($state.CacheRoot):/src/.cache" -v "${report}:/reports" -v "${name}-binaries:/proof" -e HOME=/tmp -e "BUTLER_TEST_POSTGRES_URL=$dsn" -w /src golang:1.27.2 bash scripts/dev/issue17-tests.sh core
  Assert-LastExitCode -Action 'Current-source resource/workspace/Broker/owner acceptance'
  & docker run --rm --user 10001:10001 --network "$name-net" --memory 1536m --cpus 4 --pids-limit 256 --security-opt no-new-privileges:true -v "$($state.RepoRoot):/src:ro" -v "${report}:/reports" -v "${name}-binaries:/proof:ro" -e "BUTLER_TEST_POSTGRES_URL=$dsn" -e "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=$dsn" --entrypoint bash butler-issue17-browser:1.52.0 /src/scripts/dev/issue17-tests.sh browser
  Assert-LastExitCode -Action 'Actual local Chromium/native-host arbitration acceptance'
  $events = @(Get-ChildItem -LiteralPath $report -Filter '*.jsonl' | ForEach-Object { Get-Content -LiteralPath $_.FullName | ForEach-Object { $_ | ConvertFrom-Json } })
  $passed = @($events | Where-Object { $_.Action -eq 'pass' -and $_.PSObject.Properties['Test'] } | ForEach-Object Test)
  $required = @(
    'TestResourceReadersAndExclusiveWriterAcrossSessionsIntegration',
    'TestResourceSetAtomicFailureAndOppositeOrderIntegration',
    'TestResourceExpiredWriterRemainsBlockedUntilEvidenceReconciliationIntegration',
    'TestResourceDeadlineIsRecheckedAfterContendedLockIntegration',
    'TestConcurrentRealPatchesCannotOverwriteAcrossStoreInstancesIntegration',
    'TestIsolatedCopiesScopesAndPrivateTempsIntegration',
    'TestExpiredOrSupersededAuthorityCannotWriteAfterWorkspaceWaitIntegration',
    'TestWorkspaceOwnerDiscardActualRuntimeProcessLossIntegration',
    'TestWorkspaceOwnerDiscardUnknownPhysicalCopyIntegration',
    'TestWorkspaceOwnerDiscardSupersedesOnlyStaleUnattemptedIntentIntegration',
    'TestChildAndToolReservationsShareAtomicParentLimitsIntegration',
    'TestSharedReservationsSurviveRestartAndRejectParallelOverspendIntegration',
    'TestChildAdmissionCannotResetRootBudgetIntegration',
    'TestExtractionConcurrentWorkerCandidatesNeedOwnerHeadCASIntegration',
    'TestMemoryCandidateDispatchedIntegration',
    'TestWorkspaceActualApplicationBootstrapIntegration',
    'TestGuardedTabRuntimeBootstrap',
    'TestGuardedTabRequestCoreSuspendsBeforeGenericResult',
    'TestOwnerWorkspaceRecoveryHTTPRejectsCallerAuthorityAndPreservesUnknown'
  )
  foreach ($test in $required) {
    if ($test -notin $passed) { throw "Mandatory acceptance test did not pass: $test" }
  }
  Write-SourceManifest (Join-Path $report 'source-after.sha256')
  if ((Get-FileHash (Join-Path $report 'source-before.sha256')).Hash -ne (Get-FileHash (Join-Path $report 'source-after.sha256')).Hash) { throw 'Source changed during acceptance' }
  Get-ChildItem -LiteralPath $report -File | Where-Object Name -ne 'SHA256SUMS' | Get-FileHash -Algorithm SHA256 | ForEach-Object { "$($_.Hash.ToLowerInvariant())  $([IO.Path]::GetFileName($_.Path))" } | Set-Content (Join-Path $report 'SHA256SUMS')
  'PASS' | Set-Content (Join-Path $report 'complete')
  Write-Host 'All mandatory issue17 local gates passed. No remote host or native model was used.'
}
finally {
  if ($databaseCreated) {
    & docker logs --tail 60 $name > (Join-Path $report 'postgres.log') 2>&1
    & docker rm -f -v $name | Out-Null
  }
  if ($binariesCreated) { & docker volume rm "$name-binaries" | Out-Null }
  if ($networkCreated) { & docker network rm "$name-net" | Out-Null }
  Pop-Location
}
