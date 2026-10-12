// Process-scoped Chromium proxy. Only the SSH-tunnelled fixture UI is reachable.
import net from 'node:net';
const server=net.createServer(socket=>{
  let header=Buffer.alloc(0);
  const receive=chunk=>{
    header=Buffer.concat([header,chunk]);
    if(header.length>8192){socket.destroy();return;}
    const end=header.indexOf('\r\n\r\n');if(end<0)return;
    socket.off('data',receive);
    const first=header.subarray(0,header.indexOf('\r\n')).toString();
    if(first!=='CONNECT 127.0.0.1:49631 HTTP/1.1'){socket.end('HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n');return;}
    const upstream=net.connect(49631,'127.0.0.1',()=>{
      socket.write('HTTP/1.1 200 Connection Established\r\n\r\n');
      const rest=header.subarray(end+4);if(rest.length)upstream.write(rest);
      header.fill(0);socket.pipe(upstream);upstream.pipe(socket);
    });
    const close=()=>{socket.destroy();upstream.destroy();};
    upstream.on('error',close);socket.on('error',close);socket.on('close',()=>upstream.destroy());
  };
  socket.on('data',receive);socket.on('error',()=>socket.destroy());
});
server.on('error',()=>{console.log(JSON.stringify({desktop_proxy_failed:true}));process.exit(1);});
server.listen(35128,'127.0.0.1',()=>console.log(JSON.stringify({desktop_proxy_ready:true,external_network_denied:true})));
