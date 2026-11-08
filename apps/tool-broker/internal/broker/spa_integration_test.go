package broker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserspa"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/credentials"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/metadata"
)

type spaPGReader struct {
	resourceReader
	assignment actionauth.Assignment
}

func (r spaPGReader) GetActionContext(ctx context.Context, id string) (actionauth.ContextSnapshot, error) {
	c, err := r.resourceReader.GetActionContext(ctx, id)
	c.Authority = &r.assignment
	c.MandateID = r.assignment.MandateID
	var input, claim string
	if err == nil {
		err = r.pool.QueryRow(ctx, `SELECT input_id,claim_id FROM accepted_inputs WHERE run_id=$1`, id).Scan(&input, &claim)
		c.InputClaimHash = actionauth.InputClaimHash(input, claim)
	}
	return c, err
}

type spaPGBackend struct {
	entered  chan struct{}
	release  chan struct{}
	target   string
	accepted atomic.Int32
}

func (b *spaPGBackend) Execute(ctx context.Context, r *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	call := r.GetToolCall()
	check, ok := resourceexec.LiveCheck(ctx)
	fail := func() (*runtimev1.ExecuteResponse, error) {
		return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "failed"}}, nil
	}
	if !ok || check(ctx, "https://rp.example/app/graphql") != nil {
		return fail()
	}
	b.accepted.Add(1)
	close(b.entered)
	select {
	case <-ctx.Done():
		return fail()
	case <-b.release:
	}
	if check(ctx, b.target) != nil {
		return fail()
	}
	b.accepted.Add(1)
	raw, _ := json.Marshal(map[string]any{"ok": true, "final_url": "https://rp.example/app/", "spa_profile": browserspa.Profile, "network_state": "frozen", "spa_network": browserspa.NetworkStats{Requests: 2, Queries: 1, CompletedQueries: 1, MeteredBytes: 1024}})
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: string(raw)}}, nil
}

type spaPGExecutor struct {
	backend *resourceexec.Runtime
	token   string
}

func (e spaPGExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, _ []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	// The production router's same private DTO/metadata contract; the native
	// matrix separately exercises the actual router/process/Chromium composition.
	identity := actionauth.DispatchIdentityFromContext(ctx)
	ctx = resourceexec.OutgoingContext(ctx)
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set("authorization", "Bearer "+e.token)
	ctx = metadata.NewIncomingContext(ctx, md)
	response, err := e.backend.Execute(ctx, &runtimev1.ExecuteRequest{ToolCall: call, Contract: c, Context: &runtimev1.ExecutionContext{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, RuntimeTarget: c.RuntimeTarget, ActionId: identity.ActionID, ActionFingerprint: identity.Fingerprint}})
	return response.GetResult(), err
}

type spaLoseFinish struct{ *resourceexec.Store }

func (spaLoseFinish) Finish(context.Context, resourceexec.Receipt) error {
	return resourceexec.ErrDenied
}

func spaPGGoal(t *testing.T, pool *pgxpool.Pool) actionauth.Assignment {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `ALTER TABLE accepted_inputs ADD COLUMN owner_id TEXT,ADD COLUMN run_id TEXT,ADD COLUMN session_key TEXT,ADD COLUMN task_id TEXT,ADD COLUMN status TEXT,ADD COLUMN claim_id TEXT,ADD COLUMN claim_expires_at TIMESTAMPTZ,ADD COLUMN expires_at TIMESTAMPTZ;INSERT INTO accepted_inputs(input_id,owner_id,run_id,session_key,task_id,status,claim_id,claim_expires_at,expires_at) VALUES('spa-input','owner','run','web:owner','task','running','spa-claim',clock_timestamp()+interval '1 hour',clock_timestamp()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m := actionauth.Mandate{ID: "spa-mandate", OwnerID: "owner", ScopeKind: "owner_goal", GoalID: "spa-goal", GoalRevision: 1, Revision: 1, Status: "active", Purpose: "spa-read", Tools: []string{browserspa.Tool}, Effects: []actionauth.Effect{actionauth.EffectRead}, ExpiresAt: now.Add(time.Hour), BudgetDeadline: now.Add(time.Hour), Budget: actionauth.MandateBudget{ExternalCalls: 10, ActiveMillis: 600000, EgressBytes: 64 * 1024 * 1024}, Privacy: "cloud_allowed", Intervention: []string{"effect_unknown", "scope_change", "budget_exhausted"}, Reporting: "exceptions", ResourceScopes: []actionauth.ResourceScope{{Origin: "https://rp.example", PathPrefix: "/app"}}, DenyResources: []actionauth.ResourceScope{{Origin: "https://rp.example", PathPrefix: "/app/denied"}}}
	g := actionauth.GoalPolicy{ID: m.GoalID, OwnerID: m.OwnerID, Kind: "objective", Status: "active", Revision: 1, Purpose: m.Purpose, ExpiresAt: m.ExpiresAt}
	mr, _ := json.Marshal(m)
	gr, _ := json.Marshal(g)
	_, err := pool.Exec(t.Context(), `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES($1,'owner','objective','active',1,$2,$3,'{}')`, g.ID, g.ExpiresAt, gr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(t.Context(), `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status) VALUES($1,'owner','',1,$2,$3,'owner_goal',$4,1,'active')`, m.ID, mr, m.ExpiresAt, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(t.Context(), `INSERT INTO owner_purpose_policies VALUES('owner','spa-read',0);INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref) VALUES('task','owner','spa-goal',1,'spa-mandate',1,'spa-read','owner-request','fixture')`)
	if err != nil {
		t.Fatal(err)
	}
	return actionauth.Assignment{TaskID: "task", OwnerID: "owner", GoalID: g.ID, GoalRevision: 1, MandateID: m.ID, MandateRevision: 1, Purpose: m.Purpose}
}

func TestSPABrokerRunningAuthorityAndNoReplayIntegration(t *testing.T) {
	for _, mode := range []string{"allowed", "path_denied", "foreign_origin", "cancelled", "revoked", "constraint_changed", "source_removed", "input_replaced", "input_expired", "lost_finish", "expired"} {
		t.Run(mode, func(t *testing.T) {
			pool := resourceBrokerDB(t)
			assignment := spaPGGoal(t, pool)
			policy := budget.DefaultPolicy()
			policy.ToolMillis = 30000
			budgets := budget.NewController(pool, policy)
			budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
			leases, err := resources.New(resources.WorkerGuard{}, denyResourceReconcile{})
			if err != nil {
				t.Fatal(err)
			}
			source := func(ctx context.Context, tx pgx.Tx, run string) error {
				var current bool
				err := tx.QueryRow(ctx, `SELECT status='current' FROM current_resource_sources WHERE run_id=$1`, run).Scan(&current)
				if err != nil || !current {
					return resourceexec.ErrDenied
				}
				return nil
			}
			c := browserspa.Contract()
			hash, _ := actionauth.ContractHash(c)
			store, err := resourceexec.New(pool, leases, budgets, source, resourceexec.Profile{ID: browserspa.Profile, RuntimeTarget: c.RuntimeTarget, ContractHashes: map[string]string{browserspa.Tool: hash}})
			if err != nil {
				t.Fatal(err)
			}
			var authority resourceexec.RuntimeAuthority = store
			if mode == "lost_finish" {
				authority = spaLoseFinish{store}
			}
			backend := &spaPGBackend{entered: make(chan struct{}), release: make(chan struct{}), target: "https://rp.example/app/graphql"}
			if mode == "path_denied" {
				backend.target = "https://rp.example/app/denied/data"
			}
			if mode == "foreign_origin" {
				backend.target = "https://foreign.example/app/graphql"
			}
			token := strings.Repeat("private-spa-fixture-", 3)
			guarded, err := resourceexec.NewRuntime(backend, authority, token)
			if err != nil {
				t.Fatal(err)
			}
			executor, err := NewSPAExecutor(store, spaPGExecutor{guarded, token})
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(map[string]any{"tools": []*toolbrokerv1.ToolContract{c}})
			registryPath := filepath.Join(t.TempDir(), "tools.json")
			if os.WriteFile(registryPath, raw, 0600) != nil {
				t.Fatal("registry fixture")
			}
			reg, err := registry.Load(registryPath, c.RuntimeTarget)
			if err != nil {
				t.Fatal(err)
			}
			reader := spaPGReader{resourceReader: resourceReader{authFixture: newAuthFixture(), pool: pool}, assignment: assignment}
			auth := actionauth.NewPostgresStore(pool)
			auth.SetBudgets(budgets)
			auth.SetRunDataGuard(source)
			admissions := ResourceAdmissions{ServerBrowser: true, SPA: &GuardedTabAdmission{Store: store}}
			auth.SetDispatchClaimGuard(admissions)
			server := NewServer(reg, executor, nil, nil)
			server.SetActionAuthorizer(actionauth.NewService(auth, reader, nil))
			server.SetBudgets(budgets)
			server.SetResourceAdmission(admissions)
			call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "spa-call", ToolName: browserspa.Tool, AutonomyMode: 3, ArgsJson: `{"url":"https://rp.example/app/"}`}
			prepared, err := server.PrepareToolCall(t.Context(), &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
			if err != nil || prepared.GetError() != nil {
				t.Fatal("SPA prepare", err, prepared.GetError())
			}
			grant, err := server.AuthorizeToolCall(t.Context(), &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: prepared.GetAction().GetActionId()})
			if err != nil || grant.GetError() != nil {
				t.Fatal("SPA authorize", err, grant.GetError())
			}
			request := &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grant.GetAuthorizationId()}
			type execution struct {
				response *toolbrokerv1.ExecuteToolCallResponse
				err      error
			}
			done := make(chan execution, 1)
			go func() {
				response, err := server.ExecuteToolCall(t.Context(), request)
				done <- execution{response, err}
			}()
			select {
			case <-backend.entered:
			case x := <-done:
				t.Fatal("backend not admitted", x.err, x.response.GetResult().GetError())
			case <-time.After(25 * time.Second):
				t.Fatal("SPA admission stalled")
			}
			switch mode {
			case "cancelled":
				_, err = pool.Exec(t.Context(), `UPDATE tasks SET status='cancelled' WHERE task_id='task'`)
			case "revoked":
				_, err = pool.Exec(t.Context(), `UPDATE action_mandates SET status='revoked',revoked_at=clock_timestamp() WHERE mandate_id='spa-mandate'`)
			case "constraint_changed":
				_, err = pool.Exec(t.Context(), `UPDATE tasks SET constraints_revision=2 WHERE task_id='task'`)
			case "source_removed":
				_, err = pool.Exec(t.Context(), `UPDATE current_resource_sources SET status='revoked' WHERE run_id='run'`)
			case "input_replaced":
				_, err = pool.Exec(t.Context(), `UPDATE accepted_inputs SET claim_id='replacement' WHERE input_id='spa-input'`)
			case "input_expired":
				_, err = pool.Exec(t.Context(), `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()+interval '50 milliseconds' WHERE input_id='spa-input'`)
				time.Sleep(100 * time.Millisecond)
			case "expired":
				_, err = pool.Exec(t.Context(), `UPDATE execution_resource_dispatches SET deadline=clock_timestamp()+interval '50 milliseconds'`)
				time.Sleep(100 * time.Millisecond)
			}
			if err != nil {
				t.Fatal(err)
			}
			close(backend.release)
			var x execution
			select {
			case x = <-done:
			case <-time.After(8 * time.Second):
				t.Fatal("SPA execution stalled")
			}
			if x.err != nil {
				t.Fatal(x.err)
			}
			allowed := mode == "allowed"
			if (x.response.GetResult().GetStatus() == "completed") != allowed {
				t.Fatal("incorrect SPA outcome", mode, x.response.GetResult().GetStatus())
			}
			want := int32(1)
			if allowed || mode == "lost_finish" {
				want = 2
			}
			if backend.accepted.Load() != want {
				t.Fatal("released data after authority/URL denial")
			}
			var beforeReservations int
			if pool.QueryRow(t.Context(), `SELECT count(*) FROM execution_budget_reservations WHERE operation_id LIKE 'effect:%'`).Scan(&beforeReservations) != nil || beforeReservations < 1 {
				t.Fatal("missing original budget charge")
			}
			_, _ = server.ExecuteToolCall(t.Context(), request)
			if backend.accepted.Load() != want {
				t.Fatal("one-use dispatch replayed")
			}
			var dispatches, reservations int
			if pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM action_effect_dispatches),(SELECT count(*) FROM execution_budget_reservations WHERE operation_id LIKE 'effect:%')`).Scan(&dispatches, &reservations) != nil || dispatches != 1 || reservations != beforeReservations {
				t.Fatal("check/replay changed durable spending")
			}
		})
	}
}
