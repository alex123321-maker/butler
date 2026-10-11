package broker

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/workspace"
	"github.com/butler/butler/internal/workspaceexec"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

// Only exact human approval is a fixture. Source policy, input, worker, journal,
// budget, resource, Broker and private runtime use their real implementations.
type workspaceReader struct {
	*authFixture
	pool  *pgxpool.Pool
	ready bool
}

func (r *workspaceReader) GetActionContext(ctx context.Context, run string) (actionauth.ContextSnapshot, error) {
	c := actionauth.ContextSnapshot{RunID: run, AutonomyMode: int32(commonv1.AutonomyMode_AUTONOMY_MODE_1), WorkspaceReady: r.ready}
	err := r.pool.QueryRow(ctx, `SELECT r.current_state,r.session_key,s.user_id,r.task_id,t.revision,r.task_revision,t.constraints_revision,t.status,o.lease_id,o.fence_epoch,o.expires_at FROM runs r JOIN sessions s USING(session_key) JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.session_key=r.session_key WHERE r.run_id=$1 AND o.run_id=r.run_id`, run).Scan(&c.RunState, &c.SessionKey, &c.OwnerID, &c.TaskID, &c.TaskRevision, &c.PinnedTaskRevision, &c.ConstraintsRevision, &c.TaskStatus, &c.LeaseID, &c.LeaseEpoch, &c.LeaseExpiresAt)
	if err != nil {
		return c, err
	}
	var input, claim string
	err = r.pool.QueryRow(ctx, `SELECT input_id,claim_id FROM accepted_inputs WHERE run_id=$1 AND status='running'`, run).Scan(&input, &claim)
	c.InputClaimHash = actionauth.InputClaimHash(input, claim)
	return c, err
}

type workspaceBrokerFixture struct {
	ctx          context.Context
	pool         *pgxpool.Pool
	server       *Server
	auth         *actionauth.PostgresStore
	reader       *workspaceReader
	root         string
	provenance   *workspaceexec.Provenance
	runtime      *workspaceexec.Runtime
	privateToken string
}

func workspaceBroker(t *testing.T) *workspaceBrokerFixture {
	t.Helper()
	pool := memorytest.Database(t)
	ctx, stop := context.WithTimeout(context.Background(), 35*time.Second)
	t.Cleanup(stop)
	_, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('wb-session','wb-owner','internal'); INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('wb-task','wb-session','wb-owner','bounded artifacts'); INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('wb-run','wb-session','fixture','tool_running','tool_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),'wb-task',1); INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('wb-session','wb-run','wb-lease',1,clock_timestamp()+interval '1 hour'); INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,task_id,run_id,status,checkpoint,claim_id,claim_expires_at,expires_at) VALUES('wb-input','wb-session','fixture','workspace-broker','test','wb-owner','{}','wb-task','wb-run','running','tool_dispatching','wb-input-claim',clock_timestamp()+interval '1 hour',clock_timestamp()+interval '1 hour')`)
	if err != nil {
		t.Fatal(err)
	}
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions.log"))
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(pool, manifest)
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "wb-owner", Channel: "web", DeliveryID: "fixture"})
	src, err := sources.Put(owner, catalog.PutSource{ExternalKey: "workspace-broker-input", Kind: "document", ScopeType: "user", ScopeID: "wb-owner", Title: "Original", Body: "synthetic original workspace input"})
	if err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(ctx, nil, ownership.Token{SessionKey: "wb-session", RunID: "wb-run", LeaseID: "wb-lease", FenceEpoch: 1})
	tx, err := pool.Begin(owned)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sources.AccessTx(owned, tx, []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, "memory_read"); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatal(err)
	}
	if err = tx.Commit(owned); err != nil {
		t.Fatal(err)
	}
	provenance, err := workspaceexec.NewProvenance(pool, sources)
	if err != nil {
		t.Fatal(err)
	}
	leases, err := resources.New(resources.WorkerGuard{}, denyResourceReconcile{})
	if err != nil {
		t.Fatal(err)
	}
	budgets := budget.NewController(pool, budget.DefaultPolicy())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	target := listener.Addr().String()
	dispatch, err := workspaceexec.NewDispatchStore(pool, leases, budgets, provenance, workspaceexec.Profile{ID: "fixture-private-workspaces", Target: target})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("fixture-private-workspaces-", 3)
	runtime, err := workspaceexec.NewRuntime(ctx, dispatch, root, token)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	grpcServer := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(grpcServer, runtime)
	go grpcServer.Serve(listener)
	t.Cleanup(grpcServer.Stop)
	router := runtimeclient.New(nil)
	t.Cleanup(func() { _ = router.Close() })
	if err = router.SetTargetAuthentication(target, token); err != nil {
		t.Fatal(err)
	}
	executor, err := NewWorkspaceExecutor(dispatch, router)
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(t.TempDir(), "tools.json")
	if err = os.WriteFile(registryPath, []byte(`{"tools":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load(registryPath, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = reg.RegisterOwned(workspaceexec.Contracts(target)); err != nil {
		t.Fatal(err)
	}
	server := NewServer(reg, executor, nil, nil)
	auth := actionauth.NewPostgresStore(pool)
	auth.SetBudgets(budgets)
	admission := ResourceAdmissions{Workspace: dispatch}
	auth.SetDispatchClaimGuard(admission)
	server.SetResourceAdmission(admission)
	reader := &workspaceReader{newAuthFixture(), pool, true}
	server.SetActionAuthorizer(actionauth.NewService(auth, reader, nil))
	server.SetBudgets(budgets)
	server.SetScopedCatalog(&TaskCatalog{Registry: reg, Contexts: reader, Mandates: auth})
	return &workspaceBrokerFixture{ctx, pool, server, auth, reader, root, provenance, runtime, token}
}
func (f *workspaceBrokerFixture) prepared(t *testing.T, id, tool, raw string) (*toolbrokerv1.ToolCall, string) {
	t.Helper()
	call := &toolbrokerv1.ToolCall{RunId: "wb-run", ToolCallId: id, ToolName: tool, ArgsJson: raw, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1}
	p, err := f.server.PrepareToolCall(f.ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
	if err != nil || p.GetError() != nil {
		t.Fatal("prepare", err, p.GetError())
	}
	i, err := f.auth.GetIntent(f.ctx, p.GetAction().GetActionId())
	if err != nil {
		t.Fatal(err)
	}
	f.reader.approvals["fixture-approval"] = actionauth.ApprovalSnapshot{ApprovalID: "fixture-approval", Status: "approved", Binding: i.Binding, ResolvedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
	a, err := f.server.AuthorizeToolCall(f.ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: i.Binding.ActionID, ApprovalId: "fixture-approval"})
	if err != nil || a.GetError() != nil {
		t.Fatal("authorize", err, a.GetError())
	}
	checkpoint, _ := json.Marshal(map[string]any{"checkpoint_data": map[string]string{"tool_call_id": id, "effect_id": i.EffectID, "authorization_id": a.AuthorizationId}})
	if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET checkpoint_json=$1 WHERE input_id='wb-input'`, checkpoint); err != nil {
		t.Fatal(err)
	}
	return call, a.AuthorizationId
}
func (f *workspaceBrokerFixture) execute(t *testing.T, call *toolbrokerv1.ToolCall, authorization string) *toolbrokerv1.ToolResult {
	t.Helper()
	r, err := f.server.ExecuteToolCall(f.ctx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: authorization})
	if err != nil {
		t.Fatal(err)
	}
	return r.GetResult()
}
func TestWorkspaceBrokerActualFileFlowAndStalePatchBeforeSpendIntegration(t *testing.T) {
	f := workspaceBroker(t)
	call, authorization := f.prepared(t, "create", workspaceexec.CreateTool, `{}`)
	created := f.execute(t, call, authorization)
	if created.Status != "completed" || created.EffectState != string(actionauth.StateConfirmed) {
		t.Fatal("create", created.Status, created.EffectState, created.Error)
	}
	var namespace struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if json.Unmarshal([]byte(created.ResultJson), &namespace) != nil || len(namespace.WorkspaceID) != 32 {
		t.Fatal("opaque namespace")
	}
	if replay := f.execute(t, call, authorization); replay.Status == "completed" {
		t.Fatal("spent Broker permit replayed")
	}
	write, _ := json.Marshal(map[string]any{"workspace_id": namespace.WorkspaceID, "path": "src/main.go", "expected_version": 0, "expected_sha256": "", "expected_bytes": 0, "content": "package main // first"})
	call, authorization = f.prepared(t, "write", workspaceexec.WriteTool, string(write))
	written := f.execute(t, call, authorization)
	if written.Status != "completed" || written.EffectState != string(actionauth.StateConfirmed) {
		t.Fatal("write", written.Status, written.Error)
	}
	call, authorization = f.prepared(t, "stale-patch", workspaceexec.WriteTool, string(write))
	denied := f.execute(t, call, authorization)
	if denied.Status == "completed" {
		t.Fatal("stale patch overwrite")
	}
	var permits, reservations int
	var state string
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM code_workspace_dispatches),(SELECT count(*) FROM execution_budget_reservations),(SELECT state FROM action_authorizations WHERE authorization_id=$1)`, authorization).Scan(&permits, &reservations, &state); err != nil || permits != 2 || reservations != 2 || state != "authorized" {
		t.Fatal("stale patch spent admission", permits, reservations, state, err)
	}
	read, _ := json.Marshal(map[string]string{"workspace_id": namespace.WorkspaceID, "path": "src/main.go"})
	call, authorization = f.prepared(t, "read", workspaceexec.ReadTool, string(read))
	viewed := f.execute(t, call, authorization)
	var content struct {
		Snapshot workspace.Snapshot `json:"snapshot"`
		Content  string             `json:"content"`
	}
	if viewed.Status != "completed" || json.Unmarshal([]byte(viewed.ResultJson), &content) != nil || content.Snapshot.Version != 1 || content.Content != "package main // first" {
		t.Fatal("current physical read", viewed.Status, viewed.Error)
	}
	for _, raw := range []string{created.ResultJson, written.ResultJson, viewed.ResultJson} {
		if strings.Contains(raw, f.root) || strings.Contains(raw, f.privateToken) || strings.Contains(raw, "butler-workspace-ticket") {
			t.Fatal("private material leaked")
		}
	}
}
func TestWorkspaceBrokerMissingReadinessAndChangedInputDenyIntegration(t *testing.T) {
	f := workspaceBroker(t)
	f.reader.ready = false
	if listed, err := f.server.scopedCatalog.ListForRun(f.ctx, "wb-run", "", 64); err != nil || len(listed) != 0 {
		t.Fatal("unready catalog", len(listed), err)
	}
	if _, err := f.server.PrepareToolCall(f.ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: &toolbrokerv1.ToolCall{RunId: "wb-run", ToolCallId: "unready", ToolName: workspaceexec.CreateTool, ArgsJson: `{}`, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1}}); err == nil {
		t.Fatal("missing readiness admitted")
	}
	f.reader.ready = true
	call, authorization := f.prepared(t, "changed-input", workspaceexec.CreateTool, `{}`)
	if _, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_id='replacement' WHERE input_id='wb-input'`); err != nil {
		t.Fatal(err)
	}
	if r := f.execute(t, call, authorization); r.Status == "completed" {
		t.Fatal("replacement input admitted")
	}
	var permits, reservations int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM code_workspace_dispatches),(SELECT count(*) FROM execution_budget_reservations)`).Scan(&permits, &reservations); err != nil || permits != 0 || reservations != 0 {
		t.Fatal("denial spent permit/budget", permits, reservations, err)
	}
	if entries, err := os.ReadDir(f.root); err != nil || len(entries) != 0 {
		t.Fatal("denial touched filesystem", err)
	}
}

type lostWorkspaceAcknowledgement struct{ RuntimeExecutor }

func (e lostWorkspaceAcknowledgement) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	r, err := e.RuntimeExecutor.Execute(ctx, call, c, secrets)
	if err != nil || r == nil || r.GetStatus() != "completed" {
		return r, err
	}
	return nil, errors.New("synthetic lost acknowledgement after actual committed file effect")
}
func TestWorkspaceBrokerActualCommittedEffectLostAcknowledgementCannotReplayIntegration(t *testing.T) {
	f := workspaceBroker(t)
	f.server.SetCatalogExecutor(lostWorkspaceAcknowledgement{f.server.executor})
	call, authorization := f.prepared(t, "lost-create", workspaceexec.CreateTool, `{}`)
	r := f.execute(t, call, authorization)
	if r.EffectState != string(actionauth.StateUnknown) {
		t.Fatal("lost acknowledgement fabricated success", r.Status, r.EffectState)
	}
	var namespaces, evidence int
	var phase string
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM code_workspaces),(SELECT count(*) FROM code_workspace_effect_evidence),phase FROM code_workspace_dispatches WHERE authorization_id=$1`, authorization).Scan(&namespaces, &evidence, &phase); err != nil || namespaces != 1 || evidence != 1 || phase != "completed" {
		t.Fatal("actual effect did not precede lost ACK", namespaces, evidence, phase, err)
	}
	if again := f.execute(t, call, authorization); again.EffectState != string(actionauth.StateUnknown) || again.Status == "completed" {
		t.Fatal("unknown Broker effect replayed", again.Status, again.EffectState)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM code_workspace_dispatches`).Scan(&count); err != nil || count != 1 {
		t.Fatal("replay created another dispatch", count, err)
	}
	call, authorization = f.prepared(t, "different-tool", workspaceexec.CreateTool, `{}`)
	stopped := f.execute(t, call, authorization)
	if stopped.GetStatus() != "execution_stopped" || !strings.Contains(stopped.GetError().GetDetailsJson(), "external_effect_unknown") {
		t.Fatal("unknown effect silently dispatched another tool", stopped.Status, stopped.Error)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&count); err != nil || count != 1 {
		t.Fatal("stopped continuation spent another budget", count, err)
	}
}
