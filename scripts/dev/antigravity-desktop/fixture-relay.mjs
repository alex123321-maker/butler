// Private operator ingress. TLS bytes go only between a local UI and fixed fixture.
// No traffic, headers, browser state or credentials are logged.
import net from 'node:net';
import {spawn} from 'node:child_process';
const server=net.createServer(socket=>{
  const child=spawn('docker',['exec','-i','butler-agy-desktop-fixture','/fixture','relay'],{stdio:['pipe','pipe','ignore']});
  socket.pipe(child.stdin);child.stdout.pipe(socket);
  const close=()=>{socket.destroy();child.kill();};
  child.on('error',close);child.on('exit',()=>socket.end());
  child.stdin.on('error',close);socket.on('error',close);socket.on('close',()=>child.kill());
});
server.on('error',()=>{console.log(JSON.stringify({fixture_relay_failed:true}));process.exit(1);});
server.listen(35129,'127.0.0.1',()=>console.log(JSON.stringify({fixture_relay_ready:true,real_model_calls:0})));
const stop=()=>server.close(()=>process.exit());process.on('SIGTERM',stop);process.on('SIGINT',stop);
