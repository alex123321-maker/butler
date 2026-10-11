// Read-only operator backend. Never exports browser state, tokens or dialogue.
import fs from 'node:fs';
import path from 'node:path';
import https from 'node:https';
import {execFileSync} from 'node:child_process';

async function main(){

const nodeMode = process.argv[2] ?? 'inspect';
if (!['inspect', 'public-assets', 'credits', 'reload','fixture-controls','fixture-next','fixture-onboard','fixture-new','fixture-close','fixture-send','metadata-send'].includes(nodeMode)) process.exit(2);
if(nodeMode==='metadata-send'&&process.argv[3]!=='fixture')process.exit(2);
if(nodeMode.startsWith('fixture-')&&process.argv[3]!=='fixture')process.exit(2);
if(nodeMode==='reload'&&process.argv[3]!=='fixture')process.exit(2);
const reference=process.argv[3]==='fixture'?path.resolve('.cache/antigravity-desktop-fixture/userdata/DevToolsActivePort'):path.join(process.env.APPDATA,'Antigravity','DevToolsActivePort');
const active = fs.readFileSync(reference, 'utf8').split(/\r?\n/);
if (!/^\d{1,5}$/.test(active[0])) process.exit(2);
const port = Number(active[0]);
if(process.argv[3]==='fixture'&&port!==35130)process.exit(2);
const targets = await fetch(`http://127.0.0.1:${port}/json/list`).then(r => r.json());
const target = targets.find(t => {
  try { const u = new URL(t.url); return t.type === 'page' && u.protocol === 'https:' && u.hostname === '127.0.0.1'; } catch { return false; }
});
if (!target) { console.log(JSON.stringify({desktop_local_page_available:false})); process.exit(1); }
const wsURL = new URL(target.webSocketDebuggerUrl);
if (wsURL.protocol !== 'ws:' || !['localhost', '127.0.0.1'].includes(wsURL.hostname) || Number(wsURL.port) !== port) process.exit(2);
const ws = new WebSocket(wsURL);
await new Promise((resolve,reject) => {ws.addEventListener('open',resolve,{once:true});ws.addEventListener('error',reject,{once:true});});
let next = 0; const pending = new Map();
ws.addEventListener('message', e => { const r=JSON.parse(e.data); if(r.id && pending.has(r.id)){const p=pending.get(r.id);pending.delete(r.id);r.error?p.reject(Error('CDP_FAILED')):p.resolve(r.result);} });
function call(method,params){return new Promise((resolve,reject)=>{const id=++next;pending.set(id,{resolve,reject});ws.send(JSON.stringify({id,method,params}));});}
try {
  if(nodeMode==='fixture-close'){
    await call('Browser.close',{});console.log(JSON.stringify({isolated_desktop_closed:true}));return;
  }
  if(nodeMode==='fixture-new'){
    const r=await call('Runtime.evaluate',{expression:`JSON.stringify((()=>{const b=Array.from(document.querySelectorAll('button')).find(e=>['New Conversation','New conversation'].includes(e.getAttribute('aria-label')||e.getAttribute('title')||e.innerText));if(b){b.click();return {fresh_dialogue_selected:true};}return {fresh_dialogue_selected:false};})())`,returnByValue:true,timeout:5000});
    if(r.exceptionDetails)throw Error('EVALUATION_FAILED');console.log(r.result.value);ws.close();process.exit();
  }
  if(nodeMode==='fixture-onboard'){
    await call('Page.reload',{});await new Promise(r=>setTimeout(r,1000));
    let ready=false;
    for(let i=0;i<8;i++){
      const response=await call('Runtime.evaluate',{expression:`JSON.stringify((()=>{if(document.body.innerText.includes('Gemini 3.8 Flash High')&&document.querySelector('[role="combobox"]'))return {ready:true};const c=document.querySelector('input[type="checkbox"]');if(c&&!c.checked){c.click();return {ready:false};}const b=Array.from(document.querySelectorAll('button')).find(e=>e.innerText==='Next'||e.innerText==='Finish');if(b&&!b.disabled)b.click();return {ready:false};})())`,returnByValue:true,timeout:5000});
      if(response.exceptionDetails)throw Error('EVALUATION_FAILED');
      ready=JSON.parse(response.result.value).ready;if(ready)break;await new Promise(r=>setTimeout(r,700));
    }
    console.log(JSON.stringify({isolated_profile_ready:ready,real_model_calls:0}));
    ws.close();process.exit(ready?0:1);
  }
  if(nodeMode==='fixture-next'){
    await call('Runtime.evaluate',{expression:`(()=>{const c=document.querySelector('input[type="checkbox"]');if(c&&!c.checked)c.click();const b=Array.from(document.querySelectorAll('button')).find(e=>e.innerText==='Next'||e.innerText==='Finish');if(b&&!b.disabled)b.click();})()`,returnByValue:true,timeout:5000});
    console.log(JSON.stringify({isolated_onboarding_next:true,real_model_calls:0}));
    ws.close();process.exit();
  }
  if(nodeMode==='fixture-send'||nodeMode==='metadata-send'){
    const gate=JSON.parse(execFileSync('ssh',['laba','docker exec butler-agy-desktop-fixture /fixture report'],{encoding:'utf8',stdio:['ignore','pipe','ignore'],timeout:5000}));
    const correctMode=nodeMode==='fixture-send'?gate.synthetic_credentials_only===true:gate.mode==='metadata-only'&&gate.origin_audit_matched===true;
    if(!correctMode||gate.real_model_calls!==0||gate.generation_forwarding_enabled!==false)throw Error('FIXTURE_GATE_REQUIRED');
    const result=await call('Runtime.evaluate',{expression:`JSON.stringify((()=>{if(!document.body.innerText.includes('Gemini 3.8 Flash High'))return {submitted:false,reason:'HIGH_NOT_VISIBLE'};const e=document.querySelector('textarea')||document.querySelector('[role="combobox"]')||document.querySelector('[contenteditable="true"]');if(!e)return {submitted:false,reason:'NO_EDITOR'};e.focus();return {editor_filled:true,submitted:false};})())`,returnByValue:true,timeout:5000});
    if(result.exceptionDetails)throw Error('EVALUATION_FAILED');
    const state=JSON.parse(result.result.value);if(!state.editor_filled)throw Error('FIXTURE_EDITOR_REQUIRED');
    await call('Input.insertText',{text:'Return only DESKTOP_OFFLINE_OK. Do not use tools.'});
    await new Promise(r=>setTimeout(r,500));
    const sent=await call('Runtime.evaluate',{expression:`JSON.stringify((()=>{const b=Array.from(document.querySelectorAll('button')).find(e=>e.getAttribute('aria-label')==='Send message');if(b&&!b.disabled){b.click();return {submitted:true};}return {submitted:false};})())`,returnByValue:true,timeout:5000});
    console.log(JSON.stringify({...JSON.parse(sent.result.value),synthetic_only:nodeMode==='fixture-send',metadata_only:nodeMode==='metadata-send',real_model_calls:0}));
    ws.close();process.exit();
  }
  if(nodeMode==='fixture-controls'){
    const gate=JSON.parse(execFileSync('ssh',['laba','docker exec butler-agy-desktop-fixture /fixture report'],{encoding:'utf8',stdio:['ignore','pipe','ignore'],timeout:5000}));
    if(gate.synthetic_credentials_only!==true)throw Error('SYNTHETIC_PROFILE_REQUIRED');
    const response=await call('Runtime.evaluate',{expression:`JSON.stringify({buttons:Array.from(document.querySelectorAll('button')).map(e=>e.innerText||e.getAttribute('aria-label')||'').filter(x=>/^[A-Za-z .&?!'-]{1,60}$/.test(x)),headings:Array.from(document.querySelectorAll('h1,h2,h3,label')).map(e=>e.innerText).filter(x=>/^[A-Za-z .&?!'-]{1,60}$/.test(x)),inputs:Array.from(document.querySelectorAll('input')).map(e=>({type:e.type,placeholder:/^[A-Za-z .&?!'-]{1,60}$/.test(e.placeholder)?e.placeholder:'SUPPRESSED'}))})`,returnByValue:true,timeout:5000});
    if(response.exceptionDetails)throw Error('EVALUATION_FAILED');
    console.log(JSON.stringify({...JSON.parse(response.result.value),synthetic_profile_only:true,real_model_calls:0}));
    ws.close();process.exit();
  }
  if(nodeMode==='reload'){
    await call('Page.reload',{});
    console.log(JSON.stringify({isolated_page_reloaded:true,real_model_calls:0}));
    ws.close();process.exit();
  }
  if(nodeMode==='credits'){
    await call('Runtime.evaluate',{expression:`Array.from(document.querySelectorAll('button')).find(e=>e.getAttribute('aria-label')==='Settings'||e.getAttribute('title')==='Settings'||e.innerText==='Settings')?.click()`,returnByValue:true,timeout:5000});
    await new Promise(r=>setTimeout(r,600));
    await call('Runtime.evaluate',{expression:`Array.from(document.querySelectorAll('button,[role="tab"],a')).find(e=>e.innerText==='Models & Usage')?.click()`,returnByValue:true,timeout:5000});
    await new Promise(r=>setTimeout(r,600));
    const result=await call('Runtime.evaluate',{expression:`JSON.stringify((()=>{const label=Array.from(document.querySelectorAll('*')).find(e=>e.childElementCount===0&&e.textContent==='Enable AI Credit Overages');let toggle=null;let p=label;for(let i=0;p&&i<6;i++,p=p.parentElement){toggle=p.querySelector('[role="switch"]');if(toggle)break;}return{models_usage_visible:document.body.innerText.includes('Models & Usage'),overages_label_visible:!!label,overages_toggle_visible:!!toggle,overages_checked:toggle?.getAttribute('aria-checked')==='true'?true:toggle?.getAttribute('aria-checked')==='false'?false:null};})())`,returnByValue:true,timeout:5000});
    if(result.exceptionDetails)throw Error('EVALUATION_FAILED');
    console.log(JSON.stringify({...JSON.parse(result.result.value),raw_browser_state_suppressed:true}));
    ws.close();process.exit();
  }
  const expression = nodeMode === 'inspect' ? `JSON.stringify({
    local_page:location.hostname==='127.0.0.1',
    document_state:document.readyState,
    body_text_bytes:document.body.innerText.length,
    script_count:document.scripts.length,
    button_count:document.querySelectorAll('button').length,
    login_text_visible:/Sign in|Sign In|Log in/.test(document.body.innerText),
    loading_text_visible:/Loading|Connecting/.test(document.body.innerText),
    known_error_text:/Unable to connect|Something went wrong|Connection error|Disconnected/.test(document.body.innerText),
    native_storage_bridge:typeof window.nativeStorage?.getItems==='function',
    model_high_visible:document.body.innerText.includes('Gemini 3.8 Flash High'),
    credits_word_visible:/AI Credit|Overage/.test(document.body.innerText),
    known_controls:Array.from(document.querySelectorAll('button,[role="button"],input,textarea,[contenteditable="true"]')).map(e=>({tag:e.tagName,role:e.getAttribute('role'),label:e.getAttribute('aria-label')||e.getAttribute('title')||e.innerText||'',editable:e.tagName==='TEXTAREA'||e.tagName==='INPUT'||e.isContentEditable})).filter(x=>x.editable||/^(Settings|New conversation|New chat|Model|View Usage|Gemini 3[.]8 Flash High|Send|Send message|Open Settings|Account Settings)$/.test(x.label)).map(x=>({...x,label:/^(Settings|New conversation|New chat|Model|View Usage|Gemini 3[.]8 Flash High|Send|Send message|Open Settings|Account Settings)$/.test(x.label)?x.label:'EDITABLE'}))
  })` : `JSON.stringify({scripts:Array.from(document.scripts).map(s=>s.src).filter(Boolean),links:Array.from(document.querySelectorAll('link[rel="modulepreload"]')).map(l=>l.href)})`;
  const result = await call('Runtime.evaluate',{expression,returnByValue:true,timeout:5000});
  if(result.exceptionDetails) throw Error('EVALUATION_FAILED');
  const projected = JSON.parse(result.result.value);
  if(nodeMode==='inspect'){console.log(JSON.stringify({...projected,raw_browser_state_suppressed:true}));}
  else {
    const origin=new URL(target.url).origin;
    const candidates=[...projected.scripts,...projected.links].map(x=>new URL(x)).filter(u=>u.origin===origin&&(/^(\/main[.]js|\/prism_bundle[.]js)$/.test(u.pathname)||/^\/(assets|static|_next|jetski|webui|frontend)\/[A-Za-z0-9_./-]+[.]js$/.test(u.pathname))&&!u.search);
    const root=path.resolve('.cache/antigravity-desktop-public/web');fs.mkdirSync(root,{recursive:true});
    const names=[];
    for(const u of candidates.slice(0,30)){
      const data=await new Promise((resolve,reject)=>{https.get(u,{rejectUnauthorized:false},r=>{if(r.statusCode!==200){r.resume();reject(Error('ASSET_FAILED'));return;}const chunks=[];let size=0;r.on('data',b=>{size+=b.length;if(size>30*1024*1024)r.destroy(Error('ASSET_BOUND'));else chunks.push(b);});r.on('end',()=>resolve(Buffer.concat(chunks)));r.on('error',reject);}).on('error',reject);});
      const name=path.basename(u.pathname);fs.writeFileSync(path.join(root,name),data);names.push({name,bytes:data.length});
    }
    console.log(JSON.stringify({public_static_assets:names,script_paths:[...projected.scripts,...projected.links].map(x=>{try{const u=new URL(x);return {path:/^\/[A-Za-z0-9_./-]{1,150}[.]js$/.test(u.pathname)?u.pathname:'UNKNOWN',same_origin:u.origin===origin,query_present:!!u.search};}catch{return {path:'UNKNOWN'};}}),raw_browser_state_suppressed:true}));
  }
} catch { console.log(JSON.stringify({inspection_failed:true,raw_browser_state_suppressed:true}));process.exitCode=1; }
finally { ws.close(); }
}
main().catch(()=>{console.log(JSON.stringify({inspection_failed:true,raw_browser_state_suppressed:true}));process.exitCode=1;});
