[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$state=Initialize-ButlerAgentEnvironment
if (-not $env:BUTLER_TEST_ACTIONAUTH_POSTGRES_URL) { throw 'Set a disposable *_test PostgreSQL DSN in BUTLER_TEST_ACTIONAUTH_POSTGRES_URL' }
$runRoot=Join-Path $state.CacheRoot ('browser-page-execution-ci/local-'+[DateTime]::UtcNow.ToString('yyyyMMddTHHmmss')+'-'+$PID)
if(Test-Path -LiteralPath $runRoot){throw 'Refusing to overwrite page execution evidence'}
New-Item -ItemType Directory -Path $runRoot | Out-Null
$env:TEMP=Join-Path $runRoot 'tmp';$env:TMP=$env:TEMP
New-Item -ItemType Directory -Path $env:TEMP | Out-Null
function Free-PagePort {
 $listener=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0)
 try{$listener.Start();return ([Net.IPEndPoint]$listener.LocalEndpoint).Port}finally{$listener.Stop()}
}
$apiPort=Free-PagePort;$webPort=Free-PagePort
$env:BUTLER_WEB_TEST_PORT=[string]$webPort
$env:NITRO_PORT=[string]$webPort;$env:NITRO_HOST='127.0.0.1'
$env:BUTLER_MANAGED_PAGES_UI_ADDR='127.0.0.1:'+$apiPort
$env:BUTLER_MANAGED_PAGES_REAL_API='http://127.0.0.1:'+$apiPort
$env:BUTLER_MANAGED_PAGES_UI_READY=Join-Path $runRoot 'ready'
$env:BUTLER_MANAGED_PAGES_UI_DONE=Join-Path $runRoot 'done'
$env:BUTLER_MANAGED_PAGES_UI_ABORT=Join-Path $runRoot 'abort'
$env:BUTLER_BROWSER_PAGES_BROKER_E2E='1'
$env:BUTLER_BROWSER_E2E_SCRIPT=Join-Path $state.RepoRoot 'apps/tool-browser/scripts/browser_runtime.mjs'
$env:BUTLER_BROWSER_E2E_BINARY=Join-Path $runRoot 'browser.test.exe'
$env:BUTLER_SERVER_BROWSER_API_BINARY=Join-Path $runRoot 'api.test.exe'
$brokerBinary=Join-Path $runRoot 'broker.test.exe'
$preview=$null;$backend=$null
Push-Location $state.RepoRoot
try {
 foreach($item in @(@('./apps/tool-browser/internal/runtime',$env:BUTLER_BROWSER_E2E_BINARY),@('./apps/orchestrator/internal/api',$env:BUTLER_SERVER_BROWSER_API_BINARY),@('./apps/tool-broker/internal/broker',$brokerBinary))){
  & go test -c -race -o $item[1] $item[0]
  Assert-LastExitCode -Action 'compile actual page fixture'
 }
 npm run --prefix web build *> (Join-Path $runRoot 'web-build.log')
 Assert-LastExitCode -Action 'build Web'
 $preview=Start-Process -FilePath (Get-Command node).Source -ArgumentList @('.output/server/index.mjs') -WorkingDirectory (Join-Path $state.RepoRoot 'web') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runRoot 'web-preview.log') -RedirectStandardError (Join-Path $runRoot 'web-preview-error.log')
 $deadline=[DateTime]::UtcNow.AddSeconds(25)
 $healthy=$false
 while([DateTime]::UtcNow -lt $deadline){
  try{$response=Invoke-WebRequest -Uri ('http://127.0.0.1:'+$webPort) -TimeoutSec 2;if($response.StatusCode -eq 200){$healthy=$true;break}}catch{}
  Start-Sleep -Milliseconds 200
 }
 if(-not $healthy){throw 'Isolated Web preview unavailable'}
 $backend=Start-Process -FilePath (Get-Command go).Source -ArgumentList @('tool','test2json','-t','-p','broker-managed-pages',$brokerBinary,'-test.v','-test.run=^TestManagedPagesBrokerChromiumIntegration$','-test.timeout=150s') -WorkingDirectory (Join-Path $state.RepoRoot 'apps/tool-broker/internal/broker') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runRoot 'broker.jsonl') -RedirectStandardError (Join-Path $runRoot 'broker-error.log')
 $deadline=[DateTime]::UtcNow.AddSeconds(35)
 while(-not (Test-Path -LiteralPath $env:BUTLER_MANAGED_PAGES_UI_READY)){
  if($backend.HasExited -or [DateTime]::UtcNow -ge $deadline){throw 'Actual page Broker/API readiness failed'}
  Start-Sleep -Milliseconds 100
 }
 Push-Location (Join-Path $state.RepoRoot 'web')
 try{
  npx playwright test tests/e2e/server-browser-pages-real.spec.ts --workers=1 --reporter=json --output (Join-Path $runRoot 'web-real-results') > (Join-Path $runRoot 'web-real.json')
  Assert-LastExitCode -Action 'actual managed page owner UI'
 }finally{Pop-Location}
 if(-not $backend.WaitForExit(30000) -or $backend.ExitCode -ne 0){throw 'Actual page Broker/kernel acceptance failed'}
 $events=@(Get-Content (Join-Path $runRoot 'broker.jsonl') | ConvertFrom-Json -AsHashtable)
 if(@($events|Where-Object{$_['Action'] -eq 'skip' -or $_['Action'] -eq 'fail'}).Count){throw 'Required page Broker case skipped/failed'}
 if(-not @($events|Where-Object{$_['Action'] -eq 'pass' -and $_['Test'] -eq 'TestManagedPagesBrokerChromiumIntegration'}).Count){throw 'Required page Broker acceptance missing'}
 Push-Location (Join-Path $state.RepoRoot 'web')
 try{
  npx playwright test tests/e2e/server-browser.spec.ts tests/e2e/server-browser-pages.spec.ts --workers=1 --reporter=json --output (Join-Path $runRoot 'web-component-results') > (Join-Path $runRoot 'web-component.json')
  Assert-LastExitCode -Action 'owner page UI and existing single-page regressions'
 }finally{Pop-Location}
 Write-Host ('Actual page UI/Core/Broker/PG/resource/Chromium PASS; model_calls=0; evidence='+$runRoot)
}finally{
 if($backend -and -not $backend.HasExited){
  [IO.File]::WriteAllText($env:BUTLER_MANAGED_PAGES_UI_ABORT,'aborted')
  if(-not $backend.WaitForExit(5000)){Stop-Process -Id $backend.Id -ErrorAction SilentlyContinue}
 }
 if($preview -and -not $preview.HasExited){Stop-Process -Id $preview.Id -ErrorAction SilentlyContinue}
 Pop-Location
}
