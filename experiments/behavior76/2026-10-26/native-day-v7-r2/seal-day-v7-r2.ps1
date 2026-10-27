$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath('.cache/behavior76/native-day-v7-r2')
$files = @(Get-ChildItem -LiteralPath $root -File -Recurse | Sort-Object FullName)
$inventory = @($files | ForEach-Object {
  [ordered]@{path=[IO.Path]::GetRelativePath($root,$_.FullName).Replace('\','/');bytes=$_.Length;sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
})
$inv = '.cache/behavior76/native-day-v7-r2-raw-inventory-v1.json'
if (Test-Path $inv) { throw 'Old inventory preserved' }
[IO.File]::WriteAllText([IO.Path]::GetFullPath($inv),($inventory | ConvertTo-Json -Depth 5)+"`n",[Text.UTF8Encoding]::new($false))
foreach ($file in $files) { $file.IsReadOnly = $true }
$zipPath = [IO.Path]::GetFullPath('.cache/behavior76/native-day-v7-r2-raw-evidence-v1.zip')
if (Test-Path $zipPath) { throw 'Old archive preserved' }
Add-Type -AssemblyName System.IO.Compression
$zip = [IO.Compression.ZipFile]::Open($zipPath,[IO.Compression.ZipArchiveMode]::Create)
try {
  foreach ($item in $inventory) {
    [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip,(Join-Path $root $item.path),$item.path,[IO.Compression.CompressionLevel]::Optimal) | Out-Null
  }
} finally { $zip.Dispose() }
$archive = [IO.Compression.ZipFile]::OpenRead($zipPath)
try {
  if ($archive.Entries.Count -ne $inventory.Count) { throw 'Archive count differs' }
  foreach ($item in $inventory) {
    $entry = $archive.GetEntry($item.path)
    $stream = $entry.Open()
    try { $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($stream)).ToLowerInvariant() }
    finally { $stream.Dispose() }
    if ($hash -ne $item.sha256 -or $entry.Length -ne $item.bytes) { throw 'Archive bytes differ' }
  }
} finally { $archive.Dispose() }
$receipt = [ordered]@{
  version='issue76-native-day-v7-r2-raw-seal-v1'
  window='issue76-fixture-day-20261026T050548'
  producer='42c01c3a1912bc8ae401566ad918f3adfa4c61ba'
  files=$inventory.Count
  bytes=($files | Measure-Object Length -Sum).Sum
  inventory_sha256=(Get-FileHash $inv -Algorithm SHA256).Hash.ToLowerInvariant()
  archive_sha256=(Get-FileHash $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
  every_archive_entry_byte_verified=$true
  all_windows_files_read_only=(@($files | Where-Object { -not $_.IsReadOnly }).Count -eq 0)
  quality_scored=$false;mapping_opened=$false;goldens_opened=$false
  result='CLOSED_GLOBAL_PROTOCOL_FAILURE_NOT_QUALIFIED'
}
$receipt | ConvertTo-Json | Set-Content -Encoding utf8NoBOM .cache/behavior76/native-day-v7-r2-raw-seal-v1.json
$receipt | ConvertTo-Json
