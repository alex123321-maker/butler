package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"google.golang.org/grpc"
)

type doctorObservedRuntime struct {
	runtimev1.UnimplementedToolRuntimeServiceServer
	calls atomic.Int64
}

func (r *doctorObservedRuntime) Execute(_ context.Context, q *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	if q.GetToolCall().GetToolName() != "doctor.check_system" || q.GetToolCall().GetArgsJson() != "{}" {
		return nil, fmt.Errorf("unexpected operator command")
	}
	r.calls.Add(1)
	b, _ := json.Marshal(map[string]any{"status": "healthy", "checked_at": time.Now().UTC(), "checks": []any{map[string]any{"name": "controlled_runtime", "status": "healthy"}}})
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{Status: "completed", ResultJson: string(b)}}, nil
}
func doctorFreeAddress(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	a := l.Addr().String()
	_ = l.Close()
	return a
}

func TestOperatorDoctorActualBrokerIntegration(t *testing.T) {
	binary := os.Getenv("BUTLER_TEST_DOCTOR_BROKER_BINARY")
	if binary == "" {
		t.Skip("actual Broker binary required in isolated lab")
	}
	for _, mode := range []commonv1.AutonomyMode{commonv1.AutonomyMode_AUTONOMY_MODE_2, commonv1.AutonomyMode_AUTONOMY_MODE_1} {
		t.Run(mode.String(), func(t *testing.T) {
			pool := memorytest.Database(t)
			dsn, parseErr := url.Parse(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			query := dsn.Query()
			query.Set("search_path", pool.Config().ConnConfig.RuntimeParams["search_path"])
			dsn.RawQuery = query.Encode()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			db, e := postgresstore.Open(ctx, postgresstore.Config{URL: dsn.String(), MaxConns: 4}, nil)
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			private := t.TempDir()
			token := strings.Repeat("fixture-token-", 4)
			tokenPath := filepath.Join(private, "authority")
			if e = os.WriteFile(tokenPath, []byte(token), 0600); e != nil {
				t.Fatal(e)
			}
			manifest := catalog.NewFileManifest(filepath.Join(private, "deletions"))
			if e = manifest.Initialize(); e != nil {
				t.Fatal(e)
			}
			inputs := durableinput.NewStore(pool)
			sessions := session.NewPostgresRepository(pool)
			leases := session.NewPostgresLeaseManager(pool)
			runs := run.NewPostgresRepository(pool)
			runManager := run.NewService(runs, nil)
			bc := budget.NewController(pool, budget.DefaultPolicy())
			bc.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
			tasks := task.NewStore(pool)
			tasks.SetBudgets(bc, "")
			approvalRepo := approvals.NewPostgresRepository(pool)
			approvalRepo.SetCheckpointHooks(inputs)
			approvalService := approvals.NewService(approvalRepo, nil)
			reader := &actionContextReader{runs: runManager, sessions: sessions, leases: leases, tasks: tasks, approvals: approvalService, pool: pool}
			privateServer := httptest.NewServer(actionauth.NewContextHandler(token, reader))
			defer privateServer.Close()
			listener, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			runtime := &doctorObservedRuntime{}
			rpc := grpc.NewServer()
			runtimev1.RegisterToolRuntimeServiceServer(rpc, runtime)
			go rpc.Serve(listener)
			defer rpc.Stop()
			original, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "configs", "tools.json"))
			if e != nil {
				t.Fatal(e)
			}
			var registry struct {
				Tools []map[string]any `json:"tools"`
			}
			if e = json.Unmarshal(original, &registry); e != nil {
				t.Fatal(e)
			}
			var only []map[string]any
			for _, tool := range registry.Tools {
				if tool["tool_name"] == "doctor.check_system" {
					tool["runtime_target"] = listener.Addr().String()
					only = append(only, tool)
				}
			}
			data, _ := json.Marshal(map[string]any{"tools": only})
			registryPath := filepath.Join(private, "tools.json")
			if e = os.WriteFile(registryPath, data, 0600); e != nil {
				t.Fatal(e)
			}
			brokerAddr := doctorFreeAddress(t)
			cmd := exec.CommandContext(ctx, binary)
			cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
			cmd.Env = append(os.Environ(), "BUTLER_POSTGRES_URL="+dsn.String(), "BUTLER_HTTP_ADDR="+doctorFreeAddress(t), "BUTLER_GRPC_ADDR="+brokerAddr, "BUTLER_TOOL_REGISTRY_PATH="+registryPath, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+tokenPath, "BUTLER_ACTION_CONTEXT_URL="+privateServer.URL+"/internal/action-context", "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+filepath.Join(private, "deletions"))
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			for until := time.Now().Add(10 * time.Second); ; {
				conn, err := net.DialTimeout("tcp", brokerAddr, 50*time.Millisecond)
				if err == nil {
					_ = conn.Close()
					break
				}
				if time.Now().After(until) {
					t.Fatal("actual Broker startup failed")
				}
				time.Sleep(25 * time.Millisecond)
			}
			client, e := tools.DialWithTokenFile(brokerAddr, tokenPath)
			if e != nil {
				t.Fatal(e)
			}
			defer client.Close()
			final := completion.New(pool)
			executor := flow.NewService(sessions, leases, runManager, transcript.NewStore(pool), nil, flow.Config{Tasks: tasks, Checkpoints: inputs, Tools: client, ActionAuthorizer: client, ApprovalService: approvalService, Completion: final, AutonomyMode: mode}, nil)
			worker := durableinput.NewService(inputs, sessions, executor, runManager, nil)
			workerCtx, stop := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() { defer close(done); _ = worker.Run(workerCtx) }()
			defer func() { stop(); <-done }()
			checker := &operatorDoctorChecker{deps: httpServerDeps{postgres: db, inputStore: inputs, taskStore: tasks, sessionRepo: sessions, runRepo: runs, finalResults: final}}
			owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "doctor-owner", Channel: "web", DeliveryID: "doctor-admission"})
			_, raw, e := checker.RunCheck(owner)
			if e != nil {
				t.Fatal(e)
			}
			var receipt struct {
				InputID string `json:"input_id"`
			}
			_ = json.Unmarshal(raw, &receipt)
			var record durableinput.Record
			for {
				record, e = inputs.GetDoctor(owner, receipt.InputID)
				if e != nil {
					t.Fatal(e)
				}
				if record.Status == "waiting_for_user" || record.Status == "completed" || record.Status == "blocked" {
					break
				}
				if ctx.Err() != nil {
					t.Fatal("operator run timed out")
				}
				time.Sleep(30 * time.Millisecond)
			}
			if mode == commonv1.AutonomyMode_AUTONOMY_MODE_1 {
				if record.Status != "waiting_for_user" || runtime.calls.Load() != 0 {
					t.Fatal("approval was bypassed", record.Status)
				}
				// Restart the existing worker before the exact owner decision.
				stop()
				<-done
				var id string
				if e = pool.QueryRow(ctx, `SELECT approval_id FROM approvals WHERE run_id=$1`, record.RunID).Scan(&id); e != nil {
					t.Fatal(e)
				}
				actor, _ := owneridentity.FromContext(owner)
				actor.SessionKey = record.SessionKey
				actor.DeliveryID = "approve-doctor"
				_, _, e = approvalService.ResolveByApprovalID(owneridentity.WithActor(ctx, actor), approvals.ResolveByApprovalIDParams{ApprovalID: id, Approved: true})
				if e != nil {
					t.Fatal(e)
				}
				workerCtx, stop = context.WithCancel(ctx)
				defer stop()
				done = make(chan struct{})
				go func() { defer close(done); _ = worker.Run(workerCtx) }()
				for {
					record, e = inputs.GetDoctor(owner, receipt.InputID)
					if e != nil {
						t.Fatal(e)
					}
					if record.Status == "completed" || record.Status == "blocked" {
						break
					}
					if ctx.Err() != nil {
						t.Fatal("approved restart timed out")
					}
					time.Sleep(30 * time.Millisecond)
				}
			}
			if record.Status != "completed" || runtime.calls.Load() != 1 {
				t.Fatal("operator flow failed", record.Status, record.ErrorCode, record.ErrorMessage, runtime.calls.Load())
			}
			status, report, e := checker.ReadCheck(owner, receipt.InputID)
			if e != nil || status != "healthy" || !strings.Contains(string(report), "controlled_runtime") {
				t.Fatal(status, string(report), e)
			}
			_, _ = client.ExecuteToolCall(ctx, &toolbrokerv1.ToolCall{RunId: record.RunID, ToolCallId: "unapproved", ToolName: "doctor.check_system", ArgsJson: "{}"})
			if runtime.calls.Load() != 1 {
				t.Fatal("missing grant reached runtime")
			}
		})
	}
}
