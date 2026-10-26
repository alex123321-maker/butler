$ErrorActionPreference='Stop'
$repo='D:\CodexData\home\worktrees\issue29-arc-local\butler'
Set-Location $repo
$cache=Join-Path $repo '.cache/voice29-arc'
$base='C:\Users\alexa\.cache\codex-runtimes\codex-primary-runtime\dependencies\python\python.exe'
$venv=Join-Path $cache 'venv312-v1'
$logs=Join-Path $cache 'setup-v1'
if(Test-Path $venv){throw 'Fresh virtual environment required'}
if(Test-Path $logs){throw 'Fresh SDK preparation identity required'}
New-Item -ItemType Directory -Path $logs|Out-Null
foreach($name in @('tmp','pip')){New-Item -ItemType Directory -Path (Join-Path $cache $name) -Force|Out-Null}
function RunClean([string]$Executable,[string[]]$Arguments,[string]$Name,[int]$Seconds){
 $start=[Diagnostics.ProcessStartInfo]::new()
 $start.FileName=$Executable;$start.WorkingDirectory=$repo;$start.UseShellExecute=$false;$start.CreateNoWindow=$true
 $start.RedirectStandardOutput=$true;$start.RedirectStandardError=$true
 foreach($arg in $Arguments){$start.ArgumentList.Add($arg)}
 $start.Environment.Clear()
 foreach($key in @('SystemRoot','WINDIR')){$start.Environment[$key]=[Environment]::GetEnvironmentVariable($key)}
 $start.Environment['PATH']=(Split-Path $Executable -Parent)+';'+(Split-Path $base -Parent)+';'+(Join-Path ([Environment]::GetEnvironmentVariable('SystemRoot')) 'System32')
 foreach($key in @('TEMP','TMP')){$start.Environment[$key]=Join-Path $cache 'tmp'}
 $start.Environment['PIP_CONFIG_FILE']='NUL';$start.Environment['PIP_DISABLE_PIP_VERSION_CHECK']='1';$start.Environment['PIP_NO_INPUT']='1'
 $p=[Diagnostics.Process]::Start($start)
 $stdout=$p.StandardOutput.ReadToEndAsync();$stderr=$p.StandardError.ReadToEndAsync()
 try{
  if(-not$p.WaitForExit($Seconds*1000)){$p.Kill($true);[void]$p.WaitForExit(10000);throw ('SDK preparation deadline: '+$Name)}
  [IO.File]::WriteAllText((Join-Path $logs ($Name+'.stdout.log')),$stdout.GetAwaiter().GetResult(),[Text.UTF8Encoding]::new($false))
  [IO.File]::WriteAllText((Join-Path $logs ($Name+'.stderr.log')),$stderr.GetAwaiter().GetResult(),[Text.UTF8Encoding]::new($false))
  if($p.ExitCode){throw ('SDK preparation failed: '+$Name)}
 }finally{if(-not$p.HasExited){$p.Kill($true);[void]$p.WaitForExit(10000)};$p.Dispose()}
}
RunClean $base @('-I','-c','import sys,struct,json; assert sys.version_info[:2]==(3,12) and struct.calcsize("P")==8; print(json.dumps({"python":sys.version.split()[0],"pointer_bytes":8}))') 'python-identity' 30
RunClean $base @('-I','-m','venv',$venv) 'venv' 120
$python=Join-Path $venv 'Scripts/python.exe'
$wheel='https://download-r2.pytorch.org/whl/xpu/torch-2.10.0%2Bxpu-cp312-cp312-win_amd64.whl#sha256=3ad605be4728b6d3a28a44d07dd794b1a9e45551b0057815bf25eb2a6d6a56a7'
$report=Join-Path $cache 'sdk-install-report-v1.json'
if(Test-Path $report){throw 'SDK report identity exists'}
RunClean $python @('-I','-m','pip','--isolated','install','--only-binary=:all:','--disable-pip-version-check','--no-input','--progress-bar','off','--timeout','30','--retries','1','--cache-dir',(Join-Path $cache 'pip'),'--index-url','https://download.pytorch.org/whl/xpu','--report',$report,$wheel) 'pip' 1200
$r=Get-Content $report -Raw|ConvertFrom-Json
$torch=@($r.install|Where-Object {$_.metadata.name-ceq'torch'})
if($torch.Count-ne1-or$torch[0].download_info.archive_info.hashes.sha256-cne'3ad605be4728b6d3a28a44d07dd794b1a9e45551b0057815bf25eb2a6d6a56a7'){throw 'Pinned official torch wheel not confirmed'}
$receipt=[ordered]@{version='issue29-repo-local-sdk-preparation-v1';completed_utc=[DateTimeOffset]::UtcNow;venv=$venv;SDK_report_SHA256=(Get-FileHash $report).Hash.ToLowerInvariant();torch_wheel_sha256=$torch[0].download_info.archive_info.hashes.sha256;resolved_packages=$r.install.Count;global_environment_modified=$false;tensor_probe_executed=$false;models_downloaded=$false;microphone_capture=$false;core_backend_changed=$false}
[IO.File]::WriteAllText((Join-Path $cache 'sdk-preparation-receipt-v1.json'),($receipt|ConvertTo-Json -Depth 6)+"`n",[Text.UTF8Encoding]::new($false))
$receipt|ConvertTo-Json -Compress
