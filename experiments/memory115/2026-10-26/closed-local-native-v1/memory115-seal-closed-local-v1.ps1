$ErrorActionPreference='Stop'
$repo='D:\CodexData\home\worktrees\issue106-local\butler'
$private='\\wsl.localhost\docker-desktop\mnt\host\wsl\butler-effort76\private'
$root=Join-Path $private 'memory115-execution-native-20261026-v2'
$actual=Join-Path $root 'actual'
$output=Join-Path $repo '.cache/memory115-closed-local-v1'
if(Test-Path -LiteralPath $output){throw 'Closed evidence identity already exists'}
$hand=Get-Content -LiteralPath (Join-Path $root 'local-handback.json') -Raw|ConvertFrom-Json -AsHashtable -Depth 30
foreach($field in @('cmd_wait_returned','collector_started','whole_cgroup_empty','native_database_backends_gone','opaque_source_cas','local_auth_resolved','fresh_local_encrypted_snapshot_saved')){if($hand[$field] -ne $true){throw 'Final local handback proof incomplete'}}
if($hand.forced_cleanup -or $hand.raw_material_printed -or $hand.release_qualified){throw 'Unexpected handback disposition'}

# Read private material only inside this local process, for a publication guard.
# No value, plaintext, encoded secret or key is written to output/arguments/logs.
$key=[IO.File]::ReadAllText((Join-Path $private 'behavior76/settings.key')).Trim()
$keyBytes=[Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($key))
$secrets=[Collections.Generic.List[string]]::new()
$secrets.Add($key)
$aes=[Security.Cryptography.AesGcm]::new($keyBytes,16)
try {
 foreach($name in @('expected-source.value','auth-import.value','auth-return.value','source-after-native.value')){
  $cipher=[IO.File]::ReadAllText((Join-Path $root $name)).Trim()
  $secrets.Add($cipher)
  $bytes=[Convert]::FromBase64String($cipher)
  if($bytes.Length -lt 29 -or $bytes.Length -gt 131072){throw 'Private snapshot bound invalid'}
  $nonce=[byte[]]$bytes[0..11]
  $payload=[byte[]]$bytes[12..($bytes.Length-17)]
  $tag=[byte[]]$bytes[($bytes.Length-16)..($bytes.Length-1)]
  $plain=[byte[]]::new($payload.Length)
  $aes.Decrypt($nonce,$payload,$tag,$plain,[Text.Encoding]::UTF8.GetBytes('BUTLER_PROVIDER_AUTH_ANTIGRAVITY'))
  $credentials=[Text.Encoding]::UTF8.GetString($plain)|ConvertFrom-Json -AsHashtable -Depth 10
  foreach($field in @('access_token','refresh_token','client_secret')){if(-not[string]::IsNullOrEmpty($credentials[$field])){$secrets.Add([string]$credentials[$field])}}
  [Array]::Clear($plain)
 }
}finally{$aes.Dispose();[Array]::Clear($keyBytes)}
$patterns=@($secrets|Sort-Object -Unique)
$files=@(Get-ChildItem -LiteralPath $actual -File -Recurse|Sort-Object FullName)
if($files.Count -lt 1 -or $files.Count -gt 10000 -or ($files|Measure-Object Length -Sum).Sum -gt 536870912){throw 'Evidence bound invalid'}
$extras=@('local-handback.json','process-exit.json','execution-consumed.json','runner.log','pre-auth-pins-check.log')
$scan=@($files)+@($extras|ForEach-Object {Get-Item -LiteralPath (Join-Path $root $_)})
foreach($file in $scan){
 if($file.Length -gt 67108864 -or ($file.Attributes-band[IO.FileAttributes]::ReparsePoint)){throw 'Nonregular or oversized evidence'}
 $text=[Text.Encoding]::UTF8.GetString([IO.File]::ReadAllBytes($file.FullName))
 foreach($secret in $patterns){if($text.Contains($secret,[StringComparison]::Ordinal)){throw 'Publication blocked: private authorization material detected'}}
}
$secrets.Clear();$patterns=$null;$credentials=$null;$key=$null;$cipher=$null;$text=$null
[IO.Directory]::CreateDirectory($output)|Out-Null
$inventory=@($files|ForEach-Object {
 $rel=[IO.Path]::GetRelativePath($actual,$_.FullName).Replace('\','/')
 if($rel.StartsWith('../') -or [IO.Path]::IsPathRooted($rel)){throw 'Evidence path escaped'}
 [ordered]@{path=$rel;bytes=$_.Length;sha256=(Get-FileHash -LiteralPath $_.FullName).Hash.ToLowerInvariant()}
})
[IO.File]::WriteAllText((Join-Path $output 'raw-inventory.json'),($inventory|ConvertTo-Json -Depth 5)+"`n",[Text.UTF8Encoding]::new($false))
Add-Type -AssemblyName System.IO.Compression
$archivePath=Join-Path $output 'raw-evidence.zip'
$zip=[IO.Compression.ZipFile]::Open($archivePath,[IO.Compression.ZipArchiveMode]::Create)
try{foreach($item in $inventory){[IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip,(Join-Path $actual $item.path),$item.path,[IO.Compression.CompressionLevel]::Optimal)|Out-Null}}finally{$zip.Dispose()}
$zip=[IO.Compression.ZipFile]::OpenRead($archivePath)
try{
 if($zip.Entries.Count -ne $inventory.Count){throw 'Archive count mismatch'}
 foreach($item in $inventory){$entry=$zip.GetEntry($item.path);$stream=$entry.Open();try{$hash=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($stream)).ToLowerInvariant()}finally{$stream.Dispose()};if($hash-cne$item.sha256-or$entry.Length-ne$item.bytes){throw 'Archive entry byte mismatch'}}
}finally{$zip.Dispose()}
foreach($name in $extras){Copy-Item -LiteralPath (Join-Path $root $name) -Destination (Join-Path $output $name)}
& wsl -d docker-desktop --exec /bin/sh -c 'find /mnt/host/wsl/butler-effort76/private/memory115-execution-native-20261026-v2/actual -type f -exec chmod a-w {} +'
if($LASTEXITCODE -ne 0){throw 'Closed source read-only seal failed'}
$receipt=[ordered]@{version='butler115-closed-local-raw-seal.v1';result='NATIVE_FAILED_LOCAL_AUTH_HANDBACK_PROVED_NOT_QUALIFIED';files=$files.Count;bytes=($files|Measure-Object Length -Sum).Sum;inventory_sha256=(Get-FileHash (Join-Path $output 'raw-inventory.json')).Hash.ToLowerInvariant();archive_sha256=(Get-FileHash $archivePath).Hash.ToLowerInvariant();every_archive_entry_byte_verified=$true;private_provider_material_publication_scan_pass=$true;private_material_written=$false;local_handback_proved=$true;release_qualified=$false;replayed=$false}
[IO.File]::WriteAllText((Join-Path $output 'raw-seal.json'),($receipt|ConvertTo-Json -Depth 10)+"`n",[Text.UTF8Encoding]::new($false))
$receipt|ConvertTo-Json -Depth 10
