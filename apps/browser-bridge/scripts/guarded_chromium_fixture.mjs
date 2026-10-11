// Isolated public-data fixture only. Drives the actual unpacked extension and
// production guarded host subprocess through native frames. It does not prove
// human tab-selection UX or Chromium's installed native-host manifest launch.
import { chromium } from '/app/apps/tool-browser/node_modules/playwright/index.mjs';
import { spawn } from 'node:child_process';
import { createServer } from 'node:http';
import { mkdtemp, rm, cp, appendFile, mkdir, writeFile, readFile, chmod } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomBytes } from 'node:crypto';

const [binary, extensionPath, target, authority, tokenFile] = process.argv.slice(2);
const installed=process.env.BUTLER_SINGLE_TAB_INSTALLED==='1';
const profile = await mkdtemp(join(tmpdir(), 'butler-guarded-profile-'));
const fixtureExtension=join(profile,'fixture-extension');
await cp(extensionPath,fixtureExtension,{recursive:true});
// Chrome MV3 service workers disallow dynamic import(). Expose only test-driver
// references in this temporary copy; the imported production code is unchanged.
await appendFile(join(fixtureExtension,'background.js'),'\nglobalThis.__butlerFixture={executeGuarded,acknowledgeGuarded,runLegacyTab,guardedTabMetadata}; globalThis.__butlerFixtureBoot=crypto.randomUUID();\n');
let effects = 0, lostAck = false, host, context, worker, pending = Buffer.alloc(0), nativeRequests=0, nativeCompleted=0, nativeACKs=0, nativeDenied=0;
let holdNext=false, releaseNative=null, lastGuard=null, restartTabMatches=null;
const site = createServer((req,res) => {
  if (req.url === '/effect') { effects++; res.end('accepted'); return; }
  res.setHeader('content-type','text/html');
  res.end('<!doctype html><title>Public guarded fixture</title><input id="edit"><button id="target" onclick="fetch(\'/effect\')">Execute once</button><div id="note">Original</div>');
});
await new Promise((r) => site.listen(0,'127.0.0.1',r));
const url = `http://127.0.0.1:${site.address().port}/`;
async function startChrome() {
  context = await chromium.launchPersistentContext(profile,{ headless:false,env:{...process.env,HOME:profile},
    args:[`--disable-extensions-except=${fixtureExtension}`,`--load-extension=${fixtureExtension}`] });
  worker = context.serviceWorkers()[0] || await context.waitForEvent('serviceworker');
  const page = context.pages()[0] || await context.newPage();
  await page.goto(url);
  return page;
}
let page = await startChrome();
// Explicit trusted fixture selection identity; no user profile/account is used.
await worker.evaluate(()=>chrome.storage.local.set({'butler.chromiumBridge.browserInstanceID':'actual-fixed-host'}));
let tabRef = await worker.evaluate(async (expected) => String((await chrome.tabs.query({})).find((t) => t.url === expected).id),url);
function writeFrame(value) {
  const body=Buffer.from(JSON.stringify(value)); const header=Buffer.alloc(4);header.writeUInt32LE(body.length);
  host.stdin.write(Buffer.concat([header,body]));
}
async function onNative(request) {
  nativeRequests++;
  try {
    if(request.method==='action.guarded') {
      lastGuard=request.params;
      if(holdNext){holdNext=false;await new Promise((r)=>{releaseNative=r;});}
    }
    if (request.method === 'action.guarded.ack' && lostAck) {
      writeFrame({id:request.id,ok:false,error:{code:'fixture_lost_ack',message:'ack lost'}});return;
    }
    const result=await worker.evaluate(async ({method,params}) => {
      const guarded=globalThis.__butlerFixture;
      if (method === 'action.guarded') return guarded.executeGuarded(params);
      if (method === 'action.guarded.ack') return guarded.acknowledgeGuarded(params);
      if (method === 'tabs.guarded.list' || method === 'tabs.guarded.check') return guarded.guardedTabMetadata(method,params);
      throw new Error('unsupported fixture method');
    },request);
    if(request.method==='action.guarded') nativeCompleted++;else if(request.method==='action.guarded.ack') nativeACKs++;
    writeFrame({id:request.id,ok:true,result});
  } catch {nativeDenied++;writeFrame({id:request.id,ok:false,error:{code:'guarded_tab_denied',message:'Guarded tab is stale, busy or uncertain'}});}
}
const metadataReservation=createServer();await new Promise(r=>metadataReservation.listen(0,'127.0.0.1',r));
const metadataAddr=`127.0.0.1:${metadataReservation.address().port}`;
await new Promise(r=>metadataReservation.close(r));
if(!installed){
host=spawn(binary,[],{env:{...process.env,BUTLER_GUARDED_BROWSER_TOKEN_FILE:tokenFile,
  BUTLER_GUARDED_BROWSER_METADATA_ADDR:metadataAddr,
  BUTLER_GUARDED_BROWSER_GRPC_ADDR:target,BUTLER_GUARDED_BROWSER_AUTHORITY_URL:authority},stdio:['pipe','pipe','ignore']});
host.stdout.on('data',(chunk)=>{
  pending=Buffer.concat([pending,chunk]);
  while(pending.length>=4){
    const n=pending.readUInt32LE(0);if(n<1||n>65536){host.kill();return;}
    if(pending.length<n+4)return;
    const value=JSON.parse(pending.subarray(4,n+4).toString());pending=pending.subarray(n+4);
    void onNative(value);
  }
});
}else{
  const extensionID=new URL(worker.url()).hostname;
  const wrapper=join(profile,'guarded-host.sh');
  const quote=v=>"'"+v.replaceAll("'","'\\''")+"'";
  const body='#!/bin/sh\n'+[
    ['BUTLER_GUARDED_BROWSER_TOKEN_FILE',tokenFile],['BUTLER_GUARDED_BROWSER_GRPC_ADDR',target],
    ['BUTLER_GUARDED_BROWSER_AUTHORITY_URL',authority],['BUTLER_GUARDED_BROWSER_METADATA_ADDR',metadataAddr]
  ].map(([k,v])=>`export ${k}=${quote(v)}\n`).join('')+
    `printf 'launched' > ${quote(join(profile,'native-launched'))}\nexec ${quote(binary)} "$@"\n`;
  await writeFile(wrapper,body,{mode:0o700});await chmod(wrapper,0o700);
  const manifest=JSON.stringify({name:'com.butler.browser_bridge',description:'Isolated guarded fixture',path:wrapper,type:'stdio',allowed_origins:[`chrome-extension://${extensionID}/`]});
  // Chrome chooses the user-data directory; default per-product paths are
  // isolated as well. No host-wide installation, profile/account or root used.
  for(const dir of [join(profile,'NativeMessagingHosts'),...['google-chrome','google-chrome-for-testing','chromium'].map(v=>join(profile,'.config',v,'NativeMessagingHosts'))]){
    await mkdir(dir,{recursive:true,mode:0o700});await writeFile(join(dir,'com.butler.browser_bridge.json'),manifest,{mode:0o600});
  }
  await context.close();page=await startChrome();
  const secondPage=await context.newPage();await secondPage.goto(url);await page.bringToFront();
  tabRef=await worker.evaluate(async(expected)=>String((await chrome.tabs.query({})).find(t=>t.url===expected).id),url);
}
const control=createServer(async(req,res)=>{
 let phase='operation';
 try {
  if(req.url==='/manual') await page.fill('#edit','Owner intervened');
  else if(req.url==='/mutate') await page.evaluate(()=>document.querySelector('#note').textContent='Independent mutation');
  else if(req.url==='/reload') await page.reload();
  else if(req.url==='/foreign-host') await worker.evaluate(()=>chrome.storage.local.set({'butler.chromiumBridge.browserInstanceID':'different-fixed-host'}));
  else if(req.url==='/foreign-origin') await page.goto(url.replace('127.0.0.1','localhost'));
  else if(req.url==='/lose-ack') lostAck=true;
  else if(req.url==='/hold-next') holdNext=true;
  else if(req.url==='/release') {releaseNative?.();releaseNative=null;}
  else if(req.url==='/successor-denied') {
    let denied=false;
    const p={...lastGuard,action_type:'status',args_json:JSON.stringify({session_id:lastGuard.single_tab_session_id}),
      dispatch_id:randomBytes(24).toString('hex'),resource_fence:lastGuard.resource_fence+1,deadline:new Date(Date.now()+30000).toISOString()};
    try{await worker.evaluate(async(params)=>globalThis.__butlerFixture.executeGuarded(params),p);}catch{denied=true;}
    res.end(JSON.stringify({denied,effects}));return;
  }
  else if(req.url==='/restart-browser') {
    const priorBoot=await worker.evaluate(()=>globalThis.__butlerFixtureBoot);
    phase='restart_close';
    await context.close();
    phase='restart_open';
    page=await startChrome();
    if(await worker.evaluate(()=>globalThis.__butlerFixtureBoot)===priorBoot)throw new Error('worker not restarted');
    const currentTab=await worker.evaluate(async(expected)=>String((await chrome.tabs.query({})).find((t)=>t.url===expected).id),url);
    restartTabMatches=currentTab===tabRef;
  }
  else if(req.url==='/legacy') {
    let denied=false;
    try {await worker.evaluate(async(id)=>{await globalThis.__butlerFixture.runLegacyTab(Number(id),()=>{});},tabRef);}catch{denied=true;}
    res.end(JSON.stringify({denied,effects}));return;
  }
  else if(req.url==='/state') {
    const state=await worker.evaluate(async(id)=>{const key='butler.guardedTab.v1:'+id;const all=await chrome.storage.local.get(key);return all[key]?.phase||'absent';},tabRef);
    let manifestLaunched=false;if(installed){try{manifestLaunched=(await readFile(join(profile,'native-launched'),'utf8'))==='launched';}catch{}}
    res.end(JSON.stringify({phase:state,effects,nativeRequests,nativeCompleted,nativeACKs,nativeDenied,restartTabMatches,manifestLaunched}));return;
  } else if(req.url!=='/effects') {res.writeHead(404);res.end();return;}
  res.end(JSON.stringify({effects}));
 } catch {res.writeHead(500);res.end(JSON.stringify({failure:phase}));}
});
await new Promise((r)=>control.listen(0,'127.0.0.1',r));
process.stdout.write(JSON.stringify({tab_ref:tabRef,origin:new URL(url).origin,metadata:`http://${metadataAddr}`,control:`http://127.0.0.1:${control.address().port}`})+'\n');
process.stdin.resume();
let closed=false;
async function close(){if(closed)return;closed=true;host?.kill();control.close();site.close();await context.close();await rm(profile,{recursive:true,force:true});process.exit(0);}
process.on('SIGTERM',close);process.on('SIGINT',close);process.stdin.on('end',close);
