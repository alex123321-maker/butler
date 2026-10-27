[CmdletBinding()]
param([Parameter(Mandatory)][string]$OutputRoot)
$ErrorActionPreference='Stop'
if(Test-Path -LiteralPath $OutputRoot){throw 'New public output required'}
$out=(New-Item -ItemType Directory -Path $OutputRoot).FullName
. (Join-Path $PSScriptRoot 'literal-json-token-boundary-reanalysis-v7.ps1')
. (Join-Path $PSScriptRoot 'literal-kernel-proof-v1.ps1')
. (Join-Path $PSScriptRoot 'local-scoring-composition-v4.ps1')
$checks=[Collections.Generic.List[object]]::new()
function NeedToken([bool]$Condition){if(-not $Condition){throw 'public_JSON_token_identity_assertion_failed'}}
function PublicCheck([string]$Name,[scriptblock]$Body){try{& $Body;$checks.Add(@{name=$Name;verdict='pass'})}catch{$checks.Add(@{name=$Name;verdict='fail';public_error=$_.Exception.Message});Write-Output ('PUBLIC_CHECK_FAIL '+$Name+': '+$_.Exception.Message)}}
function RejectToken([scriptblock]$Body){$failed=$false;try{& $Body|Out-Null}catch{$failed=$true};NeedToken $failed}
function SameTreeToken($Element,$Actual){
 switch($Element.ValueKind.ToString()){
  'Object'{
   NeedToken ($Actual -is [Collections.IDictionary])
   $expected=@($Element.EnumerateObject());NeedToken ($Actual.Count -eq $expected.Count)
   foreach($p in $expected){NeedToken (@($Actual.Keys|Where-Object {$_ -ceq $p.Name}).Count -eq 1);SameTreeToken $p.Value $Actual[$p.Name]}
  }
  'Array'{NeedToken ($Actual -is [array]);$items=@($Element.EnumerateArray());NeedToken ($Actual.Count -eq $items.Count);for($i=0;$i -lt $items.Count;$i++){SameTreeToken $items[$i] $Actual[$i]}}
  'String'{NeedToken ($Actual -is [string] -and $Actual -ceq $Element.GetString());NeedToken ([Convert]::ToHexString([Text.Encoding]::UTF8.GetBytes($Actual)) -ceq [Convert]::ToHexString([Text.Encoding]::UTF8.GetBytes($Element.GetString())))}
  'Number'{NeedToken ($Actual -isnot [string] -and $Actual -isnot [bool] -and $Actual -isnot [datetime] -and $Actual -isnot [datetimeoffset]);$integer=[long]0;if($Element.TryGetInt64([ref]$integer)){NeedToken ([System.Numerics.BigInteger]$Actual -eq [System.Numerics.BigInteger]$integer)}else{NeedToken ([decimal]$Actual -eq $Element.GetDecimal())}}
  'True'{NeedToken ($Actual -is [bool] -and $Actual)}
  'False'{NeedToken ($Actual -is [bool] -and -not $Actual)}
  'Null'{NeedToken ($null -eq $Actual)}
  default{throw 'public_json_kind_unexpected'}
 }
}
$sample='2041-02-03T04:05:06.007Z'
$publicObject='{"needle":"'+$sample+'","haystack":"before '+$sample+' after"}'
PublicCheck 'original-eleven-runtime-byte-pins-unchanged' {Assert-Issue76LiteralV7OriginalRuntimePins}
PublicCheck 'installed-cmdlet-capability-DateKind-String' {$c=Get-Command Microsoft.PowerShell.Utility\ConvertFrom-Json -CommandType Cmdlet;NeedToken ($c.Parameters.ContainsKey('DateKind') -and 'String' -cin [Enum]::GetNames($c.Parameters['DateKind'].ParameterType))}
PublicCheck 'public-baseline-default-ISO-string-converted-to-DateTime' {$v=ConvertFrom-Json $publicObject -AsHashtable;NeedToken ($v.needle -is [datetime])}
PublicCheck 'public-baseline-original-Contains-operation-reproduces-type-failure' {$v=ConvertFrom-Json $publicObject -AsHashtable;RejectToken {[void]$v.haystack.Contains($v.needle)}}
PublicCheck 'same-public-Contains-operation-type-preserved-no-predicate-edit' {Invoke-Issue76JsonStringTokenScopeV7 {$v=ConvertFrom-Json $publicObject -AsHashtable;NeedToken ($v.needle -is [string] -and $v.needle -ceq $sample -and $v.haystack.Contains($v.needle))}}
PublicCheck 'unchanged-literal-reader-inherits-string-token-binding' {Invoke-Issue76JsonStringTokenScopeV7 {$v=ConvertFrom-Issue76LiteralJsonV6 $publicObject;NeedToken ($v.needle -is [string] -and $v.needle -ceq $sample)}}
PublicCheck 'qualified-cmdlet-without-explicit-DateKind-inherits-string-token-binding' {Invoke-Issue76JsonStringTokenScopeV7 {$v=Microsoft.PowerShell.Utility\ConvertFrom-Json -InputObject $publicObject -AsHashtable;NeedToken ($v.needle -is [string] -and $v.needle -ceq $sample)}}
PublicCheck 'unchanged-nested-expected-copy-retains-JSON-token' {Invoke-Issue76JsonStringTokenScopeV7 {$v=ConvertFrom-Issue76LiteralJsonV6 $publicObject;$copy=ConvertFrom-Json (ConvertTo-Json -InputObject $v -Depth 100) -AsHashtable -Depth 100;NeedToken ($copy.needle -is [string] -and $copy.needle -ceq $sample -and $copy.haystack.Contains($copy.needle))}}
PublicCheck 'caller-default-DateTime-behavior-not-mutated-after-return' {NeedToken ((ConvertFrom-Json $publicObject -AsHashtable).needle -is [datetime])}
PublicCheck 'scope-cleanup-on-exception-leaves-caller-unchanged' {RejectToken {Invoke-Issue76JsonStringTokenScopeV7 {throw 'public_scope_only_failure'}};NeedToken ((ConvertFrom-Json $publicObject -AsHashtable).needle -is [datetime])}
PublicCheck 'disabled-parameter-defaults-fail-before-work' {& {$local:PSDefaultParameterValues=@{Disabled=$true};$state=@{called=$false};RejectToken {Invoke-Issue76JsonStringTokenScopeV7 {$state.called=$true}};NeedToken (-not $state.called)}}
PublicCheck 'unqualified-script-shadow-rejected-before-work' {& {function ConvertFrom-Json {param($InputObject) throw 'public_shadow_must_not_run'};$state=@{called=$false};RejectToken {Invoke-Issue76JsonStringTokenScopeV7 {$state.called=$true}};NeedToken (-not $state.called)}}
PublicCheck 'unrelated-caller-default-retained-and-local-copy-does-not-leak' {& {$local:PSDefaultParameterValues=@{'Get-Random:Minimum'=1};Invoke-Issue76JsonStringTokenScopeV7 {NeedToken ($PSDefaultParameterValues['Get-Random:Minimum'] -eq 1)};NeedToken ($PSDefaultParameterValues.Count -eq 1 -and $PSDefaultParameterValues['Get-Random:Minimum'] -eq 1)}}
PublicCheck 'prepared-entrypoint-does-not-read-absent-paths-or-run-evaluation' {$v=Invoke-Issue76LiteralOfflineReanalysisFilesV7 -ReportRoot 'PUBLIC_ABSENT_ROOT' -StimuliPath 'PUBLIC_ABSENT_STIMULI' -GoldensPath 'PUBLIC_ABSENT_GOLDENS' -Pin @{} -OriginalFrozenFailureReceiptPath 'PUBLIC_ABSENT_FAILURE' -OriginalFrozenFailureReceiptSHA256 ('0'*64);NeedToken ($v.status -ceq 'PREPARED_ONLY_NO_EVALUATION' -and -not $v.scoring_executed)}
$examples=@(
 'null','true','false','0','-7','9007199254740993','1.25','1e3','""',
 '"2041-02-03T04:05:06.007Z"','"2041-02-03T04:05:06.123456789Z"','"2041-02-03T04:05:06+07:00"','"2041-02-03T04:05:06-03:30"','"2041-02-03"','"prefix 2041-02-03T04:05:06Z suffix"',
 '"2041-02-03T04:05:06\u002e007Z"','"Данные Ω 2041-02-03T04:05:06Z"',
 '[]','[null]','["2041-02-03T04:05:06.007Z"]','{}','{"A":1,"a":2}',
 '{"nested":{"when":"2041-02-03T04:05:06+07:00","negative":false,"missing":null},"values":["2041-02-03T04:05:06.007Z",true,null,1.25,{"stamp":"2041-02-03"}]}',
 '["2041-02-03T04:05:06Z",[1,2],{"stamp":"2041-02-03T04:05:06.123456789Z"}]'
)
for($number=0;$number -lt $examples.Count;$number++){
 $fixture=$examples[$number]
 PublicCheck ('generic-JSON-kind-tree-and-exact-string-UTF8-'+$number) {
  Invoke-Issue76JsonStringTokenScopeV7 {
   # NoEnumerate is only a PUBLIC test's holding container for a root array;
   # it is not installed as an old-reader/default behavior change.
   $actual=Microsoft.PowerShell.Utility\ConvertFrom-Json -InputObject $fixture -AsHashtable -DateKind String -NoEnumerate -Depth 100
   $d=[Text.Json.JsonDocument]::Parse($fixture);try{SameTreeToken $d.RootElement $actual}finally{$d.Dispose()}
  }
 }
}
PublicCheck 'unchanged-literal-reader-duplicate-rejection-preserved' {Invoke-Issue76JsonStringTokenScopeV7 {RejectToken {ConvertFrom-Issue76LiteralJsonV6 '{"when":"2041-02-03","when":"2042-02-03"}'}}}
PublicCheck 'unchanged-typed-case-alias-rejection-preserved' {Invoke-Issue76JsonStringTokenScopeV7 {$v=ConvertFrom-Issue76LiteralJsonV6 '{"When":"2041-02-03"}';RejectToken {Assert-Issue76LiteralKeysV6 $v @('when')}}}
PublicCheck 'arbitrary-DATA-case-distinction-preserved' {Invoke-Issue76JsonStringTokenScopeV7 {$v=ConvertFrom-Issue76LiteralJsonV6 '{"A":"2041-02-03","a":"2042-02-03"}';NeedToken ($v.Count -eq 2 -and $v['A'] -cne $v['a'])}}
$scorer=Join-Path $PSScriptRoot 'score-mechanical-fresh-v3.ps1'
$before=Get-Issue76FrozenKernel $scorer
$after=Invoke-Issue76JsonStringTokenScopeV7 {Get-Issue76FrozenKernel $scorer}
PublicCheck 'exact-frozen-ReadJson-function-inherits-token-binding-on-public-file' {
 $publicPath=Join-Path $out 'generic-public-legacy-reader.json'
 [IO.File]::WriteAllText($publicPath,$publicObject,[Text.UTF8Encoding]::new($false))
 Invoke-Issue76JsonStringTokenScopeV7 {
  $evidenceRoot=$out
  foreach($definition in $before.functions){. ([scriptblock]::Create($definition.Extent.Text))}
  $value=ReadJson 'generic-public-legacy-reader.json'
  NeedToken ($value.needle -is [string] -and $value.needle -ceq $sample -and $value.haystack.Contains($value.needle))
 }
}
$formal=Microsoft.PowerShell.Utility\ConvertFrom-Json -InputObject ([IO.File]::ReadAllText((Join-Path $PSScriptRoot 'literal-v11-formal-criterion-witness-v1.json'))) -AsHashtable -DateKind String -Depth 100
$oldStatements=@($before.witnesses|Where-Object kind -CEQ 'statement');$newStatements=@($after.witnesses|Where-Object kind -CEQ 'statement')
PublicCheck 'all27-original-and-new-statement-AST-witnesses-identical' {NeedToken ($oldStatements.Count -eq 27 -and $newStatements.Count -eq 27);for($i=0;$i -lt 27;$i++){NeedToken ($oldStatements[$i].sha256 -ceq $newStatements[$i].sha256 -and $oldStatements[$i].sha256 -ceq $formal.statement_extents[$i].sha256)}}
PublicCheck 'all12-original-helper-AST-witnesses-identical' {$a=@($before.witnesses|Where-Object kind -CEQ 'function');$b=@($after.witnesses|Where-Object kind -CEQ 'function');NeedToken ($a.Count -eq 12 -and $b.Count -eq 12);for($i=0;$i -lt 12;$i++){NeedToken ($a[$i].sha256 -ceq $b[$i].sha256)}}
PublicCheck 'old11-byte-pins-still-identical-after-all-public-tests' {Assert-Issue76LiteralV7OriginalRuntimePins}
$witness=[ordered]@{version='issue76-literal-JSON-token-reanalysis-v7-public-criterion-witness-v1';old_source_sha256=$before.source_sha256;new_source_sha256=$after.source_sha256;unchanged=$true;old_per_case_types=25;old_global_matrix_types=2;original_statement_extents=$oldStatements;new_statement_extents=$newStatements;helper_function_extents=@($after.witnesses|Where-Object kind -CEQ 'function');actual_evaluation_run=$false}
$wpath=Join-Path $out 'criterion-witness.json';[IO.File]::WriteAllText($wpath,($witness|ConvertTo-Json -Depth 20),[Text.UTF8Encoding]::new($false))
$receipt=[ordered]@{version='issue76-literal-JSON-token-boundary-v7-public-tests-v1';status='PREPARED_ONLY_PUBLIC_GENERIC_VALUES_NO_REANALYSIS';PowerShell=$PSVersionTable.PSVersion.ToString();cmdlet_module=(Get-Command Microsoft.PowerShell.Utility\ConvertFrom-Json).ModuleName;actual_DateKind_String_capability=$true;PASS=@($checks|Where-Object verdict -CEQ 'pass').Count;FAIL=@($checks|Where-Object verdict -CEQ 'fail').Count;checks=$checks;criterion_witness_sha256=(Get-FileHash -LiteralPath $wpath -Algorithm SHA256).Hash.ToLowerInvariant();original_runtime_files_changed=0;original_frozen_FAIL_retained=$true;actual_corpus_or_answers_or_mapping_opened=$false;actual_judgment_or_scoring_run=$false;Go_PG_Docker_provider_auth_native=0;input_examples='Only generic public 2041/2042 ISO strings, scalars/objects/arrays; no actual private case/date or response.'}
$rpath=Join-Path $out 'receipt.json';[IO.File]::WriteAllText($rpath,($receipt|ConvertTo-Json -Depth 15),[Text.UTF8Encoding]::new($false))
@{path=$rpath;sha256=(Get-FileHash -LiteralPath $rpath -Algorithm SHA256).Hash.ToLowerInvariant();PASS=$receipt.PASS;FAIL=$receipt.FAIL;scoring_run=$false}|ConvertTo-Json -Compress
if($receipt.FAIL){exit 1}
