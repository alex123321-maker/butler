[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$agentRoot = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
if ($agentRoot -ne 'D:\CodexData\home\worktrees\initiatives-completion\butler') { throw 'Agent worktree required' }
$fixtureRoot = Join-Path $agentRoot '.cache/antigravity-desktop-fixture'
$nodePath = 'C:\Users\alexa\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'
$desktopExe = Join-Path $env:LOCALAPPDATA 'Programs/antigravity/Antigravity.exe'
$ownerProcess = Get-Process Antigravity | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
if (-not $ownerProcess -or $ownerProcess.Path -ne $desktopExe) { throw 'Installed Desktop reference unavailable' }
if ((Get-Item -LiteralPath $desktopExe).VersionInfo.FileVersion -ne '2.22.0') { throw 'Desktop version changed' }
if (Test-Path -LiteralPath (Join-Path $fixtureRoot 'processes.json')) { throw 'Fixture launcher already invoked; preserve its state' }
foreach ($part in @('userdata','home','home/AppData/Roaming','home/AppData/Local','logs')) { New-Item -ItemType Directory -Force -Path (Join-Path $fixtureRoot $part) | Out-Null }
$sshArgs = @('-N','-o','ExitOnForwardFailure=yes','-o','ServerAliveInterval=15','-o','ServerAliveCountMax=2','-L','127.0.0.1:49631:127.0.0.1:35129','laba')
$tunnel = Start-Process -FilePath ssh -ArgumentList $sshArgs -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $fixtureRoot 'logs/ssh-out') -RedirectStandardError (Join-Path $fixtureRoot 'logs/ssh-err')
$proxy = Start-Process -FilePath $nodePath -ArgumentList (Join-Path $PSScriptRoot 'deny-proxy.mjs') -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $fixtureRoot 'logs/proxy-out') -RedirectStandardError (Join-Path $fixtureRoot 'logs/proxy-err')
$homePath = Join-Path $fixtureRoot 'home'
$isolatedEnvironment = @{
 DEV_URL='https://127.0.0.1:49631/'
 USERPROFILE=$homePath; HOME=$homePath
 APPDATA=(Join-Path $homePath 'AppData/Roaming')
 LOCALAPPDATA=(Join-Path $homePath 'AppData/Local')
 HTTP_PROXY='http://127.0.0.1:35128'; HTTPS_PROXY='http://127.0.0.1:35128'
}
$desktopArgs = @("--user-data-dir=$(Join-Path $fixtureRoot 'userdata')",'--remote-debugging-port=35130','--remote-debugging-address=127.0.0.1','--allow-insecure-localhost','--disable-component-update','--disable-background-networking','--no-first-run','--proxy-server=http://127.0.0.1:35128','--proxy-bypass-list=<-loopback>')
$desktop = Start-Process -FilePath $desktopExe -ArgumentList $desktopArgs -Environment $isolatedEnvironment -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $fixtureRoot 'logs/desktop-out') -RedirectStandardError (Join-Path $fixtureRoot 'logs/desktop-err')
[pscustomobject]@{tunnel_pid=$tunnel.Id;proxy_pid=$proxy.Id;desktop_pid=$desktop.Id;owner_pid=$ownerProcess.Id;source='Installed Desktop2.22; DEV_URL skips normal LS; synthetic Linux LS same CL';real_model_calls=0} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $fixtureRoot 'processes.json') -Encoding utf8
[pscustomobject]@{fixture_launched=$true;owner_process_unchanged=$true;normal_profile_imported=$false;system_proxy_or_trust_changed=$false;real_model_calls=0} | ConvertTo-Json
