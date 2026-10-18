[CmdletBinding()]
param()
. (Join-Path $PSScriptRoot "common.ps1")
$state = Initialize-ButlerAgentEnvironment
Assert-CommandAvailable -Name docker
$databaseName = "butler-schedules-test-" + [guid]::NewGuid().ToString('N').Substring(0, 12)
$created = $false
Push-Location $state.RepoRoot
try {
  & docker run -d --name $databaseName --memory 768m --cpus 2 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=butler_schedules_test pgvector/pgvector:pg17
  Assert-LastExitCode -Action "Create isolated schedule test database"
  $created = $true
  $ready = $false
  for ($attempt=0; $attempt -lt 60; $attempt++) {
    & docker exec $databaseName pg_isready -U postgres -d butler_schedules_test *> $null
    if ($LASTEXITCODE -eq 0) { $ready=$true; break }
    Start-Sleep -Milliseconds 250
  }
  if (-not $ready) { throw "Isolated schedule database did not become ready" }
  & docker run --rm --network "container:$databaseName" --cpus 4 --memory 4g -v "$($state.RepoRoot):/src" -w /src golang:1.27.2 bash scripts/dev/schedules-tests.sh
  Assert-LastExitCode -Action "Schedule integration and race validation"
}
finally {
  if ($created) { & docker rm -f $databaseName | Out-Null }
  Pop-Location
}
