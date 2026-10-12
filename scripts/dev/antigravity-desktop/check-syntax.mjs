// Run this syntax check only on laba; no dependency install or network.
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
for(const name of ['inspect-local.mjs','deny-proxy.mjs','fixture-relay.mjs']){
  execFileSync(process.execPath,['--check',fileURLToPath(new URL(name,import.meta.url))],{stdio:'pipe'});
}
console.log(JSON.stringify({operator_scripts_syntax_passed:true,real_model_calls:0}));
