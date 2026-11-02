package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browsercontrol"
	"github.com/butler/butler/internal/browserpage"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type pageControlTrace struct {
	*browsercontrol.Store
	status  string
	reports int
	failed  bool
}

func (c *pageControlTrace) Record(ctx context.Context, call *toolbrokerv1.ToolCall, g actionauth.Grant, r *toolbrokerv1.ToolResult) error {
	c.status = r.GetStatus()
	var raw struct {
		Pages []browserpage.Report `json:"private_pages"`
	}
	_ = json.Unmarshal([]byte(r.GetResultJson()), &raw)
	c.reports = len(raw.Pages)
	e := c.Store.Record(ctx, call, g, r)
	c.failed = e != nil
	return e
}

// Real Broker, permanent grant/budget/resource claims, PG page ledger, isolated
// runtime process/Chromium and private owner channel. Optional Web uses the
// production Core owner HTTP handler, not a mocked frame or command response.
func TestManagedPagesBrokerChromiumIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_PAGES_BROKER_E2E") != "1" {
		t.Skip("requires isolated PostgreSQL and actual Chromium")
	}
	began := time.Now()
	t.Cleanup(func() {
		t.Logf("scope=broker_pages wall_ms=%.3f model_calls=0", float64(time.Since(began).Microseconds())/1000)
	})
	var children, posts, lastChild atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/managed":
			fmt.Fprint(w, `<html><title>Twin</title><body><p>root-marker</p><button style="position:absolute;left:20px;top:80px;width:200px;height:40px" onclick="window.open('/managed-child','_blank')">Open popup</button><button id=post onclick="fetch('/managed-effect',{method:'POST',body:'root'})">Root POST</button></body></html>`)
		case "/managed-child":
			n := children.Add(1)
			fmt.Fprintf(w, `<html><title>Twin</title><body><p>child-marker-%d</p><input type=password style="position:absolute;left:20px;top:100px;width:200px;height:40px" oninput="document.querySelector('#echo').textContent=this.value"><span id=echo></span><button id=post style="position:absolute;left:20px;top:180px;width:200px;height:40px" onclick="fetch('/managed-effect',{method:'POST',body:'%d'})">Submit</button></body></html>`, n, n)
		case "/managed-effect":
			body, _ := io.ReadAll(io.LimitReader(r.Body, 16))
			if string(body) == "2" {
				lastChild.Store(2)
			} else {
				lastChild.Store(1)
			}
			posts.Add(1)
			fmt.Fprint(w, "ok")
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()
	t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
	t.Setenv("BUTLER_RESOURCE_MANAGED_PAGES", "1")
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ownerAddress := listener.Addr().String()
	listener.Close()
	t.Setenv("BUTLER_BROWSER_OWNER_HARNESS_ADDR", ownerAddress)
	f := resourceBroker(t, nil, false)
	for _, name := range []string{"086_server_browser_control.up.sql", "096_credential_vault.up.sql", "105_browser_owner_outcomes.up.sql", "107_browser_pages.up.sql", "108_browser_page_execution.up.sql"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.pool.Exec(t.Context(), string(raw)); e != nil {
			t.Fatal("page schema", name, e)
		}
	}
	if _, e = f.pool.Exec(t.Context(), `CREATE TABLE tool_calls(tool_call_id TEXT PRIMARY KEY,run_id TEXT,tool_name TEXT,args JSONB,result JSONB,status TEXT,finished_at TIMESTAMPTZ)`); e != nil {
		t.Fatal(e)
	}
	runtimeControl, e := browsercontrol.NewClient("http://"+ownerAddress+browsercontrol.Path, strings.Repeat("browser-fixture-", 3))
	if e != nil {
		t.Fatal(e)
	}
	control := &browsercontrol.Store{Pool: f.pool, Contexts: f.reader, Runtime: runtimeControl}
	authorizer := actionauth.NewService(f.auth, f.reader, nil)
	authorizer.SetBrowserScopes(control)
	f.server.SetActionAuthorizer(authorizer)
	trace := &pageControlTrace{Store: control}
	f.server.SetBrowserControl(trace)
	privateHTTP := httptest.NewServer(browsercontrol.Handler(strings.Repeat("core-private-", 3), control))
	defer privateHTTP.Close()
	owner, e := browsercontrol.NewClient(privateHTTP.URL+browsercontrol.Path, strings.Repeat("core-private-", 3))
	if e != nil {
		t.Fatal(e)
	}
	serial := 0
	prepare := func(tool string, a browserpage.Args) *toolbrokerv1.ExecuteToolCallRequest {
		t.Helper()
		serial++
		raw, _ := json.Marshal(a)
		return f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: fmt.Sprintf("managed-%d", serial), ToolName: tool, ArgsJson: string(raw), AutonomyMode: 3})
	}
	type pageOutput struct {
		ID    string             `json:"browser_session_id"`
		Page  *browserpage.Ref   `json:"page"`
		Pages []browserpage.View `json:"pages"`
		Text  string             `json:"text"`
	}
	invoke := func(tool string, a browserpage.Args) pageOutput {
		t.Helper()
		req := prepare(tool, a)
		response, e := f.server.ExecuteToolCall(t.Context(), req)
		if e != nil || response.GetResult().GetStatus() != "completed" {
			var roots int
			_ = f.pool.QueryRow(t.Context(), `SELECT count(*) FROM server_browser_sessions`).Scan(&roots)
			t.Logf("record_status=%s private_reports=%d record_failed=%t root_rows=%d", trace.status, trace.reports, trace.failed, roots)
			t.Fatal("managed execution failed", tool, e, response.GetResult().GetError())
		}
		raw := response.Result.ResultJson
		if strings.Contains(raw, "private_pages") || strings.Contains(raw, "runtime_version") || strings.Contains(raw, "jpeg") || strings.Contains(raw, "owner-web-private-canary") {
			t.Fatal("private bridge material exported")
		}
		var out pageOutput
		if json.Unmarshal([]byte(raw), &out) != nil {
			t.Fatal("invalid public page output")
		}
		return out
	}
	opened := invoke(browserpage.Open, browserpage.Args{URL: egresstest.Origin + "/managed"})
	if len(opened.Pages) != 1 || opened.Page == nil {
		t.Fatal("root binding missing")
	}
	id := opened.ID
	root := *opened.Page
	oldGrant := prepare(browserpage.Snapshot, browserpage.Args{SessionID: id, Page: &root})
	taken, e := owner.Control(t.Context(), browsercontrol.Command{Operation: "take", SessionID: id, OwnerID: "owner", Controller: strings.Repeat("a", 32), Epoch: 0, PageID: root.ID})
	if e != nil || taken.Frame == nil {
		t.Fatal("real owner takeover failed")
	}
	_, e = owner.Control(t.Context(), browsercontrol.Command{Operation: "click", SessionID: id, OwnerID: "owner", Controller: strings.Repeat("a", 32), Epoch: 1, PageID: root.ID, DocumentID: taken.Frame.DocumentID, FrameID: taken.Frame.ID, X: 40, Y: 100})
	if e != nil {
		t.Fatal("owner popup gesture failed", e)
	}
	var pageFrame browsercontrol.Reply
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pageFrame, e = owner.Control(t.Context(), browsercontrol.Command{Operation: "frame", SessionID: id, OwnerID: "owner", PageID: root.ID})
		if e == nil && len(pageFrame.Pages) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(pageFrame.Pages) != 2 {
		t.Fatal("owner did not see actual second page")
	}
	if _, e = owner.Control(t.Context(), browsercontrol.Command{Operation: "release", SessionID: id, OwnerID: "owner", Controller: strings.Repeat("a", 32), Epoch: 1}); e != nil {
		t.Fatal("owner return failed")
	}
	denied, e := f.server.ExecuteToolCall(t.Context(), oldGrant)
	if e != nil || denied.GetResult().GetStatus() == "completed" {
		t.Fatal("old page grant survived owner return")
	}
	listed := invoke(browserpage.List, browserpage.Args{SessionID: id})
	if len(listed.Pages) != 2 {
		t.Fatal("fresh page listing missing")
	}
	var firstChild browserpage.Ref
	for _, p := range listed.Pages {
		if p.ID != root.ID {
			firstChild = p.Ref
		} else {
			root = p.Ref
		}
	}
	rootRead := invoke(browserpage.Snapshot, browserpage.Args{SessionID: id, Page: &root})
	if !strings.Contains(rootRead.Text, "root-marker") {
		t.Fatal("durable/runtime version translation selected wrong root")
	}
	root = *rootRead.Page
	read := invoke(browserpage.Snapshot, browserpage.Args{SessionID: id, Page: &firstChild})
	if !strings.Contains(read.Text, "child-marker-1") {
		t.Fatal("wrong page read")
	}
	firstChild = *read.Page
	// Explicit page identity remains independent from the selected owner page.
	bad := firstChild
	bad.DocumentID = root.DocumentID
	raw, _ := json.Marshal(browserpage.Args{SessionID: id, Page: &bad})
	p, e := f.server.PrepareToolCall(t.Context(), &toolbrokerv1.PrepareToolCallRequest{ToolCall: &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "managed-swapped", ToolName: browserpage.Snapshot, ArgsJson: string(raw), AutonomyMode: 3}})
	if status.Code(e) != codes.PermissionDenied || p.GetAction() != nil {
		t.Fatal("swapped document prepared")
	}
	var webChild browserpage.Ref
	if address := os.Getenv("BUTLER_MANAGED_PAGES_UI_ADDR"); address != "" {
		bounded, cancel := context.WithTimeout(t.Context(), 100*time.Second)
		defer cancel()
		process := exec.CommandContext(bounded, os.Getenv("BUTLER_SERVER_BROWSER_API_BINARY"), "-test.run=^TestServerBrowserOwnerAPIHarness$")
		process.Env = append(os.Environ(), "BUTLER_SERVER_BROWSER_API_HARNESS=1", "BUTLER_SERVER_BROWSER_BROKER_ENDPOINT="+privateHTTP.URL+browsercontrol.Path, "BUTLER_SERVER_BROWSER_OWNER_API_ADDR="+address)
		process.Stdout, process.Stderr = io.Discard, io.Discard
		if process.Start() != nil {
			t.Fatal("production owner API unavailable")
		}
		defer func() { cancel(); _ = process.Wait() }()
		if os.WriteFile(os.Getenv("BUTLER_MANAGED_PAGES_UI_READY"), []byte("ready"), 0600) != nil {
			t.Fatal("UI readiness unavailable")
		}
		for {
			if _, e := os.Stat(os.Getenv("BUTLER_MANAGED_PAGES_UI_DONE")); e == nil {
				break
			}
			if path := os.Getenv("BUTLER_MANAGED_PAGES_UI_ABORT"); path != "" {
				if _, e := os.Stat(path); e == nil {
					t.Fatal("actual owner UI failed; preserving attempt")
				}
			}
			select {
			case <-bounded.Done():
				t.Fatal("actual owner UI did not complete")
			case <-time.After(100 * time.Millisecond):
			}
		}
		if posts.Load() != 1 || lastChild.Load() != 2 {
			t.Fatal("owner UI selected/submitted the wrong identical page")
		}
		listed = invoke(browserpage.List, browserpage.Args{SessionID: id})
		if len(listed.Pages) != 3 {
			t.Fatal("second identical child missing")
		}
		for _, page := range listed.Pages {
			if page.ID != root.ID && page.ID != firstChild.ID {
				webChild = page.Ref
			}
			if page.ID == firstChild.ID {
				firstChild = page.Ref
			}
		}
		seen := invoke(browserpage.Snapshot, browserpage.Args{SessionID: id, Page: &webChild})
		if !strings.Contains(seen.Text, "child-marker-2") {
			t.Fatal("agent read wrong same-URL page after owner selection")
		}
		webChild = *seen.Page
	} else {
		webChild = firstChild
	}
	// A new constraints revision must deny the old grant without dispatching.
	stale := prepare(browserpage.Snapshot, browserpage.Args{SessionID: id, Page: &webChild})
	if _, e = f.pool.Exec(t.Context(), `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id='task'`); e != nil {
		t.Fatal(e)
	}
	response, e := f.server.ExecuteToolCall(t.Context(), stale)
	if e != nil || response.GetResult().GetStatus() == "completed" {
		t.Fatal("old constraints grant executed")
	}
	latest := invoke(browserpage.Snapshot, browserpage.Args{SessionID: id, Page: &webChild})
	webChild = *latest.Page
	before := posts.Load()
	click := prepare(browserpage.Click, browserpage.Args{SessionID: id, Page: &webChild, Selector: "#post"})
	result, e := f.server.ExecuteToolCall(t.Context(), click)
	if e != nil || result.GetResult().GetStatus() != "effect_unknown" || result.GetResult().GetEffectState() != "effect_unknown" || posts.Load() != before+1 {
		t.Fatal("approved page POST/journal result wrong", e, result.GetResult().GetEffectState())
	}
	// Preserve genuine Core-style transcript entries alongside actual runtime
	// effect evidence. The probe ref comes only from this recorded POST result.
	record := func(request *toolbrokerv1.ExecuteToolCallRequest, response *toolbrokerv1.ExecuteToolCallResponse) {
		t.Helper()
		c, r := request.ToolCall, response.Result
		if _, err := f.pool.Exec(t.Context(), `INSERT INTO tool_calls VALUES($1,$2,$3,$4,$5,$6,clock_timestamp())`, c.ToolCallId, c.RunId, c.ToolName, c.ArgsJson, r.ResultJson, r.Status); err != nil {
			t.Fatal(err)
		}
	}
	record(click, result)
	var post pageOutput
	if json.Unmarshal([]byte(result.Result.ResultJson), &post) != nil || post.Page == nil {
		t.Fatal("missing canonical post-effect ref")
	}
	for _, page := range post.Pages {
		if page.ID == firstChild.ID {
			firstChild = page.Ref
		}
		if page.ID == root.ID {
			root = page.Ref
		}
	}
	otherPage := firstChild
	if otherPage.ID == webChild.ID {
		otherPage = root
	}
	probe := func(target browserpage.Ref) *toolbrokerv1.ExecuteToolCallRequest {
		t.Helper()
		request := prepare(browserpage.Freeze, browserpage.Args{SessionID: id, Page: &target})
		response, err := f.server.ExecuteToolCall(t.Context(), request)
		if err != nil || response.GetResult().GetStatus() != "completed" {
			t.Fatal("actual frozen probe unavailable", err)
		}
		record(request, response)
		var body browserpage.Observation
		if json.Unmarshal([]byte(response.Result.ResultJson), &body) != nil || body.Page == nil || body.Network != "frozen" {
			t.Fatal("actual freeze proof missing")
		}
		if target.ID == otherPage.ID {
			otherPage = *body.Page
		}
		return request
	}
	wrong := probe(otherPage)
	current, err := f.reader.GetActionContext(t.Context(), "run")
	if err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(t.Context(), ownership.NewAuthority(f.pool), ownership.Token{SessionKey: current.SessionKey, RunID: "run", LeaseID: current.LeaseID, FenceEpoch: current.LeaseEpoch})
	reviews := &actionauth.OwnerOutcomes{Pool: f.pool}
	tx, err := f.pool.Begin(owned)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reviews.RequestTx(owned, tx, result.Result.ActionId, wrong.ToolCall.ToolCallId, false); err == nil {
		tx.Rollback(owned)
		t.Fatal("another genuine frozen page satisfied POST review")
	}
	tx.Rollback(owned)
	right := probe(*post.Page)
	tx, err = f.pool.Begin(owned)
	if err != nil {
		t.Fatal(err)
	}
	review, err := reviews.RequestTx(owned, tx, result.Result.ActionId, right.ToolCall.ToolCallId, false)
	if err != nil {
		tx.Rollback(owned)
		t.Fatal("exact actual page review denied", err)
	}
	if err = tx.Commit(owned); err != nil {
		t.Fatal(err)
	}
	// Another page cannot escape UNKNOWN while owner review is pending.
	other := prepare(browserpage.Click, browserpage.Args{SessionID: id, Page: &otherPage, Selector: "#post"})
	blocked, e := f.server.ExecuteToolCall(t.Context(), other)
	if e != nil || blocked.GetResult().GetStatus() != "execution_stopped" || posts.Load() != before+1 {
		t.Fatal("another page bypassed retained UNKNOWN")
	}
	if _, err = f.pool.Exec(t.Context(), `UPDATE session_ownership SET lease_id='',expires_at=clock_timestamp()-interval '1 second' WHERE run_id='run';UPDATE tasks SET status='waiting_for_user' WHERE task_id='task';UPDATE runs SET current_state='awaiting_approval' WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	actor := owneridentity.WithActor(t.Context(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "actual-page-report"})
	if _, err = reviews.Decide(actor, actionauth.OwnerOutcomeDecision{ID: review.ID, Fingerprint: review.Fingerprint, Revision: 1, Outcome: "effect_confirmed", RecipientChecked: true, ProcessingSettled: true, ReceiptReference: "synthetic-recipient-count-checked"}); err != nil {
		t.Fatal("canonical owner page report denied", err)
	}
	var effectState, originalStatus string
	if f.pool.QueryRow(t.Context(), `SELECT e.state,c.status FROM action_effects e JOIN tool_calls c ON c.tool_call_id=$2 WHERE e.action_id=$1`, result.Result.ActionId, click.ToolCall.ToolCallId).Scan(&effectState, &originalStatus) != nil || effectState != "effect_confirmed" || originalStatus != "effect_unknown" || posts.Load() != before+1 {
		t.Fatal("report replayed POST or rewrote history")
	}
	var resources int
	if f.pool.QueryRow(t.Context(), `SELECT count(DISTINCT resource_id) FROM execution_browser_resources`).Scan(&resources) != nil || resources != 1 {
		t.Fatal("page gained another resource lane")
	}
}
