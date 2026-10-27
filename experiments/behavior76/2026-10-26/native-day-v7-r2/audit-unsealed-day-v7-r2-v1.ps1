$ErrorActionPreference='Stop'
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$root=Join-Path $repo '.cache/behavior76/native-day-v7-r2'
$source=Join-Path $PSScriptRoot 'authority-stream-forensic-unsealed-v1.cs'
if((Get-FileHash $source).Hash.ToLowerInvariant() -cne 'dbadf5b0cc99500d72094f04962e18117bb8a6b4a3e4051d350ca45693fe4110'){throw 'Forensic reader changed'}
Add-Type -TypeDefinition ([IO.File]::ReadAllText($source))
$index=Get-Content (Join-Path $root 'index.json') -Raw|ConvertFrom-Json -AsHashtable
$manifest=Get-Content (Join-Path $root 'manifest.json') -Raw|ConvertFrom-Json -AsHashtable -Depth 50
if($index.episodes.Count -ne 1){throw 'One indexed arm required'}
$case=$index.episodes[0];$arm=$case.tables_dir.Substring(0,$case.tables_dir.Length-7)
$directory=Join-Path $root $arm;$headerPath=Join-Path $directory 'authority-trace.json'
$h=Get-Content $headerPath -Raw|ConvertFrom-Json -AsHashtable -Depth 30
$identity=@{window=$manifest.window;arm=$arm;schema=$h.identity.schema;producer=$manifest.source;protocol_sha256=$manifest.protocol_sha256}
$hash=(Get-FileHash $headerPath).Hash.ToLowerInvariant()
$validation=ConvertFrom-Json ([Issue76AuthorityForensicUnsealedV1]::Read($directory,(ConvertTo-Json $identity -Compress),$hash)) -AsHashtable
$result=[ordered]@{version='issue76-day-v7-r2-unsealed-stream-forensics-v1';header_sha256=$hash;forensic_source_sha256='dbadf5b0cc99500d72094f04962e18117bb8a6b4a3e4051d350ca45693fe4110';stream=$validation;schema_binding='header_claim_only_due_failed_quiescence';requires_actual_unsealed_header=$true;actual_raw_unchanged=$true;native_calls=0;pg_calls=0;mapping_read=$false;goldens_read=$false;quality_assessed=$false;qualification=$false}
$path=Join-Path $PSScriptRoot 'public-unsealed-day-v7-r2-forensics-v1.json'
$f=[IO.File]::Open($path,[IO.FileMode]::CreateNew);try{$b=[Text.UTF8Encoding]::new($false).GetBytes((ConvertTo-Json $result -Depth 20)+"`n");$f.Write($b)}finally{$f.Dispose()}
(Get-Item $path).IsReadOnly=$true
@{stream=$validation;receipt_sha256=(Get-FileHash $path).Hash.ToLowerInvariant()}|ConvertTo-Json -Depth 12
