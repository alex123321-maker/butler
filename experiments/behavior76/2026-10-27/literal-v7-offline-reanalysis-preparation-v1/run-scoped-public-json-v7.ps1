[CmdletBinding()]
param([Parameter(Mandatory)][string]$PublicScript,[string]$PublicOutputRoot='',[string]$PublicInputRoot='')
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'literal-json-token-boundary-reanalysis-v7.ps1')
Assert-Issue76LiteralV7OriginalRuntimePins
$arguments=@{}
if($PublicOutputRoot){$arguments.OutputRoot=$PublicOutputRoot}
if($PublicInputRoot){$arguments.InputRoot=$PublicInputRoot}
Invoke-Issue76JsonStringTokenScopeV7 {& $PublicScript @arguments}
Assert-Issue76LiteralV7OriginalRuntimePins
