package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Test-only composition of real owner services. The context projection is a
// fixture adapter over their actual PG state; it cannot manufacture approvals.
type browserSafetyOwnerReader struct{ pool *pgxpool.Pool }

func (r browserSafetyOwnerReader) VerifyActionApproval(context.Context, actionauth.ApprovalRequest) (actionauth.ApprovalSnapshot, error) {
	return actionauth.ApprovalSnapshot{}, errors.New("no approvals in this fixture")
}
func (r browserSafetyOwnerReader) GetActionContext(ctx context.Context, id string) (actionauth.ContextSnapshot, error) {
	c := actionauth.ContextSnapshot{RunID: id, AutonomyMode: 3}
	err := r.pool.QueryRow(ctx, `SELECT r.current_state,r.session_key,s.user_id,r.task_id,t.revision,r.task_revision,t.constraints_revision,t.status,o.lease_id,o.fence_epoch,o.expires_at FROM runs r JOIN sessions s USING(session_key) JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.session_key=r.session_key WHERE r.run_id=$1 AND o.run_id=r.run_id`, id).Scan(&c.RunState, &c.SessionKey, &c.OwnerID, &c.TaskID, &c.TaskRevision, &c.PinnedTaskRevision, &c.ConstraintsRevision, &c.TaskStatus, &c.LeaseID, &c.LeaseEpoch, &c.LeaseExpiresAt)
	if err != nil {
		return c, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return c, err
	}
	defer tx.Rollback(ctx)
	a, err := actionauth.CheckTaskAuthorityTx(ctx, tx, c.TaskID, c.OwnerID, nil, time.Now())
	if err != nil {
		return c, err
	}
	m, g, err := actionauth.CheckAssignmentTx(ctx, tx, a, time.Now())
	if err != nil {
		return c, err
	}
	c.Authority, c.MandateID, c.ModelPrivacy = &a, a.MandateID, m.Privacy
	c.AuthorityDeadline, err = actionauth.AuthorityDeadline(m, g, time.Now())
	return c, err
}

func TestBrowserSafetyOwnerAPIHarness(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_SAFETY_OWNER_HARNESS") != "1" {
		t.Skip("isolated owner UI/Broker harness required")
	}
	pool := memorytest.Database(t)
	ctx := context.Background()
	tasks := task.NewStore(pool)
	sessions := session.NewPostgresRepository(pool)
	runs := run.NewPostgresRepository(pool)
	leases := session.NewPostgresLeaseManager(pool)
	controller := task.NewController(tasks, leases.Authority(), leases, runs, approval.NewPostgresRepository(pool), durableinput.NewStore(pool), transcript.NewStore(pool))
	manager := goal.New(pool, tasks, controller)
	actor := func(delivery string) context.Context {
		return owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: delivery})
	}
	g := actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Isolated browser owner control", Purpose: "browser_safety", ExpiresAt: time.Now().Add(time.Hour)}
	gr, err := manager.PutGoal(actor("goal"), goal.GoalRequest{Goal: g})
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]string{}
	for _, kind := range []string{"stop", "revoke", "control"} {
		key := "web:browser-owner-" + kind
		if _, _, err = sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: "owner", Channel: "web", MetadataJSON: `{}`}); err != nil {
			t.Fatal(err)
		}
		m := actionauth.Mandate{GoalID: gr.SubjectID, GoalRevision: 1, Status: "active", Purpose: g.Purpose, Tools: []string{"browser.open", "browser.click"}, ResourceScopes: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/"}}, Effects: []actionauth.Effect{actionauth.EffectRead, actionauth.EffectUnknown}, Privacy: "cloud_allowed", Budget: actionauth.MandateBudget{ModelTurns: 20, Tokens: 20000, ExternalCalls: 10, Children: 2, ActiveMillis: 600000, ProviderInputBytes: 100000000, EgressBytes: 100000000}, ExpiresAt: time.Now().Add(30 * time.Minute), Intervention: []string{"effect_unknown", "budget_exhausted", "scope_change"}, Reporting: "exceptions"}
		mr, err := manager.PutMandate(actor(kind+"-mandate"), goal.MandateRequest{Mandate: m})
		if err != nil {
			t.Fatal(err)
		}
		record, err := manager.CreateOwnerTask(actor(kind+"-task"), goal.TaskRequest{SessionKey: key, GoalID: gr.SubjectID, MandateID: mr.SubjectID, Purpose: g.Purpose, Goal: "Check " + kind + " before a browser POST", CompletionCriteria: []string{"No dispatch after owner control"}, ObservationRef: "observation:synthetic", InitiativeReason: "Owner acceptance test"})
		if err != nil {
			t.Fatal(err)
		}
		id := "browser-owner-" + kind
		if _, err = runs.CreateRun(ctx, run.Record{RunID: id, SessionKey: key, InputEventID: id, Status: "tool_pending", CurrentState: "tool_pending", AutonomyMode: "mode_2", ModelProvider: "fixture", MetadataJSON: `{}`, StartedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if err = tasks.AttachRun(ctx, record.TaskID, 1, id, key); err != nil {
			t.Fatal(err)
		}
		lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: key, RunID: id, OwnerID: "fixture-worker", TTL: 10 * time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `UPDATE runs SET lease_id=$2 WHERE run_id=$1`, id, lease.LeaseID); err != nil {
			t.Fatal(err)
		}
		refs[kind+"_task"], refs[kind+"_run"], refs[kind+"_mandate"] = record.TaskID, id, mr.SubjectID
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE current_resource_sources(run_id TEXT PRIMARY KEY,status TEXT NOT NULL);INSERT INTO current_resource_sources VALUES('browser-owner-stop','current'),('browser-owner-revoke','current'),('browser-owner-control','current')`); err != nil {
		t.Fatal(err)
	}
	auth := NewOwnerControlAuth("fixture-owner-token-000000000000000", "owner")
	controls := NewTaskControlServer(tasks)
	controls.SetOwnerControlAuth(auth)
	controls.SetControls(controller)
	mux := http.NewServeMux()
	mux.Handle("/api/v3/tasks/", controls.HandleItem("/api/v3/tasks/"))
	view := NewTaskViewServer(runs, runs, sessions, transcript.NewStore(pool), run.NewPostgresTransitionRepository(pool), artifacts.NewPostgresRepository(pool), nil)
	view.SetDurableTasks(tasks)
	mux.Handle("/api/v2/tasks", view.HandleListTasks())
	mux.Handle("/api/v2/tasks/", view.HandleGetTaskDetail("/api/v2/tasks/"))
	goals := NewGoalServer(manager, auth).Handle()
	mux.Handle("/api/v3/owner-authority", goals)
	mux.Handle("/api/v3/owner-mandates", goals)
	mux.Handle("/private/context", actionauth.NewContextHandler(strings.Repeat("owner-context-fixture-", 3), browserSafetyOwnerReader{pool}))
	mux.HandleFunc("/fixture/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, refs) })
	done := make(chan struct{}, 1)
	mux.HandleFunc("/fixture/done", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		writeJSON(w, 200, true)
		select {
		case done <- struct{}{}:
		default:
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	ready := map[string]any{"DSN": pool.Config().ConnString(), "SearchPath": pool.Config().ConnConfig.RuntimeParams["search_path"], "Address": "http://" + listener.Addr().String(), "Refs": refs}
	data, _ := json.Marshal(ready)
	if os.WriteFile(os.Getenv("BUTLER_BROWSER_SAFETY_OWNER_READY"), data, 0600) != nil {
		t.Fatal("private fixture ready write")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Minute):
		t.Fatal("owner UI harness timed out")
	}
}
