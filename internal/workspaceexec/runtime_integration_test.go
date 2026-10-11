package workspaceexec

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/workspace"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

func TestWorkspaceRuntimeCloseDrainsActualExecuteAndKeepsUnknownLeaseIntegration(t *testing.T) {
	f := newRuntimeFixture(t)
	if duplicate, err := NewRuntime(f.ctx, f.dispatch, f.root, f.token); err == nil {
		_ = duplicate.Close()
		t.Fatal("runtime borrowed active artifact root")
	}
	req, ticket := f.claim(t, "close-active-create", CreateTool, `{}`)
	var lease, resource string
	var version, fence int64
	if err := f.pool.QueryRow(f.ctx, `SELECT d.resource_id,r.inflight_lease_id,r.version,r.fence_epoch FROM code_workspace_dispatches d JOIN execution_resources r ON r.resource_id=d.resource_id WHERE d.ticket_id=$1`, ticket).Scan(&resource, &lease, &version, &fence); err != nil || lease == "" {
		t.Fatal("claimed resource not retained before runtime entry")
	}
	blocker, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(f.ctx, `SELECT input_id FROM accepted_inputs WHERE input_id='wp-input' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	executeCtx, stopExecute := context.WithCancel(metadata.NewIncomingContext(f.ctx, metadata.Pairs("authorization", "Bearer "+f.token, TicketHeader, ticket)))
	defer stopExecute()
	results := make(chan *runtimev1.ExecuteResponse, 1)
	go func() { response, _ := f.runtime.Execute(executeCtx, req); results <- response }()
	// The fixture has pool2. Observe the actual second connection's row-lock
	// wait using the already borrowed blocker, without a third pool checkout.
	waitWorkspaceInputBlock(t, f.ctx, blocker)
	closed := make(chan error, 1)
	go func() { closed <- f.runtime.Close() }()
	select {
	case <-closed:
		t.Fatal("Close released artifact root during actual Execute")
	case <-time.After(100 * time.Millisecond):
	}
	if owner, err := workspace.LockRoot(f.root); err != workspace.ErrDenied {
		if owner != nil {
			_ = owner.Close()
		}
		t.Fatal("replacement owner entered while old Execute was active")
	}
	stopExecute()
	select {
	case response := <-results:
		if response.GetResult().GetStatus() != "effect_unknown" {
			t.Fatal("cancelled row-lock wait invented completed effect")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("actual Execute did not stop after cancellation")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal("drained runtime did not release root")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not finish after actual Execute returned")
	}
	if err := blocker.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.runtime.Close() != nil {
		t.Fatal("runtime Close was not idempotent")
	}
	response, err := f.runtime.Execute(metadata.NewIncomingContext(f.ctx, metadata.Pairs("authorization", "Bearer "+f.token, TicketHeader, ticket)), req)
	if err != nil || response.GetResult().GetStatus() != "effect_unknown" {
		t.Fatal("closed runtime retained execution admission")
	}
	if CheckReadiness(f.ctx, f.dispatch.profile, f.token) == nil {
		t.Fatal("closed runtime retained readiness")
	}
	replacement, err := NewRuntime(f.ctx, f.dispatch, f.root, f.token)
	if err != nil {
		t.Fatal("drained runtime blocked fresh process ownership")
	}
	t.Cleanup(func() { _ = replacement.Close() })
	var currentResource, currentLease, phase, effect string
	var currentVersion, currentFence int64
	if err := f.pool.QueryRow(f.ctx, `SELECT d.resource_id,r.inflight_lease_id,r.version,r.fence_epoch,d.phase,e.state FROM code_workspace_dispatches d JOIN execution_resources r ON r.resource_id=d.resource_id JOIN action_effects e ON e.action_id=d.effect_id WHERE d.ticket_id=$1`, ticket).Scan(&currentResource, &currentLease, &currentVersion, &currentFence, &phase, &effect); err != nil || currentResource != resource || currentLease != lease || currentVersion != version || currentFence != fence || phase != "prepared" || effect != "executing" {
		t.Fatal("close/restart reconciled retained unknown dispatch/resource history")
	}
	if entries, err := os.ReadDir(f.root); err != nil || len(entries) != 0 {
		t.Fatal("cancelled Execute touched artifact/lock/temp files")
	}
}

func TestWorkspaceRuntimeRootReplacementAfterAuthorityWaitDeniesEffectIntegration(t *testing.T) {
	f := newRuntimeFixture(t)
	req, ticket := f.claim(t, "root-replaced-create", CreateTool, `{}`)
	blocker, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(f.ctx, `SELECT input_id FROM accepted_inputs WHERE input_id='wp-input' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	results := make(chan *runtimev1.ExecuteResponse, 1)
	ctx := metadata.NewIncomingContext(f.ctx, metadata.Pairs("authorization", "Bearer "+f.token, TicketHeader, ticket))
	go func() { response, _ := f.runtime.Execute(ctx, req); results <- response }()
	waitWorkspaceInputBlock(t, f.ctx, blocker)
	old := f.root + "-replaced"
	if err := os.Rename(f.root, old); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(old) })
	if err := os.Mkdir(f.root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-results:
		if response.GetResult().GetStatus() != "effect_unknown" {
			t.Fatal("root replaced during authority wait admitted physical effect")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not finish after actual authority wait")
	}
	if CheckReadiness(f.ctx, f.dispatch.profile, f.token) == nil {
		t.Fatal("replaced root borrowed installed readiness")
	}
	for _, path := range []string{f.root, old} {
		if entries, err := os.ReadDir(path); err != nil || len(entries) != 0 {
			t.Fatal("root replacement reached detached or replacement files")
		}
	}
	var phase string
	var evidence, copies int
	var quarantined bool
	if err := f.pool.QueryRow(f.ctx, `SELECT d.phase,(SELECT count(*) FROM code_workspace_effect_evidence),(SELECT count(*) FROM code_workspaces),r.inflight_lease_id=d.resource_lease_id FROM code_workspace_dispatches d JOIN execution_resources r ON r.resource_id=d.resource_id WHERE d.ticket_id=$1`, ticket).Scan(&phase, &evidence, &copies, &quarantined); err != nil || phase != "running" || evidence != 0 || copies != 0 || !quarantined {
		t.Fatal("root replacement invented evidence or cleared irreversible resource admission")
	}
}

func waitWorkspaceInputBlock(t *testing.T, ctx context.Context, blocker pgx.Tx) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := blocker.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatal(err)
		}
		var waiting bool
		if err := blocker.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pg_backend_pid()=ANY(pg_blocking_pids(pid)))`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("actual Execute did not reach owning input row-lock wait")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type runtimeFixture struct {
	*sourceFixture
	dispatch *DispatchStore
	auth     *actionauth.PostgresStore
	service  *actionauth.Service
	reader   *admissionReader
	runtime  *Runtime
	client   runtimev1.ToolRuntimeServiceClient
	token    string
}

func newRuntimeFixture(t *testing.T) *runtimeFixture {
	t.Helper()
	f := newSourceFixture(t)
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,task_id,run_id,status,checkpoint,claim_id,claim_expires_at,expires_at) VALUES('wp-input','wp-session','fixture','workspace-runtime','test','wp-owner','{}','wp-task','wp-run','running','tool_dispatching','wp-input-claim',clock_timestamp()+interval '1 hour',clock_timestamp()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	b := budget.NewController(f.pool, budget.DefaultPolicy())
	d, err := NewDispatchStore(f.pool, f.resource, b, f.provenance, Profile{"fixture-runtime", listener.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	a := actionauth.NewPostgresStore(f.pool)
	a.SetBudgets(b)
	a.SetDispatchClaimGuard(d)
	reader := &admissionReader{pool: f.pool}
	token := strings.Repeat("fixture-workspace-service-", 3)
	r, err := NewRuntime(f.ctx, d, f.root, token)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	server := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(server, r)
	h, err := NewHealth(r)
	if err != nil {
		t.Fatal(err)
	}
	healthv1.RegisterHealthServer(server, h)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &runtimeFixture{f, d, a, actionauth.NewService(a, reader, nil), reader, r, runtimev1.NewToolRuntimeServiceClient(conn), token}
}

func TestWorkspaceInstalledReadinessRejectsWrongProfileRootAndDisabledHistoryIntegration(t *testing.T) {
	f := newRuntimeFixture(t)
	if err := CheckReadiness(f.ctx, f.dispatch.profile, f.token); err != nil {
		t.Fatal("actual runtime not ready", err)
	}
	wrong := f.dispatch.profile
	wrong.ID = "other-profile"
	if CheckReadiness(f.ctx, wrong, f.token) == nil || CheckReadiness(f.ctx, f.dispatch.profile, strings.Repeat("wrong-token", 4)) == nil {
		t.Fatal("different profile/auth borrowed readiness")
	}
	if err := os.Chmod(f.root, 0755); err != nil {
		t.Fatal(err)
	}
	if CheckReadiness(f.ctx, f.dispatch.profile, f.token) == nil {
		t.Fatal("shared root advertised ready")
	}
	if err := os.Chmod(f.root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `ALTER TABLE code_workspace_effect_evidence DISABLE TRIGGER code_workspace_evidence_immutable`); err != nil {
		t.Fatal(err)
	}
	if CheckReadiness(f.ctx, f.dispatch.profile, f.token) == nil {
		t.Fatal("disabled immutable history advertised ready")
	}
	if _, err := f.pool.Exec(f.ctx, `ALTER TABLE code_workspace_effect_evidence ENABLE TRIGGER code_workspace_evidence_immutable`); err != nil {
		t.Fatal(err)
	}
	if err := CheckReadiness(f.ctx, f.dispatch.profile, f.token); err != nil {
		t.Fatal("restored exact runtime unavailable", err)
	}
}
func (f *runtimeFixture) claim(t *testing.T, id, tool, raw string) (*runtimev1.ExecuteRequest, string) {
	t.Helper()
	call := &toolbrokerv1.ToolCall{RunId: "wp-run", ToolCallId: id, ToolName: tool, ArgsJson: raw, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1}
	var contract *toolbrokerv1.ToolContract
	for _, c := range Contracts(f.dispatch.profile.Target) {
		if c.ToolName == tool {
			contract = c
		}
	}
	p, err := f.service.Prepare(f.ctx, call, contract)
	if err != nil {
		t.Fatal(err)
	}
	f.reader.approval = actionauth.ApprovalSnapshot{ApprovalID: "fixture-approval", Status: "approved", Binding: p.Binding, ExpiresAt: time.Now().Add(time.Minute), ResolvedAt: time.Now()}
	g, err := f.service.Authorize(f.ctx, call, contract, p.Binding.ActionID, "fixture-approval", "")
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, _ := json.Marshal(map[string]any{"checkpoint_data": map[string]string{"tool_call_id": id, "effect_id": p.EffectID, "authorization_id": g.ID}})
	if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET checkpoint_json=$1 WHERE input_id='wp-input'`, checkpoint); err != nil {
		t.Fatal(err)
	}
	owned, err := f.dispatch.PrepareContext(f.ctx, call, contract, g)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.auth.ClaimGrant(owned, g.ID); err != nil {
		t.Fatal(err)
	}
	ticket, err := f.dispatch.TicketForAction(f.ctx, call, contract, p.EffectID)
	if err != nil {
		t.Fatal(err)
	}
	var fingerprint string
	if err = f.pool.QueryRow(f.ctx, `SELECT fingerprint FROM action_effects WHERE action_id=$1`, p.EffectID).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	return &runtimev1.ExecuteRequest{ToolCall: call, Contract: contract, Context: &runtimev1.ExecutionContext{RunId: call.RunId, ToolCallId: id, ToolName: tool, RuntimeTarget: f.dispatch.profile.Target, ActionId: p.EffectID, ActionFingerprint: fingerprint}}, ticket
}
func (f *runtimeFixture) execute(t *testing.T, req *runtimev1.ExecuteRequest, ticket string) *toolbrokerv1.ToolResult {
	t.Helper()
	ctx := metadata.AppendToOutgoingContext(f.ctx, "authorization", "Bearer "+f.token, TicketHeader, ticket)
	response, err := f.client.Execute(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if response.GetResult().GetStatus() == "completed" {
		var authorization string
		if err = f.pool.QueryRow(f.ctx, `SELECT authorization_id FROM code_workspace_dispatches WHERE ticket_id=$1`, ticket).Scan(&authorization); err != nil {
			t.Fatal(err)
		}
		g, loadErr := f.auth.GetGrant(f.ctx, authorization)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		// As in Broker, a trusted, correlated runtime response completes the
		// permanent action journal before a subsequent tool is prepared.
		if _, err = f.service.Outcome(f.ctx, g, response.GetResult(), false); err != nil {
			t.Fatal(err)
		}
	}
	return response.GetResult()
}

func TestWorkspaceRuntimeActualPrivateRPCAndPhysicalConfirmationIntegration(t *testing.T) {
	f := newRuntimeFixture(t)
	req, ticket := f.claim(t, "create", CreateTool, `{}`)
	for _, ctx := range []context.Context{f.ctx, metadata.AppendToOutgoingContext(f.ctx, "authorization", "Bearer "+f.token), metadata.AppendToOutgoingContext(f.ctx, "authorization", "Bearer "+f.token, "authorization", "Bearer "+f.token, TicketHeader, ticket)} {
		response, err := f.client.Execute(ctx, req)
		if err != nil || response.GetResult().GetStatus() != "effect_unknown" {
			t.Fatal("anonymous/incomplete private dispatch admitted", err)
		}
	}
	result := f.execute(t, req, ticket)
	if result.Status != "completed" || result.Error != nil {
		t.Fatal("actual namespace creation", result.Status)
	}
	var created struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if json.Unmarshal([]byte(result.ResultJson), &created) != nil || !opaqueID(created.WorkspaceID) {
		t.Fatal("private namespace handle missing")
	}
	if again := f.execute(t, req, ticket); again.Status != "effect_unknown" {
		t.Fatal("one-use creation ticket replayed")
	}
	write, _ := json.Marshal(map[string]any{"workspace_id": created.WorkspaceID, "path": "src/main.go", "expected_version": 0, "expected_sha256": "", "expected_bytes": 0, "content": "package main // actual runtime fixture"})
	req, ticket = f.claim(t, "write", WriteTool, string(write))
	result = f.execute(t, req, ticket)
	if result.Status != "completed" || result.Error != nil {
		t.Fatal("actual write confirmation", result.Status)
	}
	var written struct {
		WorkspaceID string             `json:"workspace_id"`
		Snapshot    workspace.Snapshot `json:"snapshot"`
	}
	if json.Unmarshal([]byte(result.ResultJson), &written) != nil || written.WorkspaceID != created.WorkspaceID || written.Snapshot.Version != 1 {
		t.Fatal("actual file snapshot missing")
	}
	physical, err := os.ReadFile(filepath.Join(f.root, created.WorkspaceID, "tree", "src", "main.go"))
	if err != nil || string(physical) != "package main // actual runtime fixture" || bodyDigest(physical) != written.Snapshot.SHA256 {
		t.Fatal("physical bytes differ from confirmed snapshot", err)
	}
	read, _ := json.Marshal(map[string]string{"workspace_id": created.WorkspaceID, "path": "src/main.go"})
	req, ticket = f.claim(t, "read", ReadTool, string(read))
	result = f.execute(t, req, ticket)
	if result.Status != "completed" || result.Error != nil {
		t.Fatal("actual current read", result.Status)
	}
	var viewed struct {
		WorkspaceID string             `json:"workspace_id"`
		Snapshot    workspace.Snapshot `json:"snapshot"`
		Content     string             `json:"content"`
	}
	if json.Unmarshal([]byte(result.ResultJson), &viewed) != nil || viewed.Snapshot != written.Snapshot || viewed.Content != string(physical) {
		t.Fatal("read did not retain actual metadata")
	}
	var evidence, contexts, completed, released, reservations int
	if err = f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM code_workspace_effect_evidence),(SELECT count(*) FROM code_workspace_sources),(SELECT count(*) FROM execution_resource_leases WHERE state='completed'),(SELECT count(*) FROM execution_resource_leases WHERE state='released'),(SELECT count(*) FROM execution_budget_reservations)`).Scan(&evidence, &contexts, &completed, &released, &reservations); err != nil || evidence != 3 || contexts != 2 || completed != 2 || released != 1 || reservations != 3 {
		t.Fatal("separate file/resource/source completion", evidence, contexts, completed, released, reservations, err)
	}
	if strings.Contains(result.ResultJson, f.root) || strings.Contains(result.ResultJson, f.token) || strings.Contains(result.ResultJson, ticket) {
		t.Fatal("private runtime material leaked")
	}
}

func TestWorkspaceRuntimeRejectsChangedInputBeforePhysicalEffectIntegration(t *testing.T) {
	f := newRuntimeFixture(t)
	req, ticket := f.claim(t, "create", CreateTool, `{}`)
	if _, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_id='replacement' WHERE input_id='wp-input'`); err != nil {
		t.Fatal(err)
	}
	if result := f.execute(t, req, ticket); result.Status != "effect_unknown" {
		t.Fatal("replacement input used old runtime authority")
	}
	entries, err := os.ReadDir(f.root)
	if err != nil || len(entries) != 0 {
		t.Fatal("stale input reached filesystem", err)
	}
	var phase string
	if err = f.pool.QueryRow(f.ctx, `SELECT phase FROM code_workspace_dispatches WHERE ticket_id=$1`, ticket).Scan(&phase); err != nil || phase != "prepared" {
		t.Fatal("denial consumed one-use runtime start", phase, err)
	}
}

func TestWorkspaceRuntimeRejectsPrivateRPCMutationBeforePhysicalEffectIntegration(t *testing.T) {
	for _, mode := range []string{"credential", "contract", "args", "fingerprint", "target", "expired_input", "task", "deleted_source", "duplicate_ticket"} {
		t.Run(mode, func(t *testing.T) {
			f := newRuntimeFixture(t)
			req, ticket := f.claim(t, "create", CreateTool, `{}`)
			switch mode {
			case "credential":
				req.ResolvedCredentials = []*runtimev1.ResolvedCredential{{Alias: "fixture", Field: "token", Value: "synthetic forbidden credential"}}
			case "contract":
				req.Contract.Description += " changed"
			case "args":
				req.ToolCall.ArgsJson = `{"root":"/tmp"}`
			case "fingerprint":
				req.Context.ActionFingerprint = strings.Repeat("0", 64)
			case "target":
				req.ToolCall.RuntimeTarget = "127.0.0.1:1"
			case "expired_input":
				if _, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-interval '1 second' WHERE input_id='wp-input'`); err != nil {
					t.Fatal(err)
				}
			case "task":
				if _, err := f.pool.Exec(f.ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id='wp-task'`); err != nil {
					t.Fatal(err)
				}
			case "deleted_source":
				owner := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "delete"})
				if _, err := f.catalog.Delete(owner, f.source.ID, "delete", f.source.Revision); err != nil {
					t.Fatal(err)
				}
			}
			ctx := metadata.AppendToOutgoingContext(f.ctx, "authorization", "Bearer "+f.token, TicketHeader, ticket)
			if mode == "duplicate_ticket" {
				ctx = metadata.AppendToOutgoingContext(ctx, TicketHeader, ticket)
			}
			response, err := f.client.Execute(ctx, req)
			if err != nil || response.GetResult().GetStatus() != "effect_unknown" {
				t.Fatal("mutated private RPC admitted", err)
			}
			if entries, err := os.ReadDir(f.root); err != nil || len(entries) != 0 {
				t.Fatal("denied private RPC touched filesystem", err)
			}
			var phase string
			var evidence int
			if err := f.pool.QueryRow(f.ctx, `SELECT phase,(SELECT count(*) FROM code_workspace_effect_evidence) FROM code_workspace_dispatches WHERE ticket_id=$1`, ticket).Scan(&phase, &evidence); err != nil || phase != "prepared" || evidence != 0 {
				t.Fatal("denial invented execution/evidence", phase, evidence, err)
			}
		})
	}
}
