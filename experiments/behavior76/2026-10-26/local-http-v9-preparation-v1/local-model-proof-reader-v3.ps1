# Public offline native-failure proof reader. Never reads a mapping or provider
# response text. Called only after the actual root journals/case bytes are sealed.
function Assert-Issue76ModelProofCondition([bool]$OK,[string]$Code){if(-not $OK){throw ('Local model proof: '+$Code)}}
function Assert-Issue76ExactKeys($Object,[string[]]$Required,[string[]]$Optional=@()){
 Assert-Issue76ModelProofCondition ($Object -is [Collections.IDictionary]) 'object required'
 $allowed=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
 foreach($k in @($Required)+@($Optional)){$null=$allowed.Add($k)}
 foreach($k in $Object.Keys){Assert-Issue76ModelProofCondition ($allowed.Contains([string]$k)) 'unknown or case-aliased property'}
 foreach($k in $Required){Assert-Issue76ModelProofCondition (@($Object.Keys|Where-Object {$_ -ceq $k}).Count -eq 1) 'required property absent'}
}
function Test-Issue76Integer($Value){return ($Value -is [int] -or $Value -is [long] -or $Value -is [uint32] -or $Value -is [uint64])}
function Get-Issue76ProofTime($Value){
 Assert-Issue76ModelProofCondition ($Value -is [string] -or $Value -is [datetime] -or $Value -is [datetimeoffset]) 'timestamp type'
 try{$t=[datetimeoffset]$Value}catch{throw 'Local model proof: timestamp invalid'}
 Assert-Issue76ModelProofCondition ($t -gt [datetimeoffset]::MinValue) 'timestamp absent'
 return $t
}
function Get-Issue76ProofRow($Rows,[string]$Field,$Value){$r=@($Rows|Where-Object {$_[''+$Field] -ceq $Value});Assert-Issue76ModelProofCondition ($r.Count -eq 1) 'missing or ambiguous durable row';return $r[0]}
function Get-Issue76NativeProjectionV3($Attempt){
 $v=[ordered]@{};foreach($k in @('window_id','n','slot','run_id','kind','status','stage','milliseconds','input_tokens','output_tokens','total_tokens','thinking_tokens','model_version','events')){
  Assert-Issue76ModelProofCondition (@($Attempt.Keys|Where-Object {$_ -ceq $k}).Count -eq 1) 'native metadata field missing';$v[$k]=$Attempt[$k]
 };return $v
}
function Assert-Issue76LocalNativeAttemptV3($Attempt,$Tables,[string]$Window,[string]$ArmDir){
 $projection=Get-Issue76NativeProjectionV3 $Attempt
 Assert-Issue76ModelProofCondition ($Attempt.window_id -ceq $Window -and $Attempt.slot -is [string] -and $Attempt.slot.StartsWith($ArmDir+'/',[StringComparison]::Ordinal)) 'foreign native window or arm'
 Assert-Issue76ModelProofCondition ((Test-Issue76Integer $Attempt.n) -and $Attempt.n -ge 1 -and $Attempt.n -le 768 -and (Test-Issue76Integer $Attempt.milliseconds) -and $Attempt.milliseconds -ge 0) 'native integer bounds'
 Assert-Issue76ModelProofCondition ($Attempt.run_id -is [string] -and -not [string]::IsNullOrWhiteSpace($Attempt.run_id) -and $Attempt.kind -cin @('start','continue','submit') -and $Attempt.status -ceq 'transport_error') 'native terminal class'
 $events=@($Attempt.events);Assert-Issue76ModelProofCondition ($events.Count -eq 2) 'exact event chain required'
 Assert-Issue76ExactKeys $events[0] @('type')
 $http=$events[1].safe_reason -ceq 'http_do_failed_without_response'
 if($http){Assert-Issue76ExactKeys $events[1] @('type','error_code','safe_reason','http_status')}
 else{Assert-Issue76ExactKeys $events[1] @('type','error_code','safe_reason','http_status') @('usage')}
 Assert-Issue76ModelProofCondition ($events[0].type -ceq 'run_started' -and $events[1].type -ceq 'transport_error' -and (($http -and $Attempt.stage -ceq 'http_round_trip' -and $events[1].error_code -ceq 'transport_connection_error') -or (-not $http -and $Attempt.stage -ceq 'response_stream' -and $events[1].error_code -ceq 'provider_protocol_error' -and $events[1].safe_reason -ceq 'stream_empty_answer')) -and (Test-Issue76Integer $events[1].http_status) -and $events[1].http_status -eq 0) 'not an exact locally eligible native branch'
 if($http){foreach($k in @('input_tokens','output_tokens','total_tokens','thinking_tokens','model_version')){Assert-Issue76ModelProofCondition ($null -eq $Attempt[$k]) 'HTTP no-response metadata must remain NULL'}}
 $usage=$events[1].usage
 if(@($events[1].Keys|Where-Object {$_ -ceq 'usage'}).Count){Assert-Issue76ModelProofCondition ($usage -is [Collections.IDictionary]) 'explicit usage must be an actual object'}
 if($null -eq $usage){foreach($k in @('input_tokens','output_tokens','total_tokens','thinking_tokens')){Assert-Issue76ModelProofCondition ($null -eq $Attempt[$k]) 'unobserved usage fabricated'}}
 else{
  Assert-Issue76ExactKeys $usage @('InputTokens','OutputTokens','TotalTokens') @('ThinkingTokens')
  foreach($k in @('InputTokens','OutputTokens','TotalTokens')){Assert-Issue76ModelProofCondition ((Test-Issue76Integer $usage[$k]) -and $usage[$k] -ge 0) 'usage count type or sign'}
  Assert-Issue76ModelProofCondition ([decimal]$usage.TotalTokens -ge [decimal]$usage.InputTokens+[decimal]$usage.OutputTokens) 'usage total inconsistent'
  if($null -ne $usage.ThinkingTokens){Assert-Issue76ModelProofCondition ((Test-Issue76Integer $usage.ThinkingTokens) -and $usage.ThinkingTokens -ge 0 -and [decimal]$usage.ThinkingTokens -le [decimal]$usage.TotalTokens-[decimal]$usage.InputTokens-[decimal]$usage.OutputTokens) 'thinking usage inconsistent'}
  foreach($pair in @(@('input_tokens','InputTokens'),@('output_tokens','OutputTokens'),@('total_tokens','TotalTokens'),@('thinking_tokens','ThinkingTokens'))){Assert-Issue76ModelProofCondition ((Get-Issue76Canonical $Attempt[$pair[0]]) -ceq (Get-Issue76Canonical $usage[$pair[1]])) 'event versus row usage mismatch'}
 }
 Assert-Issue76ModelProofCondition ($null -eq $Attempt.model_version -or ($Attempt.model_version -is [string] -and $Attempt.model_version -cmatch '^gemini-[a-zA-Z0-9][a-zA-Z0-9._-]{0,110}$')) 'observed version shape'
 $run=Get-Issue76ProofRow $Tables.runs 'run_id' $Attempt.run_id
 $input=Get-Issue76ProofRow $Tables.accepted_inputs 'run_id' $Attempt.run_id
 Assert-Issue76ModelProofCondition ($run.session_key -ceq $input.session_key -and -not [string]::IsNullOrWhiteSpace([string]$input.input_id) -and (Test-Issue76Integer $input.session_sequence) -and $input.session_sequence -ge 1) 'native run and input ownership mismatch'
 if(-not [string]::IsNullOrWhiteSpace([string]$run.task_id)){$null=Get-Issue76ProofRow $Tables.tasks 'task_id' $run.task_id}
 return @{projection=$projection;run=$run;input=$input;cause=$(if($http){'model_http_no_response'}else{'model_empty_answer'})}
}
function Assert-Issue76ModelStopV3($Stop,$Tables,$ArmAttempts,[string]$Window,[string]$ArmDir){
 $proof=$Stop.native_evidence
 Assert-Issue76ExactKeys $proof @('window_id','n','slot','run_id','kind','status','stage','milliseconds','input_tokens','output_tokens','total_tokens','thinking_tokens','model_version','events','owner_inputs','native_admission_cutoff_n')
 Assert-Issue76ModelProofCondition ($Stop.cause -cin @('model_empty_answer','model_http_no_response') -and @($Stop.effects).Count -eq 0) 'model cause union'
 $attempt=Get-Issue76ProofRow $ArmAttempts 'n' $proof.n
 $validated=Assert-Issue76LocalNativeAttemptV3 $attempt $Tables $Window $ArmDir
 Assert-Issue76ModelProofCondition ($Stop.cause -ceq $validated.cause) 'native cause does not match actual branch'
 $actual=$validated.projection;$copy=[ordered]@{};foreach($k in $actual.Keys){$copy[$k]=$proof[$k]}
 Assert-Issue76ModelProofCondition ((Get-Issue76Canonical $copy) -ceq (Get-Issue76Canonical $actual)) 'native proof is not exact journal metadata'
 $w=@($proof.owner_inputs);Assert-Issue76ModelProofCondition ($w.Count -eq 1) 'one actual input witness required'
 Assert-Issue76ExactKeys $w[0] @('input_id','run_id','session_key','session_sequence')
 foreach($k in @('input_id','run_id','session_key','session_sequence')){Assert-Issue76ModelProofCondition ((Get-Issue76Canonical $w[0][$k]) -ceq (Get-Issue76Canonical $validated.input[$k])) 'foreign input witness'}
 Assert-Issue76ModelProofCondition ((Test-Issue76Integer $proof.native_admission_cutoff_n) -and $proof.native_admission_cutoff_n -ge $proof.n -and $proof.native_admission_cutoff_n -le 768) 'native reservation cutoff'
 $barrier=Get-Issue76ProofTime $Stop.observed_at
 foreach($a in $ArmAttempts){Assert-Issue76ModelProofCondition ($a.n -le $proof.native_admission_cutoff_n -and (Get-Issue76ProofTime $a.started_at) -le $barrier) 'same-arm reservation after barrier'}
 return $validated
}
