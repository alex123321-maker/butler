[CmdletBinding()]
param(
 [ValidateSet('Validate','Build','Inventory','Preflight','Run','Evidence','Export','Login')][string]$Action='Validate',
 [string]$PrivateDirectory,
 [string]$TrialId,
 [string]$EvidenceDirectory,
 [switch]$EnableNative
)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$state=Initialize-ButlerAgentEnvironment
Push-Location $state.RepoRoot
try {
 $env:BUTLER_EFFORT_LAB_POSTGRES_URL='postgres://postgres@127.0.0.1:55449/effort97_test?sslmode=disable'
 $labCache=Join-Path $state.RepoRoot '.cache/effort97'
 New-Item -ItemType Directory -Force $labCache | Out-Null
 if($Action -eq 'Validate') {
  & go test -count=1 -json ./cmd/effort-eval ./internal/acceptance/effort ./internal/transport/antigravity ./internal/transport -skip 'TestMemoryJobActualNativeAdapterHTTPIntegration|TestTemporalFailedV1RequestPreHTTPDiagnostic' > (Join-Path $labCache 'offline-windows.jsonl')
  if($LASTEXITCODE -ne 0){throw 'Local research tests failed'}
  $events=Get-Content (Join-Path $labCache 'offline-windows.jsonl') | ConvertFrom-Json
  if(@($events | Where-Object Action -eq skip).Count){throw 'Required local research test skipped'}
  & go vet ./cmd/effort-eval ./internal/acceptance/effort ./internal/transport/antigravity ./internal/transport
  if($LASTEXITCODE -ne 0){throw 'Local research vet failed'}
  Write-Output 'Local research tests and vet passed without skips.'
  return
 }
 $binary=Join-Path $labCache 'effort-eval.exe'
 & go build -o $binary ./cmd/effort-eval
 if($LASTEXITCODE -ne 0){throw 'Local research build failed'}
 if($Action -eq 'Build'){return}
 if(-not $PrivateDirectory){throw 'PrivateDirectory file reference required'}
 if($Action -in @('Inventory','Preflight','Run','Evidence','Login') -and -not $EnableNative){throw 'EnableNative is required for an authorized Google operation'}
 $env:BUTLER_EFFORT_LAB_ENABLE='synthetic-local-only'
 $arguments=@('-action',$Action.ToLowerInvariant(),'-key-file',(Join-Path $PrivateDirectory 'settings.key'))
 if($Action -eq 'Login') {
  $arguments+=@('-oauth-client-file',(Join-Path $PrivateDirectory 'oauth-client.json'),'-login-url-file',(Join-Path $PrivateDirectory 'login-url.txt'))
 }
 if($Action -in @('Run','Preflight','Evidence','Export')){
  if(-not $TrialId -or -not $EvidenceDirectory){throw 'TrialId and EvidenceDirectory required'}
  $arguments+=@('-trial',$TrialId,'-out',$EvidenceDirectory)
 }
 if($Action -in @('Run','Preflight','Evidence')){
  & git diff --quiet HEAD
  if($LASTEXITCODE -ne 0 -or @(git ls-files --others --exclude-standard).Count){throw 'Commit the prospective source before native trial execution'}
  $commit=(& git rev-parse HEAD).Trim()
  $binaryDigest=(Get-FileHash $binary -Algorithm SHA256).Hash.ToLowerInvariant()
  $arguments+=@('-source',"git:$commit;binary-sha256:$binaryDigest")
 }
 & $binary @arguments
 if($LASTEXITCODE -ne 0){throw 'Local research command failed; retained attempts must not be retried in the same window'}
}
finally {Pop-Location}
