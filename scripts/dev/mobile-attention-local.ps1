[CmdletBinding()]
param([switch]$Configure,[switch]$Restart,[string]$BinaryName='attention-orchestrator-final')
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
$taskPrivate = Join-Path $taskRoot '.cache/issue57/local-native'
function Invoke-TaskDocker { & docker @args; if ($LASTEXITCODE -ne 0) { throw 'Local attention Docker step failed' } }
function Invoke-OwnerRequest([string]$Path, $Body) {
  $headers = @{ Authorization = 'Bearer ' + [IO.File]::ReadAllText((Join-Path $taskPrivate 'owner.token')).Trim(); 'X-Butler-Delivery-ID' = 'mobile-local-' + [guid]::NewGuid().ToString('N') }
  Invoke-RestMethod -Uri ('http://127.0.0.1:18062' + $Path) -Method Post -Headers $headers -ContentType application/json -Body ($Body | ConvertTo-Json -Depth 12 -Compress)
}
if($Restart){
  if($BinaryName -notmatch '^attention-orchestrator-[a-z0-9-]+$' -or -not (Test-Path -LiteralPath (Join-Path $taskRoot ".cache/issue57/$BinaryName"))){throw 'Built local acceptance binary required'}
  Invoke-TaskDocker stop butler-issue57-attention-wifi butler-issue57-attention-agent | Out-Null
  Invoke-TaskDocker rm butler-issue57-attention-wifi butler-issue57-attention-agent | Out-Null
  Invoke-TaskDocker run -d --name butler-issue57-attention-agent --network butler-issue57-attention --network-alias orchestrator -p 127.0.0.1:18062:8080 -p 18061:8081 -v "${taskRoot}:/src" -v butler-issue57-attention-state:/state -w /src --env-file (Join-Path $taskPrivate 'agent.env') golang:1.27.2 "/src/.cache/issue57/$BinaryName" | Out-Null
  Invoke-TaskDocker run -d --name butler-issue57-attention-wifi --network container:butler-issue57-attention-agent -v "${taskRoot}:/src" golang:1.27.2 /src/.cache/issue57/mobile-wifi-proxy-linux -listen :8081 -upstream http://127.0.0.1:8080 | Out-Null
  Write-Output 'Local attention application restarted; database, decisions and credentials preserved.'
  exit
}
if (-not $Configure) {
  New-Item -ItemType Directory -Force -Path $taskPrivate | Out-Null
  foreach ($name in @('owner.token','action.token')) {
    $path = Join-Path $taskPrivate $name
    if (Test-Path -LiteralPath $path) { throw 'Existing local attention state requires explicit reuse, not provisioning' }
    [IO.File]::WriteAllText($path, [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant())
  }
  $encryption = @([IO.File]::ReadAllLines((Join-Path $taskPrivate 'provider-encryption.env')) | Where-Object { $_.StartsWith('BUTLER_SETTINGS_ENCRYPTION_KEY=') })
  if ($encryption.Count -ne 1) { throw 'One matching encrypted provider reference required' }
  Invoke-TaskDocker network create butler-issue57-attention | Out-Null
  Invoke-TaskDocker network connect --alias postgres butler-issue57-attention butler-issue57-postgres
  Invoke-TaskDocker exec butler-issue57-postgres psql -X -q -v ON_ERROR_STOP=1 -U postgres -d postgres -c 'CREATE DATABASE mobile_attention_test'
  Invoke-TaskDocker volume create butler-issue57-attention-state | Out-Null
  Invoke-TaskDocker run --rm --network butler-issue57-attention -v "${taskRoot}:/src" -v butler-issue57-attention-state:/state -w /src -e MOBILE_LAB_POSTGRES_URL=postgres://postgres@postgres:5432/mobile_attention_test?sslmode=disable -e MOBILE_LAB_MANIFEST=/state/deletions.log golang:1.27.2 /src/.cache/issue57/attention-init -initialize-only
  $encryptedAuth = [IO.File]::ReadAllText((Join-Path $taskPrivate 'provider-auth.value')).Trim()
  $sql = "COPY system_settings(key,value,component,is_secret,updated_by) FROM STDIN;`nBUTLER_PROVIDER_AUTH_ANTIGRAVITY`t$encryptedAuth`torchestrator`tt`tmobile-local-clone`n\.`nINSERT INTO sessions(session_key,user_id,channel) VALUES('mobile-attention','mobile-attention-owner','web');`n"
  $sql | & docker exec -i butler-issue57-postgres psql -X -q -v ON_ERROR_STOP=1 -U postgres -d mobile_attention_test
  if ($LASTEXITCODE -ne 0) { throw 'Encrypted provider import failed' }
  $units = @{ model_turns = 48; tokens = 67108864; cost_micros = 0; external_calls = 128; children = 0; active_millis = 14400000; egress_bytes = 134217728; provider_input_bytes = 268435456 }
  $allowance = @{ model_turns = 96; tokens = 134217728; cost_micros = 0; external_calls = 256; children = 0; active_millis = 28800000; egress_bytes = 268435456; provider_input_bytes = 536870912 }
  $policy = @{ models = @{ 'antigravity:gemini-3.8-flash-high' = @{ basis = 'subscription_no_overage' } }; model_millis = 300000; output_tokens = 8192; task = $units; mandate = $allowance } | ConvertTo-Json -Depth 6 -Compress
  $common = @('BUTLER_POSTGRES_URL=postgres://postgres@postgres:5432/mobile_attention_test?sslmode=disable','BUTLER_POSTGRES_MAX_CONNS=4','BUTLER_POSTGRES_MIN_CONNS=1','BUTLER_ACTION_AUTHORITY_TOKEN_FILE=/src/.cache/issue57/local-native/action.token','BUTLER_MEMORY_READ_ENABLED=true','BUTLER_MEMORY_DELETION_MANIFEST_FILE=/state/deletions.log',"BUTLER_EXECUTION_BUDGET_POLICY_JSON=$policy",'BUTLER_LOG_LEVEL=warn')
  [IO.File]::WriteAllLines((Join-Path $taskPrivate 'broker.env'), $common + @('BUTLER_HTTP_ADDR=:8080','BUTLER_GRPC_ADDR=:9090','BUTLER_ACTION_CONTEXT_URL=http://orchestrator:8080/internal/action-context','BUTLER_MEMORY_READ_CONTROL_URL=http://orchestrator:8080/internal/memory-read','BUTLER_TOOL_REGISTRY_PATH=/src/.cache/issue57/local-native/tools.json'))
  [IO.File]::WriteAllText((Join-Path $taskPrivate 'tools.json'), '{"tools":[]}')
  [IO.File]::WriteAllLines((Join-Path $taskPrivate 'agent.env'), $common + $encryption + @('BUTLER_HTTP_ADDR=:8080','BUTLER_GRPC_ADDR=:9090','BUTLER_REDIS_URL=redis://redis:6379/0','BUTLER_TOOL_BROKER_ADDR=tool-broker:9090','BUTLER_MODEL_PROVIDER=antigravity','BUTLER_ANTIGRAVITY_MODEL=gemini-3.8-flash-high','BUTLER_ANTIGRAVITY_MAXIMUM_OUTPUT_TOKENS=8192','BUTLER_OPENAI_TIMEOUT_SECONDS=300','BUTLER_AUTONOMY_MODE=mode_2','BUTLER_OWNER_CONTROL_TOKEN_FILE=/src/.cache/issue57/local-native/owner.token','BUTLER_OWNER_CONTROL_OWNER_ID=mobile-attention-owner','BUTLER_MEMORY_PIPELINE_ENABLED=false','NO_PROXY=orchestrator,tool-broker,postgres,redis,127.0.0.1,localhost'))
  Invoke-TaskDocker run -d --name butler-issue57-attention-redis --network butler-issue57-attention --network-alias redis redis:7.4-alpine | Out-Null
  Invoke-TaskDocker run -d --name butler-issue57-attention-broker --network butler-issue57-attention --network-alias tool-broker -v "${taskRoot}:/src" -v butler-issue57-attention-state:/state -w /src --env-file (Join-Path $taskPrivate 'broker.env') golang:1.27.2 /src/.cache/issue57/attention-broker | Out-Null
  Invoke-TaskDocker run -d --name butler-issue57-attention-agent --network butler-issue57-attention --network-alias orchestrator -p 127.0.0.1:18062:8080 -p 18061:8081 -v "${taskRoot}:/src" -v butler-issue57-attention-state:/state -w /src --env-file (Join-Path $taskPrivate 'agent.env') golang:1.27.2 /src/.cache/issue57/attention-orchestrator | Out-Null
  Invoke-TaskDocker run -d --name butler-issue57-attention-wifi --network container:butler-issue57-attention-agent -v "${taskRoot}:/src" golang:1.27.2 /src/.cache/issue57/mobile-wifi-proxy-linux -listen :8081 -upstream http://127.0.0.1:8080 | Out-Null
  Write-Output 'Local isolated agent, Broker and Wi-Fi ingress started; owner API is loopback only.'
  exit
}
$expires = [DateTimeOffset]::UtcNow.AddHours(3).ToString('o')
$goal = Invoke-OwnerRequest '/api/v3/owner-goals' @{ goal = @{ kind='objective'; status='active'; title='Самостоятельно помогать по разрешённым уведомлениям'; purpose='mobile_attention'; expires_at=$expires }; expected_revision=0 }
$names = @('messages.inbox','messages.search','messages.process','memory.search','memory.open','memory.neighbors')
$scopes = @($names | ForEach-Object { @{origin='tool:' + $_} })
$limits = @{ model_turns=96; tokens=134217728; cost_micros=0; external_calls=256; children=0; active_millis=28800000; egress_bytes=268435456; provider_input_bytes=536870912 }
$mandate = Invoke-OwnerRequest '/api/v3/owner-mandates' @{ mandate=@{goal_id=$goal.subject_id;goal_revision=$goal.revision;status='active';purpose='mobile_attention';tools=$names;resource_scopes=$scopes;effects=@('read','prepare');privacy='cloud_allowed';budget=$limits;expires_at=$expires;allow_initiative=$true;intervention=@('budget_exhausted','effect_unknown');reporting='exceptions'};expected_revision=0 }
$source = Invoke-OwnerRequest '/api/v3/initiatives/sources' @{expected_revision=0;source=@{source_id='mobile-attention-source';goal_id=$goal.subject_id;mandate_id=$mandate.subject_id;session_key='mobile-attention';observer_kind='mobile_inbox';purpose='mobile_attention';time_zone='Asia/Tomsk';enabled=$true}}
@{goal=$goal;mandate=$mandate;source=$source} | ConvertTo-Json -Depth 10 | Set-Content (Join-Path $taskPrivate 'authority-receipts.json')
Write-Output 'Local permanent read/memory mandate and 20-message/hour source enabled.'
