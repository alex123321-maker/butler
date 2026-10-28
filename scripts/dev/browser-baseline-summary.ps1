[CmdletBinding()]
param(
 [string]$EvidenceDirectory='docs/testing/browser-baseline/matrix',
 [Parameter(Mandatory=$true)][string]$Output
)
$ErrorActionPreference='Stop'
function As-Offset($value) {
 if ($value -is [DateTime]) {return [DateTimeOffset]$value}
 return [DateTimeOffset]::Parse([string]$value,[Globalization.CultureInfo]::InvariantCulture)
}
$rows=@()
foreach ($id in @('read-01','read-02','read-03')) {
 $m=Get-Content -Raw -LiteralPath (Join-Path $EvidenceDirectory "$id-metadata-final.json") | ConvertFrom-Json
 $v=Get-Content -Raw -LiteralPath (Join-Path $EvidenceDirectory "$id-verification.json") | ConvertFrom-Json
 if ($m.event -ne "browser-baseline-matrix-20261028-$id") {throw 'Mismatched evidence identity'}
 $publication=@($m.publications | Where-Object purpose -eq 'result')
 if ($publication.Count -ne 1 -or $publication[0].ref -ne $v.publication_ref) {throw 'Mismatched publication proof'}
 $browser=@($m.tools | Where-Object { $_.name -like 'browser.*' -and $_.status -eq 'completed' -and -not $_.has_error })
 $sent=@($m.deliveries | Where-Object { $_.channel -eq 'telegram' -and $_.state -eq 'sent' })
 $start=As-Offset $m.accepted_at
 $toolMilliseconds=0.0
 foreach ($tool in $m.tools) {
  if ($tool.finished_at) { $toolMilliseconds+=((As-Offset $tool.finished_at)-(As-Offset $tool.started_at)).TotalMilliseconds }
 }
 $rows+= [ordered]@{
  attempt=$id; input_status=$m.status
  narrow_read_pass=($m.status -eq 'completed' -and $browser.Count -gt 0 -and $v.recipient_and_answer_criteria_match -and $sent.Count -gt 0)
  publication_ms=[math]::Round(((As-Offset $publication[0].created_at)-$start).TotalMilliseconds)
  telegram_sent_ms=if ($sent.Count -gt 0) {[math]::Round(((As-Offset $sent[0].updated_at)-$start).TotalMilliseconds)} else {$null}
  tool_calls=@($m.tools).Count; browser_calls=$browser.Count
  tool_duration_sum_ms=[math]::Round($toolMilliseconds)
  model_attempts=$m.usage.attempts; input_tokens=$m.usage.input_tokens; output_tokens=$m.usage.output_tokens
  unknown_usage_pairs=$m.usage.unknown_usage_pairs; monetary_cost=$null
 }
}
$nav=Get-Content -Raw -LiteralPath (Join-Path $EvidenceDirectory 'nav-01-metadata-post-owner.json') | ConvertFrom-Json
if ($nav.event -ne 'browser-baseline-matrix-20261028-nav-01' -or $nav.error_code -ne 'stopped_attempt_uncertain') {throw 'Navigation blocker evidence changed; review before summarizing'}
$times=@($rows | ForEach-Object telegram_sent_ms | Sort-Object)
$report=[ordered]@{
 scope='current-deployed-composition-limited-baseline'; overall='NO_GO'
 origin='https://butler-eval.report-template.ru'; planned_matrix_attempts=15
 observed_matrix_attempts=4; narrow_read_passes=@($rows | Where-Object narrow_read_pass).Count
 stopped_navigation_attempts=1; unexecuted_matrix_attempts=11
 stop_reason=$nav.error_code
 read_telegram_sent_ms=[ordered]@{min=$times[0];median=$times[1];max=$times[2];samples=3;p95=$null}
 reads=$rows
 navigation=[ordered]@{input_status=$nav.status;model_attempts=$nav.usage.attempts;input_tokens=$nav.usage.input_tokens;output_tokens=$nav.usage.output_tokens;operator_exact_approvals=1;monetary_cost=$null}
 note='No reset or bypass of old uncertain effects. Recipient counts alone do not attribute every request to the model. No account/form acceptance claimed.'
}
$stream=[IO.File]::Open($Output,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write)
try {
 $bytes=[Text.UTF8Encoding]::new($false).GetBytes(($report | ConvertTo-Json -Depth 8))
 $stream.Write($bytes,0,$bytes.Length)
} finally {$stream.Dispose()}
Write-Host 'Limited baseline summary written; full matrix remains incomplete.'
