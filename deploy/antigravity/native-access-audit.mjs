// Closed provider auth resolver: credentials/project IDs and raw errors never leave.
import fs from 'node:fs';
let credentials;
try { credentials=JSON.parse(fs.readFileSync(process.env.BUTLER_ANTIGRAVITY_NATIVE_IMPORT??'/home/butler/.gemini/antigravity-cli/butler-native-import.json','utf8')); }
catch { console.log(JSON.stringify({privateCredentialUnavailable:true,rawDiagnosticsSuppressed:true})); process.exit(2); }
const token=credentials.access_token??credentials.accessToken??credentials.token?.access_token;
if(typeof token!=='string'||token.length<20){console.log(JSON.stringify({authMaterialPresent:false}));process.exit(2);}
const headers={Authorization:'Bearer '+token,'Content-Type':'application/json','User-Agent':'antigravity/1.3.3 butler/0.1'};
const id=x=>typeof x==='string'&&/^[A-Za-z0-9_. -]{1,100}$/.test(x)?x:undefined;
const modelID=x=>typeof x==='string'&&/^(?:gemini[0-9A-Za-z_.-]*|MODEL_[A-Z0-9_]+|API_PROVIDER_[A-Z0-9_]+)$/.test(x)?x:undefined;
const timestamp=x=>typeof x==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:[.]\d+)?Z$/.test(x)?x:undefined;
async function api(host,path,body){
 const response=await fetch('https://'+host+path,{method:body===undefined?'GET':'POST',redirect:'error',headers,body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(45000)});
 let data={};try{data=await response.json();}catch{}
 const details=Array.isArray(data.error?.details)?data.error.details:[];
 console.log(JSON.stringify({host,path,status:response.status,apiStatus:id(data.error?.status),detailTypes:details.map(d=>d['@type']).filter(t=>typeof t==='string'&&/^type.googleapis.com\/[A-Za-z0-9_.]{1,100}$/.test(t)),reasons:details.map(d=>d.reason).filter(t=>typeof t==='string'&&/^[A-Z_]{1,80}$/.test(t)),metadataKeys:details.flatMap(d=>Object.keys(d.metadata??{})).filter(k=>/^[A-Za-z0-9_]{1,80}$/.test(k)),detectedCountryISO:details.map(d=>String(d.metadata?.detected_location??'').toUpperCase()).find(c=>/^[A-Z]{2}$/.test(c)),retryDelay:details.map(d=>d.retryDelay).filter(t=>typeof t==='string'&&/^\d+(?:[.]\d+)?s$/.test(t)),errorFlags:['quota','capacity','credit','limit','overload','model','unsupported','missing','project','exhaust','authentication'].filter(t=>String(data.error?.message??'').toLowerCase().includes(t))}));
 return {ok:response.ok,data};
}
try{
 console.log(JSON.stringify({authMaterialPresent:true}));
 const loaded=await api('cloudcode-pa.googleapis.com','/v1internal:loadCodeAssist',{metadata:{ideType:'ANTIGRAVITY',ideName:'Butler',ideVersion:'1.3.3',pluginVersion:'1.3.3',platform:'LINUX_AMD64',pluginType:'GEMINI'}});
 if(!loaded.ok)process.exit(3);
 const project=typeof loaded.data.cloudaicompanionProject==='string'?loaded.data.cloudaicompanionProject:loaded.data.cloudaicompanionProject?.id;
 console.log(JSON.stringify({projectResolved:typeof project==='string'&&project.length>0,currentTier:id(loaded.data.currentTier?.id),paidTier:id(loaded.data.paidTier?.id),g1Tier:id(loaded.data.g1Tier),projectValidationErrorPresent:!!loaded.data.projectValidationError,ineligibleTierReasons:(loaded.data.ineligibleTiers??[]).map(t=>id(t.reasonCode))}));
 if(project){
  for(const method of ['retrieveUserQuota','retrieveUserQuotaSummary']){
   const quota=await api('cloudcode-pa.googleapis.com','/v1internal:'+method,{project});
   if(quota.ok){const all=[...(quota.data.buckets??[]),...(quota.data.groups??[]).flatMap(g=>g.buckets??[])];console.log(JSON.stringify({quotaMethod:method,buckets:all.filter(b=>b.modelId==='gemini-3.8-flash-tiered'||b.bucketId==='gemini-weekly'||b.bucketId==='gemini-5h').map(b=>({modelId:modelID(b.modelId),bucketId:id(b.bucketId),tokenType:id(b.tokenType),remainingFraction:typeof b.remainingFraction==='number'?b.remainingFraction:undefined,remainingAmount:typeof b.remainingAmount==='string'&&/^\d{1,20}$/.test(b.remainingAmount)?b.remainingAmount:undefined,disabled:b.disabled===true,resetTime:timestamp(b.resetTime),window:id(b.window)}))}));}
  }
  const catalog=await api('cloudcode-pa.googleapis.com','/v1internal:fetchAvailableModels',{project});
  const target=catalog.data.models?.['gemini-3.8-flash-tiered'];
  console.log(JSON.stringify({targetAdvertised:!!target,modelEnum:modelID(target?.model),apiProvider:modelID(target?.apiProvider),targetRemainingFraction:target?.quotaInfo?.remainingFraction,targetResetTime:timestamp(target?.quotaInfo?.resetTime),targetInfoFields:Object.keys(target??{}).filter(k=>/^[A-Za-z0-9_]{1,80}$/.test(k)),tieredModelIds:catalog.data.tieredModelIds?Object.fromEntries(Object.entries(catalog.data.tieredModelIds).map(([k,v])=>[id(k),modelID(v)])):undefined}));
 }
 const config=await api('aicode.googleapis.com','/v1:fetchConfig');
 if(config.ok){const models=config.data.availableModels??[];console.log(JSON.stringify({nativeConfigFields:Object.keys(config.data).filter(k=>/^[A-Za-z0-9_]{1,80}$/.test(k)),nativeFlashModels:models.filter(m=>/3[.]8.*flash|flash.*3[.]8/i.test(m.modelId??m.name??m.displayName??'')).map(m=>({modelId:modelID(m.modelId),name:id(m.name),baseModelId:modelID(m.baseModelId),version:id(m.version),inputTokenLimit:m.inputTokenLimit,outputTokenLimit:m.outputTokenLimit})),nativeSurfaceFields:Object.keys(config.data.agySurfaceConfig??{}).filter(k=>/^[A-Za-z0-9_]{1,80}$/.test(k))}));}
 const nativeQuota=await api('aicode.googleapis.com','/v1:fetchQuotaStatus');
 if(nativeQuota.ok)console.log(JSON.stringify({nativeQuotaBuckets:(nativeQuota.data.buckets??[]).map(b=>({quotaType:id(b.quotaType),remainingFraction:b.remainingFraction,resetTime:timestamp(b.resetTime),modelId:modelID(b.model?.modelId)}))}));
}catch{console.log(JSON.stringify({auditFailed:true,rawDiagnosticsSuppressed:true}));process.exitCode=1;}
