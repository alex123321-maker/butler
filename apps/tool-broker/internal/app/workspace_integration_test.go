package app

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/workspaceexec"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// This HTTP fixture stands in for owning context/approval service only. It reads
// current identity/input/lease/task from real PG; both deployed applications,
// source Catalog, journal/budget/resources, physical observer and erasure are real.
type workspaceBootstrapContexts struct {
	pool     *pgxpool.Pool
	mu       sync.Mutex
	approval actionauth.ApprovalSnapshot
}

func (r *workspaceBootstrapContexts) GetActionContext(ctx context.Context, run string) (actionauth.ContextSnapshot, error) {
	c := actionauth.ContextSnapshot{RunID: run, AutonomyMode: int32(commonv1.AutonomyMode_AUTONOMY_MODE_1), WorkspaceReady: run != "workspace-disabled"}
	id := run
	if id == "workspace-disabled" {
		id = "wa-run"
	}
	err := r.pool.QueryRow(ctx, `SELECT r.current_state,r.session_key,s.user_id,r.task_id,t.revision,r.task_revision,t.constraints_revision,t.status,o.lease_id,o.fence_epoch,o.expires_at FROM runs r JOIN sessions s USING(session_key) JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.session_key=r.session_key WHERE r.run_id=$1 AND o.run_id=r.run_id`, id).Scan(&c.RunState, &c.SessionKey, &c.OwnerID, &c.TaskID, &c.TaskRevision, &c.PinnedTaskRevision, &c.ConstraintsRevision, &c.TaskStatus, &c.LeaseID, &c.LeaseEpoch, &c.LeaseExpiresAt)
	if err != nil {
		return c, err
	}
	var input, claim string
	err = r.pool.QueryRow(ctx, `SELECT input_id,claim_id FROM accepted_inputs WHERE run_id=$1 AND status='running' AND claim_expires_at>clock_timestamp()`, id).Scan(&input, &claim)
	c.InputClaimHash = actionauth.InputClaimHash(input, claim)
	return c, err
}
func (r *workspaceBootstrapContexts) VerifyActionApproval(_ context.Context, req actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if req.ApprovalID != r.approval.ApprovalID || req.Binding.ActionHash != r.approval.Binding.ActionHash {
		return actionauth.ApprovalSnapshot{}, actionauth.ErrOwnerAuthority
	}
	return r.approval, nil
}
func workspacePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}
func TestWorkspaceActualApplicationBootstrapIntegration(t *testing.T) {
	binary := os.Getenv("BUTLER_WORKSPACE_E2E_BINARY")
	if binary == "" {
		t.Skip("actual private workspace app subprocess required")
	}
	for _, transport := range []string{"loopback", "unix"} {
		t.Run(transport, func(t *testing.T) { testWorkspaceBootstrap(t, binary, transport) })
	}
}
func testWorkspaceBootstrap(t *testing.T, binary, transport string) {
	dsn := guardedBootstrapDatabase(t)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx, stop := context.WithTimeout(context.Background(), 35*time.Second)
	t.Cleanup(stop)
	_, err = pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('wa-session','wa-owner','internal'); INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('wa-task','wa-session','wa-owner','bounded artifacts'); INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,lease_id,started_at,updated_at,task_id,task_revision) VALUES('wa-run','wa-session','fixture','tool_running','tool_running','mode_1','fixture','wa-lease',clock_timestamp(),clock_timestamp(),'wa-task',1); INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('wa-session','wa-run','wa-lease',1,clock_timestamp()+interval '1 hour'); INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,task_id,run_id,status,checkpoint,claim_id,claim_expires_at,expires_at) VALUES('wa-input','wa-session','fixture','workspace-app','test','wa-owner','{}','wa-task','wa-run','running','tool_dispatching','wa-input-claim',clock_timestamp()+interval '1 hour',clock_timestamp()+interval '1 hour')`)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	root := filepath.Join(dir, "workspaces")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "deletions.log")
	manifest := catalog.NewFileManifest(manifestPath)
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(pool, manifest)
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "wa-owner", Channel: "web", DeliveryID: "fixture"})
	source, err := sources.Put(owner, catalog.PutSource{ExternalKey: "workspace-app-original", Kind: "document", ScopeType: "user", ScopeID: "wa-owner", Title: "Original", Body: "synthetic original DATA"})
	if err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(ctx, nil, ownership.Token{SessionKey: "wa-session", RunID: "wa-run", LeaseID: "wa-lease", FenceEpoch: 1})
	tx, err := pool.Begin(owned)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sources.AccessTx(owned, tx, []catalog.SourceRef{{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}}, "memory_read"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(owned); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("synthetic-workspace-runtime-", 3)
	serviceToken := strings.Repeat("synthetic-action-authority-", 3)
	tokenPath := write("runtime-token", token)
	servicePath := write("action-token", serviceToken)
	registryPath := write("registry.json", `{"tools":[{"tool_name":"browser.snapshot","input_schema_json":"{}","output_schema_json":"{}"},{"tool_name":"single_tab.status","input_schema_json":"{}","output_schema_json":"{}"}]}`)
	target := workspacePort(t)
	if transport == "unix" {
		target = workspaceexec.UnixTarget
	}
	profile := workspaceexec.Profile{ID: "fixture-app-workspaces", Target: target}
	cmd := exec.Command(binary, "-test.run=^TestWorkspaceRuntimeAppHarness$")
	cmd.Env = append(os.Environ(), "BUTLER_WORKSPACE_APP_HARNESS=1", "BUTLER_POSTGRES_URL="+dsn, "BUTLER_POSTGRES_MAX_CONNS=2", "BUTLER_GRPC_ADDR="+target, "BUTLER_HTTP_ADDR="+workspacePort(t), "BUTLER_WORKSPACE_PROFILE_ID="+profile.ID, "BUTLER_WORKSPACE_ROOT="+root, "BUTLER_WORKSPACE_RUNTIME_TOKEN_FILE="+tokenPath, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+manifestPath)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	childDone := make(chan error, 1)
	go func() { childDone <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case e := <-childDone:
			if e != nil {
				t.Error("workspace subprocess failed")
			}
		case <-time.After(8 * time.Second):
			_ = cmd.Process.Kill()
			<-childDone
			t.Error("workspace subprocess did not stop")
		}
	})
	ready := false
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		bounded, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		err = workspaceexec.CheckReadiness(bounded, profile, token)
		cancel()
		if err == nil {
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("actual workspace process failed authenticated readiness")
	}
	contexts := &workspaceBootstrapContexts{pool: pool}
	contextServer := httptest.NewServer(actionauth.NewContextHandler(serviceToken, contexts))
	t.Cleanup(contextServer.Close)
	rpcAddr, httpAddr := workspacePort(t), workspacePort(t)
	env := map[string]string{"BUTLER_POSTGRES_URL": dsn, "BUTLER_POSTGRES_MAX_CONNS": "2", "BUTLER_POSTGRES_MIN_CONNS": "0", "BUTLER_TOOL_REGISTRY_PATH": registryPath, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE": servicePath, "BUTLER_ACTION_CONTEXT_URL": contextServer.URL, "BUTLER_WORKSPACE_ENABLED": "true", "BUTLER_WORKSPACE_PROFILE_ID": profile.ID, "BUTLER_WORKSPACE_RUNTIME_TARGET": target, "BUTLER_WORKSPACE_RUNTIME_TOKEN_FILE": tokenPath, "BUTLER_MEMORY_DELETION_MANIFEST_FILE": manifestPath, "BUTLER_GRPC_ADDR": rpcAddr, "BUTLER_HTTP_ADDR": httpAddr}
	for key, value := range env {
		t.Setenv(key, value)
	}
	for _, mode := range []string{"wrong_profile", "wrong_token", "raw_registry", "disabled_raw_registry", "missing_anchor"} {
		t.Run(mode, func(t *testing.T) {
			switch mode {
			case "wrong_profile":
				t.Setenv("BUTLER_WORKSPACE_PROFILE_ID", "another-profile")
			case "wrong_token":
				t.Setenv("BUTLER_WORKSPACE_RUNTIME_TOKEN_FILE", write("wrong-token", strings.Repeat("different-token-", 3)))
			case "raw_registry", "disabled_raw_registry":
				t.Setenv("BUTLER_TOOL_REGISTRY_PATH", write("raw-registry", `{"tools":[{"tool_name":"workspace.create","input_schema_json":"{}","output_schema_json":"{}"}]}`))
				if mode == "disabled_raw_registry" {
					t.Setenv("BUTLER_WORKSPACE_ENABLED", "false")
				}
			case "missing_anchor":
				t.Setenv("BUTLER_MEMORY_DELETION_MANIFEST_FILE", filepath.Join(dir, "absent.log"))
			}
			a, e := New(ctx)
			if e == nil {
				a.shutdown(nil)
				t.Fatal("incomplete/mismatched installed runtime accepted")
			}
		})
	}
	a, err := New(ctx)
	if err != nil {
		t.Fatal("actual Broker app bootstrap", err)
	}
	appCtx, cancel := context.WithCancel(ctx)
	appDone := make(chan error, 1)
	go func() { appDone <- a.Run(appCtx) }()
	defer func() {
		cancel()
		select {
		case e := <-appDone:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(8 * time.Second):
			t.Error("Broker app did not stop")
		}
	}()
	conn, err := grpc.NewClient(rpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := toolbrokerv1.NewToolBrokerServiceClient(conn)
	private := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+serviceToken)
	for _, run := range []string{"wa-run", "workspace-disabled"} {
		list, e := client.ListTools(private, &toolbrokerv1.ListToolsRequest{RunId: run})
		if e != nil {
			t.Fatal("actual app catalog", e)
		}
		count := 0
		for _, c := range list.GetTools() {
			if strings.HasPrefix(c.ToolName, "browser.") || strings.HasPrefix(c.ToolName, "single_tab.") {
				t.Fatal("workspace-only deployment advertised unsupported browser resource route")
			}
			if workspaceexec.Protected(c.ToolName) {
				count++
				if !workspaceexec.ExactContract(c) {
					t.Fatal("noncanonical runtime contract")
				}
			}
		}
		if run == "wa-run" && count != 3 || run == "workspace-disabled" && count != 0 {
			t.Fatal("owning readiness did not gate installed catalog", run, count)
		}
	}
	call := &toolbrokerv1.ToolCall{RunId: "wa-run", ToolCallId: "create", ToolName: workspaceexec.CreateTool, ArgsJson: `{}`, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1}
	prepared, err := client.PrepareToolCall(private, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
	if err != nil || prepared.GetError() != nil {
		t.Fatal("actual app prepare", err, prepared.GetError())
	}
	var binding actionauth.ActionBinding
	if json.Unmarshal([]byte(prepared.GetAction().GetBindingJson()), &binding) != nil {
		t.Fatal("actual prepared binding")
	}
	contexts.mu.Lock()
	contexts.approval = actionauth.ApprovalSnapshot{ApprovalID: "fixture-approval", Status: "approved", Binding: binding, ExpiresAt: time.Now().Add(time.Minute), ResolvedAt: time.Now()}
	contexts.mu.Unlock()
	grant, err := client.AuthorizeToolCall(private, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: binding.ActionID, ApprovalId: "fixture-approval"})
	if err != nil || grant.GetError() != nil {
		t.Fatal("actual app authorize", err, grant.GetError())
	}
	checkpoint, _ := json.Marshal(map[string]any{"checkpoint_data": map[string]string{"tool_call_id": call.ToolCallId, "effect_id": prepared.GetAction().GetEffectId(), "authorization_id": grant.AuthorizationId}})
	if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET checkpoint_json=$1 WHERE input_id='wa-input'`, checkpoint); err != nil {
		t.Fatal(err)
	}
	result, err := client.ExecuteToolCall(private, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grant.AuthorizationId})
	if err != nil || result.GetResult().GetStatus() != "completed" || result.GetResult().GetEffectState() != string(actionauth.StateConfirmed) {
		t.Fatal("actual app file effect", err, result.GetResult())
	}
	var created struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if json.Unmarshal([]byte(result.GetResult().GetResultJson()), &created) != nil || len(created.WorkspaceID) != 32 {
		t.Fatal("actual private namespace missing")
	}
	if _, err = os.Stat(filepath.Join(root, created.WorkspaceID, "tree")); err != nil {
		t.Fatal("actual workspace not created", err)
	}
	if _, err = sources.Delete(owner, source.ID, "delete", source.Revision); err != nil {
		t.Fatal(err)
	}
	erased := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		var completed bool
		e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM code_workspace_deletions WHERE workspace_id=$1 AND status='done')`, created.WorkspaceID).Scan(&completed)
		_, physical := os.Lstat(filepath.Join(root, created.WorkspaceID))
		if e == nil && completed && os.IsNotExist(physical) {
			erased = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !erased {
		t.Fatal("actual runtime scheduled source erasure did not complete")
	}
}
