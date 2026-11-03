package spaproxy

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/browserspa/eval"
)

func TestRelayActualChromiumWorkerPOSTWSSSSE(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("requires actual Chromium")
	}
	var mu sync.Mutex
	var events []eval.Event
	f, err := eval.New(eval.Config{Origin: "https://external.test", Variant: "mechanical01", Category: "books", Title: "Private test title", Marker: "test-receipt-9381", Stock: 73}, func(e eval.Event) { mu.Lock(); events = append(events, e); mu.Unlock() })
	if err != nil {
		t.Fatal(err)
	}
	p, _, _ := fixture(t, f)
	_, source, _, _ := runtime.Caller(0)
	module, err := filepath.Abs(filepath.Join(filepath.Dir(source), "..", "..", "..", "apps", "tool-browser", "node_modules", "playwright", "index.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]string{"module": module, "proxy": p.URL(), "pin": p.SPKI, "url": f.URL()})
	cmd := exec.Command("node", "--input-type=module", "-e", chromiumProbe)
	cmd.Stdin = bytes.NewReader(input)
	var output, diagnostics bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &diagnostics
	if err := cmd.Run(); err != nil {
		_, failure := p.Snapshot()
		// Diagnostic bytes may contain URLs/state. Only the fixed relay category
		// and Node's exit status are published, never its raw stderr or DOM.
		t.Fatalf("actual Chromium probe failed: exit=%v relay=%s", err, failure)
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if json.Unmarshal(output.Bytes(), &result) != nil || !result.OK {
		t.Fatal("invalid Chromium proof")
	}
	p.Close()
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		counts := map[string]int{}
		filtered := 0
		for _, e := range events {
			counts[e.Kind]++
			if e.Kind == "query" && e.Accepted && e.Category == "books" {
				filtered++
			}
		}
		mu.Unlock()
		if counts["sse_closed"] == 1 && counts["ws_closed"] == 1 {
			if counts["query"] != 2 || filtered != 1 || counts["worker_script"] < 2 || counts["cache_origin"] != 1 || counts["ws_client_message"] != 0 || counts["forbidden_effect"] != 0 {
				t.Fatalf("independent recipient counts inconsistent: %v", counts)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Chromium channels did not settle after terminal freeze")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stats, failure := p.Snapshot()
	if failure != "" || stats.Queries != 2 || stats.Messages != 2 || stats.SentMessages != 2 {
		t.Fatalf("relay count mismatch: %+v reason=%s", stats, failure)
	}
}

const chromiumProbe = `import fs from 'node:fs';import {pathToFileURL} from 'node:url';
const c=JSON.parse(fs.readFileSync(0,'utf8'));const {chromium}=await import(pathToFileURL(c.module).href);
const browser=await chromium.launch({headless:true,proxy:{server:c.proxy,bypass:''},args:['--disable-quic','--disable-background-networking','--force-webrtc-ip-handling-policy=disable_non_proxied_udp','--proxy-bypass-list=<-loopback>','--ignore-certificate-errors-spki-list='+c.pin]});
try{const context=await browser.newContext({serviceWorkers:'allow',acceptDownloads:false});const page=await context.newPage();await page.goto(c.url,{waitUntil:'domcontentloaded',timeout:6000});await page.locator('#result[data-ready=true]').waitFor({timeout:6000});await page.locator('input[name=category]').fill('books');await page.waitForFunction(()=>document.getElementById('result').textContent.includes('category: books'),{},{timeout:3000});const valid=await page.locator('#result').textContent();if(!valid.includes('Private test title')||!valid.includes('stock:73')||!valid.includes('worker: true'))throw Error('probe mismatch');process.stdout.write(JSON.stringify({ok:true}));}finally{await browser.close();}`
