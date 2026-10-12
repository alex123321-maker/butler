// Fixed transport metadata only. Never exports the environment or proxy URL.
import {execFileSync} from 'node:child_process';
try {
  const c=JSON.parse(execFileSync('docker',['inspect','butler-lab-orchestrator-1'],{maxBuffer:2*1024*1024}))[0];
  const e=Object.fromEntries(c.Config.Env.map(s=>{const i=s.indexOf('=');return [s.slice(0,i),s.slice(i+1)];}));
  const chosen=e.BUTLER_ANTIGRAVITY_PROXY??e.HTTPS_PROXY??e.HTTP_PROXY;
  const u=new URL(chosen);
  const noProxy=(e.NO_PROXY??e.no_proxy??'').split(',').map(s=>s.trim());
  const oauth='oauth2.googleapis.com';
  console.log(JSON.stringify({
    source_uses_host_network:c.HostConfig.NetworkMode==='host',
    custom_proxy_configured:!!e.BUTLER_ANTIGRAVITY_PROXY,
    https_proxy_configured:!!e.HTTPS_PROXY,
    chosen_matches_standard_https_proxy:chosen===e.HTTPS_PROXY,
    chosen_host_is_loopback:['localhost','127.0.0.1','::1'].includes(u.hostname),
    chosen_host_is_proxy_service:['antigravity-proxy','butler-lab-antigravity-proxy-1'].includes(u.hostname),
    proxy_scheme:['http:','https:','socks5:','socks5h:'].includes(u.protocol)?u.protocol:'UNKNOWN',
    source_no_proxy_bypasses_oauth:noProxy.some(s=>s==='*'||s===oauth||s==='googleapis.com'||s==='.googleapis.com'),
    raw_environment_suppressed:true
  }));
} catch {console.log(JSON.stringify({transport_metadata_audit_failed:true,raw_environment_suppressed:true}));process.exitCode=1;}
