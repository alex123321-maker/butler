package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/browsercontrol"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

func TestServerBrowserChromium(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("isolated Chromium image required")
	}
	var posts, cdn, private atomic.Int32
	var mutateFrame, frameMutated atomic.Bool
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "http://login.test/form", 302)
		case "/form":
			fmt.Fprint(w, `<html><title>Remote browser fixture</title><body><h1>Login fixture</h1><input id="password" type="password" style="position:absolute;left:20px;top:100px;width:200px;height:40px"><button style="position:absolute;left:20px;top:180px;width:200px;height:40px" onclick="fetch('/submit',{method:'POST'}).then(()=>document.querySelector('h1').textContent='Submitted')">Submit</button><script src="http://cdn.test/cdn.js"></script></body></html>`)
		case "/cdn.js":
			cdn.Add(1)
			w.Header().Set("Content-Type", "application/javascript")
			fmt.Fprint(w, `document.body.dataset.cdn='loaded'`)
		case "/submit":
			posts.Add(1)
			fmt.Fprint(w, "ok")
		case "/reflect":
			fmt.Fprint(w, `<body>owner-private-password</body>`)
		case "/with-frame":
			fmt.Fprint(w, `<body><iframe src="http://child.test/child" style="position:absolute;left:0;top:0;width:500px;height:400px;border:0"></iframe></body>`)
		case "/child":
			fmt.Fprint(w, `<body><button style="position:absolute;left:20px;top:180px;width:200px;height:40px" onclick="fetch('/submit',{method:'POST'})">Child submit</button><script>const poll=setInterval(async()=>{if((await fetch('/frame-state')).status===201){clearInterval(poll);document.body.dataset.changed='true';await fetch('/frame-mutated')}},30)</script></body>`)
		case "/frame-state":
			if mutateFrame.Load() {
				w.WriteHeader(201)
			}
		case "/frame-mutated":
			frameMutated.Store(true)
		case "/private":
			private.Add(1)
		default:
			fmt.Fprint(w, `<body>Another public origin</body>`)
		}
	}))
	defer site.Close()
	runner := NewPlaywrightRunnerWithPolicy("node", os.Getenv("BUTLER_BROWSER_E2E_SCRIPT"), egresstest.Policy(site))
	runner.workspaceRoot = filepath.Clean(filepath.Join(filepath.Dir(os.Getenv("BUTLER_BROWSER_E2E_SCRIPT")), "..", "..", ".."))
	server := NewServer(runner, nil)
	defer server.Close()
	invoke := func(tool string, args map[string]any) Result {
		t.Helper()
		req := browserCall(tool, "run-1", args)
		req.Contract.AllowedDomains = nil
		resp, err := server.Execute(context.Background(), req)
		if err != nil || resp.Result.Status != "completed" {
			t.Fatalf("%s: %v %v", tool, err, resp)
		}
		var result Result
		if json.Unmarshal([]byte(resp.Result.ResultJson), &result) != nil {
			t.Fatal("invalid result")
		}
		return result
	}
	obs := invoke("browser.open", map[string]any{"url": "http://start.test/start"})
	if obs.FinalURL != "http://login.test/form" || cdn.Load() != 1 {
		t.Fatal("cross-origin redirect/resource failed", obs.FinalURL, cdn.Load())
	}
	command := browsercontrol.Command{Operation: "take", RunID: "run-1", SessionID: obs.SessionID, Controller: strings.Repeat("a", 32), Epoch: 1}
	reply, err := server.Control(context.Background(), command)
	if err != nil || reply.Frame == nil {
		t.Fatal("take/frame failed", err)
	}
	pixels, err := base64.StdEncoding.DecodeString(reply.Frame.JPEG)
	if err != nil || len(pixels) < 100 || pixels[0] != 0xff || pixels[1] != 0xd8 {
		t.Fatal("not a rendered JPEG")
	}
	assertCode(t, executeTest(t, server, browserCall("browser.snapshot", "run-1", map[string]any{"browser_session_id": obs.SessionID})), "owner_control")
	control := func(op string, edit func(*browsercontrol.Command)) browsercontrol.Reply {
		t.Helper()
		c := command
		c.Operation = op
		c.FrameID = reply.Frame.ID
		if edit != nil {
			edit(&c)
		}
		r, e := server.Control(context.Background(), c)
		if e != nil {
			t.Fatalf("owner %s failed: %v", op, e)
		}
		return r
	}
	old := reply.Frame.ID
	reply = control("click", func(c *browsercontrol.Command) { c.X = 40; c.Y = 120 })
	stale := command
	stale.Operation = "text"
	stale.FrameID = old
	stale.Text = "should-not-type"
	if _, e := server.Control(context.Background(), stale); e == nil {
		t.Fatal("replayed frame accepted")
	}
	// Real keystrokes retain successive private input prefixes. Masking must
	// hide the whole later reflected value, not just its earliest prefix.
	for _, key := range "owner-private-password" {
		reply = control("key", func(c *browsercontrol.Command) { c.Key = string(key) })
	}
	reply = control("key", func(c *browsercontrol.Command) { c.Key = "Tab" })
	reply = control("click", func(c *browsercontrol.Command) { c.X = 60; c.Y = 200 })
	deadline := time.Now().Add(time.Second)
	for posts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if posts.Load() != 1 {
		t.Fatal("manual click did not submit exactly once", posts.Load())
	}
	// Refresh a changed DOM before another manual action.
	c := command
	c.Operation = "frame"
	reply, err = server.Control(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	reply = control("navigate", func(c *browsercontrol.Command) { c.URL = "http://third.test/reflect" })
	reply = control("navigate", func(c *browsercontrol.Command) { c.URL = "http://parent.test/with-frame" })
	mutateFrame.Store(true)
	deadline = time.Now().Add(3 * time.Second)
	for !frameMutated.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !frameMutated.Load() {
		t.Fatal("cross-origin child mutation did not complete")
	}
	stale = command
	stale.Operation, stale.FrameID, stale.X, stale.Y = "click", reply.Frame.ID, 50, 200
	if _, err := server.Control(context.Background(), stale); err == nil || posts.Load() != 1 {
		t.Fatal("changed child document accepted old screenshot input", err, posts.Load())
	}
	reply = control("frame", nil)
	reply = control("navigate", func(c *browsercontrol.Command) { c.URL = "http://third.test/reflect" })
	command.Operation = "release"
	if _, err := server.Control(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	assertCode(t, executeTest(t, server, browserCall("browser.click", "run-1", actionArgs(obs))), "scope_denied") // stale old contract cap still applies
	obs = invoke("browser.snapshot", map[string]any{"browser_session_id": obs.SessionID})
	if strings.TrimSpace(obs.Text) != "[REDACTED]" {
		t.Fatal("manual password leaked in model observation", obs.Text)
	}
	args := actionArgs(obs)
	args["url"] = site.URL + "/private"
	req := browserCall("browser.navigate", "run-1", args)
	req.Contract.AllowedDomains = nil
	resp, _ := server.Execute(context.Background(), req)
	if resp.Result.Status == "completed" || private.Load() != 0 {
		t.Fatal("manual/multi-origin weakened private-address guard")
	}
}
