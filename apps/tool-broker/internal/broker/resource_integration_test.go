package broker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/credentials"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

type resourceReader struct {
	*authFixture
	pool *pgxpool.Pool
}

func (r resourceReader) GetActionContext(ctx context.Context, id string) (actionauth.ContextSnapshot, error) {
	c := actionauth.ContextSnapshot{RunID: id, AutonomyMode: 3}
	e := r.pool.QueryRow(ctx, `SELECT r.current_state,r.session_key,s.user_id,r.task_id,t.revision,r.task_revision,t.constraints_revision,t.status,o.lease_id,o.fence_epoch,o.expires_at FROM runs r JOIN sessions s USING(session_key) JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.session_key=r.session_key WHERE r.run_id=$1 AND o.run_id=r.run_id`, id).Scan(&c.RunState, &c.SessionKey, &c.OwnerID, &c.TaskID, &c.TaskRevision, &c.PinnedTaskRevision, &c.ConstraintsRevision, &c.TaskStatus, &c.LeaseID, &c.LeaseEpoch, &c.LeaseExpiresAt)
	return c, e
}

type denyResourceReconcile struct{}

func (denyResourceReconcile) CheckReconciliationTx(context.Context, pgx.Tx, resources.Decision) error {
	return resources.ErrDenied
}

func resourceBrokerDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	base := brokerEffectDatabase(t)
	cfg := base.Config()
	cfg.MaxConns = 2
	pool, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	_, e = pool.Exec(context.Background(), `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT NOT NULL);INSERT INTO sessions VALUES('web:owner','owner'),('web:second','owner');
 ALTER TABLE runs ADD COLUMN session_key TEXT DEFAULT 'web:owner';
 ALTER TABLE tasks ADD COLUMN session_key TEXT DEFAULT 'web:owner',ADD COLUMN parent_task_id TEXT,ADD COLUMN created_at TIMESTAMPTZ DEFAULT clock_timestamp();
 INSERT INTO tasks(task_id,owner_id,status,revision,constraints_revision,session_key) VALUES('task2','owner','active',1,1,'web:second');
 INSERT INTO runs(run_id,task_id,task_revision,session_key) VALUES('run2','task2',1,'web:second');
 INSERT INTO session_ownership VALUES('web:second','run2','lease2',1,clock_timestamp()+INTERVAL '1 hour');
 CREATE TABLE memory_sources(source_id TEXT PRIMARY KEY);
 CREATE TABLE current_resource_sources(run_id TEXT PRIMARY KEY,status TEXT NOT NULL);INSERT INTO current_resource_sources VALUES('run','current'),('run2','current');`)
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"037_execution_budgets.up.sql", "057_task_delegations.up.sql", "058_execution_resource_leases.up.sql", "062_resource_dispatches.up.sql"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(context.Background(), string(raw)); e != nil {
			t.Fatal(name, e)
		}
	}
	return pool
}

type resourcePageBackend struct {
	calls        atomic.Int32
	entered      chan struct{}
	continueCall chan struct{}
}

func (b *resourcePageBackend) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	b.calls.Add(1)
	if b.entered != nil {
		select {
		case b.entered <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.continueCall:
		}
	}
	c := req.GetToolCall()
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: c.GetRunId(), ToolCallId: c.GetToolCallId(), ToolName: c.GetToolName(), Status: "completed", ResultJson: `{"browser_session_id":"0123456789abcdef0123456789abcdef","observation_version":1,"final_url":"https://example.com","ok":true}`}}, nil
}

type loseResourceFinish struct{ resourceexec.RuntimeAuthority }

func (loseResourceFinish) Finish(context.Context, resourceexec.Receipt) error {
	return resourceexec.ErrDenied
}

type resourceBrokerFixture struct {
	pool    *pgxpool.Pool
	server  *Server
	auth    *actionauth.PostgresStore
	reader  resourceReader
	store   *resourceexec.Store
	budgets *budget.Controller
	backend *resourcePageBackend
	router  *runtimeclient.Router
}

func resourceBroker(t *testing.T, backend *resourcePageBackend, loseReceipt bool) *resourceBrokerFixture {
	t.Helper()
	pool := resourceBrokerDB(t)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	target := listener.Addr().String()
	contract := &toolbrokerv1.ToolContract{ToolName: "browser.snapshot", ContractRevision: "1", RuntimeTarget: target, InputSchemaJson: `{"type":"object","properties":{"browser_session_id":{"type":"string"}},"required":["browser_session_id"],"additionalProperties":false}`, OutputSchemaJson: `{"type":"object"}`, Status: "enabled", ToolClass: "browser"}
	contracts := []*toolbrokerv1.ToolContract{contract}
	if backend != nil {
		// The mechanical fixture has an already created context. Real Chromium
		// below publishes its mapping ONLY through an actual open receipt.
		digest := sha256.Sum256([]byte("fixed-browser-service\x00" + "0123456789abcdef0123456789abcdef"))
		h := hex.EncodeToString(digest[:])
		_, e = pool.Exec(context.Background(), `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('fixture-context-birth','fixture','{}','{}','effect_confirmed')`)
		if e != nil {
			t.Fatal(e)
		}
		_, e = pool.Exec(context.Background(), `INSERT INTO execution_browser_resources(profile_id,session_hash,resource_id,owner_id,run_id,task_id,task_revision,created_by_effect,expires_at) VALUES('fixed-browser-service',$1,$2,'owner','run','task',1,'fixture-context-birth',clock_timestamp()+INTERVAL '15 minutes')`, h, "browser-tab:"+h)
		if e != nil {
			t.Fatal(e)
		}
	}
	if backend == nil {
		actualRegistry, err := registry.Load(filepath.Join("..", "..", "..", "..", "configs", "tools.json"), target)
		if err != nil {
			t.Fatal("actual browser contracts unavailable", err)
		}
		contracts = nil
		for _, name := range []string{"browser.open", "browser.snapshot", "browser.click"} {
			actual, ok := actualRegistry.Get(name)
			if !ok {
				t.Fatal("actual browser contract missing")
			}
			actual.RuntimeTarget = target
			actual.AllowedDomains = []string{"external.test"}
			contracts = append(contracts, actual)
		}
	}
	hashes := map[string]string{}
	for _, c := range contracts {
		h, e := actionauth.ContractHash(c)
		if e != nil {
			t.Fatal(e)
		}
		hashes[c.ToolName] = h
	}
	p := budget.DefaultPolicy()
	p.ToolMillis = 30000
	c := budget.NewController(pool, p)
	core, e := resources.New(resources.WorkerGuard{}, denyResourceReconcile{})
	if e != nil {
		t.Fatal(e)
	}
	source := func(ctx context.Context, tx pgx.Tx, run string) error {
		var status string
		e := tx.QueryRow(ctx, `SELECT status FROM current_resource_sources WHERE run_id=$1 FOR SHARE`, run).Scan(&status)
		if e != nil || status != "current" {
			return resourceexec.ErrDenied
		}
		return nil
	}
	rs, e := resourceexec.New(pool, core, c, source, resourceexec.Profile{ID: "fixed-browser-service", RuntimeTarget: target, ContractHashes: hashes})
	if e != nil {
		t.Fatal(e)
	}
	var runtimeAuthority resourceexec.RuntimeAuthority = rs
	if loseReceipt {
		runtimeAuthority = loseResourceFinish{rs}
	}
	privateToken := strings.Repeat("fixture-private-resource-", 3)
	handler, e := resourceexec.NewHTTPHandler(runtimeAuthority, privateToken)
	if e != nil {
		t.Fatal(e)
	}
	authorityHTTP := httptest.NewServer(handler)
	t.Cleanup(authorityHTTP.Close)
	client, e := resourceexec.NewHTTPAuthority(authorityHTTP.URL, privateToken)
	if e != nil {
		t.Fatal(e)
	}
	if backend == nil {
		listener.Close()
		binary := os.Getenv("BUTLER_BROWSER_E2E_BINARY")
		fixtureEndpoint := os.Getenv("BUTLER_RESOURCE_BROWSER_FIXTURE")
		if binary == "" || fixtureEndpoint == "" {
			t.Fatal("actual isolated browser harness required")
		}
		cmd := exec.Command(binary, "-test.run=^TestBrowserRuntimeEgressHarness$")
		cmd.Env = append(os.Environ(), "BUTLER_GRPC_ADDR="+target, "BUTLER_BROWSER_EGRESS_HARNESS=1", "BUTLER_BROWSER_FIXTURE_ENDPOINT="+fixtureEndpoint, "BUTLER_RESOURCE_AUTHORITY_HARNESS="+authorityHTTP.URL)
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = cmd.Process.Signal(os.Interrupt); _ = cmd.Wait() })
		deadline := time.Now().Add(10 * time.Second)
		for {
			conn, err := net.DialTimeout("tcp", target, 100*time.Millisecond)
			if err == nil {
				conn.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("actual guarded browser subprocess unavailable")
			}
			time.Sleep(25 * time.Millisecond)
		}
	} else {
		guarded, e := resourceexec.NewRuntime(backend, client, privateToken)
		if e != nil {
			t.Fatal(e)
		}
		rpc := grpc.NewServer()
		runtimev1.RegisterToolRuntimeServiceServer(rpc, guarded)
		go rpc.Serve(listener)
		t.Cleanup(func() { rpc.Stop(); listener.Close() })
	}
	router := runtimeclient.New(nil)
	t.Cleanup(func() { router.Close() })
	if e = router.SetTargetAuthentication(target, privateToken); e != nil {
		t.Fatal(e)
	}
	exec, e := NewResourceExecutor(rs, router)
	if e != nil {
		t.Fatal(e)
	}
	data, _ := json.Marshal(map[string]any{"tools": contracts})
	path := filepath.Join(t.TempDir(), "tools.json")
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	reg, e := registry.Load(path, target)
	if e != nil {
		t.Fatal(e)
	}
	reader := resourceReader{newAuthFixture(), pool}
	auth := actionauth.NewPostgresStore(pool)
	auth.SetBudgets(c)
	auth.SetRunDataGuard(source)
	auth.SetDispatchClaimGuard(rs)
	server := NewServer(reg, exec, nil, nil)
	server.SetActionAuthorizer(actionauth.NewService(auth, reader, nil))
	server.SetBudgets(c)
	server.SetResourceAdmission(rs)
	return &resourceBrokerFixture{pool, server, auth, reader, rs, c, backend, router}
}
func (f *resourceBrokerFixture) request(t *testing.T, run, id string) *toolbrokerv1.ExecuteToolCallRequest {
	t.Helper()
	call := &toolbrokerv1.ToolCall{RunId: run, ToolCallId: id, ToolName: "browser.snapshot", AutonomyMode: 3, ArgsJson: `{"browser_session_id":"0123456789abcdef0123456789abcdef"}`}
	return f.callRequest(t, call)
}
func (f *resourceBrokerFixture) callRequest(t *testing.T, call *toolbrokerv1.ToolCall) *toolbrokerv1.ExecuteToolCallRequest {
	t.Helper()
	p, e := f.server.PrepareToolCall(context.Background(), &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
	if e != nil || p.GetError() != nil {
		t.Fatal("prepare denied", e, p.GetError())
	}
	i, e := f.auth.GetIntent(context.Background(), p.GetAction().GetActionId())
	if e != nil {
		t.Fatal(e)
	}
	approval := "approval-" + call.ToolCallId
	f.reader.mu.Lock()
	f.reader.approvals[approval] = actionauth.ApprovalSnapshot{ApprovalID: approval, Status: "approved", Binding: i.Binding, ResolvedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	f.reader.mu.Unlock()
	g, e := f.server.AuthorizeToolCall(context.Background(), &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: i.Binding.ActionID, ApprovalId: approval})
	if e != nil {
		t.Fatal(e)
	}
	return &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: g.GetAuthorizationId()}
}

func TestResourceBrokerActualChromiumPageCASIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("requires isolated actual Chromium fixture")
	}
	var posts atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/effect" {
			posts.Add(1)
			_, _ = fmt.Fprint(w, "done")
			return
		}
		_, _ = fmt.Fprint(w, `<html><body><p id="state">first</p><button id="submit" onclick="fetch('/effect',{method:'POST'})">Effect</button><script>setTimeout(()=>document.getElementById('state').textContent='changed',200)</script></body></html>`)
	}))
	defer site.Close()
	t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
	f := resourceBroker(t, nil, false)
	invoke := func(id, tool string, args map[string]any) *toolbrokerv1.ToolResult {
		t.Helper()
		raw, _ := json.Marshal(args)
		req := f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: id, ToolName: tool, AutonomyMode: 3, ArgsJson: string(raw)})
		v, e := f.server.ExecuteToolCall(context.Background(), req)
		if e != nil {
			t.Fatal(e)
		}
		return v.GetResult()
	}
	opened := invoke("open", "browser.open", map[string]any{"url": egresstest.Origin + "/changing"})
	if opened.GetStatus() != "completed" {
		t.Fatal("actual guarded open failed", opened.GetError().GetMessage())
	}
	var observation struct {
		ID      string `json:"browser_session_id"`
		Version int64  `json:"observation_version"`
		URL     string `json:"final_url"`
	}
	if e := json.Unmarshal([]byte(opened.ResultJson), &observation); e != nil || observation.ID == "" || observation.Version < 1 {
		t.Fatal("actual opening observation absent")
	}
	time.Sleep(350 * time.Millisecond)
	stale := invoke("stale", "browser.click", map[string]any{"browser_session_id": observation.ID, "observation_version": observation.Version, "expected_url": observation.URL, "selector": "#submit"})
	if stale.GetStatus() != "effect_unknown" || posts.Load() != 0 {
		t.Fatal("manual page mutation reached external click", stale.GetStatus(), posts.Load())
	}
	var n int
	var inflight string
	if e := f.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM execution_budget_reservations`).Scan(&n); e != nil || n != 2 {
		t.Fatal("actual opening/click were not finitely reserved", e, n)
	}
	if e := f.pool.QueryRow(context.Background(), `SELECT r.inflight_lease_id FROM execution_resources r JOIN execution_resource_dispatches d USING(resource_id) WHERE d.tool_name='browser.click'`).Scan(&inflight); e != nil || inflight == "" {
		t.Fatal("unconfirmed browser failure silently released resource", e)
	}
}
func TestResourceBrokerRuntimeAtomicClaimAndPrivateOneUseIntegration(t *testing.T) {
	f := resourceBroker(t, &resourcePageBackend{}, false)
	req := f.request(t, "run", "first")
	v, e := f.server.ExecuteToolCall(context.Background(), req)
	if e != nil || f.backend.calls.Load() != 1 || v.GetResult().GetResultJson() == "" {
		t.Fatal("actual Broker/runtime dispatch failed", e, v.GetResult().GetError())
	}
	var phase, state string
	var version, reservations int64
	e = f.pool.QueryRow(context.Background(), `SELECT d.phase,l.state,r.version FROM execution_resource_dispatches d JOIN execution_resource_leases l ON l.lease_id=d.resource_lease_id JOIN execution_resources r ON r.resource_id=d.resource_id`).Scan(&phase, &state, &version)
	if e != nil || phase != "completed" || state != "completed" || version != 2 {
		t.Fatal("lane not confirmed", e, phase, state, version)
	}
	if e = f.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM execution_budget_reservations`).Scan(&reservations); e != nil || reservations != 1 {
		t.Fatal("missing finite permanent reservation", e)
	}
	duplicate, e := f.server.ExecuteToolCall(context.Background(), req)
	if e != nil || f.backend.calls.Load() != 1 || duplicate.GetResult().GetError().GetRetryable() {
		t.Fatal("spent action replayed")
	}
	if strings.Contains(v.GetResult().GetResultJson(), "ticket") || strings.Contains(v.GetResult().GetResultJson(), "resource_") {
		t.Fatal("resource proof reached model result")
	}
	down, _ := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "062_resource_dispatches.down.sql"))
	if _, e = f.pool.Exec(context.Background(), string(down)); e == nil {
		t.Fatal("downgrade discarded retained dispatch history")
	}
}

func TestResourceClaimWithoutActualBudgetReservationCannotDispatchIntegration(t *testing.T) {
	f := resourceBroker(t, &resourcePageBackend{}, false)
	req := f.request(t, "run", "unbudgeted")
	// Deliberately incomplete startup composition must not turn an authorized
	// action into a resource claim. The ordinary Broker budget remains present.
	f.auth.SetBudgets(nil)
	result, err := f.server.ExecuteToolCall(context.Background(), req)
	if err != nil || result.GetResult().GetStatus() == "completed" || f.backend.calls.Load() != 0 {
		t.Fatal("missing finite actual reservation reached runtime", err)
	}
	grant, err := f.auth.GetGrant(context.Background(), req.AuthorizationId)
	if err != nil || grant.State != "authorized" {
		t.Fatal("unbudgeted claim spent grant", err)
	}
	var dispatches, leases, reservations int
	err = f.pool.QueryRow(context.Background(), `SELECT (SELECT COUNT(*) FROM execution_resource_dispatches),(SELECT COUNT(*) FROM execution_resource_leases),(SELECT COUNT(*) FROM execution_budget_reservations)`).Scan(&dispatches, &leases, &reservations)
	if err != nil || dispatches != 0 || leases != 0 || reservations != 0 {
		t.Fatal("unbudgeted claim left partial admission", err)
	}
}

func TestResourceMigrationEmptyRollbackAndBrowserOnlyHistoryGuardIntegration(t *testing.T) {
	pool := resourceBrokerDB(t)
	up, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "062_resource_dispatches.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "062_resource_dispatches.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(context.Background(), string(down)); err != nil {
		t.Fatal("empty migration could not roll back", err)
	}
	if _, err = pool.Exec(context.Background(), string(up)); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(context.Background(), `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('browser-birth','fixture','{}','{}','effect_confirmed');
 INSERT INTO execution_browser_resources(profile_id,session_hash,resource_id,owner_id,run_id,task_id,task_revision,created_by_effect,expires_at) VALUES('fixed','hash','browser-tab:hash','owner','run','task',1,'browser-birth',clock_timestamp()+INTERVAL '15 minutes')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(context.Background(), string(down)); err == nil {
		t.Fatal("browser-only durable provenance was discarded")
	}
	var retained int
	if err = pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM execution_browser_resources`).Scan(&retained); err != nil || retained != 1 {
		t.Fatal("refused rollback removed history", err)
	}
}
func TestResourceBrokerConflictRollsBackBudgetAndSecondDispatchIntegration(t *testing.T) {
	backend := &resourcePageBackend{entered: make(chan struct{}, 1), continueCall: make(chan struct{})}
	f := resourceBroker(t, backend, false)
	first := f.request(t, "run", "first")
	second := f.request(t, "run", "second")
	done := make(chan error, 1)
	go func() { _, e := f.server.ExecuteToolCall(context.Background(), first); done <- e }()
	select {
	case <-backend.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first lane not entered")
	}
	v, e := f.server.ExecuteToolCall(context.Background(), second)
	if e != nil || v.GetResult().GetStatus() != "failed" || backend.calls.Load() != 1 {
		t.Fatal("second writer reached runtime", e)
	}
	var n int64
	if e = f.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM execution_budget_reservations`).Scan(&n); e != nil || n != 1 {
		t.Fatal("conflict retained partial budget reservation", e, n)
	}
	g, e := f.auth.GetGrant(context.Background(), second.AuthorizationId)
	if e != nil || g.State != "authorized" {
		t.Fatal("resource conflict partially spent grant", e)
	}
	close(backend.continueCall)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
}
func TestResourceBrokerLostReceiptBlocksSuccessorAndReplayIntegration(t *testing.T) {
	f := resourceBroker(t, &resourcePageBackend{}, true)
	first := f.request(t, "run", "first")
	v, e := f.server.ExecuteToolCall(context.Background(), first)
	if e != nil || v.GetResult().GetStatus() != "effect_unknown" || f.backend.calls.Load() != 1 {
		t.Fatal("lost receipt hidden", e)
	}
	second := f.request(t, "run", "second")
	v, e = f.server.ExecuteToolCall(context.Background(), second)
	if e != nil || v.GetResult().GetStatus() != "failed" || f.backend.calls.Load() != 1 {
		t.Fatal("successor entered uncertain resource")
	}
	v, e = f.server.ExecuteToolCall(context.Background(), first)
	if e != nil || f.backend.calls.Load() != 1 {
		t.Fatal("lost-ack replayed")
	}
	var phase, inflight string
	e = f.pool.QueryRow(context.Background(), `SELECT d.phase,r.inflight_lease_id FROM execution_resource_dispatches d JOIN execution_resources r USING(resource_id)`).Scan(&phase, &inflight)
	if e != nil || phase != "running" || inflight == "" {
		t.Fatal("unknown resource lost quarantine", e)
	}
}

func TestResourceForeignOrInventedBrowserSessionCannotClaimOrSpendIntegration(t *testing.T) {
	for _, kind := range []string{"other_run", "invented_session"} {
		t.Run(kind, func(t *testing.T) {
			f := resourceBroker(t, &resourcePageBackend{}, false)
			call := &toolbrokerv1.ToolCall{RunId: "run2", ToolCallId: "foreign", ToolName: "browser.snapshot", AutonomyMode: 3, ArgsJson: `{"browser_session_id":"0123456789abcdef0123456789abcdef"}`}
			if kind == "invented_session" {
				call.RunId = "run"
				call.ArgsJson = `{"browser_session_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
			}
			request := f.callRequest(t, call)
			v, e := f.server.ExecuteToolCall(context.Background(), request)
			if e != nil || v.GetResult().GetStatus() != "failed" || f.backend.calls.Load() != 0 {
				t.Fatal("model identifier obtained resource authority", e)
			}
			var mappings, leases, budgets int
			e = f.pool.QueryRow(context.Background(), `SELECT (SELECT COUNT(*) FROM execution_resource_dispatches),(SELECT COUNT(*) FROM execution_resource_leases),(SELECT COUNT(*) FROM execution_budget_reservations)`).Scan(&mappings, &leases, &budgets)
			if e != nil || mappings != 0 || leases != 0 || budgets != 0 {
				t.Fatal("foreign session retained partial resource/budget admission", e)
			}
		})
	}
}

type pausedResourceExecutor struct {
	next    RuntimeExecutor
	entered chan struct{}
	resume  chan struct{}
}

func (e *pausedResourceExecutor) Execute(ctx context.Context, c *toolbrokerv1.ToolCall, k *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	close(e.entered)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-e.resume:
	}
	return e.next.Execute(ctx, c, k, secrets)
}
func TestResourceRuntimeRechecksCancellationSourceAndExpiryAfterClaimIntegration(t *testing.T) {
	for _, kind := range []string{"cancel", "source", "expired_resource", "recorded_unknown"} {
		t.Run(kind, func(t *testing.T) {
			f := resourceBroker(t, &resourcePageBackend{}, false)
			req := f.request(t, "run", "paused")
			paused := &pausedResourceExecutor{next: f.server.executor, entered: make(chan struct{}), resume: make(chan struct{})}
			f.server.executor = paused
			done := make(chan *toolbrokerv1.ExecuteToolCallResponse, 1)
			go func() {
				v, e := f.server.ExecuteToolCall(context.Background(), req)
				if e != nil {
					done <- nil
					return
				}
				done <- v
			}()
			select {
			case <-paused.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("claimed dispatch never reached pause")
			}
			var query string
			switch kind {
			case "cancel":
				query = `UPDATE runs SET current_state='cancelled',status='cancelled' WHERE run_id='run';UPDATE session_ownership SET fence_epoch=2 WHERE run_id='run'`
			case "source":
				query = `UPDATE current_resource_sources SET status='deleted' WHERE run_id='run'`
			case "expired_resource":
				query = `UPDATE execution_resource_leases SET expires_at=created_at+INTERVAL '1 millisecond' WHERE run_id='run';SELECT pg_sleep(0.005)`
			case "recorded_unknown":
				query = `UPDATE action_effects SET state='effect_unknown' WHERE action_id=(SELECT effect_id FROM execution_resource_dispatches);UPDATE action_authorizations SET state='unknown' WHERE authorization_id=(SELECT authorization_id FROM execution_resource_dispatches)`
			}
			if _, e := f.pool.Exec(context.Background(), query); e != nil {
				t.Fatal(e)
			}
			close(paused.resume)
			select {
			case v := <-done:
				if v == nil || v.GetResult().GetStatus() != "effect_unknown" || f.backend.calls.Load() != 0 {
					t.Fatal("stale dispatch entered actual backend", kind)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("stale dispatch did not fail closed")
			}
			var phase, inflight string
			if e := f.pool.QueryRow(context.Background(), `SELECT d.phase,r.inflight_lease_id FROM execution_resource_dispatches d JOIN execution_resources r USING(resource_id)`).Scan(&phase, &inflight); e != nil || phase != "prepared" || inflight == "" {
				t.Fatal("stale claim silently lost permanent quarantine", e)
			}
		})
	}
}

func TestResourceClaimDeadlineAfterResourceWaitRollsBackWholeAdmissionIntegration(t *testing.T) {
	f := resourceBroker(t, &resourcePageBackend{}, false)
	first := f.request(t, "run", "initial")
	if _, e := f.server.ExecuteToolCall(context.Background(), first); e != nil || f.backend.calls.Load() != 1 {
		t.Fatal("initial resource observation failed", e)
	}
	req := f.request(t, "run", "deadline")
	_, e := f.pool.Exec(context.Background(), `WITH cutoff AS(SELECT clock_timestamp()+INTERVAL '300 milliseconds' AS expiry) UPDATE action_authorizations a SET expires_at=c.expiry,grant_data=jsonb_set(a.grant_data,'{expires_at}',to_jsonb(c.expiry)) FROM cutoff c WHERE authorization_id=$1`, req.AuthorizationId)
	if e != nil {
		t.Fatal(e)
	}
	lock, e := f.pool.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Rollback(context.Background())
	var id string
	if e = lock.QueryRow(context.Background(), `SELECT resource_id FROM execution_resources FOR UPDATE`).Scan(&id); e != nil {
		t.Fatal(e)
	}
	done := make(chan *toolbrokerv1.ExecuteToolCallResponse, 1)
	go func() { v, _ := f.server.ExecuteToolCall(context.Background(), req); done <- v }()
	observer, e := pgx.ConnectConfig(context.Background(), f.pool.Config().ConnConfig.Copy())
	if e != nil {
		t.Fatal(e)
	}
	defer observer.Close(context.Background())
	end := time.Now().Add(2 * time.Second)
	for {
		if _, e = observer.Exec(context.Background(), `SELECT pg_stat_clear_snapshot()`); e != nil {
			t.Fatal(e)
		}
		var blocked bool
		e = observer.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid<>pg_backend_pid() AND wait_event_type='Lock' AND query LIKE '%execution_resources%')`).Scan(&blocked)
		if e != nil {
			t.Fatal(e)
		}
		if blocked {
			break
		}
		if time.Now().After(end) {
			t.Fatal("resource lock barrier was not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, e = observer.Exec(context.Background(), `SELECT pg_sleep(0.35)`); e != nil {
		t.Fatal(e)
	}
	if e = lock.Commit(context.Background()); e != nil {
		t.Fatal(e)
	}
	select {
	case v := <-done:
		if v == nil || v.GetResult().GetStatus() != "failed" || f.backend.calls.Load() != 1 {
			t.Fatal("expired wait reached backend")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("resource wait did not resolve")
	}
	var reservations, mappings int
	if e = f.pool.QueryRow(context.Background(), `SELECT (SELECT COUNT(*) FROM execution_budget_reservations),(SELECT COUNT(*) FROM execution_resource_dispatches)`).Scan(&reservations, &mappings); e != nil || reservations != 1 || mappings != 1 {
		t.Fatal("expired admission retained partial state", e, reservations, mappings)
	}
	g, e := f.auth.GetGrant(context.Background(), req.AuthorizationId)
	if e != nil || g.State != "authorized" {
		t.Fatal("expired wait partially consumed grant", e)
	}
}
