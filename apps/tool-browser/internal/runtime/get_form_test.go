package runtime

import (
	"context"
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

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

func getFormContract() *toolbrokerv1.ToolContract {
	return &toolbrokerv1.ToolContract{ToolName: "browser.submit_get_form", ContractRevision: "1", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}, AllowedDomains: []string{"external.test"}}
}

func TestGetFormContractAndPrivateProtocol(t *testing.T) {
	runner := &stubRunner{result: Result{OK: true, GetFormTarget: "private-target", GetFormNonce: "private-nonce"}}
	server := NewServer(runner, nil)
	obs := openTest(t, server)
	args := actionArgs(obs)
	args["selector"] = "form"
	args["fields"] = []map[string]string{{"name": "q", "value": "query"}}
	req := browserCall("browser.submit_get_form", "run-1", args)
	req.Contract = getFormContract()
	req.Contract.AllowedDomains = []string{"example.com"}
	req.Contract.ContractRevision = "2"
	assertCode(t, executeTest(t, server, req), "contract_mismatch")
	req.Contract.ContractRevision = "1"
	req.Contract.EffectContract.VerificationMethod = "none"
	assertCode(t, executeTest(t, server, req), "contract_mismatch")
	req.Contract = getFormContract()
	req.Contract.AllowedDomains = []string{"example.com"}
	result := executeTest(t, server, req)
	if result.GetStatus() != "completed" || strings.Contains(result.ResultJson, "private-") {
		t.Fatal("private form plan leaked", result.GetStatus())
	}
	for _, raw := range []string{`{"fields":[{"name":"q","value":{"type":"credential_ref","alias":"a","field":"password"}}]}`, `{"fields":[{"name":"q","value":null}]}`, `{"fields":{}}`, `{"fields":[{"name":"q","value":"a"},{"name":"q","value":"b"}]}`, `{}`} {
		if _, err := decodeRequest("browser.submit_get_form", raw, nil); err == nil {
			t.Fatal("non-text form field accepted", raw)
		}
	}
	parsed, err := decodeRequest("browser.submit_get_form", `{"fields":[{"name":"q","value":"x"}],"get_form_phase":"navigate","get_form_nonce":"forged"}`, nil)
	if err != nil || parsed.GetFormPhase != "" || parsed.GetFormNonce != "" {
		t.Fatal("model supplied private form protocol")
	}
	if _, err := decodeRequest("browser.submit_get_form", `{"fields":[{"name":"q","value":"x"}]}`, []*runtimev1.ResolvedCredential{{Value: "private"}}); err == nil {
		t.Fatal("credential entered GET form")
	}
}

// Actual Chromium and Go proxy; POST and input/submit callbacks are traps.
func TestBrowserGetFormChromium(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("requires isolated Chromium image")
	}
	var gets, writes, handlers atomic.Int64
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			writes.Add(1)
			w.WriteHeader(204)
			return
		}
		switch r.URL.Path {
		case "/query":
			gets.Add(1)
			fmt.Fprintf(w, `<body>Found %s<script>fetch('/write',{method:'POST'}).catch(()=>{});setTimeout(()=>fetch('/late').catch(()=>{}),200)</script></body>`, r.URL.Query().Get("q"))
		case "/handler", "/late":
			handlers.Add(1)
			fmt.Fprint(w, "unexpected")
		default:
			form := `<form action="/query" method="get" onsubmit="fetch('/handler');fetch('/write',{method:'POST'})"><input name="q" value="old" oninput="fetch('/handler')"><button>Search</button></form>`
			switch r.URL.Path {
			case "/post":
				form = strings.ReplaceAll(form, `method="get"`, `method="post"`)
			case "/hidden":
				form = strings.ReplaceAll(form, `<button>`, `<input type="hidden" name="csrf" value="private"><button>`)
			case "/password":
				form = strings.ReplaceAll(form, `<button>`, `<input type="password" name="password"><button>`)
			case "/file":
				form = strings.ReplaceAll(form, `<button>`, `<input type="file" name="file"><button>`)
			case "/cross":
				form = strings.ReplaceAll(form, `action="/query"`, `action="http://foreign.test/query"`)
			case "/private":
				form = strings.ReplaceAll(form, `action="/query"`, `action="http://127.0.0.1/query"`)
			case "/cookie":
				http.SetCookie(w, &http.Cookie{Name: "auth", Value: "private-cookie-canary"})
			case "/sensitive":
				form = strings.ReplaceAll(form, `name="q"`, `name="access_token"`)
			case "/stale":
				form += `<script>setTimeout(()=>document.forms[0].method='post',200)</script>`
			}
			fmt.Fprint(w, "<html><body>"+form+"</body></html>")
		}
	}))
	defer site.Close()
	script := os.Getenv("BUTLER_BROWSER_E2E_SCRIPT")
	if script == "" {
		script = filepath.Join("..", "..", "scripts", "browser_runtime.mjs")
	}
	runner := NewPlaywrightRunnerWithPolicy("node", script, egresstest.Policy(site))
	runner.workspaceRoot, _ = filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if filepath.IsAbs(script) {
		runner.workspaceRoot = filepath.Clean(filepath.Join(filepath.Dir(script), "..", "..", ".."))
	}
	server := NewServer(runner, nil)
	defer server.Close()
	invoke := func(tool string, args map[string]any) (*toolbrokerv1.ToolResult, Result) {
		t.Helper()
		req := browserCall(tool, "run-1", args)
		req.Contract.AllowedDomains = []string{"external.test"}
		if tool == "browser.submit_get_form" {
			req.Contract = getFormContract()
		}
		response, err := server.Execute(context.Background(), req)
		if err != nil {
			t.Fatal("RPC", err)
		}
		var obs Result
		if response.Result.ResultJson != "" && json.Unmarshal([]byte(response.Result.ResultJson), &obs) != nil {
			t.Fatal("invalid result")
		}
		if strings.Contains(response.Result.ResultJson, "private-cookie-canary") || strings.Contains(response.Result.ResultJson, "get_form_nonce") || strings.Contains(response.Result.ResultJson, "get_form_target") {
			t.Fatal("private state leaked")
		}
		return response.Result, obs
	}
	_, obs := invoke("browser.open", map[string]any{"url": egresstest.Origin + "/search"})
	if len(obs.Forms) != 1 || obs.Forms[0].Fields[0].Name != "q" {
		t.Fatal("missing value-free form metadata")
	}
	args := actionArgs(obs)
	args["selector"] = obs.Forms[0].Selector
	args["fields"] = []map[string]string{{"name": "q", "value": "Arctic delivery"}}
	result, obs := invoke("browser.submit_get_form", args)
	if result.Status != "completed" || !strings.Contains(obs.Text, "Found Arctic delivery") || obs.NetworkState != "frozen" || gets.Load() != 1 {
		t.Fatal("GET form did not navigate", result.GetStatus(), obs.Text)
	}
	// Frozen observation must not release delayed callbacks or another POST.
	_, _ = invoke("browser.snapshot", map[string]any{"browser_session_id": obs.SessionID})
	_, stale := invoke("browser.open", map[string]any{"url": egresstest.Origin + "/stale"})
	time.Sleep(300 * time.Millisecond)
	a := actionArgs(stale)
	a["selector"] = "form"
	a["fields"] = []map[string]string{{"name": "q", "value": "stale"}}
	if rejected, _ := invoke("browser.submit_get_form", a); rejected.Status == "completed" {
		t.Fatal("live stale form navigated")
	}
	for _, path := range []string{"post", "hidden", "password", "file", "cross", "private", "cookie", "sensitive"} {
		_, current := invoke("browser.open", map[string]any{"url": egresstest.Origin + "/" + path})
		a := actionArgs(current)
		a["selector"] = "form"
		a["fields"] = []map[string]string{{"name": "q", "value": "no-send"}}
		rejected, _ := invoke("browser.submit_get_form", a)
		if rejected.Status == "completed" {
			t.Fatal("unsafe GET form accepted", path)
		}
	}
	if gets.Load() != 1 || writes.Load() != 0 || handlers.Load() != 0 {
		t.Fatal("form handlers or write escaped", gets.Load(), writes.Load(), handlers.Load())
	}
}
