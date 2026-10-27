# Additive representation prevalidation. The original scorer/adapter run verbatim.
function Invoke-Issue76StreamScoringFilesV2 {
 [CmdletBinding()]
 param([Parameter(Mandatory)][string]$ReportRoot,[Parameter(Mandatory)][string]$StimuliPath,[Parameter(Mandatory)][string]$GoldensPath,[Parameter(Mandatory)][hashtable]$Pin,[Parameter(Mandatory)][string]$FrozenScorerPath,[Parameter(Mandatory)][string]$AdapterPath,[Parameter(Mandatory)][string]$CompositionPath)
 $ErrorActionPreference='Stop'
 foreach($p in @(
  @((Join-Path $PSScriptRoot 'authority-stream-reader-v2a.cs'),'836e02f4d40d0c52449f32f8560102a8ed5e4f06ff0b95a8f5dd0a5128a74641'),
  @((Join-Path $PSScriptRoot 'authority-stream-reader-v2a.ps1'),'b03c60b3fbcdd4ee8613545738951ecbfc9f7f204409aaea20de70a25d0332c5'),
  @($CompositionPath,'8719c35f525a0d3d417e9cd0ee037fe357462a3827585dde14426723bef9ed63'),
  @($AdapterPath,'897beea0409d4fb526ff434ba03ab51df07b6c919b8c680d81cc97d6176b32fd'),
  @($FrozenScorerPath,'e481f9c062a3bc4500b9f0b94da8819fbf287bec075490583cad17d54198721d')
 )){if((Get-FileHash -LiteralPath $p[0]).Hash.ToLowerInvariant() -cne $p[1]){throw 'Streaming/frozen source pin mismatch'}}
 . (Join-Path $PSScriptRoot 'authority-stream-reader-v2a.ps1')
 Import-Issue76AuthorityStreamReader
 $root=(Resolve-Path -LiteralPath $ReportRoot).Path
 foreach($f in @(@('index.json','index_sha256'),@('manifest.json','manifest_sha256'))){$path=[Issue76AuthorityStreamV2A]::SafePath($root,$f[0]);if((Get-FileHash -LiteralPath $path).Hash.ToLowerInvariant() -cne $Pin[$f[1]]){throw 'Streaming root hash mismatch'}}
 $index=ConvertFrom-Json ([IO.File]::ReadAllText((Join-Path $root 'index.json'))) -AsHashtable -Depth 100
 $manifest=ConvertFrom-Json ([IO.File]::ReadAllText((Join-Path $root 'manifest.json'))) -AsHashtable -Depth 100
 if($manifest.window -cne $Pin.window -or $manifest.source -cne $Pin.producer -or $manifest.protocol_sha256 -cne $Pin.protocol_sha256){throw 'Streaming root identity mismatch'}
 if(@($index.episodes).Count -gt 48){throw 'Streaming arm bound changed'}
 $validations=[Collections.Generic.List[object]]::new();$seen=@{}
 foreach($case in $index.episodes){
  $tables=[string]$case.tables_dir
  if($tables -cnotmatch '^slot-[0-9]{2}/[A-Za-z0-9_-]+/tables$'){throw 'Streaming case path invalid'}
  $arm=$tables.Substring(0,$tables.Length-7)
  if($seen.ContainsKey($arm)){throw 'Streaming duplicate arm'};$seen[$arm]=$true
  $qpath=[Issue76AuthorityStreamV2A]::SafePath($root,($arm+'/quiescence.json'))
  $q=ConvertFrom-Json ([IO.File]::ReadAllText($qpath)) -AsHashtable
  $sealpath=[Issue76AuthorityStreamV2A]::SafePath($root,($arm+'/sealed-files.json'))
  $seal=ConvertFrom-Json ([IO.File]::ReadAllText($sealpath)) -AsHashtable -Depth 30
  $tracePin=@($seal.files|Where-Object path -CEQ 'authority-trace.json')
  if($tracePin.Count -ne 1){throw 'Streaming header not uniquely sealed'}
  $identity=@{window=$Pin.window;arm=$arm;schema=$q.schema;producer=$Pin.producer;protocol_sha256=$Pin.protocol_sha256}
  $validation=Read-Issue76AuthorityStream -Directory (Join-Path $root $arm) -ExpectedIdentity $identity -HeaderSha256 $tracePin[0].sha256
  # The unchanged adapter independently verifies every segment's inventory bytes,
  # all durable joins, stops, quiescence and snapshots after stream validation.
  $header=ConvertFrom-Json ([IO.File]::ReadAllText((Join-Path $root ($arm+'/authority-trace.json')))) -AsHashtable -Depth 30
  foreach($segment in $header.segments){$listed=@($seal.files|Where-Object path -CEQ $segment.path);if($listed.Count -ne 1 -or $listed[0].sha256 -cne $segment.sha256 -or $listed[0].bytes -ne $segment.compressed_bytes){throw 'Streaming segment not exactly sealed'}}
  $validations.Add(@{arm=$arm;header_sha256=$tracePin[0].sha256;validation=$validation})
 }
 . $CompositionPath
 $result=Invoke-Issue76LocalScoringFilesV2 -ReportRoot $root -StimuliPath $StimuliPath -GoldensPath $GoldensPath -Pin $Pin -FrozenScorerPath $FrozenScorerPath -AdapterPath $AdapterPath
 $result['authority_stream_prevalidation']=@{version='issue76-stream-prevalidation-v2-model-tail';traces=$validations.ToArray();criteria_changed=$false;thresholds_changed=$false}
 return $result
}
