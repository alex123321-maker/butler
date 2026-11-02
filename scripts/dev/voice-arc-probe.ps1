[CmdletBinding()]
param(
  [Parameter(Mandatory)][string]$PythonExecutable,
  [Parameter(Mandatory)][ValidatePattern('^[a-f0-9]{64}$')][string]$SDKReportSHA256,
  [Parameter(Mandatory)][ValidatePattern('^[a-z0-9][a-z0-9-]{0,47}$')][string]$RunName
)
$ErrorActionPreference='Stop'
$repo=(Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$cache=Join-Path $repo '.cache/voice29-arc'
$python=(Resolve-Path -LiteralPath $PythonExecutable).Path
if(-not $IsWindows -or [IO.Path]::GetExtension($python)-cne'.exe' -or -not(Test-Path -LiteralPath (Join-Path (Split-Path (Split-Path $python -Parent) -Parent) 'pyvenv.cfg'))){throw 'Explicit Windows virtual-environment Python required'}
if(-not $python.StartsWith($cache+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Use a fresh repository-local SDK environment'}
$sdkReport=Join-Path $cache 'sdk-install-report-v1.json'
if(-not(Test-Path -LiteralPath $sdkReport) -or (Get-Item $sdkReport).Length-gt4MB -or (Get-FileHash $sdkReport).Hash.ToLowerInvariant()-cne$SDKReportSHA256){throw 'Exact retained SDK installation report required'}
$packages=(Get-Content $sdkReport -Raw|ConvertFrom-Json).install
$torch=@($packages|Where-Object {$_.metadata.name-ceq'torch'})
if($torch.Count-ne1-or$torch[0].download_info.archive_info.hashes.sha256-cne'3ad605be4728b6d3a28a44d07dd794b1a9e45551b0057815bf25eb2a6d6a56a7'){throw 'The SDK report does not bind the pinned official torch wheel'}
if((git -C $repo status --porcelain)){throw 'Commit the reviewed probe source before execution'}
$out=Join-Path $cache $RunName
if(Test-Path -LiteralPath $out){throw 'Use a new diagnostic identity; preserve earlier results'}
New-Item -ItemType Directory -Path $out|Out-Null
foreach($name in @('tmp','torch','hf','sycl')){New-Item -ItemType Directory -Path (Join-Path $cache $name) -Force|Out-Null}
$probe=Join-Path $PSScriptRoot 'voice-arc-probe.py'
$start=[Diagnostics.ProcessStartInfo]::new()
$start.FileName=$python
$start.WorkingDirectory=$repo
$start.UseShellExecute=$false
$start.CreateNoWindow=$true
$start.RedirectStandardOutput=$true
$start.RedirectStandardError=$true
$start.ArgumentList.Add('-I')
$start.ArgumentList.Add($probe)
$start.Environment.Clear()
foreach($name in @('SystemRoot','WINDIR')){if([Environment]::GetEnvironmentVariable($name)){$start.Environment[$name]=[Environment]::GetEnvironmentVariable($name)}}
$start.Environment['PATH']=(Split-Path $python -Parent)+';'+(Join-Path ([Environment]::GetEnvironmentVariable('SystemRoot')) 'System32')
foreach($name in @('TEMP','TMP')){$start.Environment[$name]=Join-Path $cache 'tmp'}
$start.Environment['TORCH_HOME']=Join-Path $cache 'torch'
$start.Environment['HF_HOME']=Join-Path $cache 'hf'
$start.Environment['SYCL_CACHE_DIR']=Join-Path $cache 'sycl'
foreach($name in @('HF_HUB_OFFLINE','TRANSFORMERS_OFFLINE')){$start.Environment[$name]='1'}
foreach($name in @('OMP_NUM_THREADS','MKL_NUM_THREADS')){$start.Environment[$name]='2'}
$started=[DateTimeOffset]::UtcNow
$process=[Diagnostics.Process]::Start($start)
$stdout=$process.StandardOutput.ReadToEndAsync()
$stderr=$process.StandardError.ReadToEndAsync()
$deadline=$started.AddSeconds(120)
$sampledRSS=0L
$stopped=$false
try{
  while(-not $process.WaitForExit(250)){
    $process.Refresh()
    $sampledRSS=[Math]::Max($sampledRSS,$process.WorkingSet64)
    if([DateTimeOffset]::UtcNow-ge$deadline-or$sampledRSS-ge2GB){$stopped=$true;$process.Kill($true);break}
  }
  if(-not $process.WaitForExit(10000)){throw 'Diagnostic tree termination not confirmed'}
  $output=$stdout.GetAwaiter().GetResult()
  $errorOutput=$stderr.GetAwaiter().GetResult()
  [IO.File]::WriteAllText((Join-Path $out 'stdout.json'),$output,[Text.UTF8Encoding]::new($false))
  [IO.File]::WriteAllText((Join-Path $out 'stderr.log'),$errorOutput,[Text.UTF8Encoding]::new($false))
  $record=[ordered]@{version='issue29-local-arc-probe-process-v1';started_utc=$started;finished_utc=[DateTimeOffset]::UtcNow;producer=(git -C $repo rev-parse HEAD).Trim();script_sha256=(Get-FileHash $probe).Hash.ToLowerInvariant();sdk_install_report_sha256=$SDKReportSHA256;exit_code=$process.ExitCode;stopped_at_bound=$stopped;sampled_peak_RSS_bytes=$sampledRSS;RSS_sampling_millis=250;deadline_seconds=120;hard_memory_limit_enforced=$false;model_loads=0;microphone_capture=$false;resource_admission_qualified=$false;voice_service_changed=$false}
  [IO.File]::WriteAllText((Join-Path $out 'process.json'),($record|ConvertTo-Json -Depth 6)+"`n",[Text.UTF8Encoding]::new($false))
  if($stopped-or$process.ExitCode-ne0){throw 'Diagnostic failed; retain output without retrying this identity'}
  $result=$output|ConvertFrom-Json
  if($result.format-cne'butler.voice.arc-hardware-probe.v1'-or$result.result-cne'FIXED_BF16_TENSOR_PASS_NOT_VOICE_ACCEPTANCE'-or$result.resource_admission_qualified-cne$false){throw 'Fixed probe output unavailable'}
  $record|ConvertTo-Json -Compress
}finally{
  if(-not$process.HasExited){$process.Kill($true);[void]$process.WaitForExit(10000)}
  $process.Dispose()
}
