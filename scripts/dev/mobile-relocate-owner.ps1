[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
# Explicit operator relocation of the same restored Butler identity, not pairing
# with a different server. No credentials or private XML are printed.
$taskAdb='D:/ProgramFiles/AndroidSDK/platform-tools/adb.exe'
$taskPackage='dev.butler.companion.debug'
$taskBackup='D:/CodexData/deployments/butler-local/private/backups/phone-relocation-20261019'
New-Item -ItemType Directory -Path $taskBackup -Force -ErrorAction Stop | Out-Null
function Invoke-PrivateAdb([string[]]$Arguments,[byte[]]$InputBytes=$null){
 $taskProc=[Diagnostics.Process]::new()
 $taskProc.StartInfo.FileName=$taskAdb
 foreach($taskArg in $Arguments){$taskProc.StartInfo.ArgumentList.Add($taskArg)}
 $taskProc.StartInfo.UseShellExecute=$false
 $taskProc.StartInfo.CreateNoWindow=$true
 $taskProc.StartInfo.RedirectStandardOutput=$true
 $taskProc.StartInfo.RedirectStandardError=$true
 $taskProc.StartInfo.RedirectStandardInput=$true
 [void]$taskProc.Start()
 $taskError=$taskProc.StandardError.ReadToEndAsync()
 if($InputBytes){$taskProc.StandardInput.BaseStream.Write($InputBytes)}
 $taskProc.StandardInput.Close()
 $taskMemory=[IO.MemoryStream]::new()
 $taskProc.StandardOutput.BaseStream.CopyTo($taskMemory)
 $taskProc.WaitForExit()
 if($taskProc.ExitCode -ne 0){throw "Private ADB operation failed in $script:taskAdbPhase; private stderr withheld"}
 return ,$taskMemory.ToArray()
}
[void](Invoke-PrivateAdb @('shell','cmd','notification','disallow_listener',"$taskPackage/com.example.service.ButlerNotificationListenerService"))
[void](Invoke-PrivateAdb @('shell','am','force-stop',$taskPackage))
$taskArchive=Invoke-PrivateAdb @('exec-out','run-as',$taskPackage,'tar','cf','-','shared_prefs','databases')
if(-not (Test-Path (Join-Path $taskBackup 'encrypted-app-state.tar'))){[IO.File]::WriteAllBytes((Join-Path $taskBackup 'encrypted-app-state.tar'),$taskArchive)}
$taskPath='shared_prefs/butler_device_credentials_prefs.xml'
$taskOriginal=Invoke-PrivateAdb @('exec-out','run-as',$taskPackage,'cat',$taskPath)
if(-not (Test-Path (Join-Path $taskBackup 'preferences-before.xml'))){[IO.File]::WriteAllBytes((Join-Path $taskBackup 'preferences-before.xml'),$taskOriginal)}
$taskText=[Text.Encoding]::UTF8.GetString($taskOriginal)
$taskOld='http://192.168.0.112:18061'
$taskNew='http://192.168.0.5:18061'
if(([regex]::Matches($taskText,[regex]::Escape($taskOld))).Count -ne 1){throw 'Expected exactly the old owner endpoint'}
[xml]$taskDoc=$taskText
$taskIdentity=($taskDoc.map.string|Where-Object name -eq 'device_id').'#text'
$taskToken=($taskDoc.map.string|Where-Object name -eq 'encrypted_device_token').'#text'
if(-not $taskIdentity -or -not $taskToken){throw 'Existing paired encrypted identity required'}
$taskReplaced=$taskText.Replace($taskOld,$taskNew)
[xml]$taskCheck=$taskReplaced
if(($taskCheck.map.string|Where-Object name -eq 'encrypted_device_token').'#text' -cne $taskToken){throw 'Credential changed'}
if(($taskCheck.map.string|Where-Object name -eq 'device_id').'#text' -cne $taskIdentity){throw 'Device identity changed'}
$taskBytes=[Text.UTF8Encoding]::new($false).GetBytes($taskReplaced)
$script:taskAdbPhase='private endpoint write'
[void](Invoke-PrivateAdb @('exec-in','run-as',$taskPackage,'tee',"$taskPath.relocation") $taskBytes)
$script:taskAdbPhase='atomic endpoint commit'
[void](Invoke-PrivateAdb @('shell','run-as',$taskPackage,'chmod','600',"$taskPath.relocation"))
[void](Invoke-PrivateAdb @('shell','run-as',$taskPackage,'mv',"$taskPath.relocation",$taskPath))
[void](Invoke-PrivateAdb @('shell','run-as',$taskPackage,'rm','-f',"$taskPath.bak"))
$taskAfter=Invoke-PrivateAdb @('exec-out','run-as',$taskPackage,'cat',$taskPath)
if([Text.Encoding]::UTF8.GetString($taskAfter) -cne $taskReplaced){throw 'Relocated preferences read-back mismatch'}
$script:taskAdbPhase='launch relocated application'
[void](Invoke-PrivateAdb @('shell','am','start','-n',"$taskPackage/com.example.MainActivity"))
[void](Invoke-PrivateAdb @('shell','cmd','notification','allow_listener',"$taskPackage/com.example.service.ButlerNotificationListenerService"))
@{relocated=$true;server=$taskNew;device_identity_preserved=$true;encrypted_token_preserved=$true;queue_files_preserved=$true;private_backup=$taskBackup}|ConvertTo-Json
