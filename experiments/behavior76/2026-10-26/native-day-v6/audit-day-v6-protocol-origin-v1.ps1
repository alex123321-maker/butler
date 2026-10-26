$ErrorActionPreference='Stop'
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$root=Join-Path $repo '.cache/behavior76/native-day-v6'
$idx=Get-Content (Join-Path $root 'index.json') -Raw|ConvertFrom-Json -AsHashtable -Depth 65
$tables=Join-Path $root $idx.episodes[-1].tables_dir
$fixed=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
$rx='protocolError\("([^"\r\n]*)"\)'
foreach($file in Get-ChildItem (Join-Path $repo 'internal/transport/antigravity') -File -Filter '*.go'){
 foreach($m in [regex]::Matches([IO.File]::ReadAllText($file.FullName),$rx)){$null=$fixed.Add($m.Groups[1].Value)}
}
$null=$fixed.Add('provider_protocol_error')
$observed=@()
foreach($name in @('runs','accepted_inputs')){
 foreach($line in [IO.File]::ReadLines((Join-Path $tables ($name+'.jsonl')))){
  if(-not $line.Trim()){continue};$r=$line|ConvertFrom-Json -AsHashtable -Depth 65
  if($r.run_id -cne 'run-1792982100747209153-28e7d983ebea439e'){continue}
  $message=if($null -ne $r.error_message -and $fixed.Contains([string]$r.error_message)){$r.error_message}else{'no_exact_public_fixed_subtype_retained'}
  $observed+=@{table=$name;run_id=$r.run_id;status=$r.status;current_state=$r.current_state;error_type=$r.error_type;error_code=$r.error_code;fixed_public_error_message=$message;checkpoint=$r.checkpoint}
 }
}
$sources=@('internal/transport/antigravity/protocol.go','internal/transport/antigravity/provider.go','internal/transport/errors.go','internal/transport/events.go','apps/orchestrator/internal/app/behavior76_native_test.go')
$receipt=@{version='issue76-day-v6-protocol-origin-metadata-v1';result='NORMALIZED_PROTOCOL_CATEGORY_OBSERVED_SUBTYPE_NOT_ESTABLISHED';actual=$observed;entry=134;stage='response_stream';events=@('run_started','assistant_delta','assistant_delta','transport_error');code='provider_protocol_error';safe_reason='';http_status_available=$false;usage='NULL';partial_text_read=$false;model_answers_read=$false;goldens_read=$false;mapping_read=$false;source_chain=@('Native Antigravity valid text parts invoke onText and emit assistant_delta before completion','Later readStreamProtocol checks can fail on JSON/envelope/part/candidate/usage/terminal/bounds conditions after prior text','Post-terminal truncated function proposal can also produce the same protocol type; no subtype may be selected from the retained metadata','protocolError uses typed provider_protocol_error; NewTransportErrorEvent normalizes that typed error','Research observer retains DiagnosticCode, allowlisted DiagnosticReason and numeric status only; no fixed subtype appears in retained safe_reason');excluded_as_explanations=@('No normalized tool or batch proposal was delivered in failed134; catalog/schema function validation ordinarily uses invalid_tool_request/capability_mismatch','Initial request/auth/non-SSE rejection alone cannot account for two same-entry text deltas','No authority stream operation failure or expired budget scope is observed for this operation');uncertainty='Exact rejecting branch and upstream versus adapter origin remain UNKNOWN. Do not guess malformed SSE, EOF, token limit, function proposal or upstream fault. No retry or relaxation follows this gap.';source_pins=@($sources|ForEach-Object {@{path=$_;sha256=(Get-FileHash -LiteralPath (Join-Path $repo $_)).Hash.ToLowerInvariant()}});pg_calls=0;native_calls=0;quality_assessed=$false}
$out=Join-Path $PSScriptRoot 'public-day-v6-protocol-origin-v1.json'
$file=[IO.File]::Open($out,[IO.FileMode]::CreateNew)
try{$bytes=[Text.Encoding]::UTF8.GetBytes(($receipt|ConvertTo-Json -Depth 20)+"`n");$file.Write($bytes)}finally{$file.Dispose()}
(Get-Item -LiteralPath $out).IsReadOnly=$true
@{actual=$observed;receipt_sha256=(Get-FileHash $out).Hash.ToLowerInvariant()}|ConvertTo-Json -Depth 9
