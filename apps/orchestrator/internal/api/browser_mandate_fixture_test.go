package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
)

type browserMandateOwnerReader struct {
	browserSafetyOwnerReader
	approvals *approval.Service
}

func (r browserMandateOwnerReader) VerifyActionApproval(ctx context.Context, req actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	return r.approvals.VerifyActionApproval(ctx, req)
}

// Test-only worker projection: real owner APIs admit Task/Input and exact
// approvals. A mechanical worker claims the Input and creates its run/lease.
// There is no model, production bypass, fabricated approval or delivery proof.
func TestBrowserMandateOwnerAPIHarness(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_MANDATE_OWNER_HARNESS") != "1" {
		t.Skip("isolated owner API/Broker/Chromium composition required")
	}
	pool := memorytest.Database(t)
	ctx := context.Background()
	tasks := task.NewStore(pool)
	sessions := session.NewPostgresRepository(pool)
	runs := run.NewPostgresRepository(pool)
	leases := session.NewPostgresLeaseManager(pool)
	inputs := durableinput.NewStore(pool)
	repo := approval.NewPostgresRepository(pool)
	repo.SetCheckpointHooks(inputs)
	approvals := approval.NewService(repo, nil)
	controller := task.NewController(tasks, leases.Authority(), leases, runs, repo, inputs, transcript.NewStore(pool))
	manager := goal.New(pool, tasks, controller)
	for _, kind := range []string{"prepare_only", "unsafe_unknown", "strict", "changed_args", "revoke", "cancel", "expiry", "foreign_origin", "stale_contract", "restart", "budget", "scope_origin", "scope_epoch"} {
		if _, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: "web:mandate-" + kind, UserID: "owner", Channel: "web", MetadataJSON: `{}`}); err != nil {
			t.Fatal("synthetic session bootstrap failed")
		}
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE current_resource_sources(run_id TEXT PRIMARY KEY,status TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	auth := NewOwnerControlAuth("fixture-owner-token-000000000000000", "owner")
	mux := http.NewServeMux()
	goals := NewGoalServer(manager, auth).Handle()
	for _, path := range []string{"/api/v3/owner-goals", "/api/v3/owner-mandates", "/api/v3/owner-goals/tasks"} {
		mux.Handle(path, goals)
	}
	controls := NewTaskControlServer(tasks)
	controls.SetOwnerControlAuth(auth)
	controls.SetControls(controller)
	mux.Handle("/api/v3/tasks/", controls.HandleItem("/api/v3/tasks/"))
	approvalAPI := NewApprovalsServer(repo, approvals, nil)
	approvalAPI.SetOwnerControlAuth(auth)
	mux.Handle("/api/v2/approvals/", approvalAPI.HandleApprove("/api/v2/approvals/"))
	mux.Handle("/private/context", actionauth.NewContextHandler(strings.Repeat("owner-context-fixture-", 3), browserMandateOwnerReader{browserSafetyOwnerReader{pool}, approvals}))
	workerContexts := map[string]context.Context{}
	var workerMu sync.Mutex
	mux.HandleFunc("/fixture/claim", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Task, Input, Run string }
		if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&q) != nil {
			w.WriteHeader(400)
			return
		}
		claim, err := inputs.Claim(ctx, 10*time.Minute)
		if err != nil || claim.InputID != q.Input || claim.TaskID != q.Task {
			w.WriteHeader(409)
			return
		}
		record, err := tasks.Get(ctx, q.Task)
		if err != nil {
			w.WriteHeader(409)
			return
		}
		_, err = runs.CreateRun(ctx, run.Record{RunID: q.Run, SessionKey: claim.SessionKey, InputEventID: claim.Event.EventID, Status: "tool_pending", CurrentState: "tool_pending", AutonomyMode: "mode_2", ModelProvider: "fixture", MetadataJSON: `{}`, StartedAt: time.Now(), UpdatedAt: time.Now()})
		if err == nil {
			err = tasks.AttachRun(ctx, record.TaskID, record.Revision, q.Run, claim.SessionKey)
		}
		lease, leaseErr := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: claim.SessionKey, RunID: q.Run, OwnerID: "fixture-worker", TTL: 10 * time.Minute})
		if err != nil || leaseErr != nil {
			w.WriteHeader(409)
			return
		}
		if _, err = pool.Exec(ctx, `UPDATE runs SET lease_id=$2 WHERE run_id=$1;`, q.Run, lease.LeaseID); err != nil {
			w.WriteHeader(409)
			return
		}
		worker := ownership.WithToken(durableinput.WithClaim(ctx, claim), leases.Authority(), ownership.Token{SessionKey: claim.SessionKey, RunID: q.Run, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
		if err = inputs.SaveCheckpoint(worker, q.Run, "prepared", `{}`); err != nil {
			w.WriteHeader(409)
			return
		}
		workerMu.Lock()
		workerContexts[q.Run] = worker
		workerMu.Unlock()
		if _, err = pool.Exec(ctx, `INSERT INTO current_resource_sources VALUES($1,'current')`, q.Run); err != nil {
			w.WriteHeader(409)
			return
		}
		writeJSON(w, 200, map[string]any{"claimed": true, "task_revision": record.Revision, "result_delivery": record.ResultDelivery})
	})
	mux.HandleFunc("/fixture/approval", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Binding actionauth.ActionBinding
			Args    string
		}
		if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&q) != nil {
			w.WriteHeader(400)
			return
		}
		workerMu.Lock()
		worker, ok := workerContexts[q.Binding.RunID]
		workerMu.Unlock()
		if !ok {
			w.WriteHeader(409)
			return
		}
		expires := time.Now().Add(time.Minute)
		v, err := approvals.CreatePendingApproval(worker, approval.CreatePendingParams{ActionBinding: q.Binding, ContinuationJSON: `{"version":1}`, RunID: q.Binding.RunID, SessionKey: "web:mandate-" + strings.TrimPrefix(q.Binding.RunID, "mandate-"), ToolCallID: q.Binding.ToolCallID, ToolName: q.Binding.ToolName, ArgsJSON: q.Args, RequestedVia: "web", ExpiresAt: &expires})
		if err != nil {
			w.WriteHeader(409)
			return
		}
		writeJSON(w, 200, map[string]string{"approval_id": v.ApprovalID})
	})
	// Serial parent requests only; no policy/result state is changed by this exit.
	done := make(chan struct{}, 1)
	mux.HandleFunc("/fixture/done", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, true); done <- struct{}{} })
	server := httptest.NewServer(mux)
	defer server.Close()
	data, _ := json.Marshal(map[string]string{"DSN": pool.Config().ConnString(), "SearchPath": pool.Config().ConnConfig.RuntimeParams["search_path"], "Address": server.URL})
	if os.WriteFile(os.Getenv("BUTLER_BROWSER_MANDATE_OWNER_READY"), data, 0600) != nil {
		t.Fatal("private readiness write failed")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Minute):
		t.Fatal("mandate harness timeout")
	}
}
