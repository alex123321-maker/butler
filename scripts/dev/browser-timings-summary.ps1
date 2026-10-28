[CmdletBinding()]
param(
 [string]$SourceDirectory='docs/testing/browser-baseline/timings',
 [string]$OutputDirectory='.cache/browser-timings-summary'
)
$ErrorActionPreference='Stop'
if(Test-Path -LiteralPath $OutputDirectory){throw 'Use a new output directory; prior measurements must be retained.'}
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
$steps=@(); $models=@(); $trials=@(); $tails=@()
function Seconds($value){if($null -eq $value){return $null}; return [math]::Round([double]$value/1000,3)}
function Timestamp($value){if($null -eq $value){return $null}; return ([datetimeoffset]$value).ToUniversalTime().ToString('o')}
foreach($file in Get-ChildItem -LiteralPath $SourceDirectory -Filter '*.json'|Sort-Object Name){
 $m=Get-Content -LiteralPath $file.FullName -Raw|ConvertFrom-Json
 if($m.scope -ne 'existing_record_timings_only'){continue}
 $partitionKnown=$null -ne $m.input_duration_ms
 $stepTotalMs=0.0; $toolTotalMs=0.0; $gapTotalMs=0.0; $previousEndMs=0.0
 $stepStart=$m.accepted_at
 foreach($s in $m.tool_spans){
  $known=$null -ne $s.duration_ms -and $null -ne $s.gap_before_ms -and $null -ne $s.since_input_ms -and -not $s.overlaps_previous
  if($known){$known=[double]$s.duration_ms -ge 0 -and [double]$s.gap_before_ms -ge 0}
  $totalMs=$null; $endMs=$null
  if($known){
   $totalMs=[double]$s.gap_before_ms+[double]$s.duration_ms
   $endMs=[double]$s.since_input_ms+[double]$s.duration_ms
   # Verify the recorded gaps form one continuous partition. Do not silently
   # attribute missing or overlapping intervals to model thinking.
   if([math]::Abs(([double]$s.since_input_ms-[double]$s.gap_before_ms)-$previousEndMs) -gt 0.01){$partitionKnown=$false}
   $stepTotalMs+=$totalMs; $toolTotalMs+=[double]$s.duration_ms; $gapTotalMs+=[double]$s.gap_before_ms
  }else{$partitionKnown=$false}
  $steps += [pscustomobject]@{event=$m.event;step=$s.ordinal;tool=$s.tool;status=$s.status;step_total_s=(Seconds $totalMs);decision_context_wait_s=(Seconds $s.gap_before_ms);tool_span_s=(Seconds $s.duration_ms);since_input_at_end_s=(Seconds $endMs);step_started_at=(Timestamp $stepStart);tool_started_at=(Timestamp $s.started_at);finished_at=(Timestamp $s.finished_at);overlaps_previous=$s.overlaps_previous;local_validation_without_dispatch=$s.local_validation_without_dispatch}
  $stepStart=$s.finished_at; $previousEndMs=$endMs
 }
 $tailMs=$null; $coverageErrorMs=$null
 if($partitionKnown){
  $tailMs=[double]$m.input_duration_ms-$previousEndMs
  if($tailMs -lt 0){$partitionKnown=$false; $tailMs=$null}else{
   $coverageErrorMs=[double]$m.input_duration_ms-($stepTotalMs+$tailMs)
   if([math]::Abs($coverageErrorMs) -gt 0.01){$partitionKnown=$false}
  }
 }
 $tails += [pscustomobject]@{event=$m.event;phase=$(if(@($m.tool_spans).Count){'after_last_tool'}else{'entire_input_without_tool'});duration_s=(Seconds $tailMs);started_at=(Timestamp $stepStart);partition_complete=$partitionKnown}
 foreach($a in $m.model_attempt_spans){$models += [pscustomobject]@{event=$m.event;attempt=$a.ordinal;reservation_to_usage_s=(Seconds $a.reservation_to_usage_ms);input_tokens=$a.input_tokens;output_tokens=$a.output_tokens;usage_known=$a.usage_known}}
 $tg=@($m.deliveries|Where-Object {$_.channel -eq 'telegram' -and $_.state -eq 'sent'})
 $trials += [pscustomobject]@{event=$m.event;status=$m.input_status;error_code=$m.error_code;input_duration_s=(Seconds $m.input_duration_ms);telegram_sent_s=$(if($tg.Count -eq 1){Seconds $tg[0].since_input_ms}else{$null});steps_total_s=$(if($partitionKnown){Seconds $stepTotalMs}else{$null});decision_context_wait_s=$(if($partitionKnown){Seconds $gapTotalMs}else{$null});tool_spans_s=$(if($partitionKnown){Seconds $toolTotalMs}else{$null});tail_or_no_tool_s=(Seconds $tailMs);partition_complete=$partitionKnown;partition_error_ms=$coverageErrorMs;tool_calls=@($m.tool_spans).Count;model_attempts=@($m.model_attempt_spans).Count;unknown_usage_pairs=@($m.model_attempt_spans|Where-Object {-not $_.usage_known}).Count}
}
$aggregate=foreach($group in $steps|Group-Object tool,status|Sort-Object Name){
 $values=@($group.Group|Where-Object {$null -ne $_.step_total_s}|ForEach-Object {[double]$_.step_total_s}|Sort-Object)
 $median=$null
 if($values.Count){$center=[int][math]::Floor($values.Count/2); $median=if($values.Count%2){$values[$center]}else{($values[$center-1]+$values[$center])/2}}
 [pscustomobject]@{tool=$group.Group[0].tool;status=$group.Group[0].status;calls=$group.Count;measured_calls=$values.Count;sum_s=$(if($values.Count){[math]::Round(($values|Measure-Object -Sum).Sum,3)}else{$null});median_s=$(if($null -ne $median){[math]::Round($median,3)}else{$null});min_s=$(if($values.Count){$values[0]}else{$null});max_s=$(if($values.Count){$values[-1]}else{$null})}
}
$steps|Export-Csv -NoTypeInformation -Encoding utf8 (Join-Path $OutputDirectory 'actions.csv')
$models|Export-Csv -NoTypeInformation -Encoding utf8 (Join-Path $OutputDirectory 'model-accounting-spans.csv')
$trials|Export-Csv -NoTypeInformation -Encoding utf8 (Join-Path $OutputDirectory 'trials.csv')
$tails|Export-Csv -NoTypeInformation -Encoding utf8 (Join-Path $OutputDirectory 'finalization.csv')
$aggregate|Export-Csv -NoTypeInformation -Encoding utf8 (Join-Path $OutputDirectory 'tools.csv')
$lines=@('# Inclusive action timings from existing records','','No scenario, Input, model or browser action was rerun. Empty CSV fields mean unknown, not zero. CSV timestamps are explicit UTC.','','The primary step_total_s runs from the previous tool completion (Input acceptance for the first step) through this tool completion. It INCLUDES model deliberation, context preparation, queueing, approval/user waits if any, and the tool. decision_context_wait_s is the whole recorded gap, not an isolated model-only measurement. tool_span_s is diagnostic Core/Broker/runtime time. These sequential steps plus finalization.csv partition Input wall time when partition_complete=true; trials.csv reports the unrounded residual in milliseconds. Finalization includes processing after the last tool, or the entire Input if no tool ran.','','Telegram sent is time from Input acceptance to a persisted delivery API acknowledgement, not a human-read receipt. It may overlap processing/finalization and must not be added to Input duration. Model reservation-to-usage spans are accounting observations, not provider-only latency; do not add them or state spans to the partition. Failed/unknown calls remain separate from completed calls.','','| Step ending in tool | Status | Calls | Measured | Inclusive median, s | Min, s | Max, s |','| --- | --- | --- | --- | --- | --- | --- |')
foreach($g in $aggregate){$lines += "| $($g.tool) | $($g.status) | $($g.calls) | $($g.measured_calls) | $($g.median_s) | $($g.min_s) | $($g.max_s) |"}
$lines += @('','actions.csv contains every inclusive step with its gap and tool subinterval; trials.csv retains failures, unknown usage, complete wall time and partition coverage. finalization.csv prevents the post-tool/no-tool interval from disappearing. Input status is a lifecycle fact, not browser/task qualification; HTTP fallbacks are not full browser coverage (see private-comparison observation audits). model-accounting-spans.csv keeps NULL/unobserved spans empty. These heterogeneous trial aggregates include differing startup/approval/context conditions; they are descriptive, not isolated gesture benchmarks, an SLO or p95. Raw JSON retains state spans and publication/delivery timestamps. No pure model deliberation time can be recovered from these boundaries.')
[IO.File]::WriteAllText((Join-Path $OutputDirectory 'README.md'),($lines -join "`n")+"`n",[Text.UTF8Encoding]::new($false))
Write-Output "Derived $($trials.Count) existing trials and $($steps.Count) tool spans; zero new model calls or action replays."
$aggregate|ConvertTo-Json -Compress
