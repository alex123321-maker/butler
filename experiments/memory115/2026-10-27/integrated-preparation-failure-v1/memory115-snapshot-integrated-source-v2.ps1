$ErrorActionPreference='Stop'
$snapshotCommit='baaf94ef6298258d5590709cd649541ab371e93f'
$snapshotRepo=(Get-Location).Path
$snapshotRoot=Join-Path $snapshotRepo '.cache/memory115-source-skillprep-baaf-v2'
$snapshotTar=Join-Path $snapshotRepo '.cache/memory115-source-skillprep-baaf-v2.tar'
$snapshotInventory=Join-Path $snapshotRepo '.cache/memory115-source-skillprep-baaf-v2-inventory.json'
foreach($path in @($snapshotRoot,$snapshotTar,$snapshotInventory)){if(Test-Path -LiteralPath $path){throw 'Snapshot identity already exists'}}
if((git rev-parse HEAD).Trim() -cne $snapshotCommit -or (git status --porcelain | Out-String).Trim()){throw 'Exact clean integrated producer required'}

$snapshotExport='D:\CodexData\private\butler115-export-baaf-v2.git'
if(Test-Path -LiteralPath $snapshotExport){throw 'Private export repository identity exists'}
$snapshotObjectPath=(git rev-parse --git-path objects).Trim()
if(-not [IO.Path]::IsPathRooted($snapshotObjectPath)){$snapshotObjectPath=Join-Path $snapshotRepo $snapshotObjectPath}
$snapshotObjectPath=[IO.Path]::GetFullPath($snapshotObjectPath).Replace('\','/')
git init --bare --quiet $snapshotExport
if($LASTEXITCODE){throw 'Private export repository preparation failed'}
[IO.File]::WriteAllText((Join-Path $snapshotExport 'objects/info/alternates'),$snapshotObjectPath+"`n",[Text.UTF8Encoding]::new($false))
# Only this private repository's highest-priority attributes are changed. Shared
# repository/worktrees/config remain untouched. Complete raw blob checks follow.
[IO.File]::WriteAllText((Join-Path $snapshotExport 'info/attributes'),"* -text -eol -ident -filter -export-ignore -export-subst`n",[Text.UTF8Encoding]::new($false))
# Git supplies immutable bytes. No shell pipeline/text transcoding or author
# working-tree/export-filter trust is used: every extracted blob is checked.
function Invoke-SnapshotGit([string[]]$GitSnapshotArgs,[string]$FileOutput='') {
 $start=[Diagnostics.ProcessStartInfo]::new()
 $start.FileName=(Get-Command git).Source
 $start.WorkingDirectory=$snapshotRepo
 $start.UseShellExecute=$false
 $start.CreateNoWindow=$true
 $start.RedirectStandardOutput=$true
 $start.RedirectStandardError=$true
 $start.ArgumentList.Add("--git-dir="+$snapshotExport)
 foreach($argument in $GitSnapshotArgs){$start.ArgumentList.Add($argument)}
 $process=[Diagnostics.Process]::new();$process.StartInfo=$start
 $null=$process.Start()
 $errorTask=$process.StandardError.ReadToEndAsync()
 $output=if($FileOutput){[IO.File]::Open($FileOutput,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None)}else{[IO.MemoryStream]::new()}
 try {
  $process.StandardOutput.BaseStream.CopyTo($output)
  $process.WaitForExit()
  $null=$errorTask.GetAwaiter().GetResult()
  if($process.ExitCode -ne 0){throw 'Immutable Git extraction failed'}
  if($FileOutput){$output.Flush($true)}else{return ,$output.ToArray()}
 } finally {$output.Dispose();if(-not $process.HasExited){$process.Kill($true);$process.WaitForExit()};$process.Dispose()}
}

$treeBytes=Invoke-SnapshotGit @('ls-tree','-r','-l','-z','--full-tree',$snapshotCommit)
if($treeBytes.Length -gt 16MB){throw 'Source tree metadata exceeds bound'}
$treeText=[Text.UTF8Encoding]::new($false,$true).GetString($treeBytes)
$snapshotEntries=[Collections.Generic.List[object]]::new()
$snapshotNames=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
[long]$snapshotTotal=0
foreach($record in $treeText.Split([char]0,[StringSplitOptions]::RemoveEmptyEntries)){
 if($record -cnotmatch '^([0-7]{6}) ([a-z]+) ([0-9a-f]{40})\s+([0-9-]+)\t(.+)$'){throw 'Unexpected Git tree record'}
 $mode=$Matches[1];$kind=$Matches[2];$blob=$Matches[3];$bytesText=$Matches[4];$relative=$Matches[5]
 if($mode -cnotin @('100644','100755') -or $kind -cne 'blob' -or $bytesText -cnotmatch '^[0-9]+$'){throw 'Nonregular source rejected'}
 if($relative -match '(^|/)\.\.(/|$)|[\\:\x00-\x1f]|^/' -or -not $snapshotNames.Add($relative)){throw 'Ambiguous source path rejected'}
 foreach($segment in $relative.Split('/')){if($segment -match '[. ]$|^(?i:con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\.|$)'){throw 'Unsafe host source path'}}
 $length=[long]$bytesText
 if($length -gt 512MB){throw 'Source member exceeds existing native source bound'}
 $snapshotTotal+=$length
 if($snapshotTotal -gt 1GB -or $snapshotEntries.Count -ge 20000){throw 'Source snapshot exceeds finite preparation bound'}
 $snapshotEntries.Add([ordered]@{path=$relative;mode=$mode;bytes=$length;git_blob_sha1=$blob})
}
if($snapshotEntries.Count -eq 0 -or -not $snapshotNames.Contains('go.mod')){throw 'Complete Go source missing'}
Invoke-SnapshotGit @('archive','--format=tar',$snapshotCommit) $snapshotTar
New-Item -ItemType Directory -Path $snapshotRoot|Out-Null
[System.Formats.Tar.TarFile]::ExtractToDirectory($snapshotTar,$snapshotRoot,$false)
$physical=@(Get-ChildItem -LiteralPath $snapshotRoot -Recurse -Force -File)
if($physical.Count -ne $snapshotEntries.Count){throw 'Export filters omitted or added source files; retain failed snapshot'}
$buffer=[byte[]]::new(65536)
foreach($entry in $snapshotEntries){
 $path=Join-Path $snapshotRoot $entry.path
 $info=Get-Item -LiteralPath $path -Force
 if($info.Attributes -band [IO.FileAttributes]::ReparsePoint -or $info.Length -ne $entry.bytes){throw 'Extracted member differs'}
 $blobHash=[Security.Cryptography.IncrementalHash]::CreateHash([Security.Cryptography.HashAlgorithmName]::SHA1)
 $contentHash=[Security.Cryptography.IncrementalHash]::CreateHash([Security.Cryptography.HashAlgorithmName]::SHA256)
 $blobHash.AppendData([Text.Encoding]::UTF8.GetBytes(('blob '+$info.Length+[char]0)))
 $stream=[IO.File]::OpenRead($path)
 try {while(($count=$stream.Read($buffer,0,$buffer.Length)) -gt 0){$blobHash.AppendData($buffer,0,$count);$contentHash.AppendData($buffer,0,$count)}}finally{$stream.Dispose()}
 $actualBlob=[Convert]::ToHexString($blobHash.GetHashAndReset()).ToLowerInvariant()
 $entry.sha256=[Convert]::ToHexString($contentHash.GetHashAndReset()).ToLowerInvariant()
 $blobHash.Dispose();$contentHash.Dispose()
 if($actualBlob -cne $entry.git_blob_sha1){throw 'Git export substitution or byte drift; retain failed snapshot'}
 $info.IsReadOnly=$true
}
$receipt=[ordered]@{version='issue115-integrated-immutable-source-snapshot-v2';producer=$snapshotCommit;created_utc=[DateTimeOffset]::UtcNow.ToString('o');files=$snapshotEntries.Count;bytes=$snapshotTotal;all_tracked_blobs_exact=$true;export_filters_not_trusted=$true;links_submodules_aliases_rejected=$true;readonly_files=$true;private_material_copied=$false;native_auth_calls=0;entries=$snapshotEntries}
[IO.File]::WriteAllText($snapshotInventory,($receipt|ConvertTo-Json -Depth 10)+"`n",[Text.UTF8Encoding]::new($false))
[pscustomobject]@{producer=$snapshotCommit;files=$snapshotEntries.Count;bytes=$snapshotTotal;all_blobs_exact=$true;inventory_sha256=(Get-FileHash -LiteralPath $snapshotInventory).Hash.ToLowerInvariant();native_auth_calls=0}
