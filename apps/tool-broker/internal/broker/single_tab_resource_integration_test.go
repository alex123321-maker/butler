package broker

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
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
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
)

func singleTabBroker(t *testing.T, backend resourceexec.RuntimeBackend, start func(target, authority, tokenFile string) (string, string, string)) *resourceBrokerFixture {
	return singleTabBrokerWithGuard(t, backend, start, nil)
}
func singleTabBrokerWithGuard(t *testing.T, backend resourceexec.RuntimeBackend, start func(target, authority, tokenFile string) (string, string, string), pause resourceexec.SourceGuard) *resourceBrokerFixture {
	t.Helper()
	pool := resourceBrokerDB(t)
	for _, name := range []string{"022_approvals_and_approval_events.up.sql", "027_single_tab_sessions_and_web_fetch_cache.up.sql", "028_typed_approvals.up.sql", "063_single_tab_resource_binding.up.sql", "064_guarded_tab_owner_choices.up.sql"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(context.Background(), string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	target := listener.Addr().String()
	contracts := resourceexec.SingleTabContracts(target)
	hashes := map[string]string{}
	for _, c := range contracts {
		hashes[c.ToolName], _ = actionauth.ContractHash(c)
	}
	policy := budget.DefaultPolicy()
	policy.ToolMillis = 30000
	budgets := budget.NewController(pool, policy)
	core, e := resources.New(resources.WorkerGuard{}, denyResourceReconcile{})
	if e != nil {
		t.Fatal(e)
	}
	source := func(ctx context.Context, tx pgx.Tx, run string) error {
		var current bool
		if tx.QueryRow(ctx, `SELECT status='current' FROM current_resource_sources WHERE run_id=$1 FOR SHARE`, run).Scan(&current) != nil || !current {
			return resourceexec.ErrDenied
		}
		if pause != nil {
			return pause(ctx, tx, run)
		}
		return nil
	}
	guard := singletabcontrol.Guard("actual-fixed-host")
	store, e := resourceexec.NewSingleTab(pool, core, budgets, source, guard, resourceexec.Profile{ID: "fixed-user-browser", RuntimeTarget: target, ContractHashes: hashes})
	if e != nil {
		t.Fatal(e)
	}
	token := strings.Repeat("single-tab-fixture-", 3)
	handler, e := resourceexec.NewHTTPHandler(store, token)
	if e != nil {
		t.Fatal(e)
	}
	endpoint := httptest.NewServer(handler)
	t.Cleanup(endpoint.Close)
	authority, e := resourceexec.NewHTTPAuthority(endpoint.URL, token)
	if e != nil {
		t.Fatal(e)
	}
	var metadata singletabcontrol.Metadata = fixedTabMetadata{}
	if start != nil {
		listener.Close()
		file := filepath.Join(t.TempDir(), "private-fixture-token")
		if os.WriteFile(file, []byte(token), 0600) != nil {
			t.Fatal("fixture token file unavailable")
		}
		_, _, metadataURL := start(target, endpoint.URL, file)
		if e = resourceexec.CheckSingleTabReadiness(context.Background(), target, "actual-fixed-host", token); e != nil {
			t.Fatal("installed native contract readiness denied", e)
		}
		if resourceexec.CheckSingleTabReadiness(context.Background(), target, "replacement-host", token) == nil {
			t.Fatal("installed native host accepted replacement identity")
		}
		metadata, e = singletabcontrol.NewMetadataClient(metadataURL, token)
		if e != nil {
			t.Fatal(e)
		}
	} else {
		wrapped, e := resourceexec.NewRuntime(backend, authority, token)
		if e != nil {
			t.Fatal(e)
		}
		rpc := grpc.NewServer()
		runtimev1.RegisterToolRuntimeServiceServer(rpc, wrapped)
		go rpc.Serve(listener)
		t.Cleanup(func() { rpc.Stop(); listener.Close() })
	}
	selection, e := singletabcontrol.New(pool, metadata, resources.WorkerGuard{}, source, "actual-fixed-host")
	if e != nil {
		t.Fatal(e)
	}
	ctx, binding := singleTabOwnerContext()
	choices, e := selection.Request(ctx, binding)
	if e != nil || len(choices.Candidates) < 1 || len(choices.Candidates) > singletabcontrol.MaxTabs {
		t.Fatal("actual owner candidates denied", e)
	}
	if len(choices.Candidates) > 1 {
		if len(choices.Candidates) != 2 || choices.Candidates[0].Origin != choices.Candidates[1].Origin || choices.Candidates[0].Ordinal == choices.Candidates[1].Ordinal || choices.Candidates[0].Token == choices.Candidates[1].Token || !choices.Candidates[0].Active || choices.Candidates[1].Active {
			t.Fatal("same-origin owner choices not distinguishable")
		}
		t.Log("actual two same-origin tabs: opaque choices, distinct ordinals and one active marker; no native IDs")
	}
	if _, e = selection.Select(context.Background(), binding, choices.ApprovalID, choices.Candidates[0].Token); e == nil {
		t.Fatal("anonymous selection authorized")
	}
	selected, e := selection.Select(ctx, binding, choices.ApprovalID, choices.Candidates[0].Token)
	if e != nil {
		t.Fatal("actual owner selection denied", e)
	}
	if repeat, e := selection.Select(ctx, binding, choices.ApprovalID, choices.Candidates[0].Token); e != nil || repeat != selected {
		t.Fatal("exact selection retry changed binding", e)
	}
	t.Log("owner selection: actual fixed metadata -> opaque candidates -> current atomic approval/binding; no seeded selection")
	router := runtimeclient.New(nil)
	t.Cleanup(func() { router.Close() })
	if router.SetTargetAuthentication(target, token) != nil {
		t.Fatal("private router unavailable")
	}
	executor, e := NewResourceExecutor(store, router)
	if e != nil {
		t.Fatal(e)
	}
	data, _ := json.Marshal(map[string]any{"tools": contracts})
	file := filepath.Join(t.TempDir(), "tools.json")
	if e = os.WriteFile(file, data, 0600); e != nil {
		t.Fatal(e)
	}
	reg, e := registry.Load(file, target)
	if e != nil {
		t.Fatal(e)
	}
	reader := resourceReader{authFixture: newAuthFixture(), pool: pool, guardedReady: true}
	auth := actionauth.NewPostgresStore(pool)
	auth.SetBudgets(budgets)
	auth.SetRunDataGuard(source)
	auth.SetDispatchClaimGuard(store)
	server := NewServer(reg, executor, nil, nil)
	server.SetActionAuthorizer(actionauth.NewService(auth, reader, nil))
	server.SetBudgets(budgets)
	server.SetResourceAdmission(store)
	return &resourceBrokerFixture{pool: pool, server: server, auth: auth, reader: reader, store: store, budgets: budgets, router: router, selectedTab: selected, selection: selection}
}

type fixedTabMetadata struct{}

func (fixedTabMetadata) List(context.Context) ([]singletabcontrol.Tab, error) {
	return []singletabcontrol.Tab{{BrowserInstance: "actual-fixed-host", Ref: "1", Origin: "https://example.invalid"}}, nil
}
func (fixedTabMetadata) Check(context.Context, singletabcontrol.Tab) error { return nil }
func singleTabOwnerContext() (context.Context, resources.Binding) {
	ctx := ownership.WithToken(context.Background(), nil, ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "fixture-lease", FenceEpoch: 1})
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", SessionKey: "web:owner", DeliveryID: "authenticated-fixture-selection"})
	return ctx, resources.Binding{OwnerID: "owner", SessionKey: "web:owner", RunID: "run", TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1, LeaseEpoch: 1, Deadline: time.Now().Add(10 * time.Minute)}
}

type singleTabPageBackend struct{ calls atomic.Int32 }

func (b *singleTabPageBackend) Execute(_ context.Context, r *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	b.calls.Add(1)
	c := r.GetToolCall()
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: c.RunId, ToolCallId: c.ToolCallId, ToolName: c.ToolName, Status: "completed", ResultJson: `{"ok":true,"observation_id":"01234567-89ab-cdef-0123-456789abcdef","observation_version":1}`}}, nil
}
func singleTabRequest(t *testing.T, f *resourceBrokerFixture, id, tool, args string) *toolbrokerv1.ExecuteToolCallRequest {
	t.Helper()
	args = strings.ReplaceAll(args, "approved-user-tab", f.selectedTab)
	return f.callRequest(t, &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: id, ToolName: tool, ArgsJson: args, AutonomyMode: 3})
}
func TestSingleTabResourceActualPGCurrentnessAndAtomicBudget(t *testing.T) {
	for _, change := range []string{"valid", "foreign", "revoked", "expired", "source_deleted", "owner_fenced", "task_amended"} {
		t.Run(change, func(t *testing.T) {
			backend := &singleTabPageBackend{}
			f := singleTabBroker(t, backend, nil)
			request := singleTabRequest(t, f, "observe", "single_tab.status", `{"session_id":"approved-user-tab"}`)
			query := map[string]string{"foreign": `UPDATE single_tab_sessions SET created_by_run_id='run2'`, "revoked": `UPDATE guarded_single_tab_requests SET state='revoked'`, "expired": `UPDATE single_tab_sessions SET expires_at=clock_timestamp()-INTERVAL '1 second'`, "source_deleted": `UPDATE current_resource_sources SET status='deleted' WHERE run_id='run'`, "owner_fenced": `UPDATE session_ownership SET fence_epoch=fence_epoch+1 WHERE run_id='run'`, "task_amended": `UPDATE tasks SET revision=revision+1 WHERE task_id='task'`}[change]
			if query != "" {
				if _, e := f.pool.Exec(context.Background(), query); e != nil {
					t.Fatal(e)
				}
			}
			result, e := f.server.ExecuteToolCall(context.Background(), request)
			if e != nil {
				t.Fatal(e)
			}
			if change == "valid" {
				if result.GetResult().GetStatus() != "completed" || backend.calls.Load() != 1 {
					t.Fatal("valid private dispatch denied", result.GetResult().GetError())
				}
				replay, _ := f.server.ExecuteToolCall(context.Background(), request)
				if replay.GetResult().GetStatus() == "completed" || backend.calls.Load() != 1 {
					t.Fatal("spent ticket replayed")
				}
				var count int
				f.pool.QueryRow(context.Background(), `SELECT count(*) FROM execution_resource_dispatches WHERE single_tab_binding IS NOT NULL AND phase='completed'`).Scan(&count)
				if count != 1 {
					t.Fatal("private session mapping absent")
				}
			} else {
				var reservations int
				f.pool.QueryRow(context.Background(), `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations)
				if result.GetResult().GetStatus() == "completed" || backend.calls.Load() != 0 || reservations != 0 {
					t.Fatal("stale/private resource consumed budget or executed", change)
				}
			}
		})
	}
}

func TestSingleTabResourceAfterClaimExpiryDoesNotReachRuntime(t *testing.T) {
	backend := &singleTabPageBackend{}
	f := singleTabBroker(t, backend, nil)
	req := singleTabRequest(t, f, "observe", "single_tab.status", `{"session_id":"approved-user-tab"}`)
	f.server.executor = beforeSingleRuntime{next: f.server.executor, change: func() {
		if _, e := f.pool.Exec(context.Background(), `UPDATE single_tab_sessions SET expires_at=clock_timestamp()-INTERVAL '1 second'`); e != nil {
			t.Fatal(e)
		}
	}}
	result, e := f.server.ExecuteToolCall(context.Background(), req)
	if e != nil || result.GetResult().GetStatus() != "effect_unknown" {
		t.Fatal("expired selected tab presented completed", e)
	}
	if backend.calls.Load() != 0 {
		t.Fatal("expired backend executed")
	}
}

type beforeSingleRuntime struct {
	next   RuntimeExecutor
	change func()
}

func TestSingleTabResourceMigrationKeepsBindingHistory(t *testing.T) {
	backend := &singleTabPageBackend{}
	f := singleTabBroker(t, backend, nil)
	read := func(name string) []byte {
		t.Helper()
		raw, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	ctx := context.Background()
	down := read("063_single_tab_resource_binding.down.sql")
	if _, e := f.pool.Exec(ctx, string(read("064_guarded_tab_owner_choices.down.sql"))); e == nil {
		t.Fatal("frozen owner choice history discarded")
	}
	if _, e := f.pool.Exec(ctx, string(down)); e == nil {
		t.Fatal("retained guarded selection downgrade accepted")
	}
	result, e := f.server.ExecuteToolCall(ctx, singleTabRequest(t, f, "observe", "single_tab.status", `{"session_id":"approved-user-tab"}`))
	if e != nil || result.GetResult().GetStatus() != "completed" {
		t.Fatal("binding proof unavailable", e)
	}
	if _, e = f.pool.Exec(ctx, string(down)); e == nil {
		t.Fatal("binding history downgrade accepted")
	}
	var count int
	if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_resource_dispatches WHERE single_tab_binding IS NOT NULL`).Scan(&count); e != nil || count != 1 {
		t.Fatal("binding history discarded", e, count)
	}
}

func (b beforeSingleRuntime) Execute(ctx context.Context, c *toolbrokerv1.ToolCall, k *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	b.change()
	return b.next.Execute(ctx, c, k, secrets)
}
