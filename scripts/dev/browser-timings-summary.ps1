[CmdletBinding()]
param(
 [string]$SourceDirectory='docs/testing/browser-baseline/timings',
 [string]$OutputDirectory='.cache/browser-timings-summary'
)
$ErrorActionPreference='Stop'
if(Test-Path -LiteralPath $OutputDirectory){throw 'Use a new output directory; prior measurements must be retained.'}
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
$steps=@(); $models=@(); $trials=@()
function Seconds($value){if($null -eq $value){return $null}; return [math]::Round([double]$value/1000,3)}
function Timestamp($value){if($null -eq $value){return $null}; return ([datetimeoffset]$value).ToUniversalTime().ToString('o')}
foreach($file in Get-ChildItem -LiteralPath $SourceDirectory -Filter '*.json'|Sort-Object Name){
 $m=Get-Content -LiteralPath $file.FullName -Raw|ConvertFrom-Json
 if($m.scope -ne 'existing_record_timings_only'){continue}
 foreach($s in $m.tool_spans){$steps += [pscustomobject]@{event=$m.event;step=$s.ordinal;tool=$s.tool;status=$s.status;duration_s=(Seconds $s.duration_ms);gap_before_s=(Seconds $s.gap_before_ms);started_at=(Timestamp $s.started_at);finished_at=(Timestamp $s.finished_at);overlaps_previous=$s.overlaps_previous;local_validation_without_dispatch=$s.local_validation_without_dispatch}}
 foreach($a in $m.model_attempt_spans){$models += [pscustomobject]@{event=$m.event;attempt=$a.ordinal;reservation_to_usage_s=(Seconds $a.reservation_to_usage_ms);input_tokens=$a.input_tokens;output_tokens=$a.output_tokens;usage_known=$a.usage_known}}
 $tg=@($m.deliveries|Where-Object {$_.channel -eq 'telegram' -and $_.state -eq 'sent'})
 $trials += [pscustomobject]@{event=$m.event;status=$m.input_status;error_code=$m.error_code;input_duration_s=(Seconds $m.input_duration_ms);telegram_sent_s=$(if($tg.Count -eq 1){Seconds $tg[0].since_input_ms}else{$null});tool_calls=@($m.tool_spans).Count;model_attempts=@($m.model_attempt_spans).Count;unknown_usage_pairs=@($m.model_attempt_spans|Where-Object {-not $_.usage_known}).Count}
}
$aggregate=foreach($group in $steps|Group-Object tool,status|Sort-Object Name){
 $values=@($group.Group|Where-Object {$null -ne $_.duration_s}|ForEach-Object {[double]$_.duration_s}|Sort-Object)
 $median=$null
 if($values.Count){$center=[int][math]::Floor($values.Count/2); $median=if($values.Count%2){$values[$center]}else{($values[$center-1]+$values[$center])/2}}
 [pscustomobject]@{tool=$group.Group[0].tool;status=$group.Group[0].status;calls=$group.Count;measured_calls=$values.Count;sum_s=$(if($values.Count){[math]::Round(($values|Measure-Object -Sum).Sum,3)}else{$null});median_s=$(if($null -ne $median){[math]::Round($median,3)}else{$null});min_s=$(if($values.Count){$values[0]}else{$null});max_s=$(if($values.Count){$values[-1]}else{$null})}
}
$steps|Export-Csv -NoTypeInformation -Encoding utf8 (Join-Path $OutputDirectory 'actions.csv')
$models|Export-Csv -NoTypeInformation -Encoding utf8 (Join-Path $OutputDirectory 'model-accounting-spans.csv')
$trials|Export-Csv -NoTypeInformation -Encoding utf8 (Join-Path $OutputDirectory 'trials.csv')
$aggregate|Export-Csv -NoTypeInformation -Encoding utf8 (Join-Path $OutputDirectory 'tools.csv')
$lines=@('# Browser action timings from existing records','','No scenario, Input, model or browser action was rerun. Empty CSV fields mean unknown, not zero. CSV timestamps are explicit UTC.','','Tool durations are recorded Core spans including Broker/runtime overhead. Gaps include model, scheduling and context work. Model reservation-to-usage spans are accounting observations, not provider-only latency. State/model/tool spans overlap and cannot be added as disjoint wall-time components. Telegram sent is a persisted API acknowledgement, not a human-read receipt. Failed/unknown tool spans are grouped separately from completed operations.','','| Tool | Status | Calls | Measured | Median, s | Min, s | Max, s |','| --- | --- | --- | --- | --- | --- | --- |')
foreach($g in $aggregate){$lines += "| $($g.tool) | $($g.status) | $($g.calls) | $($g.measured_calls) | $($g.median_s) | $($g.min_s) | $($g.max_s) |"}
$lines += @('','actions.csv contains every measured step and gap; trials.csv retains failures and unknown usage. Input status is a lifecycle fact, not browser/task qualification; HTTP fallbacks are not full browser coverage (see private-comparison observation audits). model-accounting-spans.csv keeps NULL/unobserved spans empty. These heterogeneous trial aggregates are descriptive; they are not an SLO or p95. Raw JSON retains state spans and publication/delivery timestamps.')
[IO.File]::WriteAllText((Join-Path $OutputDirectory 'README.md'),($lines -join "`n")+"`n",[Text.UTF8Encoding]::new($false))
Write-Output "Derived $($trials.Count) existing trials and $($steps.Count) tool spans; zero new model calls or action replays."
$aggregate|ConvertTo-Json -Compress
