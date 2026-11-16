Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$state = Initialize-ButlerAgentEnvironment
$cache = Join-Path $state.CacheRoot 'acp-runtime'
New-Item -ItemType Directory -Force $cache | Out-Null
$payload = Join-Path $cache 'cli.tar.gz'
$url = 'https://storage.googleapis.com/antigravity-public/antigravity-cli/1.5.9-4515810895593472/linux-x64/cli_linux_x64.tar.gz'
$sha512 = 'd01eb13ba7750eed18907f1ae8860864371e0ad9064415e8d033d22b4bd7b9b4e4c7f962058520b452d67db1c33163d9a889d55a7dffa159596ec6cd91a87b79'
if (-not (Test-Path -LiteralPath $payload)) {
  Invoke-WebRequest -Uri $url -OutFile $payload -TimeoutSec 120
}
if ((Get-FileHash -LiteralPath $payload -Algorithm SHA512).Hash.ToLowerInvariant() -ne $sha512) {
  throw 'Official ACP archive checksum mismatch; no runtime is built.'
}
& tar -xzf $payload -C $cache antigravity
if ($LASTEXITCODE -ne 0) { throw 'ACP archive extraction failed.' }
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$previousCGO = $env:CGO_ENABLED
Push-Location $state.RepoRoot
try {
  $env:GOOS = 'linux'
  $env:GOARCH = 'amd64'
  $env:CGO_ENABLED = '0'
  & go build -o (Join-Path $cache 'acp-runtime') ./cmd/acp-runtime
  if ($LASTEXITCODE -ne 0) { throw 'ACP supervisor build failed.' }
  [IO.File]::WriteAllLines((Join-Path $cache '.dockerignore'), @('*', '!acp-runtime', '!antigravity'))
  & docker build -f docker/acp-runtime/Dockerfile -t butler-acp-runtime:1.5.9 $cache
  if ($LASTEXITCODE -ne 0) { throw 'ACP image build failed.' }
} finally {
  Pop-Location
  $env:GOOS = $previousGOOS
  $env:GOARCH = $previousGOARCH
  $env:CGO_ENABLED = $previousCGO
}
