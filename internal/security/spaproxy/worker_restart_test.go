package spaproxy

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/butler/butler/internal/browserspa/eval"
)

func TestChromiumWorkerRestartCacheAndTerminalFreeze(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("requires actual Chromium")
	}
	var mu sync.Mutex
	counts := map[string]int{}
	f, err := eval.New(eval.Config{Origin: "https://external.test", Variant: "restarttest01", Category: "books", Title: "Restart catalog", Marker: "restart-receipt", Stock: 19}, func(e eval.Event) { mu.Lock(); counts[e.Kind]++; mu.Unlock() })
	if err != nil {
		t.Fatal(err)
	}
	p, _, _ := fixture(t, f)
	_, source, _, _ := runtime.Caller(0)
	module, err := filepath.Abs(filepath.Join(filepath.Dir(source), "..", "..", "..", "apps", "tool-browser", "node_modules", "playwright", "index.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", "--input-type=module", "-e", workerRestartProbe)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	encoder := json.NewEncoder(in)
	if encoder.Encode(map[string]string{"module": module, "url": f.URL(), "proxy": p.URL(), "pin": p.SPKI}) != nil {
		t.Fatal("probe input failed")
	}
	lines := bufio.NewScanner(out)
	if !lines.Scan() || lines.Text() != `{"restarted":true}` {
		t.Fatal("actual worker restart proof absent")
	}
	p.Close()
	// Browser remains alive, including its cache and pending event transports.
	// Terminal network closure cannot be reversed by worker activation/update.
	if encoder.Encode(map[string]bool{"frozen": true}) != nil {
		t.Fatal("probe freeze signal failed")
	}
	if !lines.Scan() || lines.Text() != `{"frozen":true}` {
		t.Fatal("worker freeze proof absent")
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal("worker probe failed")
	}
	mu.Lock()
	defer mu.Unlock()
	if counts["query"] != 2 || counts["cache_origin"] != 1 || counts["worker_script"] < 3 || counts["ws_subscribe"] != 1 || counts["sse_open"] != 1 || counts["forbidden_effect"] != 0 {
		t.Fatalf("recipient counts invalid: %v", counts)
	}
	stats, failure := p.Snapshot()
	if failure != "" || stats.Queries != 2 {
		t.Fatalf("terminal relay changed: %+v %s", stats, failure)
	}
}

const workerRestartProbe = `import readline from 'node:readline';import {pathToFileURL} from 'node:url';
const lines=readline.createInterface({input:process.stdin});const it=lines[Symbol.asyncIterator]();const c=JSON.parse((await it.next()).value);const {chromium}=await import(pathToFileURL(c.module).href);
const browser=await chromium.launch({headless:true,proxy:{server:c.proxy,bypass:''},args:['--disable-quic','--disable-background-networking','--force-webrtc-ip-handling-policy=disable_non_proxied_udp','--proxy-bypass-list=<-loopback>','--ignore-certificate-errors-spki-list='+c.pin]});
try{const context=await browser.newContext({serviceWorkers:'allow'});const page=await context.newPage();await page.goto(c.url,{timeout:6000});await page.locator('#result[data-ready=true]').waitFor({timeout:5000});
const probe=()=>page.evaluate(()=>new Promise((resolve,reject)=>{const channel=new MessageChannel();const timer=setTimeout(()=>reject(Error('worker timeout')),1500);channel.port1.onmessage=e=>{clearTimeout(timer);resolve(e.data)};navigator.serviceWorker.controller.postMessage('probe',[channel.port2])}));
const first=await probe();const cdp=await context.newCDPSession(page);await cdp.send('ServiceWorker.enable');await cdp.send('ServiceWorker.stopAllWorkers');const second=await probe();if(!first||first===second)throw Error('worker did not restart');
await page.evaluate(async()=>{const reg=await navigator.serviceWorker.getRegistration();await reg.update();await fetch(new URL('cached',location.href))});await page.locator('input').fill('books');await page.waitForFunction(()=>document.getElementById('result').textContent.includes('category: books'),{},{timeout:3000});process.stdout.write(JSON.stringify({restarted:true})+'\n');await it.next();
const proof=await page.evaluate(async()=>{const cache=await(await fetch(new URL('cached',location.href))).text();let denied=false;try{await fetch(new URL('effect',location.href),{method:'POST',body:'queued'})}catch{denied=true}let updateDenied=false;try{await(await navigator.serviceWorker.getRegistration()).update()}catch{updateDenied=true}await new Promise(r=>setTimeout(r,350));return cache.startsWith('cache-')&&denied&&updateDenied});if(!proof)throw Error('freeze bypass');process.stdout.write(JSON.stringify({frozen:true})+'\n');}finally{await browser.close();lines.close();}`
