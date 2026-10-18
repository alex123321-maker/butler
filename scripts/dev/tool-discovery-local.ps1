[CmdletBinding()]
param()
. (Join-Path $PSScriptRoot "common.ps1")
$state = Initialize-ButlerAgentEnvironment
Assert-CommandAvailable -Name docker
$databaseName = "butler-tool-discovery-test-" + [guid]::NewGuid().ToString('N').Substring(0, 12)
$created = $false
Push-Location $state.RepoRoot
try {
  & docker run -d --name $databaseName --memory 768m --cpus 2 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=butler_tool_discovery_test pgvector/pgvector:pg17
  Assert-LastExitCode -Action "Create isolated discovery test database"
  $created = $true
  $ready = $false
  for ($attempt=0; $attempt -lt 60; $attempt++) {
    & docker exec $databaseName pg_isready -U postgres -d butler_tool_discovery_test *> $null
    if ($LASTEXITCODE -eq 0) { $ready=$true; break }
    Start-Sleep -Milliseconds 250
  }
  if (-not $ready) { throw "Isolated discovery database did not become ready" }
  & docker run --rm --network "container:$databaseName" --cpus 4 --memory 4g -v "$($state.RepoRoot):/src" -w /src golang:1.27.2 bash scripts/dev/tool-discovery-tests.sh
  Assert-LastExitCode -Action "Discovery PostgreSQL, process, transport and race validation"
}
finally {
  if ($created) { & docker rm -f $databaseName | Out-Null }
  Pop-Location
}
