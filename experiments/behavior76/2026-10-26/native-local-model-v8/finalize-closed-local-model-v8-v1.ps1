$ErrorActionPreference='Stop'
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$root=Join-Path $repo '.cache/behavior76/native-local-model-v8'
function Hash([string]$p){(Get-FileHash -LiteralPath $p -Algorithm SHA256).Hash.ToLowerInvariant()}
function ReadJson([string]$p){ConvertFrom-Json ([IO.File]::ReadAllText($p)) -AsHashtable -Depth 90}
function SaveNew([string]$n,$v){$p=Join-Path $PSScriptRoot $n;$s=[IO.File]::Open($p,[IO.FileMode]::CreateNew);try{$b=[Text.UTF8Encoding]::new($false).GetBytes((ConvertTo-Json $v -Depth 60)+"`n");$s.Write($b)}finally{$s.Dispose()};(Get-Item $p).IsReadOnly=$true;return @{path=$n;sha256=(Hash $p)}}
$raw=ReadJson (Join-Path $PSScriptRoot 'public-closed-local-model-v8-raw-boundary-v1.json')
$index=ReadJson (Join-Path $root 'index.json');$lastCase=$index.episodes[-1]
$outcomePath=Join-Path $root $lastCase.steps[-1].outcome_path;$o=ReadJson $outcomePath
if($o.state -cne 'queued_after_stopped_input' -or $o.owner_run_id -cne '' -or $o.automatic_run_ids.Count -ne 1 -or $o.automatic_run_ids[0] -cne $raw.last_entry.run_id){throw 'Last automatic/owner boundary changed'}
$traceDoc=ReadJson (Join-Path $PSScriptRoot 'native-local-model-v8-trace-metadata-v1.json');$trace=$traceDoc.traces[-1]
if($trace.sealed -or $trace.observer_failed -or $trace.open_spans -ne 0 -or $trace.dropped_events -ne 0 -or $trace.operation_failures -ne 0 -or $trace.event_count -ne 58706){throw 'Last trace boundary changed'}
$sourceFiles=@('internal/transport/antigravity/protocol.go','internal/transport/errors.go')
$sourcePins=@($sourceFiles|ForEach-Object {@{path=$_;sha256=(Hash (Join-Path $repo $_))}})
$forensic=[ordered]@{
 version='issue76-independent-local-model-v8-safe-transport-forensics-v1';window=$raw.window;source=$raw.source
 last_entry=$raw.last_entry
 actual_input_boundary=@{state=$o.state;owner_input_id=$o.owner_input_id;owner_run_id=$o.owner_run_id;blocking_input_id=$o.blocking_input_id;automatic_run_ids=$o.automatic_run_ids;native_entry_ids=$o.native_entry_ids;outcome_path=$lastCase.steps[-1].outcome_path;outcome_sha256=(Hash $outcomePath);failed_entry_is_automatic=$true;owner_second_turn_has_no_execution_run=$true}
 trace=@{event_count=$trace.event_count;paired_spans=$trace.structural_validation.spans;segments=$trace.segments;tail_events=$trace.tail_events;operation_failures=0;open_spans=0;dropped_events=0;observer_failed=$false;sealed=$false;qualified=$false;quiescence=$trace.quiescence;default_zero_quiescence_fields_are_not_observed_proof=$true}
 source_pins=$sourcePins
 source_origin=@{
  protocol_stage='protocol.go:179 sets http_round_trip before current authority/dispatch checks and client.Do'
  direct_http_failure='protocol.go:189-194 maps client.Do error with live context to fixed transport_connection_error'
  cancellation='protocol.go:183-192 may return ctx.Err; errors.go:157-159 maps context.Canceled to transport_connection_error'
  stage_boundary='protocol.go:203 enters response_stream only after successful HTTP status/content-type admission'
  conclusion='Error code/stage do not distinguish local cancellation/dispatch boundary from HTTP connection failure and do not prove upstream receipt'
  retained_reason='empty';retained_http_status=0;retained_usage=$null;retained_model_version=$null
  concrete_origin='UNKNOWN';upstream_fault_attributed=$false;network_exception_or_body_inspected=$false
 }
 first_cause_interpretation='Only three durable local stops are effect_unknown. Last provider-class transport error is GLOBAL under the frozen v8 protocol; quiescence/stream seal failure retained separately. No reconstructed precise causal cancellation claim.'
 limitations=@('No retained safe branch/cancellation-cause discriminator for n64','Zero SQL errors cannot establish successful current-authority predicates','Unsealed structurally paired trace does not qualify the last arm','No cost backfill or assertion of zero cost for NULL usage')
 no_PG_auth_native_calls=$true;answers_goldens_mapping_read=$false;quality_assessed=$false;new_window_or_retry_authorized=$false
}
$fp=SaveNew 'public-local-model-v8-safe-transport-forensics-v1.json' $forensic
$history=ReadJson (Join-Path $PSScriptRoot 'public-local-model-v8-opaque-history-bytes-v1.json')
$parentSql=Join-Path $repo '.cache/behavior76/check-closed-local-model-v8-history-v1.sql'
$parentLog=Join-Path $repo '.cache/behavior76/local-model-v8-closed-history-v1.log'
$final=[ordered]@{
 version='issue76-independent-closed-local-model-v8-boundary-v1';decision='NO_GO_INCOMPLETE_GLOBAL';window=$raw.window;source=$raw.source
 raw=$raw.raw;accounting=$raw.accounting;coverage=$raw.coverage
 additional_missing_counts=@{never_executed_original_owner_opportunities=66;original_owner_opportunities_without_owner_run=67;explanation='64 unindexed opportunities plus 2 local unexecuted tails; separately 1 admitted owner input queued behind the failed automatic input'}
 first_cause_stops=@{effect_unknown=3;model_empty_answer=0;no_reclassification_of_old_window=$true}
 last_entry=$raw.last_entry;last_input_boundary=$forensic.actual_input_boundary;last_trace=$forensic.trace;transport_origin='UNKNOWN'
 independent_prior_history=@{path='public-local-model-v8-opaque-history-bytes-v1.json';sha256=(Hash (Join-Path $PSScriptRoot 'public-local-model-v8-opaque-history-bytes-v1.json'));exact_original_row_utf8_bytes_verified=643;no_datetime_coercion=$true;no_live_database_query=$true}
 parent_database_proof=@{attribution='Parent executed; independent checker reviewed files, did not run SQL';sql_path='.cache/behavior76/check-closed-local-model-v8-history-v1.sql';sql_sha256=(Hash $parentSql);log_path='.cache/behavior76/local-model-v8-closed-history-v1.log';log_sha256=(Hash $parentLog);transaction_rollback_recorded=$true;reported_rows=707;reported_original_rows_unchanged=643;reported_no_entry65=$true}
 dependencies=@(@{path='public-closed-local-model-v8-raw-boundary-v1.json';sha256=(Hash (Join-Path $PSScriptRoot 'public-closed-local-model-v8-raw-boundary-v1.json'))},$fp,@{path='native-local-model-v8-trace-metadata-v1.json';sha256=(Hash (Join-Path $PSScriptRoot 'native-local-model-v8-trace-metadata-v1.json'))},@{path='native-local-model-v8-owner-opportunity-metadata-v1.json';sha256=(Hash (Join-Path $PSScriptRoot 'native-local-model-v8-owner-opportunity-metadata-v1.json'))})
 no_owner_family_identity_or_answers_read=$true;no_goldens_mapping_quality_scoring=$true;cost_outcome_quality_distinct=$true;no_native_replay_or_new_admission=$true;old_results_immutable=$true
}
$finalPin=SaveNew 'public-closed-local-model-v8-boundary-v1.json' $final
@{forensics=$fp;final=$finalPin}|ConvertTo-Json
