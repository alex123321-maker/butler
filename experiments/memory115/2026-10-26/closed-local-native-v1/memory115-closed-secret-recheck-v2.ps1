$ErrorActionPreference='Stop'
$repo='D:\CodexData\home\worktrees\issue106-local\butler'
$private='\\wsl.localhost\docker-desktop\mnt\host\wsl\butler-effort76\private'
$root=Join-Path $private 'memory115-execution-native-20261026-v2'
$actual=Join-Path $root 'actual'
$output=Join-Path $repo '.cache/memory115-closed-local-v1'

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
}catch{throw 'Private snapshot validation failed; material withheld'}finally{$aes.Dispose();[Array]::Clear($keyBytes)}
$patterns=@($secrets|Sort-Object -Unique -CaseSensitive)
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

Write-Output 'Ordinal private-material publication scan PASS; no values disclosed.'
