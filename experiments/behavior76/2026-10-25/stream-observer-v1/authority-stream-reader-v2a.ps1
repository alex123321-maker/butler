function Import-Issue76AuthorityStreamReader {
 if(-not ('Issue76AuthorityStreamV2A' -as [type])){Add-Type -Path (Join-Path $PSScriptRoot 'authority-stream-reader-v2a.cs')}
}
function Read-Issue76AuthorityStream {
 param([Parameter(Mandatory)][string]$Directory,[Parameter(Mandatory)][hashtable]$ExpectedIdentity,[Parameter(Mandatory)][string]$HeaderSha256)
 Import-Issue76AuthorityStreamReader
 [Issue76AuthorityStreamV2A]::Read($Directory,($ExpectedIdentity|ConvertTo-Json -Compress),$HeaderSha256)|ConvertFrom-Json -AsHashtable
}
