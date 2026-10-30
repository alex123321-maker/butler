package runtime

import (
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
	"github.com/butler/butler/internal/browserpage"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/grpc/metadata"
)

func pageContract(name string) *toolbrokerv1.ToolContract {
	safety, method := "unsafe", "none"
	if browserpage.Read(name) {
		safety, method = "read_only", "read_result"
	}
	return &toolbrokerv1.ToolContract{ToolName: name, ContractRevision: browserpage.Revision, InputSchemaJson: browserpage.Schema(name), RequiresApproval: !browserpage.Read(name), EffectContract: &toolbrokerv1.EffectContract{RetrySafety: safety, VerificationMethod: method}}
}

func TestManagedPagesChromium(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_PAGES_E2E") != "1" {
		t.Skip("requires real isolated Chromium page profile")
	}
	began := time.Now()
	t.Cleanup(func() {
		t.Logf("scope=runtime_pages wall_ms=%.3f model_calls=0", float64(time.Since(began).Microseconds())/1000)
	})
	var forbidden, posts, late atomic.Int64
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/root":
			fmt.Fprint(w, `<html><title>Same title</title><body><p>root-reference</p><button id=popup onclick="window.open('/child?code=callback-private-canary','_blank')">Popup</button><button id=late onclick="setTimeout(()=>window.open('/child','_blank'),150)">Late</button><button id=foreign onclick="window.open('http://foreign.test/forbidden','_blank')">Foreign</button><button id=storm onclick="{for(let i=0;i<8;i++)window.open('/child','_blank')}">Storm</button><button id=too_late onclick="setTimeout(()=>window.open('/too-late','_blank'),800)">Too late</button><button id=post onclick="fetch('/post',{method:'POST'})">POST</button></body></html>`)
		case "/child":
			fmt.Fprint(w, `<html><title>Same title</title><body><p>child-reference</p><input autofocus id=draft oninput="document.querySelector('#echo').textContent=this.value"><span id=echo></span><input id=password type=password></body></html>`)
		case "/forbidden":
			forbidden.Add(1)
			fmt.Fprint(w, "forbidden")
		case "/post":
			posts.Add(1)
			fmt.Fprint(w, "posted")
		case "/too-late":
			late.Add(1)
			fmt.Fprint(w, "late")
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()
	script := os.Getenv("BUTLER_BROWSER_E2E_SCRIPT")
	if script == "" {
		script = "../../scripts/browser_runtime.mjs"
	}
	runner := NewPlaywrightRunnerWithPolicy("node", script, egresstest.Policy(site))
	absolute, e := filepath.Abs(script)
	if e != nil {
		t.Fatal("fixture script path unavailable")
	}
	runner.workspaceRoot = filepath.Clean(filepath.Join(filepath.Dir(absolute), "..", "..", ".."))
	defer runner.Close()
	server := NewServer(runner, nil)
	server.RequireDispatchEpoch = true
	var session string
	var epoch int64
	reports := map[string]browserpage.Report{}
	serial := 0
	call := func(t *testing.T, tool string, args browserpage.Args, expectedOK bool) Result {
		t.Helper()
		serial++
		raw, _ := json.Marshal(args)
		ctx := browsercontrol.WithDispatchEpoch(t.Context(), epoch)
		if args.Page != nil {
			p := reports[args.Page.ID]
			ctx = browsercontrol.WithPageDispatch(ctx, browserpage.Binding{Ref: *args.Page, URL: p.URL, Epoch: epoch, RuntimeVersion: p.RuntimeVersion})
		}
		md, _ := metadata.FromOutgoingContext(ctx)
		ctx = metadata.NewIncomingContext(ctx, md)
		req := &runtimev1.ExecuteRequest{ToolCall: &toolbrokerv1.ToolCall{ToolName: tool, ToolCallId: fmt.Sprintf("page-%d", serial), RunId: "managed-run", ArgsJson: string(raw)}, Contract: pageContract(tool), Context: &runtimev1.ExecutionContext{RunId: "managed-run"}}
		response, e := server.Execute(ctx, req)
		if e != nil || response == nil {
			t.Fatal("page runtime unavailable")
		}
		if (response.Result.Status == "completed") != expectedOK {
			t.Fatal("unexpected page runtime status", tool, response.Result.Status)
		}
		var out Result
		if expectedOK {
			if json.Unmarshal([]byte(response.Result.ResultJson), &out) != nil {
				t.Fatal("invalid page result")
			}
			for _, p := range out.PrivatePages {
				reports[p.ID] = p
			}
			if strings.Contains(response.Result.ResultJson, "callback-private-canary") || strings.Contains(response.Result.ResultJson, "owner-private-canary") {
				t.Fatal("private value exported")
			}
		}
		return out
	}
	ref := func(id string) *browserpage.Ref {
		p := reports[id]
		return &browserpage.Ref{ID: id, DocumentID: p.DocumentID, Version: 17}
	}
	opened := call(t, browserpage.Open, browserpage.Args{URL: egresstest.Origin + "/root"}, true)
	session = opened.SessionID
	root := opened.SelectedPageID
	if !browserpage.ID(root) || len(opened.PrivatePages) != 1 {
		t.Fatal("missing root page identity")
	}
	args := func(id string) browserpage.Args { return browserpage.Args{SessionID: session, Page: ref(id)} }
	rootClick := func(t *testing.T, selector string) Result {
		a := args(root)
		a.Selector = selector
		return call(t, browserpage.Click, a, true)
	}
	t.Run("same_title_exact_target", func(t *testing.T) {
		out := rootClick(t, "#popup")
		if len(out.PrivatePages) != 2 {
			t.Fatal("same-origin popup not admitted")
		}
		var child string
		for _, p := range out.PrivatePages {
			if p.OpenerID == root {
				child = p.ID
			}
		}
		if child == "" {
			t.Fatal("popup opener missing")
		}
		t.Logf("popup_state=%s runtime_version=%d", reports[child].State, reports[child].RuntimeVersion)
		seen := call(t, browserpage.Snapshot, args(child), true)
		if !strings.Contains(seen.Text, "child-reference") || strings.Contains(seen.Text, "root-reference") {
			t.Fatal("wrong page text")
		}
		fill := args(child)
		fill.Selector, fill.Value = "#draft", "page-draft"
		call(t, browserpage.Fill, fill, true)
		typed := args(child)
		typed.Selector, typed.Text = "#draft", "-typed"
		call(t, browserpage.Type, typed, true)
		seen = call(t, browserpage.Snapshot, args(child), true)
		if !strings.Contains(seen.Text, "page-draft-typed") {
			t.Fatal("page fill/type missed the bound document")
		}
		fill = args(child)
		fill.Selector, fill.Value = "#password", "must-not-enter"
		call(t, browserpage.Fill, fill, false)
		bad := args(child)
		bad.Page.DocumentID = reports[root].DocumentID
		call(t, browserpage.Click, bad, false)
	})
	t.Run("foreign_first_request_denied", func(t *testing.T) {
		rootClick(t, "#foreign")
		if forbidden.Load() != 0 {
			t.Fatal("foreign popup sent a request")
		}
	})
	t.Run("outside_gesture_denied", func(t *testing.T) {
		rootClick(t, "#too_late")
		time.Sleep(900 * time.Millisecond)
		call(t, browserpage.List, browserpage.Args{SessionID: session}, true)
		if late.Load() != 0 {
			t.Fatal("popup admitted after gesture finished")
		}
	})
	t.Run("owner_selection_and_global_fence", func(t *testing.T) {
		var child string
		for _, p := range reports {
			if p.OpenerID == root && p.State == "ready" {
				child = p.ID
				break
			}
		}
		controller := strings.Repeat("c", 32)
		taken, e := server.Control(t.Context(), browsercontrol.Command{Operation: "take", SessionID: session, RunID: "managed-run", Epoch: 1, Controller: controller, PageID: root})
		if e != nil || taken.Frame == nil {
			t.Fatal("takeover failed")
		}
		epoch = 1
		call(t, browserpage.Snapshot, args(child), false)
		frame, e := server.Control(t.Context(), browsercontrol.Command{Operation: "frame", SessionID: session, RunID: "managed-run", PageID: child})
		if e != nil || frame.Frame == nil || frame.Frame.PageID != child {
			t.Fatal("owner page selection failed")
		}
		if _, e = server.Control(t.Context(), browsercontrol.Command{Operation: "key", SessionID: session, RunID: "managed-run", Epoch: 1, Controller: controller, PageID: child, DocumentID: frame.Frame.DocumentID, FrameID: taken.Frame.ID, Key: "Enter"}); e == nil {
			t.Fatal("old page frame token accepted")
		}
		frame, e = server.Control(t.Context(), browsercontrol.Command{Operation: "frame", SessionID: session, RunID: "managed-run", PageID: child})
		if e != nil {
			t.Fatal("fresh owner frame failed")
		}
		if _, e = server.Control(t.Context(), browsercontrol.Command{Operation: "text", SessionID: session, RunID: "managed-run", Epoch: 1, Controller: controller, PageID: child, DocumentID: frame.Frame.DocumentID, FrameID: frame.Frame.ID, Text: "owner-private-canary"}); e != nil {
			t.Fatal("owner input failed")
		}
		if _, e = server.Control(t.Context(), browsercontrol.Command{Operation: "release", SessionID: session, RunID: "managed-run", Epoch: 1, Controller: controller}); e != nil {
			t.Fatal("owner return failed")
		}
		epoch = 2
		call(t, browserpage.Snapshot, args(child), false)
		call(t, browserpage.List, browserpage.Args{SessionID: session}, true)
		call(t, browserpage.Snapshot, args(child), true)
	})
	t.Run("late_popup_and_live_cap", func(t *testing.T) {
		rootClick(t, "#late")
		rootClick(t, "#storm")
		out := call(t, browserpage.List, browserpage.Args{SessionID: session}, true)
		live := 0
		for _, p := range out.PrivatePages {
			if p.State == "ready" || p.State == "pending" {
				live++
			}
		}
		if live != 4 {
			t.Fatal("page cap not enforced", live)
		}
	})
	t.Run("close_exact_page_retains_identity", func(t *testing.T) {
		var child string
		for _, p := range reports {
			if p.OpenerID == root && p.State == "ready" {
				child = p.ID
				break
			}
		}
		if child == "" {
			t.Fatal("missing close target")
		}
		call(t, browserpage.Close, args(child), true)
		if reports[child].State != "closed" || reports[root].State != "ready" {
			t.Fatal("close retargeted or forgot page")
		}
		call(t, browserpage.Snapshot, args(child), false)
		call(t, browserpage.Snapshot, args(root), true)
	})
	t.Run("runtime_loss_no_restore", func(t *testing.T) {
		if runner.Close() != nil {
			t.Fatal("runtime stop failed")
		}
		call(t, browserpage.Snapshot, args(root), false)
	})
	if posts.Load() != 0 {
		t.Fatal("unexpected recipient POST")
	}
}
