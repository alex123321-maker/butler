[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
$agentRoot=(Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
if($agentRoot -ne 'D:\CodexData\home\worktrees\initiatives-completion\butler'){throw 'Agent worktree required'}
$fixtureRoot=Join-Path $agentRoot '.cache/antigravity-desktop-fixture'
$manifest=Get-Content -LiteralPath (Join-Path $fixtureRoot 'processes.json') -Raw | ConvertFrom-Json
$processes=Get-CimInstance Win32_Process
$references=@(
 @{Id=$manifest.tunnel_pid;Name='ssh.exe';Literal='127.0.0.1:49631:127.0.0.1:35129'},
 @{Id=$manifest.proxy_pid;Name='node.exe';Literal=(Join-Path $PSScriptRoot 'deny-proxy.mjs')},
 @{Id=$manifest.desktop_pid;Name='Antigravity.exe';Literal=(Join-Path $fixtureRoot 'userdata')}
)
foreach($reference in $references){
 if($reference.Id -eq $manifest.owner_pid){throw 'Owner process protected'}
 $process=$processes | Where-Object ProcessId -eq $reference.Id | Select-Object -First 1
 if($process){
  if($process.Name -ne $reference.Name -or -not $process.CommandLine.Contains($reference.Literal)){throw 'Owned process identity changed'}
  Stop-Process -Id $reference.Id
 }
}
if(-not (Get-Process -Id $manifest.owner_pid -ErrorAction SilentlyContinue)){throw 'Owner process missing'}
[pscustomobject]@{owned_helpers_stopped=$true;owner_process_preserved=$true;private_files_preserved=$true} | ConvertTo-Json
