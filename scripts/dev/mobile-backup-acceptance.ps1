[CmdletBinding()]
param([Parameter(Mandatory)][string]$OwnerTokenFile)
. (Join-Path $PSScriptRoot 'common.ps1')
$state = Initialize-ButlerAgentEnvironment
# Fixed disposable receiver/database only. Never accept a production DB argument.
$ownerBase = 'http://127.0.0.1:18058/api/v1/mobile-owner'
$deviceBase = 'http://127.0.0.1:18057/api/v1/mobile'
$ownerProof = (Get-Content $OwnerTokenFile -Raw).Trim()
$ownerHeaders = @{Authorization="Bearer $ownerProof"}
function Post-JSON($url, $headers, $value) { Invoke-RestMethod -Uri $url -Method Post -Headers $headers -ContentType application/json -Body (ConvertTo-Json $value -Depth 20 -Compress) }
$code = Post-JSON "$ownerBase/pairing-code" $ownerHeaders @{}
$deviceID = [Guid]::NewGuid().ToString()
$device = Post-JSON "$deviceBase/pair/exchange" @{} @{pairing_code=$code.pairing_code;device_id=$deviceID;device_name='Synthetic backup acceptance';android_sdk=36;app_version='fixture'}
$deviceHeaders = @{Authorization="Bearer $($device.device_token)";'X-Butler-Device-Id'=$deviceID;'X-Butler-Schema-Version'='1'}
$marker = 'synthetic_deleted_' + [Guid]::NewGuid().ToString('n')
$now = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$firstID=[Guid]::NewGuid().ToString()
$secondID=[Guid]::NewGuid().ToString()
$batch=@{schema_version=1;batch_id=[Guid]::NewGuid().ToString();sent_at_epoch_ms=$now;events=@(
 @{schema_version=1;device_event_id=$firstID;notification_key='backup-first';package_name='dev.butler.fixture';change_type='POSTED';posted_at_epoch_ms=$now;observed_at_epoch_ms=$now;content_hash=('a'*64);payload=@{text=$marker}},
 @{schema_version=1;device_event_id=$secondID;notification_key='backup-independent';package_name='dev.butler.fixture';change_type='POSTED';posted_at_epoch_ms=$now;observed_at_epoch_ms=$now;content_hash=('b'*64);payload=@{text='synthetic independent source'}}
)}
$ack=Post-JSON "$deviceBase/notifications/batch" $deviceHeaders $batch
if ($ack.accepted_ids.Count -ne 2) { throw 'Fixture did not commit' }
$sources=Invoke-RestMethod "$ownerBase/events?device_id=$deviceID" -Headers $ownerHeaders
$selected=@($sources | Where-Object device_event_id -eq $firstID)[0]
$restoreDB='mobile_restore_' + [DateTimeOffset]::UtcNow.ToUnixTimeSeconds() + '_test'
$dumpPath='/tmp/' + $restoreDB + '.dump'
docker exec butler-issue57-postgres pg_dump -U postgres -Fc mobile_test -f $dumpPath
Assert-LastExitCode -Action 'Actual pre-deletion PostgreSQL dump'
Post-JSON "$ownerBase/delete" $ownerHeaders @{source_id=$selected.source_id;revision=$selected.revision} | Out-Null
docker exec butler-issue57-postgres createdb -U postgres $restoreDB
Assert-LastExitCode -Action 'Create new disposable restore DB'
docker exec butler-issue57-postgres pg_restore -U postgres -d $restoreDB $dumpPath
Assert-LastExitCode -Action 'Actual PostgreSQL restore'
$containerName='butler-issue57-restore-' + [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
docker run -d --name $containerName -p 127.0.0.1:18059:8080 -p 127.0.0.1:18060:8081 -v "$($state.RepoRoot):/workspace:ro" -v butler-issue57-state:/state -w /workspace -e "MOBILE_LAB_POSTGRES_URL=postgres://postgres@host.docker.internal:15457/$($restoreDB)?sslmode=disable" -e MOBILE_LAB_MANIFEST=/state/deletions -e MOBILE_LAB_OWNER_TOKEN_FILE=/workspace/.cache/issue57/owner.token pgvector/pgvector:pg17 /workspace/.cache/issue57/mobile-lab-new -require-pending-restore | Out-Null
Assert-LastExitCode -Action 'Start fenced restored receiver'
$ready=$false
for ($attempt=0;$attempt -lt 20;$attempt++) {
  try { Invoke-WebRequest http://127.0.0.1:18059/health -TimeoutSec 2 | Out-Null; $ready=$true; break } catch { Start-Sleep -Milliseconds 250 }
}
if (-not $ready) { throw 'Restored receiver did not reconcile' }
$restored=Invoke-RestMethod "http://127.0.0.1:18060/api/v1/mobile-owner/events?device_id=$deviceID&reveal=true" -Headers $ownerHeaders
$deleted=@($restored | Where-Object device_event_id -eq $firstID)[0]
$independent=@($restored | Where-Object device_event_id -eq $secondID)[0]
$deletedBody = $deleted.PSObject.Properties['body']
$replay=Post-JSON 'http://127.0.0.1:18059/api/v1/mobile/notifications/batch' $deviceHeaders $batch
$logs=docker logs $containerName 2>&1 | Out-String
$evidence=[ordered]@{actual_pg_dump_restore=$true;stale_restore_denied=$logs.Contains('stale restored database denied before reconciliation');deleted_status=$deleted.status;deleted_body_empty=($null -eq $deletedBody -or [string]::IsNullOrEmpty($deletedBody.Value));independent_retained=($independent.body -like '*synthetic independent source*');duplicate_ids=$replay.duplicate_ids.Count;new_accepts=$replay.accepted_ids.Count;restore_database=$restoreDB}
if (-not $evidence.stale_restore_denied -or -not $evidence.deleted_body_empty -or -not $evidence.independent_retained -or $evidence.duplicate_ids -ne 2 -or $evidence.new_accepts -ne 0) { throw 'Backup acceptance failed' }
$evidence | ConvertTo-Json | Set-Content (Join-Path $state.RepoRoot '.cache/issue57/backup-evidence.json')
$evidence | ConvertTo-Json
