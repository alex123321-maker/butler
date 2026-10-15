[CmdletBinding()]
param(
  [string[]]$Packages = @("./apps/...", "./internal/...", "./scripts/..."),
	[string]$Run = "",
  [switch]$Count1
)

. (Join-Path $PSScriptRoot "common.ps1")

$state = Initialize-ButlerAgentEnvironment
Assert-CommandAvailable -Name go

$args = @("test")
if ($Run) { $args += @("-run", $Run) }
if ($Count1) {
  $args += "-count=1"
}
$args += $Packages

Push-Location $state.RepoRoot
try {
  & go @args
  Assert-LastExitCode -Action "go test"
}
finally {
  Pop-Location
}
