[CmdletBinding()]
param()
. (Join-Path $PSScriptRoot 'common.ps1')
$state = Initialize-ButlerAgentEnvironment
$privateDir = Join-Path $state.RepoRoot '.cache/android-signing'
New-Item -ItemType Directory -Force -Path $privateDir | Out-Null
$keyPath = Join-Path $privateDir 'release.keystore'
$passwordPath = Join-Path $privateDir 'release-password.dpapi'
if ((Test-Path $keyPath) -xor (Test-Path $passwordPath)) { throw 'Signing configuration is incomplete; restore the original key/password. Never replace an existing signer.' }
if (-not (Test-Path $passwordPath)) {
  $newPassword = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
  ConvertTo-SecureString $newPassword -AsPlainText -Force | ConvertFrom-SecureString | Set-Content $passwordPath
  $newPassword = $null
}
$securePassword = (Get-Content $passwordPath -Raw).Trim() | ConvertTo-SecureString
$env:STORE_PASSWORD = [Net.NetworkCredential]::new('', $securePassword).Password
$env:KEY_PASSWORD = $env:STORE_PASSWORD
$env:KEYSTORE_PATH = $keyPath
$env:KEY_ALIAS = 'upload'
try {
  if (-not (Test-Path $keyPath)) {
    $keytool = Join-Path (Split-Path (Get-Command java).Source) 'keytool.exe'
    & $keytool -genkeypair -keystore $keyPath -storepass:env STORE_PASSWORD -keypass:env KEY_PASSWORD -alias upload -dname 'CN=Butler Companion,O=Butler,C=US' -keyalg RSA -keysize 3072 -validity 10000
    Assert-LastExitCode -Action 'Create local release signing key'
  }
  & (Join-Path $PSScriptRoot 'mobile-build.ps1') -Release
}
finally {
  Remove-Item Env:STORE_PASSWORD,Env:KEY_PASSWORD -ErrorAction SilentlyContinue
  $securePassword.Dispose()
}
