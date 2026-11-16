param(
  [int]$RaceCount = 5
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($RaceCount -lt 1 -or $RaceCount -gt 10) {
  throw 'RaceCount must be between 1 and 10.'
}

. (Join-Path $PSScriptRoot 'common.ps1')
$state = Initialize-ButlerAgentEnvironment
$repo = $state.RepoRoot
$cache = Join-Path $state.CacheRoot 'acp-linux'
New-Item -ItemType Directory -Force $cache | Out-Null

$dockerArgs = @(
  'run', '--rm', '--init', '--pull=never',
  '--network', 'none', '--read-only', '--cap-drop', 'ALL',
  '--security-opt', 'no-new-privileges', '--pids-limit', '128',
  '--memory', '2g', '--cpus', '2', '--user', '65534:65534',
  '--tmpfs', '/tmp:rw,exec,nosuid,nodev,size=256m',
  '--mount', "type=bind,src=$repo/internal,dst=/workspace/internal,readonly",
  '--mount', "type=bind,src=$repo/configs,dst=/workspace/configs,readonly",
  '--mount', "type=bind,src=$repo/cmd/acp-profile,dst=/workspace/cmd/acp-profile,readonly",
  '--mount', "type=bind,src=$repo/cmd/acp-runtime,dst=/workspace/cmd/acp-runtime,readonly",
  '--mount', "type=bind,src=$repo/go.mod,dst=/workspace/go.mod,readonly",
  '--mount', "type=bind,src=$repo/go.sum,dst=/workspace/go.sum,readonly",
  '--mount', "type=bind,src=$cache,dst=/cache",
  '--mount', "type=bind,src=$($state.GOMODCACHE),dst=/modules,readonly",
  '--workdir', '/workspace',
  '--env', 'GOCACHE=/cache/build', '--env', 'GOMODCACHE=/modules',
  '--env', 'GOTOOLCHAIN=local', '--env', 'GOPROXY=off',
  '--env', 'GOSUMDB=off', '--env', 'GOMAXPROCS=2',
  'golang:1.27.2', 'sh', '-c',
  "go test ./internal/acp ./internal/transport/antigravityacp ./internal/providerfactory ./internal/config ./cmd/acp-profile ./cmd/acp-runtime -count=1 -timeout=90s && go test -race ./internal/acp ./internal/transport/antigravityacp -count=$RaceCount -timeout=90s && go vet ./internal/acp ./internal/transport/antigravityacp ./internal/providerfactory ./internal/config ./cmd/acp-profile ./cmd/acp-runtime"
)
& docker @dockerArgs
if ($LASTEXITCODE -ne 0) {
  throw "Offline ACP process checks failed with exit code $LASTEXITCODE."
}
