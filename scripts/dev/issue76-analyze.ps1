[CmdletBinding()]
param([Parameter(Mandatory)][string]$EvidenceRoot, [Parameter(Mandatory)][string]$OutputRoot)
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$EvidenceRoot = (Resolve-Path $EvidenceRoot).Path
if (Test-Path $OutputRoot) { throw 'Use a new analysis output directory.' }
New-Item -ItemType Directory -Path $OutputRoot | Out-Null
function Read-JSON([string]$Path) { Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json -AsHashtable }
function Hash-File([string]$Path) { (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Hash-Text([string]$Text) { [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($Text))).ToLowerInvariant() }
function Save-JSON([string]$Name, $Value) { $Value | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath (Join-Path $OutputRoot $Name) -Encoding utf8NoBOM }
function Canonical-JSON($Value) {
  if ($null -eq $Value) { return 'null' }
  if ($Value -is [Collections.IDictionary]) {
    $parts = @(foreach ($key in ($Value.Keys | Sort-Object -CaseSensitive)) { (ConvertTo-Json -InputObject ([string]$key) -Compress) + ':' + (Canonical-JSON $Value[$key]) })
    return '{' + ($parts -join ',') + '}'
  }
  if ($Value -is [array]) { return '[' + (@(foreach ($item in $Value) { Canonical-JSON $item }) -join ',') + ']' }
  return ConvertTo-Json -InputObject $Value -Compress
}
$pins = Read-JSON (Join-Path $EvidenceRoot 'metadata/pins.json')
$frozenPath = Join-Path $EvidenceRoot 'metadata/frozen.json'
if ((Hash-File $frozenPath) -ne $pins.Frozen) { throw 'Frozen bytes differ from pin.' }
$frozen = Read-JSON $frozenPath
$cases = @{}
foreach ($relative in $frozen.corpora.Keys) {
  $path = Join-Path $repo $relative
  if ((Hash-File $path) -ne $frozen.corpora[$relative]) { throw "Corpus changed: $relative" }
  foreach ($case in (Read-JSON $path).cases) { $cases[$case.id] = $case }
}
$measurements = Read-JSON (Join-Path $EvidenceRoot 'private/measurements.json')
$observations = @{}
$scheduled = @{}
foreach ($slot in $frozen.slots) { $scheduled["$($slot.input.case_id)/$($slot.input.arm)"] = $true }
foreach ($o in $measurements.observations) {
  $key = "$($o.case_id)/$($o.arm)"
  if ($observations.ContainsKey($key)) { throw 'Duplicate observation.' }
  if (-not $scheduled.ContainsKey($key)) { throw 'Observation outside frozen matrix.' }
  $observations[$key] = $o
}
$rows = @(); $packet = @(); $mapping = @()
foreach ($slot in $frozen.slots) {
  $case = $cases[$slot.original_id]
  $key = "$($slot.input.case_id)/$($slot.input.arm)"
  $o = $observations[$key]
  $sources = @(@($slot.bundle.Items.profile | ForEach-Object { $_.source_refs }) + @($slot.bundle.Items.chunks | ForEach-Object { $_.source_ref }) | Where-Object { $_ } | Sort-Object -Unique)
  $required = @($case.expected.must_select_sources | Where-Object { $null -ne $_ })
  $requiredApplied = @($case.expected.required_applied_sources | Where-Object { $null -ne $_ })
  $retained = @($required | Where-Object { $_ -cin $sources })
  $answer = $null; $valid = $false; $jsonValid = $false; $raw = ''; $status = 'not_observed'
  $outside = @(); $attributed = @(); $visibleApplied = @(); $sourceValid = $null
  if ($null -ne $o) {
    $status = $o.status
    $raw = [string]$o.output
    $original = Join-Path $EvidenceRoot ('private/final-' + $slot.input.case_id.Replace('memory:', '') + '-' + $slot.input.arm + '.txt')
    if (Test-Path $original) {
      if ((Hash-File $original) -ne (Hash-Text $raw)) { throw 'Observation does not match retained original.' }
    } elseif ($raw.Length -gt 0) { throw 'Missing original output.' }
    try {
      $document = [System.Text.Json.JsonDocument]::Parse($raw)
      $document.Dispose()
      $jsonValid = $true
      $answer = $raw | ConvertFrom-Json -AsHashtable
      $keys = $frozen.schema.required
      $valid = $answer -is [Collections.IDictionary]
      foreach ($field in $keys) { $valid = $valid -and $answer.ContainsKey($field) }
      $valid = $valid -and $answer.answer -is [string] -and $answer.proposed_action -is [string] -and $answer.profile_update -is [string] -and $answer.needs_clarification -is [bool] -and $answer.applied_sources -is [array]
      if ($valid -and $frozen.schema.properties.applied_sources.ContainsKey('maxItems')) { $valid = $answer.applied_sources.Count -le [int]$frozen.schema.properties.applied_sources.maxItems }
      if ($valid -and $frozen.schema.properties.applied_sources.ContainsKey('minItems')) { $valid = $answer.applied_sources.Count -ge [int]$frozen.schema.properties.applied_sources.minItems }
      if ($valid) {
        foreach ($ref in $answer.applied_sources) { if ($ref -isnot [string]) { $valid = $false } }
      }
    } catch { $valid = $false }
    if ($valid) {
      $outside = @($answer.applied_sources | Where-Object { $_ -cnotin $sources })
      $sourceValid = $outside.Count -eq 0
      $attributed = @($requiredApplied | Where-Object { $_ -cin $answer.applied_sources })
      $visibleApplied = @($requiredApplied | Where-Object { $_ -cin $sources })
    }
  }
  $reviewID = 'R-' + (Hash-Text ($key + '/issue76-review-v1')).Substring(0, 12)
  $expenses = @($measurements.expenses | Where-Object { $_.case_id -eq $slot.input.case_id -and $_.arm -eq $slot.input.arm -and $_.stage -eq 'generation' })
  $row = [ordered]@{case_id=$slot.original_id;split=$slot.split;arm=$slot.input.arm;review_id=$reviewID;status=$status;json_valid=$jsonValid;schema_valid=$valid;selected_sources=$sources;required_sources=$required;retained_required=$retained;missing_required=@($required|Where-Object {$_ -cnotin $sources});source_handle_membership_valid=$sourceValid;unmatched_source_handles=$outside;required_attribution=$requiredApplied;visible_required_attribution=$visibleApplied;attributed_required=$attributed;milliseconds=$o.milliseconds;generation_expenses=$expenses;selection=$slot.bundle.Diagnostics;output_sha256=(Hash-Text $raw)}
  $row.context_sha256 = Hash-Text $slot.input.memory_prompt
  $inputPath = Join-Path $EvidenceRoot ('private/input-' + $slot.input.case_id.Replace('memory:', '') + '-' + $slot.input.arm + '.json')
  $row.model_projection_sha256 = $null
  $row.model_request_sha256 = $null
  $expected = @{question=$slot.input.question;as_of_utc=$slot.input.as_of_utc;current_task=$slot.input.task;memory_data=$slot.input.memory;memory_prompt_data=$slot.input.memory_prompt}
  if (Test-Path $inputPath) {
    $sent = Read-JSON $inputPath
    if ((Canonical-JSON ($sent.Input | ConvertFrom-Json -AsHashtable)) -cne (Canonical-JSON $expected) -or $sent.Instructions -cne $slot.input.instructions -or (Canonical-JSON $sent.Schema) -cne (Canonical-JSON $frozen.schema)) { throw 'Actual model input/instructions/schema differ from frozen projection.' }
    $row.model_projection_sha256 = Hash-Text $sent.Input
    $row.model_request_sha256 = Hash-Text (Canonical-JSON @{input=$sent.Input;instructions=$sent.Instructions;schema=$sent.Schema})
  } elseif ($status -eq 'completed' -or $raw.Length -gt 0) { throw 'Missing actual model input.' }
  # References to visible typed input fields are distinct from record handles.
  # Path existence establishes visibility only, never semantic support.
  $visiblePaths = @(); $unresolvedRefs = @()
  foreach ($ref in $outside) {
    $node = $expected; $exists = $true
    foreach ($part in $ref.Split('.')) {
      if ($node -isnot [Collections.IDictionary] -or -not $node.Contains($part)) { $exists = $false; break }
      $node = $node[$part]
    }
    if ($exists) { $visiblePaths += $ref } else { $unresolvedRefs += $ref }
  }
  $row.visible_input_paths = $visiblePaths
  $row.unresolved_references = $unresolvedRefs
  $row.source_visibility_valid = if ($valid -and $null -ne $row.model_projection_sha256) { $unresolvedRefs.Count -eq 0 } else { $null }
  $rows += $row
  $packet += [ordered]@{review_id=$reviewID;case_id=$case.id;fixture=$case;selected_sources=$sources;model_instructions=$slot.input.instructions;question=$slot.input.question;task=$slot.input.task;memory_prompt=$slot.input.memory_prompt;status=$status;output=$raw;output_sha256=$row.output_sha256}
  $mapping += @{review_id=$reviewID;case_id=$case.id;arm=$slot.input.arm;split=$slot.split}
}
if ($rows.Count -ne 32) { throw 'Unexpected scheduled denominator.' }
Save-JSON 'metrics.json' @{version='issue76-context-mechanical-v1';pins=$pins;scheduled=32;observed=$observations.Count;rows=$rows;semantic_review='external_review_required';money_cost='unmeasured'}
Save-JSON 'review-packet.json' @($packet | Sort-Object review_id)
Save-JSON 'review-mapping.json' $mapping
Save-JSON 'evidence-sha256.json' @(Get-ChildItem -LiteralPath $EvidenceRoot -Recurse -File | Sort-Object FullName | ForEach-Object { @{path=[IO.Path]::GetRelativePath($EvidenceRoot,$_.FullName).Replace('\','/');sha256=(Hash-File $_.FullName)} })
Write-Output "Scheduled: 32; observed: $($observations.Count). Semantic judgments intentionally pending."
