package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/activity"
	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

// This opt-in server runs only in the disposable owner-state lab network. The
// browser uses production HTTP handlers/PG stores; only initial observations
// are synthetic. There is no provider, executor, credentials or main deployment.
func TestBrowserOwnerStateFixtureServer(t *testing.T) {
	if os.Getenv("BUTLER_OWNER_UI_FIXTURE_SERVER") != "1" {
		t.Skip("isolated owner browser fixture server opt-in required")
	}
	pool := memorytest.Database(t)
	ctx := context.Background()
	store := task.NewStore(pool)
	sessions := session.NewPostgresRepository(pool)
	_, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: "telegram:chat:777", UserID: "123", Channel: "telegram", MetadataJSON: `{}`})
	if err != nil {
		t.Fatal(err)
	}
	create := func(goal string, parent string) task.Record {
		record, e := store.Create(ctx, task.CreateRequest{SessionKey: "telegram:chat:777", OwnerID: "123", ParentTaskID: parent, Goal: goal, CompletionCriteria: []string{"Owner verifies fixture evidence"}})
		if e != nil {
			t.Fatal(e)
		}
		return record
	}
	root := create("Owner controls fixture", "")
	child := create("Internal research branch", root.TaskID)
	lost := create("Lost acknowledgement fixture", "")
	resultTask := create("Saved response remains unverified", "")
	if _, err = pool.Exec(ctx, `UPDATE tasks SET status='waiting_for_user' WHERE task_id=$1`, root.TaskID); err != nil {
		t.Fatal(err)
	}
	runs := run.NewPostgresRepository(pool)
	_, err = runs.CreateRun(ctx, run.Record{RunID: "owner-ui-finished", SessionKey: resultTask.SessionKey, InputEventID: "fixture-finished", Status: "running", CurrentState: "finalizing", AutonomyMode: "mode_1", ModelProvider: "fixture", MetadataJSON: `{}`, StartedAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AttachRun(ctx, resultTask.TaskID, 1, "owner-ui-finished", resultTask.SessionKey); err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManager(pool)
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: resultTask.SessionKey, RunID: "owner-ui-finished", OwnerID: "fixture-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE runs SET lease_id=$1 WHERE run_id='owner-ui-finished'`, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: resultTask.SessionKey, RunID: "owner-ui-finished", LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	coordinator := completion.New(pool)
	saved, err := coordinator.Save(owned, completion.Request{RunID: "owner-ui-finished", SessionKey: resultTask.SessionKey, TaskID: resultTask.TaskID, TaskRevision: 1, ConstraintsRevision: 1, Content: "A stored fixture response", AnswerState: "final"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE delivery_outbox SET status='delivery_unknown' WHERE run_id='owner-ui-finished'`); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"effect_confirmed", "effect_unknown"} {
		binding, _ := json.Marshal(map[string]any{"action_id": state, "owner_id": "123", "task_id": resultTask.TaskID, "task_revision": 1, "tool_name": "http.request", "effect": "commit", "contract_hash": strings.Repeat("a", 64)})
		if _, err = pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES($1,$2,$3,'{}',$4)`, state, strings.Repeat("b", 64), binding, state); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id) VALUES($1,'owner-ui-finished',$1,$2,$3,false,clock_timestamp()+INTERVAL '1 hour',$1)`, state, strings.Repeat("a", 64), binding); err != nil {
			t.Fatal(err)
		}
	}
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "123", Channel: "web", DeliveryID: "owner-fixture-result"})
	verified, err := store.SaveVerifiedResult(owner, task.Result{TaskID: resultTask.TaskID, TaskRevision: 1, RunID: saved.RunID, Summary: "Owner retained fixture evidence", EvidenceRefs: []string{"artifact:" + saved.ArtifactID}, VerifiedCriteria: resultTask.CompletionCriteria, Verifier: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	auth := NewOwnerControlAuth("fixture-owner-token-000000000000000", "123")
	controller := task.NewController(store, leases.Authority(), leases, runs, approval.NewPostgresRepository(pool), durableinput.NewStore(pool), transcript.NewStore(pool))
	controls := NewTaskControlServer(store)
	controls.SetOwnerControlAuth(auth)
	controls.SetControls(controller)
	artifactRepo := artifacts.NewPostgresRepository(pool)
	artifactServer := NewArtifactsServer(artifactRepo, activity.NewPostgresRepository(pool), nil)
	artifactServer.SetDurableTasks(store)
	view := NewTaskViewServer(runs, runs, sessions, transcript.NewStore(pool), run.NewPostgresTransitionRepository(pool), artifactRepo, nil)
	view.SetDurableTasks(store)
	view.SetFinalResults(coordinator, deliveryoutbox.NewStore(pool))
	done := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/fixture/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"root": root.TaskID, "child": child.TaskID, "lost": lost.TaskID, "result": resultTask.TaskID, "artifact": saved.ArtifactID, "verified_result": verified.ResultID})
	})
	mux.HandleFunc("/fixture/done", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		writeJSON(w, 200, map[string]bool{"accepted": true})
		select {
		case done <- struct{}{}:
		default:
		}
	})
	mux.Handle("/api/v3/tasks/", controls.HandleItem("/api/v3/tasks/"))
	mux.Handle("/api/v2/tasks", view.HandleListTasks())
	mux.Handle("/api/v2/tasks/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/artifacts"):
			artifactServer.HandleListTaskArtifacts("/api/v2/tasks/").ServeHTTP(w, r)
		case strings.HasSuffix(r.URL.Path, "/activity"):
			artifactServer.HandleListTaskActivity("/api/v2/tasks/").ServeHTTP(w, r)
		default:
			view.HandleGetTaskDetail("/api/v2/tasks/").ServeHTTP(w, r)
		}
	}))
	mux.Handle("/api/v2/artifacts", artifactServer.HandleListArtifacts())
	mux.Handle("/api/v2/artifacts/", artifactServer.HandleGetArtifact("/api/v2/artifacts/"))
	listener, err := net.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://127.0.0.1:4173")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Butler-Delivery-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		mux.ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	select {
	case <-done:
	case <-time.After(12 * time.Minute):
		t.Fatal("owner browser fixture timed out")
	}
	current, err := store.Get(ctx, lost.TaskID)
	if err != nil || current.Revision != 2 {
		t.Fatal("actual browser lost ACK repeated amendment")
	}
	var receipts int
	if pool.QueryRow(ctx, `SELECT count(*) FROM task_control_events WHERE task_id=$1`, lost.TaskID).Scan(&receipts) != nil || receipts != 1 {
		t.Fatal("browser control did not deduplicate durable delivery")
	}
	current, err = store.Get(ctx, root.TaskID)
	if err != nil || current.Status != "active" || current.Revision != 4 {
		t.Fatal("ordinary pause/resume/amend or branch isolation failed")
	}
	current, err = store.Get(ctx, child.TaskID)
	if err != nil || current.Status != "cancelled" {
		t.Fatal("browser failed to stop actual branch")
	}
	t.Log("actual PG/HTTP/Chromium owner controls and evidence fixture passed; model_calls=0")
}
