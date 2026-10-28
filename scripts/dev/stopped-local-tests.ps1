[CmdletBinding()]
param([string]$PostgresBin='D:/Repository/butler/.cache/credential-vault-lab/runtime/pgsql/bin', [string]$Run='^TestStoppedLocal')
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$state=Initialize-ButlerAgentEnvironment
$root=Join-Path $state.CacheRoot 'stopped-local-tests'
$data=Join-Path $root 'data'
New-Item -ItemType Directory -Force $root | Out-Null
if (-not (Test-Path (Join-Path $PostgresBin 'postgres.exe'))) {throw 'Cached portable PostgreSQL runtime required'}
if (-not (Test-Path (Join-Path $data 'PG_VERSION'))) {
 & (Join-Path $PostgresBin 'initdb.exe') -D $data -U butler_stopped_test --auth=trust --encoding=UTF8 --locale=C > (Join-Path $root 'initdb.log')
 Assert-LastExitCode -Action 'initialize owned test database'
}
& (Join-Path $PostgresBin 'pg_ctl.exe') -D $data status *> $null
$started=$LASTEXITCODE -ne 0
if ($started) {
 $process=Start-Process -FilePath (Join-Path $PostgresBin 'pg_ctl.exe') -ArgumentList @('-D',('"'+$data+'"'),'-l',('"'+(Join-Path $root 'postgres.log')+'"'),'-o','"-h 127.0.0.1 -p 55447 -c max_connections=40"','start','-w') -WindowStyle Hidden -PassThru
 if (-not $process.WaitForExit(30000) -or $process.ExitCode -ne 0) {throw 'Owned PostgreSQL startup failed'}
}
try {
 $exists=& (Join-Path $PostgresBin 'psql.exe') -h 127.0.0.1 -p 55447 -U butler_stopped_test -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='stopped_recovery_test'"
 Assert-LastExitCode -Action 'test database preflight'
 if ($exists -ne '1') {& (Join-Path $PostgresBin 'createdb.exe') -h 127.0.0.1 -p 55447 -U butler_stopped_test stopped_recovery_test;Assert-LastExitCode -Action 'create test database'}
 $env:BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://butler_stopped_test@127.0.0.1:55447/stopped_recovery_test?sslmode=disable'
 $env:CC='D:/ProgramFiles/msys2/mingw64/bin/gcc.exe'
 Push-Location $state.RepoRoot
 try {
  & go test -race -p 1 -count=1 -v ./internal/actionauth -run $Run
  Assert-LastExitCode -Action 'stopped local PostgreSQL/race integration'
 } finally {Pop-Location}
} finally {
 if ($started) {& (Join-Path $PostgresBin 'pg_ctl.exe') -D $data -m fast stop > (Join-Path $root 'stop.log')}
}
