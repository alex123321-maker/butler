[CmdletBinding()]
param([ValidateSet('Setup','Start','Stop')][string]$Action='Start')
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$state=Initialize-ButlerAgentEnvironment
$labRoot=Join-Path $state.CacheRoot 'credential-vault-lab'
$archive=Join-Path $labRoot 'postgresql.zip'
$runtimeRoot=Join-Path $labRoot 'runtime'
$dataRoot=Join-Path $labRoot 'data'
$pgBin=Join-Path $runtimeRoot 'pgsql/bin'
New-Item -ItemType Directory -Force -Path $labRoot | Out-Null
if ($Action -eq 'Setup') {
  if (-not (Test-Path -LiteralPath (Join-Path $pgBin 'postgres.exe'))) {
    if (-not (Test-Path -LiteralPath $archive)) {
      Invoke-WebRequest -Uri 'https://get.enterprisedb.com/postgresql/postgresql-17.11-3-windows-x64-binaries.zip' -OutFile $archive
    }
    $expected='4b8db0930c38f6ef845db919551dedda3b6b845aeb0927b3d79a6e8e9e4537cf'
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) { throw 'PostgreSQL archive checksum mismatch' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip=[IO.Compression.ZipFile]::OpenRead($archive)
    try {
      foreach ($entry in $zip.Entries) {
        if ($entry.FullName -notmatch '^pgsql/(bin|lib|share)/' -or $entry.FullName.EndsWith('/')) { continue }
        $target=[IO.Path]::GetFullPath((Join-Path $runtimeRoot $entry.FullName))
        $prefix=[IO.Path]::GetFullPath($runtimeRoot)+[IO.Path]::DirectorySeparatorChar
        if (-not $target.StartsWith($prefix,[StringComparison]::OrdinalIgnoreCase)) { throw 'Archive path rejected' }
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
        [IO.Compression.ZipFileExtensions]::ExtractToFile($entry,$target,$true)
      }
    } finally { $zip.Dispose() }
  }
  if (-not (Test-Path -LiteralPath (Join-Path $dataRoot 'PG_VERSION'))) {
    & (Join-Path $pgBin 'initdb.exe') -D $dataRoot -U butler_vault_lab --auth=trust --encoding=UTF8 --locale=C
    Assert-LastExitCode -Action 'initdb'
  }
  Write-Host 'Local PostgreSQL test runtime ready; loopback only, synthetic data.'
  exit
}
if (-not (Test-Path -LiteralPath (Join-Path $dataRoot 'PG_VERSION'))) { throw 'Run -Action Setup first' }
if ($Action -eq 'Stop') {
  & (Join-Path $pgBin 'pg_ctl.exe') -D $dataRoot -m fast stop
  Assert-LastExitCode -Action 'PostgreSQL stop'
  exit
}
& (Join-Path $pgBin 'pg_ctl.exe') -D $dataRoot status *> $null
if ($LASTEXITCODE -ne 0) {
  $starter=Start-Process -FilePath (Join-Path $pgBin 'pg_ctl.exe') -ArgumentList @('-D',('"'+$dataRoot+'"'),'-l',('"'+(Join-Path $labRoot 'postgres.log')+'"'),'-o','"-h 127.0.0.1 -p 55439"','start','-w') -WindowStyle Hidden -PassThru
  if (-not $starter.WaitForExit(30000) -or $starter.ExitCode -ne 0) { throw 'Local PostgreSQL startup failed' }
}
$script:exists=''
& (Join-Path $pgBin 'psql.exe') -h 127.0.0.1 -p 55439 -U butler_vault_lab -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='credential_vault_test'" | ForEach-Object { $script:exists=$_.Trim() }
if ($script:exists -ne '1') { & (Join-Path $pgBin 'createdb.exe') -h 127.0.0.1 -p 55439 -U butler_vault_lab credential_vault_test; Assert-LastExitCode -Action 'create test database' }
Write-Host 'Local test PostgreSQL listening on 127.0.0.1:55439, database credential_vault_test.'
