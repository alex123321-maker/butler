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

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browsercontrol"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type loseBrowserRelease struct {
	browsercontrol.Controller
	lose atomic.Bool
}

func (c *loseBrowserRelease) Control(ctx context.Context, command browsercontrol.Command) (browsercontrol.Reply, error) {
	reply, err := c.Controller.Control(ctx, command)
	if command.Operation == "release" && err == nil && c.lose.Swap(false) {
		return browsercontrol.Reply{}, browsercontrol.ErrDenied
	}
	return reply, err
}

func serverBrowserProcess(t *testing.T, site string) (string, string) {
	t.Helper()
	addresses := []string{}
	for range 2 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addresses = append(addresses, listener.Addr().String())
		listener.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	cmd := exec.CommandContext(ctx, os.Getenv("BUTLER_BROWSER_E2E_BINARY"), "-test.run=^TestBrowserRuntimeEgressHarness$")
	cmd.Env = append(os.Environ(), "BUTLER_BROWSER_EGRESS_HARNESS=1", "BUTLER_BROWSER_FIXTURE_ENDPOINT="+site, "BUTLER_GRPC_ADDR="+addresses[0], "BUTLER_BROWSER_OWNER_HARNESS_ADDR="+addresses[1])
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	for _, addr := range addresses {
		deadline := time.Now().Add(10 * time.Second)
		for {
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				conn.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("browser subprocess unavailable")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return addresses[0], "http://" + addresses[1] + browsercontrol.Path
}

func TestServerBrowserBrokerIntegration(t *testing.T) {
	if os.Getenv("BUTLER_SERVER_BROWSER_E2E") != "1" {
		t.Skip("isolated PG and real Chromium subprocess required")
	}
	if os.Getenv("BUTLER_TEST_ACTIONAUTH_POSTGRES_URL") == "" || os.Getenv("BUTLER_BROWSER_E2E_BINARY") == "" {
		t.Fatal("mandatory isolated dependencies missing")
	}
	pool := resourceBrokerDB(t)
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "086_server_browser_control.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	var posts, cdn, private, queryReads atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			fmt.Fprint(w, `<body><form method="get" action="/query"><input name="q"><button>Search</button></form></body>`)
		case "/query":
			queryReads.Add(1)
			fmt.Fprintf(w, `<body>Query: %s<script>fetch('/submit',{method:'POST'}).catch(()=>{})</script></body>`, r.URL.Query().Get("q"))
		case "/start":
			http.Redirect(w, r, "http://login.test/form", 302)
		case "/form":
			fmt.Fprint(w, `<html><body><h1>Owner fixture</h1><input type="password" style="position:absolute;left:20px;top:100px;width:200px;height:40px"><button style="position:absolute;left:20px;top:180px;width:200px;height:40px" onclick="fetch('/submit',{method:'POST'})">Submit</button><script src="http://cdn.test/cdn.js"></script></body></html>`)
		case "/cdn.js":
			cdn.Add(1)
			w.Header().Set("Content-Type", "application/javascript")
			fmt.Fprint(w, `document.body.dataset.cdn='loaded'`)
		case "/submit":
			posts.Add(1)
			fmt.Fprint(w, "ok")
		case "/reflect":
			fmt.Fprint(w, "<body>private-owner-input</body>")
		case "/private":
			private.Add(1)
		default:
			fmt.Fprint(w, "<body>Another public origin</body>")
		}
	}))
	defer site.Close()
	target, endpoint := serverBrowserProcess(t, site.URL)
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "configs", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Tools []map[string]any `json:"tools"`
	}
	if json.Unmarshal(data, &catalog) != nil {
		t.Fatal("catalog invalid")
	}
	for _, tool := range catalog.Tools {
		if tool["tool_class"] == "browser" {
			tool["runtime_target"] = target
		}
	}
	data, _ = json.Marshal(catalog)
	path := filepath.Join(t.TempDir(), "tools.json")
	if os.WriteFile(path, data, 0600) != nil {
		t.Fatal("catalog write")
	}
	reg, err := registry.Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range reg.List("", false) {
		if strings.HasPrefix(tool.GetToolName(), "single_tab.") {
			t.Fatal("single_tab in standard tools")
		}
	}
	router := runtimeclient.New(nil)
	defer router.Close()
	server := NewServer(reg, router, nil, nil)
	budgets := budget.NewController(pool, budget.DefaultPolicy())
	journal := actionauth.NewPostgresStore(pool)
	journal.SetBudgets(budgets)
	reader := resourceReader{authFixture: newAuthFixture(), pool: pool}
	server.SetScopedCatalog(&TaskCatalog{Registry: reg, Contexts: reader})
	server.SetBudgets(budgets)
	server.SetActionAuthorizer(actionauth.NewService(journal, reader, nil))
	runtimeControl, err := browsercontrol.NewClient(endpoint, strings.Repeat("browser-fixture-", 3))
	if err != nil {
		t.Fatal(err)
	}
	flakyRuntime := &loseBrowserRelease{Controller: runtimeControl}
	control := &browsercontrol.Store{Pool: pool, Contexts: reader, Runtime: flakyRuntime}
	server.SetBrowserControl(control)
	serviceToken := strings.Repeat("core-private-", 3)
	privateHTTP := httptest.NewServer(browsercontrol.Handler(serviceToken, control))
	defer privateHTTP.Close()
	ownerClient, err := browsercontrol.NewClient(privateHTTP.URL+browsercontrol.Path, serviceToken)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rpc := grpc.NewServer(grpc.UnaryInterceptor(actionauth.ExecutionUnaryInterceptor(serviceToken)))
	toolbrokerv1.RegisterToolBrokerServiceServer(rpc, server)
	go func() { _ = rpc.Serve(listener) }()
	defer rpc.Stop()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := toolbrokerv1.NewToolBrokerServiceClient(conn)
	callCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+serviceToken)
	currentTools, catalogErr := client.ListTools(callCtx, &toolbrokerv1.ListToolsRequest{RunId: "run"})
	if catalogErr != nil {
		t.Fatal("current discovery catalog", catalogErr)
	}
	foundOpen := false
	for _, contract := range currentTools.GetTools() {
		if strings.HasPrefix(contract.ToolName, "single_tab.") {
			t.Fatal("legacy tab leaked into current run catalog")
		}
		if contract.ToolName == "browser.open" {
			foundOpen = contract.ContractRevision == "3"
		}
	}
	if !foundOpen {
		t.Fatal("current server browser unavailable to discovery")
	}
	sequence := 0
	prepare := func(tool string, args map[string]any) (*toolbrokerv1.ToolCall, string) {
		t.Helper()
		sequence++
		encoded, _ := json.Marshal(args)
		call := &toolbrokerv1.ToolCall{ToolName: tool, RunId: "run", ToolCallId: fmt.Sprintf("server-browser-%d", sequence), ArgsJson: string(encoded), AutonomyMode: 3}
		p, e := client.PrepareToolCall(callCtx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
		if e != nil || p.GetError() != nil {
			t.Fatal("prepare", e, p.GetError())
		}
		intent, e := journal.GetIntent(ctx, p.Action.ActionId)
		if e != nil {
			t.Fatal(e)
		}
		id := "approval-" + call.ToolCallId
		reader.mu.Lock()
		reader.approvals[id] = actionauth.ApprovalSnapshot{ApprovalID: id, Status: "approved", Binding: intent.Binding, ResolvedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
		reader.mu.Unlock()
		g, e := client.AuthorizeToolCall(callCtx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: intent.Binding.ActionID, ApprovalId: id})
		if e != nil || g.GetError() != nil {
			t.Fatal("authorize", e, g.GetError())
		}
		return call, g.AuthorizationId
	}
	invoke := func(tool string, args map[string]any) map[string]any {
		t.Helper()
		call, g := prepare(tool, args)
		r, e := client.ExecuteToolCall(callCtx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: g})
		if e != nil || r.GetResult().GetStatus() != "completed" {
			t.Fatal("execute", tool, e, r.GetResult())
		}
		var output map[string]any
		if json.Unmarshal([]byte(r.Result.ResultJson), &output) != nil {
			t.Fatal("output")
		}
		if strings.Contains(r.Result.ResultJson, "private-owner-input") || strings.Contains(r.Result.ResultJson, "jpeg") {
			t.Fatal("owner data leaked to model")
		}
		return output
	}
	getObs := invoke("browser.open", map[string]any{"url": "http://start.test/search"})
	getObs = invoke("browser.submit_get_form", map[string]any{"browser_session_id": getObs["browser_session_id"], "observation_version": getObs["observation_version"], "expected_url": getObs["final_url"], "selector": "form", "fields": map[string]string{"q": "Arctic delivery"}})
	if queryReads.Load() != 1 || posts.Load() != 0 || !strings.Contains(fmt.Sprint(getObs["text"]), "Arctic delivery") {
		t.Fatal("Broker GET form wrote or failed to observe")
	}
	invoke("browser.close", map[string]any{"browser_session_id": getObs["browser_session_id"]})
	obs := invoke("browser.open", map[string]any{"url": "http://start.test/start"})
	if obs["final_url"] != "http://login.test/form" || cdn.Load() != 1 {
		t.Fatal("public redirect/CDN unavailable")
	}
	listed, e := ownerClient.Control(ctx, browsercontrol.Command{Operation: "list", OwnerID: "owner"})
	if e != nil || len(listed.Sessions) != 1 {
		t.Fatal("owner mapping missing", e)
	}
	id := listed.Sessions[0].ID
	oldSnapshot := &toolbrokerv1.ToolCall{ToolName: "browser.snapshot", RunId: "run", ToolCallId: "old-snapshot", ArgsJson: fmt.Sprintf(`{"browser_session_id":%q}`, id)}
	oldAdmission, e := control.Before(ctx, oldSnapshot)
	if e != nil {
		t.Fatal("pin initial dispatch epoch", e)
	}
	if _, e = ownerClient.Control(ctx, browsercontrol.Command{Operation: "frame", SessionID: id, OwnerID: "foreign"}); e == nil {
		t.Fatal("foreign owner viewed pixels")
	}
	command := browsercontrol.Command{Operation: "take", OwnerID: "owner", SessionID: id, Controller: strings.Repeat("a", 32), Epoch: 0}
	reply, e := ownerClient.Control(ctx, command)
	if e != nil || reply.Frame == nil || reply.Frame.Epoch != 1 {
		t.Fatal("take failed", e)
	}
	command.Epoch = 1
	replayTake := command
	replayTake.Epoch = 0
	if _, e = ownerClient.Control(ctx, replayTake); e == nil {
		t.Fatal("take replay accepted")
	}
	paused, cancel := context.WithTimeout(ctx, 70*time.Millisecond)
	defer cancel()
	if control.WaitRun(paused, "run") == nil {
		t.Fatal("kernel resumed during owner control")
	}
	staleArgs := map[string]any{"browser_session_id": id, "observation_version": obs["observation_version"], "expected_url": obs["final_url"], "selector": "button"}
	staleCall, staleGrant := prepare("browser.click", staleArgs)
	blocked, cancelBlocked := context.WithTimeout(callCtx, 100*time.Millisecond)
	defer cancelBlocked()
	_, _ = client.ExecuteToolCall(blocked, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: staleCall, AuthorizationId: staleGrant})
	g, grantErr := journal.GetGrant(ctx, staleGrant)
	if grantErr != nil || g.State != "authorized" {
		t.Fatal("blocked action spent", grantErr, g.State)
	}

	input := func(op string, edit func(*browsercontrol.Command)) {
		t.Helper()
		c := command
		c.Operation = op
		c.FrameID = reply.Frame.ID
		if edit != nil {
			edit(&c)
		}
		r, e := ownerClient.Control(ctx, c)
		if e != nil {
			t.Fatal("manual", op, e)
		}
		if r.Frame != nil {
			reply = r
		}
	}
	oldFrame := reply.Frame.ID
	input("click", func(c *browsercontrol.Command) { c.X = 40; c.Y = 120 })
	stale := command
	stale.Operation = "text"
	stale.FrameID = oldFrame
	stale.Text = "must-not-run"
	if _, e = ownerClient.Control(ctx, stale); e == nil {
		t.Fatal("stale pixels accepted")
	}
	input("text", func(c *browsercontrol.Command) { c.Text = "private-owner-input" })
	input("click", func(c *browsercontrol.Command) { c.X = 50; c.Y = 200 })
	deadline := time.Now().Add(time.Second)
	for posts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if posts.Load() != 1 {
		t.Fatal("manual input did not reach actual Chromium/recipient once", posts.Load())
	}
	input("navigate", func(c *browsercontrol.Command) { c.URL = "http://third.test/reflect" })
	command.Operation = "release"
	if _, e = ownerClient.Control(ctx, command); e != nil {
		t.Fatal("release", e)
	}
	if e = control.WaitRun(ctx, "run"); e != nil {
		t.Fatal(e)
	}
	// A pre-takeover RPC delayed in transport cannot execute after release,
	// even for a read without observation_version. This uses the actual gRPC
	// runtime, with precisely the Broker admission captured before takeover.
	var delayed *toolbrokerv1.ToolResult
	var delayedErr error
	for _, contract := range reg.List("", false) {
		if contract.ToolName == "browser.snapshot" {
			delayed, delayedErr = router.Execute(oldAdmission, oldSnapshot, contract, nil)
		}
	}
	if delayedErr != nil || delayed == nil || delayed.Status == "completed" || !strings.Contains(delayed.GetError().GetDetailsJson(), "stale_control_epoch") {
		t.Fatal("old admission crossed control generation", delayedErr, delayed)
	}
	// A snapshot completed before takeover but recorded after release must
	// not clear the new generation's fresh-observation requirement.
	oldResult := &toolbrokerv1.ToolResult{RunId: "run", ToolCallId: "old-snapshot", ToolName: "browser.snapshot", Status: "completed", ResultJson: `{"ok":true}`}
	if err := control.Record(oldAdmission, oldSnapshot, actionauth.Grant{}, oldResult); err != nil {
		t.Fatal(err)
	}
	var fresh bool
	if err := pool.QueryRow(ctx, `SELECT fresh_needed FROM server_browser_sessions WHERE browser_session_id=$1`, id).Scan(&fresh); err != nil || !fresh {
		t.Fatal("late old snapshot cleared the new fence", err)
	}
	r, e := client.ExecuteToolCall(callCtx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: staleCall, AuthorizationId: staleGrant})
	if e != nil || r.GetResult().GetStatus() != "failed" || r.GetResult().GetActionId() != "" {
		t.Fatal("stale proposal dispatched before snapshot")
	}
	obs = invoke("browser.snapshot", map[string]any{"browser_session_id": id})
	if !strings.Contains(fmt.Sprint(obs["text"]), "[REDACTED]") {
		t.Fatal("manual input masking missing")
	}
	obs = invoke("browser.navigate", map[string]any{"browser_session_id": id, "url": "http://another.test/page", "observation_version": obs["observation_version"], "expected_url": obs["final_url"]})
	if obs["final_url"] != "http://another.test/page" {
		t.Fatal("agent continuation after release failed")
	}
	// A lost release ACK leaves the durable fence closed even though Chromium
	// acknowledged locally. Recovery requires a fresh explicit owner takeover.
	command.Operation = "take"
	command.Epoch = 2
	if _, e = ownerClient.Control(ctx, command); e != nil {
		t.Fatal("second take", e)
	}
	command.Epoch = 3
	command.Operation = "release"
	flakyRuntime.lose.Store(true)
	if _, e = ownerClient.Control(ctx, command); e == nil {
		t.Fatal("lost release reported success")
	}
	waitLost, cancelLost := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancelLost()
	if control.WaitRun(waitLost, "run") == nil {
		t.Fatal("lost ACK resumed agent")
	}
	command.Operation = "take"
	command.Controller = strings.Repeat("b", 32)
	if _, e = ownerClient.Control(ctx, command); e != nil {
		t.Fatal("explicit recovery take", e)
	}
	command.Epoch = 4
	command.Operation = "release"
	if _, e = ownerClient.Control(ctx, command); e != nil {
		t.Fatal("explicit recovery release", e)
	}
	expectedPosts := int32(1)
	if address := os.Getenv("BUTLER_SERVER_BROWSER_UI_API_ADDR"); address != "" {
		// Keep the same actual Broker/PG/runtime alive for Web. The independent
		// API process uses the production public owner-authenticated handler.
		obs = invoke("browser.snapshot", map[string]any{"browser_session_id": id})
		obs = invoke("browser.navigate", map[string]any{"browser_session_id": id, "url": "http://login.test/form", "observation_version": obs["observation_version"], "expected_url": obs["final_url"]})
		bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		apiProcess := exec.CommandContext(bounded, os.Getenv("BUTLER_SERVER_BROWSER_API_BINARY"), "-test.run=^TestServerBrowserOwnerAPIHarness$")
		apiProcess.Env = append(os.Environ(), "BUTLER_SERVER_BROWSER_API_HARNESS=1", "BUTLER_SERVER_BROWSER_BROKER_ENDPOINT="+privateHTTP.URL+browsercontrol.Path, "BUTLER_SERVER_BROWSER_OWNER_API_ADDR="+address)
		apiProcess.Stdout, apiProcess.Stderr = io.Discard, io.Discard
		if err := apiProcess.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { cancel(); _ = apiProcess.Wait() }()
		for {
			if _, err := os.Stat(os.Getenv("BUTLER_SERVER_BROWSER_UI_DONE")); err == nil {
				break
			}
			select {
			case <-bounded.Done():
				t.Fatal("actual Web acceptance did not complete")
			case <-time.After(100 * time.Millisecond):
			}
		}
		if posts.Load() != 2 {
			t.Fatal("Web click did not submit through real owner API/Broker/Chromium exactly once", posts.Load())
		}
		expectedPosts = 2
	}
	// Current core authority is checked for every owner request.
	listed, e = ownerClient.Control(ctx, browsercontrol.Command{Operation: "list", OwnerID: "owner"})
	if e != nil || len(listed.Sessions) != 1 {
		t.Fatal("list before cancellation", e)
	}
	command.Operation, command.Epoch = "take", listed.Sessions[0].Epoch
	if _, e = ownerClient.Control(ctx, command); e != nil {
		t.Fatal("take before cancellation", e)
	}
	command.Epoch++
	if _, err = pool.Exec(ctx, `UPDATE tasks SET status='cancelled' WHERE task_id='task'`); err != nil {
		t.Fatal(err)
	}
	if _, e = ownerClient.Control(ctx, browsercontrol.Command{Operation: "frame", SessionID: id, OwnerID: "owner"}); e == nil {
		t.Fatal("stopped task still exposed browser")
	}
	invalidRelease := command
	invalidRelease.Operation, invalidRelease.Controller = "release", strings.Repeat("f", 32)
	if _, e = ownerClient.Control(ctx, invalidRelease); e == nil {
		t.Fatal("stopped task allowed another controller to release")
	}
	command.Operation = "release"
	if _, e = ownerClient.Control(ctx, command); e != nil {
		t.Fatal("stopped task prevented removal of existing write capability", e)
	}
	if private.Load() != 0 || posts.Load() != expectedPosts {
		t.Fatal("unexpected external effects")
	}
}
