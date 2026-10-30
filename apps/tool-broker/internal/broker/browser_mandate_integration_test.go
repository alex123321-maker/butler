package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browsercontrol"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Owner HTTP -> goal/mandate/Task/control Input -> mechanical worker projection
// -> real Broker subprocess/PG/resource guard/Chromium. This is not native model
// or delivery qualification, and the stricter click registry is isolated only.
func TestBrowserOwnerMandateApprovalReductionChromiumIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_MANDATE_E2E") != "1" {
		t.Skip("explicit isolated owner API/PG/Broker/Chromium required")
	}
	t.Setenv("BUTLER_BROWSER_MANDATE_SCOPED_READS", "1")
	ready := filepath.Join(t.TempDir(), "owner-ready.json")
	cmd := exec.Command(os.Getenv("BUTLER_BROWSER_OWNER_API_BINARY"), "-test.run=^TestBrowserMandateOwnerAPIHarness$", "-test.timeout=5m")
	cmd.Env = append(os.Environ(), "BUTLER_BROWSER_MANDATE_OWNER_HARNESS=1", "BUTLER_BROWSER_MANDATE_OWNER_READY="+ready)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if cmd.Start() != nil {
		t.Fatal("owner API process unavailable")
	}
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }) })
	var cfg struct{ DSN, SearchPath, Address string }
	deadline := time.Now().Add(25 * time.Second)
	for {
		b, e := os.ReadFile(ready)
		if e == nil && json.Unmarshal(b, &cfg) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owner API readiness timed out")
		}
		time.Sleep(25 * time.Millisecond)
	}
	pc, e := pgxpool.ParseConfig(cfg.DSN)
	if e != nil {
		t.Fatal("isolated PG config invalid")
	}
	pc.ConnConfig.RuntimeParams["search_path"] = cfg.SearchPath
	pc.MaxConns = 3
	pool, e := pgxpool.NewWithConfig(context.Background(), pc)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	serial := 0
	ownerPOST := func(t *testing.T, path string, body any) map[string]json.RawMessage {
		t.Helper()
		began := time.Now()
		operation := "worker/http"
		switch path {
		case "/api/v3/owner-goals":
			operation = "owner/create_goal"
		case "/api/v3/owner-mandates":
			operation = "owner/mandate"
		case "/api/v3/owner-goals/tasks":
			operation = "owner/create_task"
		case "/fixture/claim":
			operation = "worker/claim_input"
		case "/fixture/approval":
			operation = "worker/propose_approval"
		default:
			if strings.HasPrefix(path, "/api/v3/tasks/") {
				operation = "owner/task_control"
			}
			if strings.HasPrefix(path, "/api/v2/approvals/") {
				operation = "owner/approve_submit"
			}
		}
		defer func() {
			t.Logf("operation=%s duration_ms=%.3f model_calls=0", operation, float64(time.Since(began).Microseconds())/1000)
		}()
		serial++
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", cfg.Address+path, strings.NewReader(string(raw)))
		req.Header.Set("Authorization", "Bearer fixture-owner-token-000000000000000")
		req.Header.Set("X-Butler-Delivery-ID", fmt.Sprintf("browser-mandate-%d", serial))
		req.Header.Set("Content-Type", "application/json")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("owner request failed", path)
		}
		defer r.Body.Close()
		if r.StatusCode != 200 && r.StatusCode != 201 {
			t.Fatal("owner request denied", path, r.StatusCode)
		}
		var out map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&out) != nil {
			t.Fatal("invalid owner receipt")
		}
		return out
	}
	text := func(v map[string]json.RawMessage, k string) string {
		var s string
		_ = json.Unmarshal(v[k], &s)
		return s
	}
	for _, kind := range []string{"prepare_only", "unsafe_unknown", "strict", "changed_args", "revoke", "cancel", "expiry", "foreign_origin", "stale_contract", "restart", "budget", "scope_origin", "scope_epoch"} {
		t.Run(kind, func(t *testing.T) {
			setupStart := serial
			now := time.Now().UTC()
			expires := now.Add(20 * time.Minute)
			if kind == "expiry" {
				expires = now.Add(30 * time.Second)
			}
			g := ownerPOST(t, "/api/v3/owner-goals", map[string]any{"goal": map[string]any{"kind": "objective", "status": "active", "title": "Synthetic browser mandate " + kind, "purpose": "browser_mandate_" + kind, "expires_at": now.Add(time.Hour)}})
			gid := text(g, "subject_id")
			effects := []string{"read", "prepare", "unknown"}
			if kind == "prepare_only" {
				effects = effects[:2]
			}
			mandate := map[string]any{"goal_id": gid, "goal_revision": 1, "status": "active", "purpose": "browser_mandate_" + kind, "tools": []string{"browser.open", "browser.snapshot", "browser.prepare_fill", "browser.click", "browser.freeze_snapshot"}, "resource_scopes": []map[string]string{{"origin": egresstest.Origin, "path_prefix": "/"}}, "effects": effects, "privacy": "cloud_allowed", "budget": map[string]int{"model_turns": 20, "tokens": 20000, "external_calls": 24, "children": 1, "active_millis": 600000, "provider_input_bytes": 10000000, "egress_bytes": 10000000}, "expires_at": expires, "intervention": []string{"effect_unknown", "scope_change", "budget_exhausted"}, "reporting": "exceptions", "allow_initiative": false}
			if kind != "budget" {
				mandate["budget"].(map[string]int)["egress_bytes"] = 100000000
			}
			mr := ownerPOST(t, "/api/v3/owner-mandates", map[string]any{"mandate": mandate})
			mid := text(mr, "subject_id")
			created := ownerPOST(t, "/api/v3/owner-goals/tasks", map[string]any{"session_key": "web:mandate-" + kind, "goal_id": gid, "mandate_id": mid, "purpose": "browser_mandate_" + kind, "goal": "Prepare the synthetic form, submit only with exact approval", "constraints": []string{"No replay; no other origin"}, "completion_criteria": []string{"Recipient and approvals independently checked"}, "initiative_reason": "explicit owner acceptance test", "observation_ref": "fixture:browser-mandate"})
			var task struct {
				ID       string `json:"task_id"`
				Revision int64  `json:"revision"`
			}
			_ = json.Unmarshal(created["task"], &task)
			control := ownerPOST(t, "/api/v3/tasks/"+task.ID+"/control", map[string]any{"kind": "amend", "expected_revision": task.Revision, "instruction": "Perform the bounded synthetic browser check"})
			inputID := text(control, "continuation_input_id")
			runID := "mandate-" + kind
			claim := ownerPOST(t, "/fixture/claim", map[string]string{"Task": task.ID, "Input": inputID, "Run": runID})
			if text(claim, "result_delivery") != "attempt" {
				t.Fatal("public goal Task silently changed delivery contract")
			}
			// fixture/claim is worker setup, not an owner decision; four protected
			// owner mutations above are counted separately from action approvals.
			if serial-setupStart != 5 {
				t.Fatal("unexpected setup accounting")
			}
			var posts atomic.Int32
			site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				if r.Method == "POST" {
					posts.Add(1)
					_ = r.ParseForm()
					if r.PostForm.Get("quantity") != "3" {
						t.Error("invalid submission")
					}
					fmt.Fprint(w, "saved")
					return
				}
				fmt.Fprint(w, `<form method=post action=/effect><input name=title required><input name=quantity type=number min=1 max=5 required><button id=submit>Save</button><button id=other>Other</button></form><script>document.addEventListener('input',()=>fetch('/effect',{method:'POST'}).catch(()=>{}))</script>`)
			}))
			defer site.Close()
			t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
			strict := kind != "unsafe_unknown"
			f := resourceBrokerWithPool(t, nil, false, pool, func(c *toolbrokerv1.ToolContract) {
				if c.ToolName == "browser.click" && strict {
					c.RequiresApproval = true
				}
			})
			client, ctx, stopBroker := browserSafetyBrokerWithContext(t, f, cfg.Address+"/private/context")
			call := func(id, name, args string) *toolbrokerv1.ToolCall {
				return &toolbrokerv1.ToolCall{RunId: runID, ToolCallId: kind + "-" + id, ToolName: name, AutonomyMode: 3, ArgsJson: args}
			}
			prepare := func(c *toolbrokerv1.ToolCall) *toolbrokerv1.PreparedAction {
				t.Helper()
				began := time.Now()
				defer func() {
					t.Logf("operation=prepare/%s duration_ms=%.3f model_calls=0", c.ToolName, float64(time.Since(began).Microseconds())/1000)
				}()
				p, e := client.PrepareToolCall(ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: c})
				if e != nil || p.GetError() != nil {
					t.Fatal("prepare denied", c.ToolName, p.GetError())
				}
				return p.Action
			}
			policyDenial := func(err error) bool {
				return status.Code(err) == codes.PermissionDenied || status.Code(err) == codes.FailedPrecondition || status.Code(err) == codes.InvalidArgument
			}
			authorized := func(g *toolbrokerv1.AuthorizeToolCallResponse) bool {
				return g != nil && g.GetError() == nil && g.AuthorizationId != ""
			}
			authorize := func(c *toolbrokerv1.ToolCall, p *toolbrokerv1.PreparedAction, approvalID string, allowDenial ...bool) *toolbrokerv1.AuthorizeToolCallResponse {
				t.Helper()
				began := time.Now()
				defer func() {
					t.Logf("operation=authorize/%s duration_ms=%.3f model_calls=0", c.ToolName, float64(time.Since(began).Microseconds())/1000)
				}()
				g, e := client.AuthorizeToolCall(ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: c, ActionId: p.ActionId, ApprovalId: approvalID})
				if e != nil {
					if len(allowDenial) > 0 && allowDenial[0] && policyDenial(e) {
						t.Logf("authorization_denied=%s", status.Code(e))
						return nil
					}
					t.Fatal(e)
				}
				return g
			}
			execute := func(c *toolbrokerv1.ToolCall, g string, allowDenial ...bool) *toolbrokerv1.ToolResult {
				t.Helper()
				began := time.Now()
				defer func() {
					t.Logf("operation=execute/%s duration_ms=%.3f model_calls=0", c.ToolName, float64(time.Since(began).Microseconds())/1000)
				}()
				r, e := client.ExecuteToolCall(ctx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: c, AuthorizationId: g})
				if e != nil {
					if len(allowDenial) > 0 && allowDenial[0] && policyDenial(e) {
						t.Logf("execution_denied=%s", status.Code(e))
						return nil
					}
					t.Fatal(e)
				}
				return r.Result
			}
			var view struct {
				ID      string `json:"browser_session_id"`
				URL     string `json:"final_url"`
				Version int64  `json:"observation_version"`
			}
			observe := func(r *toolbrokerv1.ToolResult) {
				t.Helper()
				if r.Status != "completed" || json.Unmarshal([]byte(r.ResultJson), &view) != nil || view.ID == "" {
					t.Fatal("browser observation failed", r.GetError())
				}
			}
			open := call("open", "browser.open", `{"url":"`+egresstest.Origin+`/form"}`)
			p := prepare(open)
			if p.RequiresApproval {
				t.Fatal("bounded read asked approval")
			}
			gnt := authorize(open, p, "")
			if !authorized(gnt) {
				t.Fatal("read mandate denied", gnt.GetError())
			}
			observe(execute(open, gnt.AuthorizationId))
			snapshot := call("scoped-snapshot", "browser.snapshot", fmt.Sprintf(`{"browser_session_id":%q}`, view.ID))
			sp := prepare(snapshot)
			var sb actionauth.ActionBinding
			if json.Unmarshal([]byte(sp.BindingJson), &sb) != nil || sb.BrowserScopeURL != view.URL || sp.RequiresApproval {
				t.Fatal("owned context URL not privately bound")
			}
			if kind == "scope_origin" || kind == "scope_epoch" {
				if kind == "scope_origin" {
					_, e = pool.Exec(ctx, `UPDATE server_browser_sessions SET observed_url='https://foreign.test/' WHERE run_id=$1`, runID)
				} else {
					_, e = pool.Exec(ctx, `UPDATE server_browser_sessions SET epoch=epoch+1 WHERE run_id=$1`, runID)
				}
				if e != nil || authorized(authorize(snapshot, sp, "", true)) || posts.Load() != 0 {
					t.Fatal("changed context scope/epoch retained permission", e)
				}
				t.Logf("case=%s context_read_authorization_denied=true recipient_posts=0", kind)
				return
			}
			sg := authorize(snapshot, sp, "")
			if !authorized(sg) {
				t.Fatal("scoped read denied")
			}
			observe(execute(snapshot, sg.AuthorizationId))
			if kind == "strict" {
				control := &browsercontrol.Store{Pool: pool}
				for _, foreign := range []string{"owner", "run", "task"} {
					bad := sb
					if foreign == "owner" {
						bad.OwnerID = "foreign"
					}
					if foreign == "run" {
						bad.RunID = "foreign"
					}
					if foreign == "task" {
						bad.TaskID = "foreign"
					}
					if _, _, err := control.BrowserReadScope(ctx, bad); err == nil {
						t.Fatal("foreign context metadata exposed")
					}
				}
				out := &toolbrokerv1.ToolResult{RunId: runID, ToolCallId: snapshot.ToolCallId, ToolName: snapshot.ToolName, Status: "completed", ResultJson: fmt.Sprintf(`{"ok":true,"browser_session_id":%q,"final_url":"https://foreign.test/","text":"out-of-scope-canary"}`, view.ID)}
				if control.Record(ctx, snapshot, actionauth.Grant{Binding: sb}, out) == nil || out.ResultJson != "{}" {
					t.Fatal("foreign observed URL released page bytes")
				}
				t.Log("context_read_foreign_owner_run_task_denied=true foreign_result_bytes_scrubbed=true")
			}
			for i, field := range []struct{ name, value string }{{"title", "Browser baseline"}, {"quantity", "8"}, {"quantity", "3"}} {
				c := call(fmt.Sprintf("prepare-%d", i), "browser.prepare_fill", fmt.Sprintf(`{"browser_session_id":%q,"expected_url":%q,"observation_version":%d,"selector":%q,"value":%q}`, view.ID, view.URL, view.Version, "input[name="+field.name+"]", field.value))
				p := prepare(c)
				if p.RequiresApproval {
					t.Fatal("bounded local preparation asked approval")
				}
				g := authorize(c, p, "")
				if !authorized(g) {
					t.Fatal("local preparation mandate denied", g.GetError())
				}
				grant, err := f.auth.GetGrant(ctx, g.AuthorizationId)
				if err != nil || grant.MandateID != mid || grant.ApprovalID != "" {
					t.Fatal("local grant did not use the selected owner mandate")
				}
				result := execute(c, g.AuthorizationId)
				if kind == "budget" && result.Status == "execution_stopped" && strings.Contains(result.GetError().GetDetailsJson(), "budget_egress_bytes_mandate") {
					if posts.Load() != 0 {
						t.Fatal("budget denial released POST")
					}
					t.Log("case=budget preparation_approvals=0 recipient_posts=0 mandate_budget_enforced=true")
					return
				}
				observe(result)
				if result.EffectState != "effect_confirmed" {
					t.Fatal("local preparation did not retain its verified local effect")
				}
				if posts.Load() != 0 {
					t.Fatal("preparation released POST")
				}
			}
			if kind == "budget" {
				t.Fatal("small mandate budget was not enforced")
			}
			var earlyApprovals int
			if pool.QueryRow(ctx, `SELECT count(*) FROM approvals WHERE run_id=$1`, runID).Scan(&earlyApprovals) != nil || earlyApprovals != 0 {
				t.Fatal("preparation silently created an approval")
			}
			click := call("submit", "browser.click", fmt.Sprintf(`{"browser_session_id":%q,"expected_url":%q,"observation_version":%d,"selector":"#submit"}`, view.ID, view.URL, view.Version))
			if kind == "prepare_only" {
				p, e := client.PrepareToolCall(ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: click})
				if !(policyDenial(e) || p.GetError() != nil) || posts.Load() != 0 {
					t.Fatal("read/prepare scope permitted submit")
				}
				t.Log("case=prepare_only setup_owner_mutations=4 preparation_approvals=0 submit_denied=true recipient_posts=0")
				return
			}
			p = prepare(click)
			gnt = authorize(click, p, "", strict)
			if kind == "unsafe_unknown" {
				if p.RequiresApproval || !authorized(gnt) {
					t.Fatal("unsafe control no longer demonstrates automatic unknown")
				}
				r := execute(click, gnt.AuthorizationId)
				if r.EffectState != "effect_unknown" || posts.Load() != 1 {
					t.Fatal("unsafe control did not submit")
				}
				t.Log("case=unsafe_unknown setup_owner_mutations=4 preparation_approvals=0 submit_approvals=0 recipient_posts=1 unsafe_configuration=true")
				return
			}
			if !p.RequiresApproval || authorized(gnt) || posts.Load() != 0 {
				t.Fatal("mandatory POST approval bypassed by mandate")
			}
			var binding actionauth.ActionBinding
			if json.Unmarshal([]byte(p.BindingJson), &binding) != nil {
				t.Fatal("binding decode")
			}
			ar := ownerPOST(t, "/fixture/approval", map[string]any{"Binding": binding, "Args": click.ArgsJson})
			aid := text(ar, "approval_id")
			ownerPOST(t, "/api/v2/approvals/"+aid+"/approve", map[string]any{})
			gnt = authorize(click, p, aid)
			if !authorized(gnt) {
				t.Fatal("exact approved submit denied", gnt.GetError())
			}
			if kind == "changed_args" || kind == "foreign_origin" {
				changed := proto.Clone(click).(*toolbrokerv1.ToolCall)
				if kind == "changed_args" {
					changed.ArgsJson = strings.Replace(changed.ArgsJson, "#submit", "#other", 1)
				} else {
					changed.ArgsJson = strings.Replace(changed.ArgsJson, egresstest.Origin, "http://foreign.test", 1)
				}
				click = changed
			}
			if kind == "revoke" {
				mandate["mandate_id"] = mid
				mandate["status"] = "revoked"
				ownerPOST(t, "/api/v3/owner-mandates", map[string]any{"mandate": mandate, "expected_revision": 1})
			}
			if kind == "cancel" {
				ownerPOST(t, "/api/v3/tasks/"+task.ID+"/control", map[string]any{"kind": "stop", "expected_revision": 2})
			}
			if kind == "expiry" {
				time.Sleep(time.Until(expires) + 50*time.Millisecond)
			}
			if kind == "stale_contract" {
				stopBroker()
				client, ctx, _ = browserSafetyBrokerWithContext(t, f, cfg.Address+"/private/context", func(c *toolbrokerv1.ToolContract) {
					if c.ToolName == "browser.click" {
						c.RequiresApproval = false
					}
				})
				loaded, err := client.GetToolContract(ctx, &toolbrokerv1.GetToolContractRequest{ToolName: "browser.click"})
				if err != nil || loaded.GetContract().GetRequiresApproval() {
					t.Fatal("changed registry was not actually loaded")
				}
				h, err := actionauth.ContractHash(loaded.GetContract())
				if err != nil || h == binding.ContractHash {
					t.Fatal("contract hash did not change")
				}
				if authorized(authorize(click, p, aid, true)) {
					t.Fatal("old contract approval reused after registry change")
				}
			}
			r := execute(click, gnt.AuthorizationId, kind != "strict" && kind != "restart")
			if kind == "strict" || kind == "restart" {
				if r.EffectState != "effect_unknown" || posts.Load() != 1 {
					t.Fatal("exact approved POST did not remain unknown")
				}
				if kind == "restart" {
					stopBroker()
					client, ctx, _ = browserSafetyBrokerWithContext(t, f, cfg.Address+"/private/context")
				}
				_ = execute(click, gnt.AuthorizationId, true)
				if posts.Load() != 1 {
					t.Fatal("spent POST grant replayed")
				}
				var approvalsCount, dispatches int
				var state string
				if pool.QueryRow(ctx, `SELECT count(*) FROM approvals WHERE run_id=$1`, runID).Scan(&approvalsCount) != nil || approvalsCount != 1 || pool.QueryRow(ctx, `SELECT state FROM action_effects WHERE action_id=$1`, p.EffectId).Scan(&state) != nil || state != "effect_unknown" || pool.QueryRow(ctx, `SELECT count(*) FROM action_effect_dispatches WHERE authorization_id=$1`, gnt.AuthorizationId).Scan(&dispatches) != nil || dispatches != 1 {
					t.Fatal("approval, unknown journal or consumed dispatch changed")
				}
				t.Logf("case=%s setup_owner_mutations=4 preparation_approvals=0 submit_approvals=1 recipient_posts=1 effect_unknown=true replay_posts=0", kind)
				return
			}
			var dispatches int
			if (r != nil && r.Status != "failed" && r.Status != "execution_stopped") || posts.Load() != 0 || pool.QueryRow(ctx, `SELECT count(*) FROM action_effect_dispatches WHERE authorization_id=$1`, gnt.AuthorizationId).Scan(&dispatches) != nil || dispatches != 0 {
				t.Fatal("changed/stale authority dispatched POST", kind)
			}
			t.Logf("case=%s preparation_approvals=0 submit_approvals=1 recipient_posts=0 original_dispatches=0", kind)
		})
	}
	r, _ := http.Post(cfg.Address+"/fixture/done", "application/json", nil)
	if r != nil {
		_, _ = io.Copy(io.Discard, r.Body)
		r.Body.Close()
	}
}
