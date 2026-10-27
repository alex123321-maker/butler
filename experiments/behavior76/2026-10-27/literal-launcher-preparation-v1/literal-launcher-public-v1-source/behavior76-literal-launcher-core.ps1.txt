# Pure host validation helpers. Importing this file never builds, consumes a
# certificate, starts a process, reads auth or contacts a provider/database.
Set-StrictMode -Version Latest

function Get-B76Property($Element, [string]$Name) {
 foreach($property in $Element.EnumerateObject()) { if($property.Name -ceq $Name) { return $property.Value } }
 return $null
}

function Assert-B76Schema($Value,$Rule,$Root,[int]$Depth=0) {
 if($Depth -gt 24){throw 'Admission JSON nesting denied'}
 $reference=Get-B76Property $Rule '$ref'
 if($null -ne $reference){
  $referenceMatch=[regex]::Match($reference.GetString(),'^#/\$defs/([A-Za-z0-9_]+)$')
  if(-not $referenceMatch.Success){throw 'Unsupported admission schema reference'}
  $definition=$referenceMatch.Groups[1].Value
  return Assert-B76Schema $Value ($Root.GetProperty('$defs').GetProperty($definition)) $Root ($Depth+1)
 }
 $constant=Get-B76Property $Rule 'const'
 if($null -ne $constant -and $Value.GetRawText() -cne $constant.GetRawText()){throw 'Admission constant mismatch'}
 $type=Get-B76Property $Rule 'type'
 if($null -ne $type){
  $wanted=$type.GetString()
  $actual=$Value.ValueKind.ToString().ToLowerInvariant()
  if($wanted -eq 'integer'){
   if($actual -ne 'number'){throw 'Admission integer required'}
   try{$number=$Value.GetInt64()}catch{throw 'Admission canonical integer required'}
   $minimum=Get-B76Property $Rule 'minimum'
   if($null -ne $minimum -and $number -lt $minimum.GetInt64()){throw 'Admission integer outside bound'}
  }elseif($actual -cne $wanted){throw 'Admission JSON type mismatch'}
 }
 if($Value.ValueKind -eq [System.Text.Json.JsonValueKind]::Object){
  $properties=Get-B76Property $Rule 'properties'
  if($null -eq $properties){throw 'Admission object shape missing'}
  $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
  foreach($property in $Value.EnumerateObject()){
   if(-not $seen.Add($property.Name)){throw 'Duplicate admission field denied'}
   $childRule=Get-B76Property $properties $property.Name
   if($null -eq $childRule){throw 'Unknown or aliased admission field denied'}
   Assert-B76Schema $property.Value $childRule $Root ($Depth+1)
  }
  $required=Get-B76Property $Rule 'required'
  if($null -ne $required){foreach($member in $required.EnumerateArray()){if(-not $seen.Contains($member.GetString())){throw 'Required admission field missing'}}}
 }elseif($Value.ValueKind -eq [System.Text.Json.JsonValueKind]::String){
  $text=$Value.GetString()
  $pattern=Get-B76Property $Rule 'pattern'
  if($null -ne $pattern -and -not [regex]::IsMatch($text,$pattern.GetString(),[Text.RegularExpressions.RegexOptions]::CultureInvariant,[TimeSpan]::FromSeconds(1))){throw 'Admission string representation denied'}
  $minimum=Get-B76Property $Rule 'minLength'
  if($null -ne $minimum -and $text.Length -lt $minimum.GetInt32()){throw 'Admission string too short'}
 }elseif($Value.ValueKind -eq [System.Text.Json.JsonValueKind]::Array){throw 'Admission array shape unsupported'}
}

function Read-B76Certificate([string]$Path,[string]$SchemaPath,[string]$ExpectedSHA256){
 if($ExpectedSHA256 -cnotmatch '^[0-9a-f]{64}$'){throw 'Canonical issuer certificate pin required'}
 $raw=[IO.File]::ReadAllBytes($Path)
 if($raw.Length -gt 262144){throw 'Admission certificate size exceeded'}
 $hash=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($raw)).ToLowerInvariant()
 if($hash -cne $ExpectedSHA256){throw 'Issuer certificate pin mismatch'}
 $utf8=[Text.UTF8Encoding]::new($false,$true)
 $text=$utf8.GetString($raw)
 $document=[Text.Json.JsonDocument]::Parse($text)
 $schema=[Text.Json.JsonDocument]::Parse($utf8.GetString([IO.File]::ReadAllBytes($SchemaPath)))
 try{
  Assert-B76Schema $document.RootElement $schema.RootElement $schema.RootElement
  $arguments=@{InputObject=$text;AsHashtable=$true}
  if((Get-Command ConvertFrom-Json).Parameters.ContainsKey('DateKind')){$arguments.DateKind='String'}
  return ConvertFrom-Json @arguments
 }finally{$document.Dispose();$schema.Dispose()}
}

function Resolve-B76RepoPath([string]$Root,[string]$Relative,[switch]$Absent){
 if($Relative -cnotmatch '^[A-Za-z0-9_./-]+$' -or $Relative.StartsWith('/') -or $Relative.EndsWith('/')){throw 'Canonical relative admission path required'}
 $parts=$Relative.Split('/')
 foreach($part in $parts){if($part -in @('','.','..') -or $part -match '^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\..*)?$'){throw 'Admission path segment denied'}}
 $base=[IO.Path]::GetFullPath($Root).TrimEnd([IO.Path]::DirectorySeparatorChar)
 $current=$base
 if((Get-Item -LiteralPath $base).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Admission root redirect denied'}
 for($n=0;$n -lt $parts.Length;$n++){
  $current=Join-Path $current $parts[$n]
  $exists=Test-Path -LiteralPath $current
  if($n -eq $parts.Length-1 -and $Absent){if($exists){throw 'One-use admission output or marker already exists'};continue}
  if(-not $exists){throw 'Pinned admission path missing'}
  if((Get-Item -LiteralPath $current).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Admission path redirect denied'}
 }
 if(-not $current.StartsWith($base+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Admission path escaped repository'}
 return $current
}

function Assert-B76Pin([string]$Root,$Pin){
 $path=Resolve-B76RepoPath $Root $Pin.path
 $item=Get-Item -LiteralPath $path
 if($item.PSIsContainer){throw 'Pinned admission artifact is not a regular file'}
 if($Pin.ContainsKey('bytes') -and $item.Length -ne $Pin.bytes){throw 'Admission artifact length mismatch'}
 if((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $Pin.sha256){throw 'Admission artifact digest mismatch'}
 return $path
}

function Get-B76Utc([string]$Value){
 if($Value -cnotmatch '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,7})?Z$'){throw 'Exact UTC admission timestamp required'}
 $parsed=[DateTimeOffset]::MinValue
 if(-not [DateTimeOffset]::TryParse($Value,[Globalization.CultureInfo]::InvariantCulture,[Globalization.DateTimeStyles]::AssumeUniversal,[ref]$parsed) -or $parsed.Offset -ne [TimeSpan]::Zero){throw 'Invalid UTC admission timestamp'}
 return $parsed
}

function Write-B76ConsumedMarker([string]$Path,[byte[]]$Bytes){
 $stream=[IO.FileStream]::new($Path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None,4096,[IO.FileOptions]::WriteThrough)
 try{$stream.Write($Bytes,0,$Bytes.Length);$stream.Flush($true)}finally{$stream.Dispose()}
 $readback=[IO.File]::ReadAllBytes($Path)
 if($readback.Length -ne $Bytes.Length -or [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($readback)) -cne [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($Bytes))){throw 'One-use marker durability/readback uncertain; never retry'}
}
