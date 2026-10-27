$ErrorActionPreference='Stop'
$repo='D:\CodexData\home\worktrees\issue106-local\butler'
Set-Location $repo
$historySource='D:\CodexData\private\butler-115-evidence-20261026'
$closed=Join-Path $repo 'experiments/memory115/2026-10-26/closed-local-native-v1'
$out=Join-Path $repo '.cache/memory115-closed-local-wrapper-v1'
if(Test-Path -LiteralPath $out){throw 'Closed wrapper identity already exists'}
function Digest([string]$p){(Get-FileHash -LiteralPath $p).Hash.ToLowerInvariant()}
function Pin([string]$p,[string]$hash){if((Digest $p)-cne$hash){throw 'Exact original input commitment differs'}}
function WriteJSON([string]$name,$value){[IO.File]::WriteAllText((Join-Path $out $name),($value|ConvertTo-Json -Depth 100)+"`n",[Text.UTF8Encoding]::new($false))}
Pin (Join-Path $closed 'raw-evidence.zip') '0c949fb275d1f69f234b07b95e312f67692f9aa1c2e7ae3d84ba4aa6fe4c468d'
Pin (Join-Path $closed 'raw-inventory.json') 'dbc8d6baa126c1b59ad42dc07a6677d57badb6430da2613f361a28e42a6ec104'
Pin (Join-Path $closed 'local-handback.json') '6ec6a4a366ed448ae460779ef7c5272ca518a91f73117259f863bf61a9febd19'
Pin '.cache/memory115/native-preseal-v2.json' 'dad3bd40c6cf6d469593b0afb8c98be4c0962e4e32e9ace6ab9020d91c203099'
Pin (Join-Path $historySource 'local-successor-20261026-v1.json') '579aa8c25bcfdba06b1be91f2cb613f2550ee685932c3919e2468bef13f11ec7'
Pin (Join-Path $historySource 'operator-diagnostics-public.json') '771675b9f7e90637b13ce5509dd72947cecc840273a48d79e412b6749c7d5ff1'
Pin (Join-Path $historySource 'closed90/closed-leaf-metadata.json') '8d9329e13da9e51e620c5fa66d6625ae11c938006b18369d2e09f068f1517b3f'
# Only the previously owner-approved synthetic evidence folder, never the sibling
# provider/auth import or settings directories. Reject links/private-material files.
$historyTree=Join-Path $historySource 'closed90'
$entries=@(Get-ChildItem -LiteralPath $historyTree -Recurse -Force)
if($entries.Count-gt8192){throw 'Bounded existing history required'}
foreach($e in $entries){if($e.Attributes-band[IO.FileAttributes]::ReparsePoint-or$e.Name-match'(?i)\.(value|pem|key|dump)$'){throw 'Foreign or linked history material refused'}}
New-Item -ItemType Directory -Path $out|Out-Null
$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
& icacls $out /inheritance:r /grant:r ('*'+$sid+':(OI)(CI)F') '*S-1-5-18:(OI)(CI)F'|Out-Null
if($LASTEXITCODE){throw 'Private wrapper ACL setup failed'}
foreach($name in @('actual','plan','history')){New-Item -ItemType Directory -Path (Join-Path $out $name)|Out-Null}
Copy-Item -LiteralPath $historyTree -Destination (Join-Path $out 'history/closed90') -Recurse
Copy-Item -LiteralPath (Join-Path $historySource 'local-successor-20261026-v1.json') -Destination (Join-Path $out 'history/history.json')
Copy-Item -LiteralPath (Join-Path $historySource 'operator-diagnostics-public.json') -Destination (Join-Path $out 'history/operator-diagnostics-public.json')
Copy-Item -LiteralPath (Join-Path $closed 'raw-inventory.json') -Destination (Join-Path $out 'sealed-local-raw-inventory.json')
Copy-Item -LiteralPath (Join-Path $closed 'raw-evidence.zip') -Destination (Join-Path $out 'sealed-local-raw-evidence.zip')
Copy-Item -LiteralPath (Join-Path $closed 'local-handback.json') -Destination (Join-Path $out 'closed-handback.json')
Copy-Item -LiteralPath (Join-Path $closed 'runner.log') -Destination (Join-Path $out 'runner.log')
Copy-Item -LiteralPath '.cache/memory115/native-preseal-v2.json' -Destination (Join-Path $out 'plan/preseal.json')
$inventory=Get-Content (Join-Path $closed 'raw-inventory.json') -Raw|ConvertFrom-Json
if($inventory.Count-ne157){throw 'Original157 raw entries required'}
$zip=[IO.Compression.ZipFile]::OpenRead((Join-Path $closed 'raw-evidence.zip'))
try{
 if($zip.Entries.Count-ne157){throw 'Original ZIP population differs'}
 foreach($row in $inventory){
  if($row.path-cnotmatch'^[a-zA-Z0-9][a-zA-Z0-9._/-]*$'-or$row.path-match'(^|/)\.\.(/|$)'-or$row.bytes-lt0-or$row.bytes-gt512MB){throw 'Bounded original raw path required'}
  $entry=$zip.GetEntry($row.path)
  if($null-eq$entry-or$entry.Length-ne$row.bytes){throw 'Original ZIP member differs'}
  $target=[IO.Path]::GetFullPath((Join-Path $out ('actual/'+$row.path)))
  if(-not$target.StartsWith((Join-Path $out 'actual')+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Original path escaped'}
  New-Item -ItemType Directory -Path (Split-Path $target -Parent) -Force|Out-Null
  $inputStream=$entry.Open();$outputStream=[IO.File]::Open($target,[IO.FileMode]::CreateNew)
  try{$inputStream.CopyTo($outputStream);$outputStream.Flush($true)}finally{$inputStream.Dispose();$outputStream.Dispose()}
  Pin $target $row.sha256
 }
}finally{$zip.Dispose()}
$plan=Get-Content (Join-Path $out 'plan/preseal.json') -Raw|ConvertFrom-Json
$window=Get-Content (Join-Path $out 'actual/window-final.json') -Raw|ConvertFrom-Json
$cost=Get-Content (Join-Path $out 'actual/unique-quoted-costs.json') -Raw|ConvertFrom-Json
if($window.slots.Count-ne57-or$plan.schedule.Count-ne57-or$cost.native_captured_entries-ne19-or$cost.quotes.inclusive_quote.model_turns-ne233){throw 'Original full plan or cost metadata differs'}
WriteJSON 'plan/schedule.json' ([ordered]@{format='experience90-window-v1';owner_id=$plan.owner;maximum_entries=640;slots=$plan.schedule})
WriteJSON 'all-original-outcomes.json' ([ordered]@{slots=$window.slots;denominator=57;original_final_window_present=$true})
$refs=@($inventory|ForEach-Object {@{path=('actual/'+$_.path);sha256=$_.sha256}})
WriteJSON 'private-artifacts.json' ([ordered]@{format='butler.experience.private-artifacts.v1';files=$refs})
$new=[ordered]@{native_entries=19;native_known_usage=19;native_unknown_usage=0;native_missing_total=0;known_full_token_lower_bound=133011;product_attempts=20;product_observed=19;product_null=1;product_without_native_entry=1}
$inclusive=[ordered]@{native_entries=232;native_known_usage=231;native_unknown_usage=1;native_missing_total=0;known_full_token_lower_bound=1738466;product_attempts=233;product_observed=231;product_null=2;product_without_native_entry=1}
$allRefs=@(Get-ChildItem -LiteralPath $out -Recurse -File|Sort-Object FullName|ForEach-Object {@{path=[IO.Path]::GetRelativePath($out,$_.FullName).Replace('\','/');sha256=Digest $_.FullName}})
$manifest=[ordered]@{format='butler.experience.closed-local-owner-leaf.v1';owner=$plan.owner;day=$plan.history.day;deadline=$plan.deadline;preseal_sha256='dad3bd40c6cf6d469593b0afb8c98be4c0962e4e32e9ace6ab9020d91c203099';previous_history_sha256='579aa8c25bcfdba06b1be91f2cb613f2550ee685932c3919e2468bef13f11ec7';denominator=57;maximum_new_entries=640;cost_population_complete=$true;inclusive_carry=$cost.quotes.inclusive_quote;usage=@{New=$new;Inclusive=$inclusive};references=$allRefs;release_qualified=$false;independent_quality_review_required=$true;invoice_cost='unknown';control_plane_handback_is_inference=$false}
WriteJSON 'closed-leaf.json' $manifest
$receipt=[ordered]@{version='issue115-local-closed-wrapper-construction-v1';created_utc=[DateTimeOffset]::UtcNow;status='WRAPPER_PREPARED_NOT_YET_CONSUMER_VALIDATED';manifest_sha256=Digest (Join-Path $out 'closed-leaf.json');original_raw_files=157;original_prior_files=@($entries|Where-Object {-not$_.PSIsContainer}).Count;original_raw_archive_unchanged=$true;original_preseal_unchanged=$true;new_usage_claims_require_independent_recomputation=$true;new_native_admissions=0;auth_access=0;Go_PG_models=0;release_qualified=$false}
[IO.File]::WriteAllText((Join-Path $repo '.cache/memory115-closed-local-wrapper-v1-construction.json'),($receipt|ConvertTo-Json -Depth 8)+"`n",[Text.UTF8Encoding]::new($false))
Get-ChildItem -LiteralPath $out -Recurse -File|ForEach-Object {$_.IsReadOnly=$true}
$receipt|ConvertTo-Json -Compress
