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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// Only fixture authority wiring is supplied here. Actual task/run/lease state,
// mandate versions, grants, budget and resource journals live in isolated PG.
type browserSafetyReader struct{ resourceReader }

func (r browserSafetyReader) GetActionContext(ctx context.Context, run string) (actionauth.ContextSnapshot, error) {
	c, err := r.resourceReader.GetActionContext(ctx, run)
	if err == nil {
		err = r.pool.QueryRow(ctx, `SELECT mandate_id FROM runs WHERE run_id=$1`, run).Scan(&c.MandateID)
	}
	return c, err
}

type browserSafetyProcessConfig struct {
	DSN, SearchPath, ToolsPath, AddressFile, PrivateToken string
	ContextEndpoint                                       string
}

// A new OS process reconstructs the real Broker from the same durable store.
// It has no fixture outcome oracle and does not create or repair journal rows.
func TestBrowserSafetyBrokerHarness(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_SAFETY_BROKER_HARNESS") != "1" {
		t.Skip("private subprocess harness only")
	}
	var cfg browserSafetyProcessConfig
	data, err := os.ReadFile(os.Getenv("BUTLER_BROWSER_SAFETY_BROKER_CONFIG"))
	if err != nil || json.Unmarshal(data, &cfg) != nil {
		t.Fatal("private fixture configuration unavailable")
	}
	pc, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil || !strings.HasSuffix(pc.ConnConfig.Database, "_test") || cfg.SearchPath == "" {
		t.Fatal("isolated database required")
	}
	pc.ConnConfig.RuntimeParams["search_path"] = cfg.SearchPath
	pc.MaxConns = 3
	pool, err := pgxpool.NewWithConfig(context.Background(), pc)
	if err != nil {
		t.Fatal("fixture database unavailable")
	}
	defer pool.Close()
	reg, err := registry.Load(cfg.ToolsPath, "")
	if err != nil {
		t.Fatal("fixture contracts unavailable")
	}
	hashes := map[string]string{}
	var target string
	for _, c := range reg.List("", false) {
		h, e := actionauth.ContractHash(c)
		if e != nil {
			t.Fatal(e)
		}
		hashes[c.ToolName], target = h, c.RuntimeTarget
	}
	policy := budget.DefaultPolicy()
	policy.ToolMillis = 30000
	budgets := budget.NewController(pool, policy)
	core, err := resources.New(resources.WorkerGuard{}, denyResourceReconcile{})
	if err != nil {
		t.Fatal(err)
	}
	source := func(ctx context.Context, tx pgx.Tx, run string) error {
		var status string
		if tx.QueryRow(ctx, `SELECT status FROM current_resource_sources WHERE run_id=$1 FOR SHARE`, run).Scan(&status) != nil || status != "current" {
			return resourceexec.ErrDenied
		}
		return nil
	}
	rs, err := resourceexec.New(pool, core, budgets, source, resourceexec.Profile{ID: "fixed-browser-service", RuntimeTarget: target, ContractHashes: hashes})
	if err != nil {
		t.Fatal(err)
	}
	router := runtimeclient.New(nil)
	defer router.Close()
	if router.SetTargetAuthentication(target, strings.Repeat("fixture-private-resource-", 3)) != nil {
		t.Fatal("runtime authentication unavailable")
	}
	executor, err := NewResourceExecutor(rs, router)
	if err != nil {
		t.Fatal(err)
	}
	store := actionauth.NewPostgresStore(pool)
	store.SetBudgets(budgets)
	store.SetRunDataGuard(source)
	store.SetDispatchClaimGuard(rs)
	var reader actionauth.ContextReader = browserSafetyReader{resourceReader{authFixture: newAuthFixture(), pool: pool}}
	if cfg.ContextEndpoint != "" {
		budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
		reader, err = actionauth.NewHTTPContextReader(cfg.ContextEndpoint, strings.Repeat("owner-context-fixture-", 3))
		if err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(reg, executor, nil, nil)
	server.SetActionAuthorizer(actionauth.NewService(store, reader, nil))
	server.SetBudgets(budgets)
	server.SetResourceAdmission(rs)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	rpc := grpc.NewServer(grpc.UnaryInterceptor(actionauth.ExecutionUnaryInterceptor(cfg.PrivateToken)))
	defer rpc.Stop()
	toolbrokerv1.RegisterToolBrokerServiceServer(rpc, server)
	if os.WriteFile(cfg.AddressFile, []byte(listener.Addr().String()), 0600) != nil {
		t.Fatal("fixture ready receipt unavailable")
	}
	if err = rpc.Serve(listener); err != nil {
		t.Fatal("fixture Broker stopped")
	}
}

func browserSafetyBrokerProcess(t *testing.T, f *resourceBrokerFixture) (toolbrokerv1.ToolBrokerServiceClient, context.Context, func()) {
	return browserSafetyBrokerWithContext(t, f, "")
}

func browserSafetyBrokerWithContext(t *testing.T, f *resourceBrokerFixture, endpoint string, configure ...func(*toolbrokerv1.ToolContract)) (toolbrokerv1.ToolBrokerServiceClient, context.Context, func()) {
	t.Helper()
	if endpoint != "" {
		f.budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	}
	dir := t.TempDir()
	contracts := f.server.registry.List("", false)
	for _, c := range contracts {
		for _, option := range configure {
			option(c)
		}
	}
	tools, _ := json.Marshal(map[string]any{"tools": contracts})
	toolsPath := filepath.Join(dir, "tools.json")
	if os.WriteFile(toolsPath, tools, 0600) != nil {
		t.Fatal("fixture contracts write")
	}
	cfg := browserSafetyProcessConfig{DSN: f.pool.Config().ConnString(), SearchPath: f.pool.Config().ConnConfig.RuntimeParams["search_path"], ToolsPath: toolsPath, AddressFile: filepath.Join(dir, "ready"), PrivateToken: strings.Repeat("fixture-core-private-", 3)}
	cfg.ContextEndpoint = endpoint
	encoded, _ := json.Marshal(cfg)
	path := filepath.Join(dir, "private.json")
	if os.WriteFile(path, encoded, 0600) != nil {
		t.Fatal("private configuration write")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestBrowserSafetyBrokerHarness$", "-test.timeout=120s")
	cmd.Env = append(os.Environ(), "BUTLER_BROWSER_SAFETY_BROKER_HARNESS=1", "BUTLER_BROWSER_SAFETY_BROKER_CONFIG="+path)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if cmd.Start() != nil {
		t.Fatal("Broker subprocess unavailable")
	}
	var once sync.Once
	stop := func() { once.Do(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }) }
	t.Cleanup(stop)
	deadline := time.Now().Add(10 * time.Second)
	var address []byte
	for {
		address, err = os.ReadFile(cfg.AddressFile)
		if err == nil && len(address) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Broker subprocess readiness timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
	conn, err := grpc.NewClient(string(address), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal("Broker connection unavailable")
	}
	t.Cleanup(func() { conn.Close() })
	client := toolbrokerv1.NewToolBrokerServiceClient(conn)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+cfg.PrivateToken)
	for _, name := range []string{"browser.open", "browser.click"} {
		actual, err := client.GetToolContract(ctx, &toolbrokerv1.GetToolContractRequest{ToolName: name})
		original, _ := f.server.registry.Get(name)
		for _, option := range configure {
			option(original)
		}
		before, _ := actionauth.ContractHash(original)
		after, _ := actionauth.ContractHash(actual.GetContract())
		if err != nil || before != after {
			t.Fatal("fixture changed contract across Broker process")
		}
	}
	return client, ctx, stop
}

func browserSafetyMandate(t *testing.T, f *resourceBrokerFixture) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `ALTER TABLE runs ADD COLUMN mandate_id TEXT NOT NULL DEFAULT '';UPDATE runs SET mandate_id='browser-safety' WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	m := actionauth.Mandate{ID: "browser-safety", ScopeKind: "task", OwnerID: "owner", TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1, Revision: 1, Tools: []string{"browser.click"}, Resources: []string{egresstest.Origin}, Effects: []actionauth.Effect{actionauth.EffectUnknown}, ExpiresAt: time.Now().Add(time.Hour)}
	if err := f.auth.PutMandate(ctx, m); err != nil {
		t.Fatal(err)
	}
}

func browserSafetyGrant(t *testing.T, c toolbrokerv1.ToolBrokerServiceClient, ctx context.Context, call *toolbrokerv1.ToolCall) *toolbrokerv1.ExecuteToolCallRequest {
	t.Helper()
	p, err := c.PrepareToolCall(ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
	if err != nil || p.GetError() != nil {
		t.Fatal("fixture prepare denied", err)
	}
	g, err := c.AuthorizeToolCall(ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: p.GetAction().GetActionId(), MandateId: "browser-safety"})
	if err != nil || g.GetError() != nil || g.GetAuthorizationId() == "" {
		t.Fatal("fixture mandate authorization denied", err)
	}
	return &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: g.AuthorizationId}
}

func browserSafetyOpen(t *testing.T, f *resourceBrokerFixture) (string, string, int64) {
	return browserSafetyOpenNamed(t, f, "open")
}

func browserSafetyOpenNamed(t *testing.T, f *resourceBrokerFixture, callID string) (string, string, int64) {
	t.Helper()
	req := f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: callID, ToolName: "browser.open", AutonomyMode: 3, ArgsJson: `{"url":"` + egresstest.Origin + `/form"}`})
	r, err := f.server.ExecuteToolCall(context.Background(), req)
	var view struct {
		ID      string `json:"browser_session_id"`
		URL     string `json:"final_url"`
		Version int64  `json:"observation_version"`
	}
	if err != nil || r.GetResult().GetStatus() != "completed" || json.Unmarshal([]byte(r.GetResult().GetResultJson()), &view) != nil || view.ID == "" {
		t.Fatal("actual Chromium open failed")
	}
	return view.ID, view.URL, view.Version
}

func TestBrowserSafetyCurrentAuthorityAndBindingChromiumIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("isolated PG and actual Chromium required")
	}
	for _, kind := range []string{"unchanged_control", "task_cancel", "mandate_revoke", "changed_origin", "changed_args"} {
		t.Run(kind, func(t *testing.T) {
			var posts atomic.Int32
			site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if r.Method == "POST" {
					posts.Add(1)
					fmt.Fprint(w, "saved")
					return
				}
				fmt.Fprint(w, `<form method=post action=/effect><button id=submit>Send</button><button id=other>Other</button></form>`)
			}))
			defer site.Close()
			t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
			f := resourceBroker(t, nil, false)
			id, url, version := browserSafetyOpen(t, f)
			browserSafetyMandate(t, f)
			client, ctx, _ := browserSafetyBrokerProcess(t, f)
			args := map[string]any{"browser_session_id": id, "expected_url": url, "observation_version": version, "selector": "#submit"}
			raw, _ := json.Marshal(args)
			original := browserSafetyGrant(t, client, ctx, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "guarded-click", ToolName: "browser.click", AutonomyMode: 3, ArgsJson: string(raw)})
			altered := proto.Clone(original).(*toolbrokerv1.ExecuteToolCallRequest)
			switch kind {
			case "task_cancel":
				if _, err := f.pool.Exec(context.Background(), `UPDATE tasks SET status='cancelled' WHERE task_id='task'`); err != nil {
					t.Fatal(err)
				}
			case "mandate_revoke":
				if err := f.auth.RevokeMandate(context.Background(), "browser-safety", "owner", 1); err != nil {
					t.Fatal(err)
				}
			case "changed_origin":
				args["expected_url"] = "https://another.invalid/form"
			case "changed_args":
				args["selector"] = "#other"
			}
			raw, _ = json.Marshal(args)
			altered.ToolCall.ArgsJson = string(raw)
			result, err := client.ExecuteToolCall(ctx, altered)
			if kind == "unchanged_control" {
				if err != nil || result.GetResult().GetStatus() != "effect_unknown" || posts.Load() != 1 {
					var phase string
					_ = f.pool.QueryRow(context.Background(), `SELECT phase FROM execution_resource_dispatches WHERE authorization_id=$1`, original.AuthorizationId).Scan(&phase)
					t.Logf("private dispatch projection: phase=%s", phase)
					t.Fatalf("unchanged positive control failed: status=%s message=%s posts=%d rpc_error=%t", result.GetResult().GetStatus(), result.GetResult().GetError().GetMessage(), posts.Load(), err != nil)
				}
				t.Log("unchanged positive control: actual Broker/PG/Chromium recipient_posts=1")
				return
			}
			if err != nil || result.GetResult().GetStatus() != "failed" || posts.Load() != 0 {
				t.Fatal("invalid authority/binding reached recipient", kind, err)
			}
			g, err := f.auth.GetGrant(context.Background(), original.AuthorizationId)
			if err != nil || g.State != "authorized" {
				t.Fatal("denial consumed or replaced original grant")
			}
			var dispatches, reservations int
			if f.pool.QueryRow(context.Background(), `SELECT count(*) FROM action_effect_dispatches WHERE authorization_id=$1`, original.AuthorizationId).Scan(&dispatches) != nil || dispatches != 0 {
				t.Fatal("denied click crossed durable dispatch")
			}
			if f.pool.QueryRow(context.Background(), `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations) != nil || reservations != 1 {
				t.Fatal("denial charged another dispatch or erased opening cost")
			}
			t.Logf("case=%s recipient_posts=0 original_grant=authorized dispatches=0 retained_open_reservations=1", kind)
		})
	}
}

func TestBrowserSafetyBrokerProcessRestartKeepsUnknownChromiumIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("isolated PG and actual Chromium required")
	}
	for _, kind := range []string{"after_opaque_ack", "during_lost_ack"} {
		t.Run(kind, func(t *testing.T) {
			var posts atomic.Int32
			applied, release := make(chan struct{}, 1), make(chan struct{})
			var releaseOnce sync.Once
			finish := func() { releaseOnce.Do(func() { close(release) }) }
			defer finish()
			site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if r.Method == "POST" {
					posts.Add(1)
					select {
					case applied <- struct{}{}:
					default:
					}
					if kind == "during_lost_ack" {
						select {
						case <-release:
						case <-r.Context().Done():
						}
						conn, _, err := http.NewResponseController(w).Hijack()
						if err == nil {
							conn.Close()
						}
						return
					}
					fmt.Fprint(w, "saved")
					return
				}
				fmt.Fprint(w, `<form method=post action=/effect><button id=submit>Send</button></form>`)
			}))
			defer func() { finish(); site.Close() }()
			t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
			f := resourceBroker(t, nil, false)
			id, url, version := browserSafetyOpen(t, f)
			browserSafetyMandate(t, f)
			client, ctx, stop := browserSafetyBrokerProcess(t, f)
			args := fmt.Sprintf(`{"browser_session_id":%q,"expected_url":%q,"observation_version":%d,"selector":"#submit"}`, id, url, version)
			original := browserSafetyGrant(t, client, ctx, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "single-click", ToolName: "browser.click", AutonomyMode: 3, ArgsJson: args})
			if kind == "during_lost_ack" {
				done := make(chan struct{})
				result := make(chan *toolbrokerv1.ExecuteToolCallResponse, 1)
				go func() { defer close(done); r, _ := client.ExecuteToolCall(ctx, original); result <- r }()
				select {
				case <-applied:
				case r := <-result:
					t.Fatalf("click stopped before recipient: status=%s message=%s", r.GetResult().GetStatus(), r.GetResult().GetError().GetMessage())
				case <-time.After(10 * time.Second):
					t.Fatal("POST never reached real recipient")
				}
				stop()
				finish()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("dead Broker RPC remained active")
				}
			} else {
				r, err := client.ExecuteToolCall(ctx, original)
				if err != nil || r.GetResult().GetStatus() != "effect_unknown" || posts.Load() != 1 {
					t.Fatalf("opaque click failed: status=%s message=%s posts=%d rpc_error=%t", r.GetResult().GetStatus(), r.GetResult().GetError().GetMessage(), posts.Load(), err != nil)
				}
				frozen := f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "independent-read", ToolName: "browser.freeze_snapshot", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"browser_session_id":%q}`, id)})
				r, err = f.server.ExecuteToolCall(context.Background(), frozen)
				if err != nil || r.GetResult().GetStatus() != "completed" {
					t.Fatal("real frozen read failed")
				}
				stop()
			}
			g, err := f.auth.GetGrant(context.Background(), original.AuthorizationId)
			if err != nil {
				t.Fatal(err)
			}
			i, err := f.auth.GetIntent(context.Background(), g.Binding.ActionID)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "during_lost_ack" {
				if err = f.auth.MarkInterruptedEffect(context.Background(), i.EffectID); err != nil {
					t.Fatal("owning interrupted-dispatch recording failed", err)
				}
			}
			before, err := f.auth.GetEffect(context.Background(), i.EffectID)
			if err != nil || before.State != actionauth.StateUnknown {
				t.Fatal("snapshot/crash upgraded original effect")
			}
			var reservations int
			if f.pool.QueryRow(context.Background(), `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations) != nil {
				t.Fatal("spend unavailable")
			}
			client, ctx, _ = browserSafetyBrokerProcess(t, f)
			r, err := client.ExecuteToolCall(ctx, original)
			if err != nil || r.GetResult().GetStatus() != "effect_unknown" || r.GetResult().GetEffectState() != "effect_unknown" || posts.Load() != 1 {
				t.Fatal("new Broker replayed or upgraded original dispatch")
			}
			next := browserSafetyGrant(t, client, ctx, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "successor-click", ToolName: "browser.click", AutonomyMode: 3, ArgsJson: args})
			r, err = client.ExecuteToolCall(ctx, next)
			if err != nil || r.GetResult().GetStatus() == "completed" || posts.Load() != 1 {
				t.Fatal("restart allowed a successor POST")
			}
			after, err := f.auth.GetEffect(context.Background(), i.EffectID)
			if err != nil || after.State != before.State || after.AuthorizationID != before.AuthorizationID {
				t.Fatal("restart replaced journal identity/state")
			}
			var now, dispatches int
			if f.pool.QueryRow(context.Background(), `SELECT count(*) FROM execution_budget_reservations`).Scan(&now) != nil || now != reservations {
				t.Fatal("restart refunded or repeated spending")
			}
			if f.pool.QueryRow(context.Background(), `SELECT count(*) FROM action_effect_dispatches WHERE authorization_id=$1`, original.AuthorizationId).Scan(&dispatches) != nil || dispatches != 1 {
				t.Fatal("restart replaced dispatch history")
			}
			t.Logf("case=%s recipient_posts=1 original_effect=effect_unknown original_dispatches=1 retained_reservations=%d broker_process_restarted=true", kind, reservations)
		})
	}
}

// Kill the actual gRPC browser process, not a fake executor or Broker. Its
// restarted helper has a fresh in-memory context map; PG cannot recreate DOM.
func TestBrowserSafetyRuntimeLossChromiumIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("isolated PG and actual Chromium required")
	}
	t.Run("fresh_context_required", func(t *testing.T) {
		var posts atomic.Int32
		site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.Method == "POST" {
				posts.Add(1)
			}
			fmt.Fprint(w, `<p>Read-only fixture</p>`)
		}))
		defer site.Close()
		t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
		f := resourceBroker(t, nil, false)
		old, _, _ := browserSafetyOpen(t, f)
		f.stopRuntime()
		f.startRuntime()
		router := runtimeclient.New(nil)
		defer router.Close()
		contract, _ := f.server.registry.Get("browser.open")
		if router.SetTargetAuthentication(contract.RuntimeTarget, strings.Repeat("fixture-private-resource-", 3)) != nil {
			t.Fatal("runtime auth unavailable")
		}
		executor, err := NewResourceExecutor(f.store, router)
		if err != nil {
			t.Fatal(err)
		}
		f.server.executor = executor
		stale := f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "stale-context-read", ToolName: "browser.snapshot", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"browser_session_id":%q}`, old)})
		r, err := f.server.ExecuteToolCall(context.Background(), stale)
		if err != nil || r.GetResult().GetStatus() == "completed" {
			t.Fatal("runtime restored an old context from PG")
		}
		fresh, _, _ := browserSafetyOpenNamed(t, f, "fresh-context-open")
		if fresh == old || posts.Load() != 0 {
			t.Fatal("new read reused old context or wrote to site")
		}
		t.Log("runtime_process_killed=true restarted=true stale_context_not_completed=true fresh_context_distinct=true recipient_posts=0")
	})
	for _, kind := range []string{"before_dispatch", "after_recipient_post"} {
		t.Run(kind, func(t *testing.T) {
			var posts atomic.Int32
			applied, release := make(chan struct{}, 1), make(chan struct{})
			var once sync.Once
			finish := func() { once.Do(func() { close(release) }) }
			site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if r.Method == "POST" {
					posts.Add(1)
					applied <- struct{}{}
					select {
					case <-release:
					case <-r.Context().Done():
					}
					fmt.Fprint(w, "saved")
					return
				}
				fmt.Fprint(w, `<form method=post action=/effect><button id=submit>Send</button></form>`)
			}))
			defer func() { finish(); site.Close() }()
			t.Setenv("BUTLER_RESOURCE_BROWSER_FIXTURE", site.URL)
			f := resourceBroker(t, nil, false)
			id, url, version := browserSafetyOpen(t, f)
			browserSafetyMandate(t, f)
			client, ctx, _ := browserSafetyBrokerProcess(t, f)
			args := fmt.Sprintf(`{"browser_session_id":%q,"expected_url":%q,"observation_version":%d,"selector":"#submit"}`, id, url, version)
			original := browserSafetyGrant(t, client, ctx, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "runtime-loss-click", ToolName: "browser.click", AutonomyMode: 3, ArgsJson: args})
			if kind == "before_dispatch" {
				f.stopRuntime()
			}
			done := make(chan *toolbrokerv1.ExecuteToolCallResponse, 1)
			go func() { r, _ := client.ExecuteToolCall(ctx, original); done <- r }()
			if kind == "after_recipient_post" {
				select {
				case <-applied:
				case <-time.After(10 * time.Second):
					t.Fatal("POST never reached recipient")
				}
				f.stopRuntime()
				finish()
			}
			select {
			case r := <-done:
				if r.GetResult().GetStatus() == "completed" {
					t.Fatal("lost runtime became success")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("runtime loss did not settle Broker call")
			}
			g, err := f.auth.GetGrant(context.Background(), original.AuthorizationId)
			if err != nil {
				t.Fatal(err)
			}
			i, err := f.auth.GetIntent(context.Background(), g.Binding.ActionID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := f.auth.GetEffect(context.Background(), i.EffectID)
			if err != nil || before.State != actionauth.StateUnknown {
				t.Fatal("runtime loss was not durably unknown")
			}
			var spent int
			if f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&spent) != nil || spent != 2 {
				t.Fatal("lost dispatch spending was not retained")
			}
			f.startRuntime()
			// A fresh Broker connection avoids depending on gRPC reconnect backoff.
			client, ctx, _ = browserSafetyBrokerProcess(t, f)
			r, err := client.ExecuteToolCall(ctx, original)
			wantPosts := int32(0)
			if kind == "after_recipient_post" {
				wantPosts = 1
			}
			if err != nil || r.GetResult().GetStatus() != "effect_unknown" || posts.Load() != wantPosts {
				t.Fatal("restart replayed or upgraded lost dispatch")
			}
			next := browserSafetyGrant(t, client, ctx, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "runtime-loss-successor", ToolName: "browser.click", AutonomyMode: 3, ArgsJson: args})
			r, err = client.ExecuteToolCall(ctx, next)
			if err != nil || r.GetResult().GetStatus() == "completed" || posts.Load() != wantPosts {
				t.Fatal("stale context allowed another POST")
			}
			after, err := f.auth.GetEffect(ctx, i.EffectID)
			var now, dispatches int
			if err != nil || after.State != before.State || after.AuthorizationID != before.AuthorizationID || f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&now) != nil || now != spent || f.pool.QueryRow(ctx, `SELECT count(*) FROM action_effect_dispatches WHERE authorization_id=$1`, original.AuthorizationId).Scan(&dispatches) != nil || dispatches != 1 {
				t.Fatal("runtime restart changed original identity/history/spend")
			}
			t.Logf("case=%s runtime_process_killed=true restarted=true recipient_posts=%d original_effect=effect_unknown retained_reservations=%d original_dispatches=1", kind, posts.Load(), spent)
		})
	}
}
