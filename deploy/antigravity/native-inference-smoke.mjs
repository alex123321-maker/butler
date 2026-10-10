// Operator-only backend probe. No URI, token, project ID or raw error is emitted.
import fs from 'node:fs';
import http from 'node:http';
import https from 'node:https';
import tls from 'node:tls';
import crypto from 'node:crypto';

const emit = x => console.log(JSON.stringify(x));
const enumValue = x => typeof x === 'string' && /^[A-Z_]{1,80}$/.test(x) ? x : undefined;
const id = x => typeof x === 'string' && /^[A-Za-z0-9_.-]{1,100}$/.test(x) ? x : undefined;
const allowedHosts = new Set(['cloudcode-pa.googleapis.com', 'aicode.googleapis.com']);
let privateAuth, proxy, token, project, transportFailure;
try {
  privateAuth = JSON.parse(fs.readFileSync(process.env.BUTLER_ANTIGRAVITY_NATIVE_IMPORT ?? '/home/butler/.gemini/antigravity-cli/butler-native-import.json', 'utf8'));
  token = privateAuth.access_token ?? privateAuth.accessToken ?? privateAuth.token?.access_token;
  proxy = new URL(process.env.BUTLER_ANTIGRAVITY_PROXY ?? process.env.HTTPS_PROXY ?? process.env.HTTP_PROXY ?? process.env.https_proxy ?? process.env.http_proxy ?? '');
  if (typeof token !== 'string' || token.length < 20) throw new Error('private-auth');
  if (!['http:', 'https:'].includes(proxy.protocol) || !proxy.hostname || proxy.hash || proxy.search) throw new Error('private-proxy');
} catch {
  emit({phase:'private-input', accepted:false, rawDiagnosticsSuppressed:true});
  process.exit(2);
}

function agentFor(host) {
  if (!allowedHosts.has(host)) throw new Error('unsupported-host');
  const agent = new https.Agent({keepAlive:false});
  agent.createConnection = (_options, callback) => {
    let done = false;
    const finish = (error, socket) => { if (done) { if (socket) socket.destroy(); return; } done=true; if(error) transportFailure=error.message; callback(error, socket); };
    const proxyHeaders = {Host:host + ':443'};
    if (proxy.username || proxy.password) {
      proxyHeaders['Proxy-Authorization'] = 'Basic ' + Buffer.from(decodeURIComponent(proxy.username) + ':' + decodeURIComponent(proxy.password)).toString('base64');
    }
    const connector = proxy.protocol === 'https:' ? https : http;
    const connect = connector.request({hostname:proxy.hostname, port:proxy.port || (proxy.protocol === 'https:' ? 443 : 80), method:'CONNECT', path:host + ':443', headers:proxyHeaders, timeout:15000, rejectUnauthorized:true});
    connect.once('connect', (response, socket, head) => {
      if (response.statusCode !== 200 || head.length !== 0) { socket.destroy(); finish(new Error('proxy-denied')); return; }
      const secure = tls.connect({socket, servername:host, rejectUnauthorized:true});
      secure.setTimeout(15000, () => {secure.destroy(); finish(new Error('proxy-tls-timeout'));});
      secure.once('secureConnect', () => finish(null, secure));
      secure.once('error', () => finish(new Error('proxy-tls')));
    });
    connect.once('error', () => finish(new Error('proxy-connect')));
    connect.once('timeout', () => {connect.destroy(); finish(new Error('proxy-timeout'));});
    connect.end();
  };
  return agent;
}

async function request(host, path, body) {
  const agent=agentFor(host);
  try {
    return await new Promise((resolve, reject) => {
      const payload=body === undefined ? undefined : JSON.stringify(body);
      const headers={Authorization:'Bearer '+token, 'User-Agent':'antigravity/1.3.3 butler/0.1', 'Content-Type':'application/json', Accept:'application/json'};
      if(payload !== undefined) headers['Content-Length']=Buffer.byteLength(payload);
      const req=https.request({hostname:host, port:443, path, method:payload === undefined ? 'GET' : 'POST', agent, headers, timeout:60000}, res => {
        let size=0; const chunks=[];
        res.on('data', part => {size+=part.length; if(size>2*1024*1024) {res.destroy(); reject(new Error('response-limit'));} else chunks.push(part);});
        res.once('error', () => reject(new Error('response-failed')));
        res.once('end', () => {
          let data={}; try {data=JSON.parse(Buffer.concat(chunks).toString('utf8'));} catch {}
          const details=Array.isArray(data.error?.details) ? data.error.details : [];
          if (host==='aicode.googleapis.com' && path==='/v1:generateContent' && process.env.BUTLER_PRIVATE_RESPONSE_PATH) {
            try { fs.writeFileSync(process.env.BUTLER_PRIVATE_RESPONSE_PATH,JSON.stringify({status:res.statusCode,data}),{mode:0o600,flag:'wx'}); } catch {emit({phase:'private-response-save',saved:false,rawDiagnosticsSuppressed:true});}
          }
          const allowedFields=['model','aicode','experience','entitlement','userTier','sessionId','contents','generationConfig','thinkingConfig','thinkingLevel','maxOutputTokens'];
          const message=String(data.error?.message??'').toLowerCase();
          emit({phase:'validation',fieldsMentioned:allowedFields.filter(f=>message.includes(f.toLowerCase())),flags:['oneof','mutually','required','missing','invalid','unsupported','unknown','deprecated','both','only'].filter(f=>message.includes(f)),fieldViolations:details.flatMap(d=>d.fieldViolations??[]).map(v=>v.field).filter(v=>typeof v==='string'&&/^(?:model|aicode|entitlement|sessionId|contents|generationConfig)(?:[.][A-Za-z_]+|\[\d+\])*$/.test(v))});
          emit({phase:'response', proxy:true, host, path, status:res.statusCode, apiStatus:enumValue(data.error?.status), reasons:details.map(d=>enumValue(d.reason)).filter(Boolean), detectedCountryISO:details.map(d=>String(d.metadata?.detected_location??'').toUpperCase()).find(c=>/^[A-Z]{2}$/.test(c)), errorFlags:['quota','capacity','credit','limit','overload','model','unsupported','missing','project','exhaust','authentication'].filter(t=>String(data.error?.message??'').toLowerCase().includes(t))});
          resolve({ok:res.statusCode >= 200 && res.statusCode < 300, data});
        });
      });
      req.once('error', () => reject(new Error('request-failed')));
      req.once('timeout', () => {req.destroy(); reject(new Error('request-timeout'));});
      req.end(payload);
    });
  } finally {agent.destroy();}
}

try {
  if(!process.env.BUTLER_PRIVATE_RESPONSE_PATH)throw new Error('private-response-required');
  const preflight=process.env.BUTLER_PRIVATE_RESPONSE_PATH+'.preflight';
  fs.closeSync(fs.openSync(preflight,'wx',0o600));fs.unlinkSync(preflight);
  emit({phase:'native-generation-request',targetModel:'gemini-3.8-flash',experience:'gemini-3.8-flash-tiered',serverDerivedEntitlement:true,flatContract:true,explicitCreditsEnabled:false});
  const result=await request('aicode.googleapis.com','/v1:generateContent',{aicode:{experience:'gemini-3.8-flash-tiered'},contents:[{role:'user',parts:[{text:'Reply with exactly BUTLER_ANTIGRAVITY_NATIVE_OK'}]}],generationConfig:{maxOutputTokens:512}});
  const response=result.data.response??result.data;
  emit({phase:'response-schema',responseWrapperPresent:!!result.data.response,candidatesFieldPresent:Array.isArray(response.candidates),modelVersionFieldPresent:typeof response.modelVersion==='string',usageMetadataFieldPresent:!!response.usageMetadata,promptFeedbackFieldPresent:!!response.promptFeedback});
  const candidates=Array.isArray(response.candidates)?response.candidates:[];
  const parts=candidates.flatMap(c=>c.content?.parts??[]);
  const text=parts.filter(p=>p.thought!==true&&typeof p.text==='string').map(p=>p.text).join('').trim();
  const safe=x=>typeof x==='string'&&/^[A-Za-z0-9_.-]{1,100}$/.test(x)?x:undefined;
  emit({phase:'native-generation',targetModel:'gemini-3.8-flash',httpOK:result.ok,candidatePresent:candidates.length>0,textPresent:text.length>0,expectedReply:text==='BUTLER_ANTIGRAVITY_NATIVE_OK',toolProposalPresent:parts.some(p=>p.functionCall!==undefined),modelVersion:safe(response.modelVersion),finishReason:safe(candidates[0]?.finishReason),promptTokens:typeof response.usageMetadata?.promptTokenCount==='number'?response.usageMetadata.promptTokenCount:undefined,outputTokens:typeof response.usageMetadata?.candidatesTokenCount==='number'?response.usageMetadata.candidatesTokenCount:undefined});
  if(!result.ok || text!=='BUTLER_ANTIGRAVITY_NATIVE_OK')process.exitCode=5;
} catch {emit({phase:'probe',failed:true,transportFailure:['proxy-denied','proxy-tls','proxy-tls-timeout','proxy-connect','proxy-timeout'].includes(transportFailure)?transportFailure:undefined,rawDiagnosticsSuppressed:true});process.exitCode=1;}