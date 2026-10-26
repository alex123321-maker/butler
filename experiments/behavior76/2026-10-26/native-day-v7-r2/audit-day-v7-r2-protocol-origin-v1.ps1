$ErrorActionPreference='Stop'
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$root=Join-Path $repo '.cache/behavior76/native-day-v7-r2'
$idx=Get-Content (Join-Path $root 'index.json') -Raw|ConvertFrom-Json -AsHashtable -Depth 40
if($idx.episodes.Count -ne 1){throw 'Closed one-arm boundary changed'}
$case=$idx.episodes[0];$tables=Join-Path $root $case.tables_dir
$rows=@([IO.File]::ReadLines((Join-Path $root 'behavior76_attempts.jsonl'))|Where-Object {$_.Trim()}|ForEach-Object {$_|ConvertFrom-Json -AsHashtable -Depth 80}|Where-Object window_id -CEQ 'issue76-fixture-day-20261026T050548')
$last=@($rows|Where-Object n -EQ 3)
if($last.Count -ne 1){throw 'Missing actual final admission'}
$runID=$last[0].run_id
$states=@();foreach($name in @('runs','accepted_inputs')){
 foreach($line in [IO.File]::ReadLines((Join-Path $tables ($name+'.jsonl')))){
  if(-not $line.Trim()){continue};$r=$line|ConvertFrom-Json -AsHashtable -Depth 60
  if($r.run_id -ceq $runID){$states+=@{table=$name;run_id=$r.run_id;status=$r.status;current_state=$r.current_state;error_type=$r.error_type;error_code=$r.error_code}}
 }
}
$effectStates=@([IO.File]::ReadLines((Join-Path $tables 'action_effects.jsonl'))|Where-Object {$_.Trim()}|ForEach-Object {$_|ConvertFrom-Json -AsHashtable -Depth 60}|Group-Object state|ForEach-Object {@{state=$_.Name;count=$_.Count}})
$finalRows=@([IO.File]::ReadLines((Join-Path $tables 'run_final_results.jsonl'))|Where-Object {$_.Trim()}).Count
$sourceFiles=@('internal/transport/antigravity/provider.go','internal/transport/antigravity/protocol.go','internal/transport/antigravity/stream_diagnostics.go','internal/transport/errors.go','apps/orchestrator/internal/app/behavior76_native_test.go')
$r=[ordered]@{version='issue76-closed-day-v7-r2-protocol-origin-v1';classification='OBSERVED_LOCAL_EMPTY_ANSWER_REJECTION; UPSTREAM_VS_ADAPTER_ROOT_CAUSE_NOT_ESTABLISHED';last_entry=@{n=$last[0].n;run_id=$runID;kind=$last[0].kind;status=$last[0].status;stage=$last[0].stage;milliseconds=$last[0].milliseconds;input_tokens=$last[0].input_tokens;output_tokens=$last[0].output_tokens;total_tokens=$last[0].total_tokens;thinking_tokens=$last[0].thinking_tokens;model_version=$last[0].model_version;events=@($last[0].events|ForEach-Object {@{type=$_.type;error_code=$_.error_code;safe_reason=$_.safe_reason;http_status=$_.http_status}})};actual_durable_run_input_metadata=$states;effect_states_not_quality_verdicts=$effectStates;owner_final_rows=$finalRows;source_chain=@('The exact transport-owned empty-answer literal occurs after readStreamProtocol returned successfully and validateFunctionCalls returned no function.','That branch reports no usable non-thought text or function; SafeReason maps this local branch to stream_empty_answer.','The same branch ALREADY assigns answer.Usage and answer.ModelVersion into its error event before sending. No discard defect is established by NULL usage.','readStreamProtocol only publishes Usage from a complete validated prompt/candidate/total set after terminal completion.','The safe research observer keeps observed model version separately from nullable usage, and seals globally on the transport_error.');limits_of_inference=@('No provider response text, thoughts, function arguments, golden expected answer or mapping was opened.','STOP versus MAX_TOKENS, empty candidate shape versus thought-only content, and whether absent/incomplete usage metadata arrived are not retained here; do not choose among them.','Model version is actually observed, not proof of proprietary weight identity or quality.','NULL usage is unavailable expense, never zero cost and never estimated from adjacent entries.','This future diagnostic cannot backfill previous missing rejection reasons.');source_pins=@($sourceFiles|ForEach-Object {@{path=$_;sha256=(Get-FileHash (Join-Path $repo $_)).Hash.ToLowerInvariant()}});partial_text_read=$false;goldens_read=$false;mapping_read=$false;quality_assessed=$false;native_calls=0;pg_calls=0;qualification=$false}
$dest=Join-Path $PSScriptRoot 'public-day-v7-r2-protocol-origin-v1.json'
$f=[IO.File]::Open($dest,[IO.FileMode]::CreateNew)
try{$b=[Text.UTF8Encoding]::new($false).GetBytes((ConvertTo-Json $r -Depth 30)+"`n");$f.Write($b)}finally{$f.Dispose()}
(Get-Item -LiteralPath $dest).IsReadOnly=$true
@{receipt_sha256=(Get-FileHash $dest).Hash.ToLowerInvariant();actual_state=$states;last_n=$last[0].n;version_observed=$last[0].model_version;usage_available=($null -ne $last[0].total_tokens);effect_states_not_quality=$effectStates;owner_final_rows=$finalRows}|ConvertTo-Json -Depth 10
