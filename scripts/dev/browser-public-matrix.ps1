[CmdletBinding()]
param([switch]$UsePreparedRead02)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$state=Initialize-ButlerAgentEnvironment
$root='/srv/storage/butler/agents/browser-baseline-20261028'
$remotePrepare='/var/tmp/butler-browser-eval-prepare-attempt.sh'
$previous='read-01'
function Read-Observation([string]$id) {
  $raw=& ssh -o BatchMode=yes -o ConnectTimeout=10 laba "cat '$root/reports/$id/02-observation.json'"
  Assert-LastExitCode -Action 'read exact attempt observation'
  return ($raw | ConvertFrom-Json)
}
$first=Read-Observation $previous
if ($first.observation -ne 'INPUT_COMPLETED') {throw 'First attempt is not completed; preserve it and inspect before switching recipient.'}
foreach ($attempt in @('read-02','read-03','nav-01','nav-02','nav-03','form-01','form-02','form-03')) {
  $parts=$attempt.Split('-')
  # Queue check must precede recipient replacement, not just later submission.
  $active=& ssh -o BatchMode=yes -o ConnectTimeout=10 laba 'docker exec butler-main-postgres-1 psql -XAt -U butler -d butler -c "SELECT count(*) FROM accepted_inputs WHERE status IN (''queued'',''running'');"'
  Assert-LastExitCode -Action 'current owner queue preflight'
  if ($active.Trim() -ne '0') {throw 'Active owner work: matrix stopped before recipient replacement.'}
  if ($UsePreparedRead02 -and $attempt -eq 'read-02') {
    $accepted=& ssh -o BatchMode=yes -o ConnectTimeout=10 laba 'docker exec butler-main-postgres-1 psql -XAt -U butler -d butler -c "SELECT count(*) FROM accepted_inputs WHERE event_id=''browser-baseline-matrix-20261028-read-02'';"'
    Assert-LastExitCode -Action 'prepared attempt has no accepted Input'
    if ($accepted.Trim() -ne '0') {throw 'Prepared recipient already has an Input; do not resubmit.'}
    & ssh -o BatchMode=yes -o ConnectTimeout=10 devops 'systemctl is-active butler-browser-eval@read-02; test ! -s /var/lib/butler-browser-eval/read-02/recipient.jsonl; curl --fail --silent http://127.0.0.1:18097/ | grep -q "<h1>Browser baseline</h1>"'
    Assert-LastExitCode -Action 'recover unused prepared recipient readiness'
  } else {
    & ssh -o BatchMode=yes -o ConnectTimeout=10 devops "bash '$remotePrepare' '$attempt' '$previous'"
    Assert-LastExitCode -Action 'prepare isolated recipient'
  }
  & ssh -o BatchMode=yes -o ConnectTimeout=10 laba "bash '$root/native-attempt.sh' '$root' '$($parts[0])' '$($parts[1])'"
  Assert-LastExitCode -Action 'native attempt observation'
  $result=Read-Observation $attempt
  & ssh -o BatchMode=yes -o ConnectTimeout=10 laba "bash '$root/native-snapshot-reviewed.sh' 'browser-baseline-matrix-20261028-$attempt' '$root/reports/$attempt-metadata-final.json'"
  Assert-LastExitCode -Action 'exact-run metadata'
  Write-Host "$attempt : $($result.observation), $($result.elapsed_ms) ms; task/effect verification separate"
  if ($result.observation -ne 'INPUT_COMPLETED') {throw 'Matrix stopped on unresolved/noncompleted attempt; no automatic retry or reset.'}
  $previous=$attempt
}
Write-Host 'Public matrix observations finished. Credential scenarios require the real protected owner flow.'
