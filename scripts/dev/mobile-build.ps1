[CmdletBinding()]
param([switch]$Release, [switch]$Test, [switch]$Connected)
. (Join-Path $PSScriptRoot 'common.ps1')
$state = Initialize-ButlerAgentEnvironment
$repoCache = [IO.Path]::GetFullPath((Join-Path $state.RepoRoot '.cache'))
if (-not $env:GRADLE_USER_HOME -or -not [IO.Path]::GetFullPath($env:GRADLE_USER_HOME).StartsWith($repoCache, [StringComparison]::OrdinalIgnoreCase)) {
  $env:GRADLE_USER_HOME = Join-Path $repoCache 'gradle'
}
$androidProject = Join-Path $state.RepoRoot 'mobile/android'
if (-not $env:ANDROID_HOME) { throw 'Set ANDROID_HOME to an installed Android SDK (platform android-36.1).' }
$arguments = @('--console', 'plain', '--no-configuration-cache', ':app:assembleDebug')
if ($Test) { $arguments += @(':app:testDebugUnitTest', ':app:lintDebug') }
if ($Connected) { $arguments += ':app:connectedDebugAndroidTest' }
if ($Release) {
  if (-not $env:KEYSTORE_PATH -or -not $env:STORE_PASSWORD -or -not $env:KEY_PASSWORD) { throw 'Release signing requires KEYSTORE_PATH, STORE_PASSWORD and KEY_PASSWORD from private operator configuration.' }
  $arguments += ':app:assembleRelease'
}
Push-Location $androidProject
try { & ./gradlew.bat @arguments; Assert-LastExitCode -Action 'Android build/checks' }
finally { Pop-Location }
