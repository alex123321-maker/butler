$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'authority-stream-scoring-v2d.ps1')
. (Join-Path $PSScriptRoot 'local-scoring-composition-v4.ps1')
. (Join-Path $PSScriptRoot 'local-tail-adapter-v4.ps1')
$frozen=Join-Path $PSScriptRoot 'score-mechanical-fresh-v3.ps1';$adapter=Join-Path $PSScriptRoot 'local-tail-adapter-v4.ps1';$composer=Join-Path $PSScriptRoot 'local-scoring-composition-v4.ps1'
$runRoot=Join-Path $PSScriptRoot ('public-stream-model-composition-'+[guid]::NewGuid().ToString('N'));[IO.Directory]::CreateDirectory($runRoot)|Out-Null
$utf8=[Text.UTF8Encoding]::new($false);$results=[Collections.Generic.List[string]]::new()
foreach($source in @(@('test-local-tail-adapter-v4.ps1',@('WriteJson','ReadJson','WriteRows','Digest','Assert','Reseal','Seed')),@('test-local-scoring-composition-v4.ps1',@('Corpus','BindPublicFiles')))){
 $tokens=$null;$errors=$null;$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $source[0]),[ref]$tokens,[ref]$errors)
 foreach($d in $ast.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.FunctionDefinitionAst] -and $_.Name -in $source[1]}){. ([scriptblock]::Create($d.Extent.Text))}
}
function StreamResult {Invoke-Issue76StreamScoringFilesV4 -ReportRoot $testRoot -StimuliPath (Join-Path $testRoot 'public-stimuli.jsonl') -GoldensPath (Join-Path $testRoot 'public-goldens.jsonl') -Pin $pin -FrozenScorerPath $frozen -AdapterPath $adapter -CompositionPath $composer}
function SetPublicStream {
 $q=ReadJson ($armDir+'/quiescence.json')
 $events=@();foreach($edge in @('start','end')){$events+=@{sequence=$events.Count+1;span=1;phase='query';edge=$edge;at_utc='2026-10-26T00:00:00Z';elapsed_us=0;original_context_deadline_utc=$null;original_context_time_left_us=$null;query_family='other_query';outcome=$(if($edge -ceq 'start'){'pending'}else{'ok'});context_cause='none';pool=@{available=$false;max_conns=0;total_conns=0;acquired_conns=0;idle_conns=0}}}
 $raw=[Text.Encoding]::UTF8.GetBytes((($events|ForEach-Object {$_|ConvertTo-Json -Compress -Depth 10}) -join "`n")+"`n")
 $rel=$armDir+'/authority-segment-000001.jsonl.gz';$path=Join-Path $testRoot $rel;$file=[IO.File]::Open($path,[IO.FileMode]::CreateNew);$gz=[IO.Compression.GZipStream]::new($file,[IO.Compression.CompressionLevel]::Optimal)
 try{$gz.Write($raw,0,$raw.Length)}finally{$gz.Dispose();$file.Dispose()}
 $bytes=(Get-Item -LiteralPath $path).Length
 WriteJson ($armDir+'/authority-trace.json') @{version='behavior76-authority-pg-trace-stream-v2';sealed=$true;event_count=2;last_sequence=2;maximum_buffered_events=4096;maximum_segments=16384;maximum_compressed_bytes=536870912;total_compressed_bytes=$bytes;identity=@{window=$pin.window;arm=$armDir;schema=$q.schema;producer=$pin.producer;protocol_sha256=$pin.protocol_sha256};segments=@(@{path='authority-segment-000001.jsonl.gz';first_sequence=1;last_sequence=2;event_count=2;compressed_bytes=$bytes;uncompressed_bytes=$raw.Length;sha256=(Digest $rel);uncompressed_sha256=[Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($raw)).ToLowerInvariant()});tail=@();open_spans=0;dropped_events=0;operation_failures=0;observer_failed=$false}
 Reseal
}
Corpus -Ordinary;SetPublicStream
$original=Invoke-Issue76LocalScoringFilesV4 -ReportRoot $testRoot -StimuliPath (Join-Path $testRoot 'public-stimuli.jsonl') -GoldensPath (Join-Path $testRoot 'public-goldens.jsonl') -Pin $pin -FrozenScorerPath $frozen -AdapterPath $adapter
$new=StreamResult
Assert ((Get-Issue76Canonical $original.mechanical_checks) -ceq (Get-Issue76Canonical $new.mechanical_checks)) 'Streaming changed criterion output'
Assert ((Get-Issue76Canonical $original.masked_semantic_template) -ceq (Get-Issue76Canonical $new.masked_semantic_template)) 'Streaming changed semantic availability'
Assert ($new.owner_rows.Count -eq 96 -and $new.authority_stream_prevalidation.traces.Count -eq 1) 'Streaming denominator or validation missing'
$results.Add('original-criteria-and-96-rows-identical')
$trace=ReadJson ($armDir+'/authority-trace.json');$trace.observer_failed=$true;WriteJson ($armDir+'/authority-trace.json') $trace;Reseal
$caught=$false;try{$null=StreamResult}catch{$caught=$_.Exception.Message.Contains('authority_stream:observer_failure')};Assert $caught 'Observer fault reached criterion execution';$results.Add('observer-fault-blocks-composition')
Corpus;SetPublicStream;$new=StreamResult
Assert (@($new.owner_rows|Where-Object disposition -CEQ 'not_executed_after_fixture_local_unknown').Count -eq 1 -and $new.owner_rows.Count -eq 96) 'Streaming local tail unavailable semantics changed'
$results.Add('local-stop-tail-remains-unavailable')
$trace=ReadJson ($armDir+'/authority-trace.json');$trace.segments[0].uncompressed_sha256='0'*64;WriteJson ($armDir+'/authority-trace.json') $trace;Reseal
$caught=$false;try{$null=StreamResult}catch{$caught=$_.Exception.Message.Contains('authority_stream:uncompressed_hash')};Assert $caught 'Corrupt stream reached criterion execution';$results.Add('resealed-corrupt-stream-blocks-composition')
foreach($source in @(@('test-local-model-proof-v4.ps1',@('Journal','Attempts','Stops','ModelSeed')),@('test-local-model-composition-v4.ps1',@('ModelCorpus')))){
 $tokens=$null;$errors=$null;$ast=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $source[0]),[ref]$tokens,[ref]$errors)
 foreach($d in $ast.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.FunctionDefinitionAst] -and $_.Name -in $source[1]}){. ([scriptblock]::Create($d.Extent.Text))}
}
ModelCorpus;SetPublicStream;$new=StreamResult
Assert ($new.masked_semantic_template.Count-eq96-and$new.native_failure_refs.Count-eq1-and$new.native_failure_refs[0].success-ceq$false-and@($new.masked_semantic_template|Where-Object {$_.failure_class-ceq'model_empty_answer'-and$_.verdict-ceq'failed'}).Count-eq1-and@($new.owner_rows|Where-Object {$_.disposition-ceq'not_executed_after_fixture_local_model_failure'-and$_.verdict-ceq'unavailable'}).Count-eq1) 'Streaming lost failed/unavailable96 model disposition'
$results.Add('model-failure-failed-tail-unavailable-full96')
ModelCorpus -Automatic;SetPublicStream;$new=StreamResult
Assert ($new.native_failure_refs[0].owner_run-ceq$false-and@($new.masked_semantic_template|Where-Object {$_.answer-ceq'PUBLIC actual owner final'-and$_.verdict-ceq'unassessed'}).Count-eq1) 'Streaming erased actual owner final after automatic failure'
$results.Add('automatic-failure-retains-actual-owner-final')
@{version='issue76-stream-model-composition-selftests-v2';passed=$results.Count;failed=0;tests=$results.ToArray();provider_calls=0;pg_calls=0;hidden_controls_read=$false;mapping_read=$false;criteria_changed=$false;thresholds_changed=$false;public_test_root=$runRoot}|ConvertTo-Json -Depth 10
