// Operator backend only. Docker inspect stays in this process; stdout is booleans.
import fs from 'node:fs';
import {execFileSync} from 'node:child_process';
let phase='INPUT_REFERENCE';
try {
  const destination = process.argv[2];
  if (!/^\/srv\/storage\/butler\/agents\/antigravity-compare\/private\/control[.]env$/.test(destination)) throw Error();
  phase='READ_OPERATOR_CONTAINER';
  const container = JSON.parse(execFileSync('docker',['inspect','butler-lab-orchestrator-1'],{maxBuffer:2*1024*1024}))[0];
  const env = Object.fromEntries(container.Config.Env.map(item=>{const i=item.indexOf('=');return [item.slice(0,i),item.slice(i+1)];}));
  const selected = {};
  for(const name of ['BUTLER_POSTGRES_URL','BUTLER_SETTINGS_ENCRYPTION_KEY']) {
    phase=name;
    if(!env[name] || /[\r\n]/.test(env[name])) throw Error();
    selected[name]=env[name];
  }
  const proxy=env.BUTLER_ANTIGRAVITY_PROXY??env.HTTPS_PROXY??env.HTTP_PROXY;
  phase='PRIVATE_PROXY';
  if(!proxy || /[\r\n]/.test(proxy)) throw Error();
  selected.HTTPS_PROXY=proxy;
  selected.HTTP_PROXY=proxy;
  selected.NO_PROXY='localhost,127.0.0.1,compare-postgres';
  let basis=false;
  phase='EXACT_FLASH_COST_BASIS';
  try {const policy=JSON.parse(env.BUTLER_EXECUTION_BUDGET_POLICY_JSON??'{}');const price=policy.models?.['antigravity:gemini-3.8-flash-tiered'];basis=price?.basis==='subscription_no_overage' && !(price.input_micros_per_token??0) && !(price.output_micros_per_token??0);} catch {}
  if(!basis) throw Error();
  selected.BUTLER_COMPARE_SUBSCRIPTION_NO_OVERAGE='1';
  phase='PRIVATE_ENV_WRITE';
  fs.writeFileSync(destination,Object.entries(selected).map(([k,v])=>k+'='+v).join('\n')+'\n',{mode:0o600,flag:'wx'});
  console.log(JSON.stringify({private_environment_prepared:true,proxy_configured:true,subscription_no_overage:true,raw_environment_suppressed:true}));
} catch {console.log(JSON.stringify({private_environment_prepared:false,failed_phase:phase,raw_environment_suppressed:true}));process.exitCode=1;}
