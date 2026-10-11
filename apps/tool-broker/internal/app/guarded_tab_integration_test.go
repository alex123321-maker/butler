package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/singletabcontrol"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

func guardedBootstrapDatabase(t *testing.T) string {
	t.Helper()
	base := os.Getenv("BUTLER_TEST_ACTIONAUTH_POSTGRES_URL")
	if base == "" {
		t.Skip("actual disposable PostgreSQL fixture not configured")
	}
	u, err := url.Parse(base)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(u.Path, "/"), "_test") {
		t.Fatal("disposable test database required")
	}
	admin, err := pgxpool.New(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("guarded_boot_%d_test", time.Now().UnixNano())
	if _, err = admin.Exec(context.Background(), "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, e := admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close()
		if e != nil {
			t.Error("disposable database cleanup failed")
		}
	})
	u.Path = "/" + name
	store, err := postgresstore.Open(context.Background(), postgresstore.Config{URL: u.String(), MaxConns: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RunMigrations(context.Background(), "../../../../migrations"); err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()
	return u.String()
}

func TestGuardedTabRuntimeBootstrap(t *testing.T) {
	dsn := guardedBootstrapDatabase(t)
	fixturePool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fixturePool.Close)
	token := strings.Repeat("synthetic-runtime-", 3)
	serviceToken := strings.Repeat("synthetic-owner-control-", 2)
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	registryPath := write("registry.json", `{"tools":[]}`)
	privateToken := write("runtime-token", token)
	controlToken := write("control-token", serviceToken)
	manifestPath := filepath.Join(dir, "fixture-deletions.log")
	if err = catalog.NewFileManifest(manifestPath).Initialize(); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	health, err := resourceexec.NewSingleTabHealth(l.Addr().String(), token, func(context.Context) (string, error) { return "installed-fixture-host", nil })
	if err != nil {
		t.Fatal(err)
	}
	runtime := grpc.NewServer()
	healthv1.RegisterHealthServer(runtime, health)
	defer runtime.Stop()
	contextServer := httptest.NewServer(actionauth.NewContextHandler(serviceToken, guardedBootstrapContexts{pool: fixturePool}))
	defer contextServer.Close()
	port := func() string {
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		s := l.Addr().String()
		l.Close()
		return s
	}
	httpAddr, rpcAddr := port(), port()
	authority, err := resourceexec.NewHTTPAuthority("http://"+httpAddr+"/internal/guarded-tab-resources", token)
	if err != nil {
		t.Fatal(err)
	}
	page := &guardedBootstrapPage{}
	guarded, err := resourceexec.NewRuntime(page, authority, token)
	if err != nil {
		t.Fatal(err)
	}
	runtimev1.RegisterToolRuntimeServiceServer(runtime, guarded)
	go runtime.Serve(l)
	env := map[string]string{"BUTLER_POSTGRES_URL": dsn, "BUTLER_POSTGRES_MAX_CONNS": "2", "BUTLER_POSTGRES_MIN_CONNS": "0", "BUTLER_TOOL_REGISTRY_PATH": registryPath, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE": controlToken, "BUTLER_ACTION_CONTEXT_URL": contextServer.URL, "BUTLER_GUARDED_TAB_REQUEST_ENABLED": "true", "BUTLER_GUARDED_TAB_REQUEST_CONTROL_URL": contextServer.URL + "/internal/guarded-tab-request", "BUTLER_GUARDED_TAB_RUNTIME_ENABLED": "true", "BUTLER_GUARDED_TAB_RUNTIME_TARGET": l.Addr().String(), "BUTLER_GUARDED_TAB_RUNTIME_TOKEN_FILE": privateToken, "BUTLER_GUARDED_TAB_HOST": "installed-fixture-host", "BUTLER_GUARDED_TAB_PROFILE_ID": "stable-fixture-profile", "BUTLER_GUARDED_TAB_ALLOWED_ORIGINS_JSON": `["https://example.invalid"]`, "BUTLER_HTTP_ADDR": httpAddr, "BUTLER_GRPC_ADDR": rpcAddr}
	for key, value := range env {
		t.Setenv(key, value)
	}
	t.Setenv("BUTLER_MEMORY_DELETION_MANIFEST_FILE", manifestPath)
	for _, scenario := range []string{"wrong_host", "wrong_token", "lan_target", "legacy_registry", "missing_origin_policy", "missing_request", "wildcard_authority", "wildcard_broker", "invalid_deletion_manifest"} {
		t.Run(scenario, func(t *testing.T) {
			switch scenario {
			case "wrong_host":
				t.Setenv("BUTLER_GUARDED_TAB_HOST", "different-host")
			case "wrong_token":
				t.Setenv("BUTLER_GUARDED_TAB_RUNTIME_TOKEN_FILE", write("wrong-token", strings.Repeat("wrong-", 8)))
			case "lan_target":
				t.Setenv("BUTLER_GUARDED_TAB_RUNTIME_TARGET", "192.168.1.2:19090")
			case "legacy_registry":
				t.Setenv("BUTLER_TOOL_REGISTRY_PATH", write("legacy.json", `{"tools":[{"tool_name":"single_tab.bind","runtime_target":"127.0.0.1:19090"}]}`))
			case "missing_origin_policy":
				t.Setenv("BUTLER_GUARDED_TAB_ALLOWED_ORIGINS_JSON", "[]")
			case "missing_request":
				t.Setenv("BUTLER_GUARDED_TAB_REQUEST_ENABLED", "false")
			case "wildcard_authority":
				t.Setenv("BUTLER_HTTP_ADDR", "0.0.0.0:8080")
			case "wildcard_broker":
				t.Setenv("BUTLER_GRPC_ADDR", "0.0.0.0:9090")
			case "invalid_deletion_manifest":
				t.Setenv("BUTLER_MEMORY_DELETION_MANIFEST_FILE", write("invalid-anchor", ""))
			}
			app, err := New(context.Background())
			if err == nil {
				app.shutdown(nil)
				t.Fatal("incomplete guarded bootstrap accepted")
			}
		})
	}
	app, err := New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(15 * time.Second):
			t.Error("Broker shutdown did not finish")
		}
	}()
	conn, err := grpc.NewClient(rpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := toolbrokerv1.NewToolBrokerServiceClient(conn)
	callCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	privateCtx := metadata.AppendToOutgoingContext(callCtx, "authorization", "Bearer "+serviceToken)
	for _, scope := range []string{"catalog-runtime", "catalog-disabled"} {
		list, e := client.ListTools(privateCtx, &toolbrokerv1.ListToolsRequest{RunId: scope})
		if e != nil {
			t.Fatal("composed scoped catalog unavailable", e)
		}
		guarded := 0
		for _, c := range list.GetTools() {
			if c.ContractRevision == resourceexec.SingleTabContractRevision {
				guarded++
				if !resourceexec.ExactSingleTabContract(c) {
					t.Fatal("startup catalog contract differs from installed runtime")
				}
			}
		}
		if scope == "catalog-runtime" && guarded != 2 || scope == "catalog-disabled" && guarded != 0 {
			t.Fatal("owning runtime readiness did not gate discovery", scope, guarded)
		}
	}
	if _, e := client.ExecuteToolCall(callCtx, &toolbrokerv1.ExecuteToolCallRequest{}); e == nil {
		t.Fatal("anonymous execution admitted")
	}
	if _, e := client.PrepareToolCall(privateCtx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: &toolbrokerv1.ToolCall{RunId: "catalog-disabled", AutonomyMode: 3, ToolCallId: "disabled-guarded-status", ToolName: "single_tab.status", ArgsJson: `{"session_id":"opaque-selected-tab"}`}}); e == nil || !strings.Contains(e.Error(), "guarded tab runtime composition is unavailable") {
		t.Fatal("owning readiness did not deny direct prepare", e)
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, "http://"+httpAddr+"/internal/guarded-tab-resources", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	// The gRPC listener is ready first; the HTTP goroutine may be scheduled later.
	var response *http.Response
	for i := 0; i < 50; i++ {
		response, err = http.DefaultClient.Do(req.Clone(callCtx))
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("resource authority accepted anonymous caller")
	}
	if _, e := fixturePool.Exec(context.Background(), `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:fixture','fixture','web');
INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('app-task','web:fixture','fixture','Observe approved tab');
INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,lease_id,started_at,updated_at,task_id,task_revision) VALUES('app-run','web:fixture','fixture-event','running','mode_2','tool_running','scripted','app-lease',clock_timestamp(),clock_timestamp(),'app-task',1);
INSERT INTO session_ownership(session_key,lease_id,run_id,owner_id,fence_epoch,expires_at) VALUES('web:fixture','app-lease','app-run','fixture',1,clock_timestamp()+INTERVAL '10 minutes')`); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(fixturePool, catalog.NewFileManifest(manifestPath))
	sourceGuard := func(ctx context.Context, tx pgx.Tx, run string) error { return sources.CheckRunSourcesTx(ctx, tx, run) }
	owned := ownership.WithToken(context.Background(), nil, ownership.Token{SessionKey: "web:fixture", RunID: "app-run", LeaseID: "app-lease", FenceEpoch: 1})
	owned = owneridentity.WithActor(owned, owneridentity.Actor{OwnerID: "fixture", Channel: "web", SessionKey: "web:fixture", DeliveryID: "independent-test-owner-selection"})
	binding := resources.Binding{OwnerID: "fixture", SessionKey: "web:fixture", RunID: "app-run", TaskID: "app-task", TaskRevision: 1, ConstraintsRevision: 1, LeaseEpoch: 1, Deadline: time.Now().Add(3 * time.Minute)}
	for _, origin := range []string{"https://example.invalid", "https://blocked.example"} {
		controller, e := singletabcontrol.New(fixturePool, guardedBootstrapTabs{origin: origin}, resources.WorkerGuard{}, sourceGuard, "installed-fixture-host")
		if e != nil {
			t.Fatal(e)
		}
		choices, e := controller.Request(owned, binding)
		if e != nil {
			t.Fatal("actual owner request denied", e)
		}
		selected, e := controller.Select(owned, binding, choices.ApprovalID, choices.Candidates[0].Token)
		if e != nil {
			t.Fatal("actual owner choice denied", e)
		}
		id := "allowed-status"
		if origin == "https://blocked.example" {
			id = "blocked-status"
		}
		call := &toolbrokerv1.ToolCall{RunId: "app-run", ToolCallId: id, ToolName: "single_tab.status", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"session_id":%q}`, selected)}
		if _, e = (guardedBootstrapContexts{pool: fixturePool}).GetActionContext(context.Background(), "app-run"); e != nil {
			t.Fatal("actual private projection read failed", e)
		}
		prepared, e := client.PrepareToolCall(privateCtx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
		if e != nil || prepared.GetError() != nil {
			t.Fatal("actual App prepare denied", e, prepared.GetError())
		}
		grant, e := client.AuthorizeToolCall(privateCtx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: prepared.GetAction().GetActionId()})
		if e != nil {
			t.Fatal("actual App authorization denied", e)
		}
		result, e := client.ExecuteToolCall(privateCtx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grant.GetAuthorizationId()})
		if e != nil {
			t.Fatal("actual private dispatch failed", e)
		}
		if (result.GetResult().GetStatus() == "completed") != (origin == "https://example.invalid") {
			t.Fatal("exact origin policy did not guard runtime", origin, result.GetResult().GetError())
		}
		if e = controller.Revoke(owned, binding, choices.ApprovalID); e != nil {
			t.Fatal("actual owner revoke denied", e)
		}
	}
	if page.calls.Load() != 1 {
		t.Fatal("blocked origin reached private backend")
	}
	var dispatches, reservations int
	if e := fixturePool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM execution_resource_dispatches),(SELECT count(*) FROM execution_budget_reservations)`).Scan(&dispatches, &reservations); e != nil || dispatches != 1 || reservations != 1 {
		t.Fatal("actual resource/shared budget admission was not atomic", e, dispatches, reservations)
	}
	t.Log("actual App.New/Run: immutable PG65 + exact authenticated guarded health + private resource route + anonymous gRPC denial; extension readiness is scripted")
}

type guardedBootstrapContexts struct{ pool *pgxpool.Pool }

func (guardedBootstrapContexts) VerifyActionApproval(context.Context, actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	return actionauth.ApprovalSnapshot{}, resourceexec.ErrDenied
}

func (f guardedBootstrapContexts) GetActionContext(ctx context.Context, run string) (actionauth.ContextSnapshot, error) {
	if run == "app-run" {
		c := actionauth.ContextSnapshot{RunID: run, AutonomyMode: 3, GuardedTabRuntimeReady: true, GuardedTabRequestReady: true}
		e := f.pool.QueryRow(ctx, `SELECT r.current_state,r.session_key,s.user_id,r.task_id,t.status,t.revision,r.task_revision,t.constraints_revision,o.lease_id,o.fence_epoch,o.expires_at FROM runs r JOIN sessions s USING(session_key) JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.session_key=r.session_key WHERE r.run_id=$1 AND o.run_id=r.run_id`, run).Scan(&c.RunState, &c.SessionKey, &c.OwnerID, &c.TaskID, &c.TaskStatus, &c.TaskRevision, &c.PinnedTaskRevision, &c.ConstraintsRevision, &c.LeaseID, &c.LeaseEpoch, &c.LeaseExpiresAt)
		return c, e
	}
	if run != "catalog-runtime" && run != "catalog-disabled" {
		return actionauth.ContextSnapshot{}, resourceexec.ErrDenied
	}
	return actionauth.ContextSnapshot{RunID: run, SessionKey: "web:fixture", OwnerID: "fixture", LeaseID: "catalog-fixture", LeaseEpoch: 1, LeaseExpiresAt: time.Now().Add(time.Minute), RunState: "tool_pending", AutonomyMode: 3, TaskID: "fixture-task", TaskStatus: "active", TaskRevision: 1, PinnedTaskRevision: 1, ConstraintsRevision: 1, GuardedTabRequestReady: true, GuardedTabRuntimeReady: run == "catalog-runtime"}, nil
}

type guardedBootstrapPage struct{ calls atomic.Int32 }

func (p *guardedBootstrapPage) Execute(_ context.Context, r *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	p.calls.Add(1)
	c := r.GetToolCall()
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: c.RunId, ToolCallId: c.ToolCallId, ToolName: c.ToolName, Status: "completed", ResultJson: `{"ok":true,"observation_id":"01234567-89ab-cdef-0123-456789abcdef","observation_version":1}`}}, nil
}

type guardedBootstrapTabs struct{ origin string }

func (m guardedBootstrapTabs) List(context.Context) ([]singletabcontrol.Tab, error) {
	return []singletabcontrol.Tab{{BrowserInstance: "installed-fixture-host", Ref: "1", Origin: m.origin}}, nil
}
func (guardedBootstrapTabs) Check(context.Context, singletabcontrol.Tab) error { return nil }
