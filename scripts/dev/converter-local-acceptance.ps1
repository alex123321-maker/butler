[CmdletBinding()]
param(
 [ValidateSet('Offline','Streaming','Inventory','Native')][string]$Action='Offline',
 [string]$LinuxRoot='/mnt/host/wsl/butler-effort97',
 [switch]$EnableNative
)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$state=Initialize-ButlerAgentEnvironment
if($LinuxRoot -notmatch '^/mnt/host/wsl/butler-[a-zA-Z0-9_-]+$'){throw 'Explicit isolated local Linux root required'}
if($Action -eq 'Native' -and -not $EnableNative){throw 'EnableNative required for actual provider inference'}
Push-Location $state.RepoRoot
try {
 $before=(& git rev-parse HEAD).Trim()
 if($LASTEXITCODE -ne 0){throw 'Source commit unavailable'}
 $command=@'
set -eu
root='__ROOT__'
profile=$(cat "$root/src/.cache/effort97/issue20/profile.json")
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/src/.cache/go-build-linux BUTLER_CONVERTER_BROKER_BINARY=/src/.cache/effort97/issue20/tool-broker "BUTLER_CONVERTER_PROFILE_JSON=$profile" BUTLER_CONVERTER_RUNTIME_TOKEN_FILE=/src/.cache/effort97/issue20/runtime.token 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://postgres@127.0.0.1:55449/converters20_test?sslmode=disable' /bin/sh -c '__RUN__'
'@
 if($Action -ne 'Native'){
  $mode=$Action.ToLowerInvariant()
  $run='cd /src20; sh scripts/dev/converter-local-acceptance.sh '+$mode
 }else{
  $run='cd /src20; mkdir -p .cache/converter20; GOMAXPROCS=4 GOFLAGS=-buildvcs=false GOTOOLCHAIN=local go test -race -c -o .cache/converter20/orchestrator-app.test ./apps/orchestrator/internal/app'
 }
 $build=$command.Replace('__ROOT__',$LinuxRoot).Replace('__RUN__',$run)
 & wsl -d docker-desktop --exec /bin/sh -c $build
 if($LASTEXITCODE -ne 0){throw 'Local converter build/proof failed'}
 if($Action -ne 'Native'){return}
 # Windows Git verifies this exact managed worktree after the Linux build.
 # Linux Git must not reinterpret the Windows .git worktree path or normalize
 # it into a different checkout/index. No foreign checkout is inspected.
 & git diff --quiet HEAD
 if($LASTEXITCODE -ne 0 -or @(git ls-files --others --exclude-standard).Count -or (& git rev-parse HEAD).Trim() -ne $before){throw 'Committed clean prospective source required'}
 $digest=(Get-FileHash (Join-Path $state.RepoRoot '.cache/converter20/orchestrator-app.test') -Algorithm SHA256).Hash.ToLowerInvariant()
 $source="git:$before;binary-sha256:$digest"
 if($source -notmatch '^git:[a-f0-9]{40};binary-sha256:[a-f0-9]{64}$'){throw 'Producer identity invalid'}
 $run='cd /src20; BUTLER_CONVERTER_LOCAL_NATIVE=synthetic-local-only BUTLER_CONVERTER_LOCAL_NATIVE_EVIDENCE=/src20/.cache/converter20/native-20261025f BUTLER_CONVERTER_LOCAL_NATIVE_SOURCE=__SOURCE__ sh scripts/dev/converter-local-acceptance.sh native'
 # Quote the semicolon-containing trusted identity inside the inner shell.
 $native=$command.Replace('__ROOT__',$LinuxRoot).Replace('__RUN__',$run)
 $native=$native.Replace('__SOURCE__','"'+$source+'"')
 & wsl -d docker-desktop --exec /bin/sh -c $native
 if($LASTEXITCODE -ne 0){throw 'Native converter window failed; retain its ledger and do not retry the same ID'}
}
finally {Pop-Location}
