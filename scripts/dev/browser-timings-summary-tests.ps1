[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
$root=Join-Path $PSScriptRoot '../../.cache' ('timings-validation-'+[guid]::NewGuid().ToString('N'))
$source=Join-Path $root 'source'
New-Item -ItemType Directory -Path $source -Force|Out-Null
function Assert($condition,$message){if(-not $condition){throw $message}}
function Fixture($name,$spans,$duration=10000){
 [ordered]@{scope='existing_record_timings_only';event=$name;accepted_at='2026-01-01T00:00:00Z';input_duration_ms=$duration;input_status='failed';error_code='test';tool_spans=@($spans);model_attempt_spans=@();deliveries=@()}|ConvertTo-Json -Depth 10|Set-Content -LiteralPath (Join-Path $source "$name.json")
}
function Span($ordinal,$start,$end,$gap,$overlap=$false){
 [ordered]@{ordinal=$ordinal;tool='browser.snapshot';status='completed';started_at="2026-01-01T00:00:0${start}Z";finished_at="2026-01-01T00:00:0${end}Z";since_input_ms=$start*1000;duration_ms=($end-$start)*1000;gap_before_ms=$gap;overlaps_previous=$overlap;local_validation_without_dispatch=$false}
}
$first=Span 1 2 3 2000
Fixture 'complete' @($first,(Span 2 7 8 4000))
Fixture 'without-tool' @() 10000
Fixture 'unknown-input' @($first) $null
$missing=Span 1 2 3 2000; $missing.duration_ms=$null
Fixture 'unfinished' @($missing)
Fixture 'overlap' @($first,(Span 2 2 4 $null $true))
Fixture 'broken-gap' @($first,(Span 2 7 8 1000))
Fixture 'negative-gap' @((Span 1 2 3 -1000))
Fixture 'tool-after-input' @($first) 2000
$out=Join-Path $root 'derived'
& (Join-Path $PSScriptRoot 'browser-timings-summary.ps1') -SourceDirectory $source -OutputDirectory $out|Out-Null
$trials=@(Import-Csv (Join-Path $out 'trials.csv'))
$actions=@(Import-Csv (Join-Path $out 'actions.csv'))
$tails=@(Import-Csv (Join-Path $out 'finalization.csv'))
Assert ($trials.Count -eq 8) 'Missing synthetic trial.'
foreach($trial in $trials){
 $expected=$trial.event -in @('complete','without-tool')
 Assert (($trial.partition_complete -eq 'True') -eq $expected) "Incorrect partition coverage for $($trial.event)."
}
$complete=@($actions|Where-Object event -eq 'complete')
Assert ([double]$complete[0].step_total_s -eq 3) 'First step must include Input-to-tool delay.'
Assert ([double]$complete[1].step_total_s -eq 5) 'Second step must include four seconds of model/context/wait time.'
Assert ([double]$complete[1].tool_span_s -eq 1) 'Tool duration must stay separately visible.'
Assert ([double]($tails|Where-Object event -eq 'complete').duration_s -eq 2) 'Post-tool processing must be retained.'
Assert ([double]($tails|Where-Object event -eq 'without-tool').duration_s -eq 10) 'No-tool failure must retain the entire duration.'
Assert (($tails|Where-Object event -eq 'unknown-input').duration_s -eq '') 'Unknown terminal duration must not become zero.'
Assert (($actions|Where-Object event -eq 'unfinished').step_total_s -eq '') 'Unknown tool duration must not become zero.'
Assert (($actions|Where-Object event -eq 'negative-gap').step_total_s -eq '') 'Negative gap must not produce a valid total.'
$refused=$false
try{& (Join-Path $PSScriptRoot 'browser-timings-summary.ps1') -SourceDirectory $source -OutputDirectory $out|Out-Null}catch{$refused=$_.Exception.Message -like 'Use a new output directory*'}
Assert $refused 'Existing measurements must never be overwritten.'
Write-Output 'PASS: eight offline timing fixtures, missing/overlap/negative/inconsistent boundaries and overwrite protection; no model or browser calls.'
