$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'authority-stream-reader-v2a.ps1')
Import-Issue76AuthorityStreamReader
$testRoot=Join-Path $PSScriptRoot ('public-stream-selftests-'+[guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($testRoot)|Out-Null
$id=@{window='public-window';arm='slot-01/public-arm';schema='public_schema';producer=('1'*40);protocol_sha256=('2'*64)}
$script:passed=[Collections.Generic.List[string]]::new()
function Hash([string]$p){(Get-FileHash -LiteralPath $p).Hash.ToLowerInvariant()}
function Event([int]$seq,[int]$span,[string]$edge,[string]$outcome=''){
 if(-not $outcome){$outcome=if($edge -ceq 'start'){'pending'}else{'ok'}}
 @{sequence=$seq;span=$span;phase='query';edge=$edge;at_utc='2026-10-26T00:00:00Z';elapsed_us=0;original_context_deadline_utc=$null;original_context_time_left_us=$null;query_family='other_query';outcome=$outcome;context_cause='none';pool=@{available=$false;max_conns=0;total_conns=0;acquired_conns=0;idle_conns=0}}
}
function Fixture([string]$name,[object[]]$events,[int[]]$sizes,[object[]]$tail=@()){
 $dir=Join-Path $testRoot $name;[IO.Directory]::CreateDirectory($dir)|Out-Null
 $h=@{version='behavior76-authority-pg-trace-stream-v2';sealed=$true;event_count=$events.Count+$tail.Count;last_sequence=$events.Count+$tail.Count;maximum_buffered_events=4096;maximum_segments=16384;maximum_compressed_bytes=536870912;total_compressed_bytes=0;identity=$id.Clone();segments=@();tail=$tail;open_spans=0;dropped_events=0;operation_failures=@($events+$tail|Where-Object {$_['edge'] -ceq 'end' -and $_['outcome'] -cne 'ok'}).Count;observer_failed=$false}
 $offset=0;$n=0
 foreach($size in $sizes){
  $part=@($events[$offset..($offset+$size-1)]);$raw=[Text.Encoding]::UTF8.GetBytes((($part|ForEach-Object {$_|ConvertTo-Json -Compress -Depth 20}) -join "`n")+"`n")
  $rel='authority-segment-'+(++$n).ToString('D6')+'.jsonl.gz';$path=Join-Path $dir $rel
  $file=[IO.File]::Open($path,[IO.FileMode]::CreateNew);$gz=[IO.Compression.GZipStream]::new($file,[IO.Compression.CompressionLevel]::Optimal)
  try{$gz.Write($raw,0,$raw.Length)}finally{$gz.Dispose();$file.Dispose()}
  $cb=(Get-Item -LiteralPath $path).Length
  $h.segments+=@{path=$rel;first_sequence=$offset+1;last_sequence=$offset+$size;event_count=$size;compressed_bytes=$cb;uncompressed_bytes=$raw.Length;sha256=(Hash $path);uncompressed_sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($raw)).ToLowerInvariant()}
  $h.total_compressed_bytes+=$cb;$offset+=$size
 }
 if($offset -ne $events.Count){throw 'Public selftest malformed'}
 @{dir=$dir;header=$h}
}
function Header($f){$p=Join-Path $f.dir 'authority-trace.json';[IO.File]::WriteAllText($p,($f.header|ConvertTo-Json -Depth 30),[Text.UTF8Encoding]::new($false));Hash $p}
function ReplaceRaw($f,[byte[]]$raw){
 $seg=$f.header.segments[0];$p=Join-Path $f.dir $seg.path;$file=[IO.File]::Open($p,[IO.FileMode]::Create);$gz=[IO.Compression.GZipStream]::new($file,[IO.Compression.CompressionLevel]::Optimal)
 try{$gz.Write($raw,0,$raw.Length)}finally{$gz.Dispose();$file.Dispose()}
 $seg.compressed_bytes=(Get-Item -LiteralPath $p).Length;$seg.uncompressed_bytes=$raw.Length;$seg.sha256=Hash $p;$seg.uncompressed_sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($raw)).ToLowerInvariant();$f.header.total_compressed_bytes=$seg.compressed_bytes
}
function Check([string]$name,$f,[bool]$valid,[scriptblock]$mutate={},[hashtable]$identity=$id){
 & $mutate $f
 $sha=Header $f
 try{$r=Read-Issue76AuthorityStream -Directory $f.dir -ExpectedIdentity $identity -HeaderSha256 $sha;if(-not $valid){throw 'SELFTEST_FALSE_ACCEPT'};if(-not $r.qualified -or $r.quality_assessed){throw 'SELFTEST_BAD_RESULT'}}
 catch{if($valid -or $_.Exception.Message.Contains('SELFTEST_')){throw};if(-not $_.Exception.Message.Contains('authority_stream:')){throw}}
 $script:passed.Add($name)
}
$ev=@((Event 1 1 start),(Event 2 2 start),(Event 3 1 end),(Event 4 2 end))
Check 'cross-segment-span-pairing' (Fixture good $ev @(2,2)) $true
Check 'bounded-tail-pairing' (Fixture tail $ev[0..1] @(2) $ev[2..3]) $true
Check 'empty-closed-trace' (Fixture empty @() @()) $true
$err=@((Event 1 1 start),(Event 2 1 end cancelled))
Check 'ordinary-cancelled-error-is-retained' (Fixture errors $err @(2)) $true
Check 'error-counter-underreport' (Fixture errorsbad $err @(2)) $false {param($f)$f.header.operation_failures=0}
Check 'identity-mismatch' (Fixture identity $ev @(4)) $false {param($f)$f.header.identity.window='foreign'}
Check 'unsealed' (Fixture unsealed $ev @(4)) $false {param($f)$f.header.sealed=$false}
Check 'observer-failed' (Fixture failed $ev @(4)) $false {param($f)$f.header.observer_failed=$true}
Check 'dropped-event' (Fixture drop $ev @(4)) $false {param($f)$f.header.dropped_events=1}
Check 'reported-open-span' (Fixture open $ev @(4)) $false {param($f)$f.header.open_spans=1}
Check 'wrong-total' (Fixture total $ev @(4)) $false {param($f)$f.header.last_sequence=5}
Check 'wrong-limit' (Fixture limit $ev @(4)) $false {param($f)$f.header.maximum_buffered_events=8192}
Check 'reordered-segments' (Fixture order $ev @(2,2)) $false {param($f)$f.header.segments=@($f.header.segments[1],$f.header.segments[0])}
Check 'path-traversal' (Fixture path $ev @(4)) $false {param($f)$f.header.segments[0].path='../authority-segment-000001.jsonl.gz'}
Check 'missing-segment' (Fixture absent $ev @(4)) $false {param($f)[IO.File]::Move((Join-Path $f.dir $f.header.segments[0].path),(Join-Path $f.dir 'moved.gz'))}
Check 'compressed-byte-tamper' (Fixture tamper $ev @(4)) $false {param($f)$p=Join-Path $f.dir $f.header.segments[0].path;$b=[IO.File]::ReadAllBytes($p);$b[12]=$b[12] -bxor 1;[IO.File]::WriteAllBytes($p,$b)}
Check 'raw-hash-tamper' (Fixture rawhash $ev @(4)) $false {param($f)$f.header.segments[0].uncompressed_sha256='0'*64}
Check 'raw-size-underreported' (Fixture bomb $ev @(4)) $false {param($f)$f.header.segments[0].uncompressed_bytes=4}
Check 'segment-range' (Fixture range $ev @(4)) $false {param($f)$f.header.segments[0].first_sequence=2}
$bad=@((Event 1 1 start),(Event 3 1 end));Check 'sequence-gap' (Fixture gap $bad @(2)) $false
$bad=@((Event 1 1 start),(Event 2 1 start));Check 'duplicate-start' (Fixture duplicate $bad @(2)) $false
$bad=@((Event 1 1 start),(Event 2 2 end));Check 'foreign-end' (Fixture foreign $bad @(2)) $false
$bad=@((Event 1 2 start),(Event 2 2 end));Check 'nonmonotonic-span-allocation' (Fixture span $bad @(2)) $false
$bad=@((Event 1 1 start));Check 'unclosed-real-span' (Fixture unmatched $bad @(1)) $false
$bad=@((Event 1 1 start),(Event 2 1 end));$bad[1].phase='pool_acquire';Check 'phase-mismatch' (Fixture phase $bad @(2)) $false
$bad=@((Event 1 1 start),(Event 2 1 end));$bad[1].sql='SELECT private';Check 'unknown-sensitive-field' (Fixture unknown $bad @(2)) $false
$bad=@((Event 1 1 start),(Event 2 1 end));$bad[1].span=1.5;Check 'fractional-identity' (Fixture fraction $bad @(2)) $false
$bad=@((Event 1 1 start),(Event 2 1 end));$bad[1].edge='unpaired_end';Check 'unpaired-end' (Fixture unpaired $bad @(2)) $false
Check 'unknown-header-field' (Fixture unknownheader $ev @(4)) $false {param($f)$f.header.events=@()}
Check 'compressed-total-tamper' (Fixture compressedtotal $ev @(4)) $false {param($f)$f.header.total_compressed_bytes++}
$script:rawText=(($ev|ForEach-Object {$_|ConvertTo-Json -Compress -Depth 20}) -join "`n")+"`n"
Check 'missing-terminal-lf' (Fixture missinglf $ev @(4)) $false {param($f)ReplaceRaw $f ([Text.Encoding]::UTF8.GetBytes($script:rawText.TrimEnd("`n")))}
Check 'crlf-not-exact-lf' (Fixture crlf $ev @(4)) $false {param($f)ReplaceRaw $f ([Text.Encoding]::UTF8.GetBytes($script:rawText.Replace("`n","`r`n")))}
Check 'blank-jsonl-record' (Fixture blank $ev @(4)) $false {param($f)ReplaceRaw $f ([Text.Encoding]::UTF8.GetBytes("`n"+$script:rawText))}
Check 'duplicate-event-property' (Fixture duplicateproperty $ev @(4)) $false {param($f)ReplaceRaw $f ([Text.Encoding]::UTF8.GetBytes($script:rawText.Replace('"sequence":1','"sequence":1,"sequence":1')))}
Check 'case-alias-event-property' (Fixture aliasproperty $ev @(4)) $false {param($f)ReplaceRaw $f ([Text.Encoding]::UTF8.GetBytes($script:rawText.Replace('"sequence":1','"sequence":1,"Sequence":1')))}
Check 'invalid-utf8' (Fixture utf8 $ev @(4)) $false {param($f)$b=[Text.Encoding]::UTF8.GetBytes($script:rawText);$b[0]=255;ReplaceRaw $f $b}
Check 'line-over-8192-bytes' (Fixture longline $ev @(4)) $false {param($f)ReplaceRaw $f ([Text.Encoding]::UTF8.GetBytes((' '*8193)+$script:rawText))}
Check 'truncated-gzip-with-repinned-compressed-hash' (Fixture truncatedgzip $ev @(4)) $false {param($f)$seg=$f.header.segments[0];$p=Join-Path $f.dir $seg.path;$b=[IO.File]::ReadAllBytes($p);[IO.File]::WriteAllBytes($p,[byte[]]$b[0..30]);$seg.compressed_bytes=31;$seg.sha256=Hash $p;$f.header.total_compressed_bytes=31}
foreach($variant in @('duplicate','case-alias')){
 $f=Fixture ('header-'+$variant) $ev @(4);$null=Header $f;$p=Join-Path $f.dir 'authority-trace.json';$text=[IO.File]::ReadAllText($p)
 $replacement=if($variant -ceq 'duplicate'){'"sealed":true,"sealed":true'}else{'"sealed":true,"Sealed":true'}
 $text=[regex]::Replace($text,'"sealed"\s*:\s*true',$replacement);[IO.File]::WriteAllText($p,$text,[Text.UTF8Encoding]::new($false))
 try{Read-Issue76AuthorityStream $f.dir $id (Hash $p)|Out-Null;throw 'SELFTEST_FALSE_ACCEPT'}catch{if(-not $_.Exception.Message.Contains('authority_stream:unknown_or_duplicate_field')){throw}}
 $passed.Add($variant+'-header-property')
}
$f=Fixture headerpin $ev @(4);$null=Header $f
try{Read-Issue76AuthorityStream $f.dir $id ('0'*64)|Out-Null;throw 'SELFTEST_FALSE_ACCEPT'}catch{if(-not $_.Exception.Message.Contains('authority_stream:header_hash')){throw}}
$passed.Add('independent-header-hash')
$bad=@((Event 1 1 start),(Event 2 1 end));$bad[1].outcome='deadline|cancelled';Check 'compound-outcome-denied' (Fixture compoundoutcome $bad @(2)) $false
$bad=@((Event 1 1 start),(Event 2 1 end));$bad[1].context_cause='deadline|cancelled';Check 'compound-cause-denied' (Fixture compoundcause $bad @(2)) $false
$bad=@((Event 1 1 start),(Event 2 1 end));$bad[1].query_family='other_query|schedule_check';Check 'compound-family-denied' (Fixture compoundfamily $bad @(2)) $false
$bad=@((Event 1 1 start),(Event 2 1 end postgres_error));$bad[1].sqlstate='40001|40P01';Check 'compound-sqlstate-denied' (Fixture compoundstate $bad @(2)) $false
$receipt=@{version='issue76-stream-reader-public-selftests-v2';passed=$passed.Count;failed=0;tests=$passed.ToArray();test_root=$testRoot;provider_calls=0;pg_calls=0;hidden_controls_read=$false;old_artifacts_changed=$false}
$receipt|ConvertTo-Json -Depth 10
