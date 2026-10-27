$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'local-tail-adapter-v2.ps1')
$utf8=[Text.UTF8Encoding]::new($false);$runRoot=Join-Path $PSScriptRoot ('public-model-union-v2-'+[guid]::NewGuid().ToString('N'));$null=New-Item -ItemType Directory $runRoot
foreach($spec in @(@('test-local-tail-adapter-v2.ps1',@('WriteJson','ReadJson','WriteRows','Digest','Assert','Reseal','Seed')),@('test-local-model-proof-v2.ps1',@('Journal','Attempts','Stops','ModelSeed')))){
 $t=$null;$e=$null;$a=[Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $spec[0]),[ref]$t,[ref]$e)
 foreach($d in $a.EndBlock.Statements|Where-Object {$_ -is [Management.Automation.Language.FunctionDefinitionAst] -and $_.Name -in $spec[1]}){. ([scriptblock]::Create($d.Extent.Text))}
}
$testRoot=Join-Path $runRoot 'effect-only';$null=New-Item -ItemType Directory $testRoot;Seed
$s=Stops;$s.native_evidence=$null;WriteRows 'behavior76_fixture_stops' $s;Reseal
$r=Read-Issue76LocalEvidenceV2 -ReportRoot $testRoot -Plan $plan -Pin $pin
Assert ($r.owner_rows.Count-eq96-and$r.native_failure_refs.Count-eq0-and$r.tail_opportunities-eq1) 'DDL NULL native_evidence on effect_unknown rejected'
Write-Output 'PASS effect_unknown_explicit_NULL_native_evidence_and_unchanged_tail'
ModelSeed -LateUnknown
$oldEffect=Get-Content (Join-Path $testRoot ($armDir+'/tables/action_effects.jsonl')) -Raw|ConvertFrom-Json -AsHashtable
$oldInput=Get-Content (Join-Path $testRoot ($armDir+'/tables/accepted_inputs.jsonl')) -Raw|ConvertFrom-Json -AsHashtable
$dispatch=Get-Content (Join-Path $testRoot ($armDir+'/tables/action_effect_dispatches.jsonl')) -Raw|ConvertFrom-Json -AsHashtable
$s=Stops;$s.cause='effect_unknown';$s.native_evidence=$null;$s.effects=@(@{effect=$oldEffect;intent_binding=$oldEffect.initial_binding;owner_inputs=@($oldInput);dispatches=@($dispatch)})
WriteRows 'behavior76_fixture_stops' $s
$i=ReadJson 'index.json';foreach($step in $i.episodes[0].steps){$o=ReadJson $step.outcome_path;$o.state=$o.state.Replace('fixture_local_model_failure','fixture_local_unknown');WriteJson $step.outcome_path $o};Reseal
$r=Read-Issue76LocalEvidenceV2 -ReportRoot $testRoot -Plan $plan -Pin $pin
Assert ($r.native_failure_refs.Count-eq1-and$r.native_failure_refs[0].success-ceq$false-and$r.owner_rows.Count-eq96-and$r.retained_observation_refs[0].actual_effect_ids.Count-eq1) 'Immutable effect-first stop hid later native failure'
Write-Output 'PASS immutable_effect_first_cause_retains_late_eligible_native_failure'
Write-Output 'SELFTEST_PASS 2 no_native_no_PG_no_hidden'
