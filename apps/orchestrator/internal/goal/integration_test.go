package goal_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	postgres "github.com/butler/butler/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	manager    *goal.Manager
	tasks      *task.Store
	controller *task.Controller
	leases     *session.PostgresLeaseManager
	owner, key string
	g          actionauth.GoalPolicy
	m          actionauth.Mandate
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_GOALS_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_GOALS_POSTGRES_URL is not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("isolated *_test database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	t.Cleanup(cancel)
	// Each test has a private schema; source database/server are never reset.
	admin, err := pgx.ConnectConfig(ctx, cfg.ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("owner_goal_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public; CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	db, err := postgres.Open(ctx, postgres.Config{URL: dsn + separator + "search_path=" + schema + ",public", MaxConns: 2, MinConns: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		cleanup, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_, e := admin.Exec(cleanup, `DROP SCHEMA `+schema+` CASCADE`)
		if e != nil {
			t.Error(e)
		}
		admin.Close(cleanup)
	})
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	f := &fixture{ctx: ctx, pool: db.Pool(), owner: "synthetic-owner", key: "web:synthetic-owner"}
	f.tasks = task.NewStore(f.pool)
	f.leases = session.NewPostgresLeaseManager(f.pool)
	f.controller = task.NewController(f.tasks, f.leases.Authority(), f.leases, run.NewPostgresRepository(f.pool), approval.NewPostgresRepository(f.pool), durableinput.NewStore(f.pool), transcript.NewStore(f.pool))
	f.manager = goal.New(f.pool, f.tasks, f.controller)
	if _, err = f.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,$2,'web')`, f.key, f.owner); err != nil {
		t.Fatal(err)
	}
	f.g = actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Maintain synthetic test service", Purpose: "service_maintenance", Constraints: []string{"Keep work service out of scope"}, ExpiresAt: time.Now().Add(48 * time.Hour)}
	receipt, err := f.manager.PutGoal(f.actor("goal-create"), goal.GoalRequest{Goal: f.g})
	if err != nil {
		t.Fatal(err)
	}
	f.g.ID = receipt.SubjectID
	f.g.Revision = receipt.Revision
	f.g.OwnerID = f.owner
	f.m = actionauth.Mandate{GoalID: f.g.ID, GoalRevision: 1, Status: "active", Purpose: f.g.Purpose, Tools: []string{"http.request"}, ResourceScopes: []actionauth.ResourceScope{{Origin: "https://example.invalid", PathPrefix: "/test-service"}}, Effects: []actionauth.Effect{actionauth.EffectRead, actionauth.EffectCommit}, Privacy: "cloud_allowed", Budget: actionauth.MandateBudget{ModelTurns: 20, Tokens: 20000, ExternalCalls: 10, Children: 5, ActiveMillis: 600000, ProviderInputBytes: 100000000, EgressBytes: 100000000}, ExpiresAt: time.Now().Add(24 * time.Hour), AllowInitiative: true, Intervention: []string{"scope_change", "effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}
	receipt, err = f.manager.PutMandate(f.actor("mandate-create"), goal.MandateRequest{Mandate: f.m})
	if err != nil {
		t.Fatal(err)
	}
	f.m, err = actionauth.NewPostgresStore(f.pool).GetMandate(ctx, receipt.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *fixture) actor(delivery string) context.Context {
	return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.owner, Channel: "web", DeliveryID: delivery})
}
func (f *fixture) create(t *testing.T, delivery string) task.Record {
	t.Helper()
	r, err := f.manager.CreateOwnerTask(f.actor(delivery), goal.TaskRequest{SessionKey: f.key, GoalID: f.g.ID, MandateID: f.m.ID, Purpose: f.g.Purpose, Goal: "Diagnose, test and recover synthetic service", CompletionCriteria: []string{"Owner verifies recovery evidence"}, ObservationRef: "observation:test-service-health", InitiativeReason: "Synthetic service health needs recovery"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func (f *fixture) run(t *testing.T, r task.Record, state string) (string, context.Context) {
	t.Helper()
	id := fmt.Sprintf("goal-run:%d", time.Now().UnixNano())
	_, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,started_at,updated_at,status,current_state,autonomy_mode,model_provider,task_id,task_revision) VALUES($1,$2,$1,clock_timestamp(),clock_timestamp(),$3,$3,'mode_2','fixture',$4,$5)`, id, f.key, state, r.TaskID, r.Revision)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.key, RunID: id, OwnerID: "worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE runs SET lease_id=$2 WHERE run_id=$1`, id, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	return id, ownership.WithToken(f.ctx, f.leases.Authority(), ownership.Token{SessionKey: f.key, RunID: id, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
}

func TestOwnerAuthorityProofAndDeliveryIntegration(t *testing.T) {
	f := setup(t)
	if _, err := f.manager.PutGoal(f.ctx, goal.GoalRequest{Goal: f.g}); !errors.Is(err, goal.ErrUnauthorized) {
		t.Fatal("unproved owner authority", err)
	}
	server := api.NewGoalServer(f.manager, api.NewOwnerControlAuth(strings.Repeat("x", 40), f.owner)).Handle()
	body := `{"goal":{"kind":"objective","status":"active","title":"Spoof","purpose":"service_maintenance","expires_at":"2027-01-01T00:00:00Z"}}`
	request := httptest.NewRequest("POST", "/api/v3/owner-goals", strings.NewReader(body))
	request.Header.Set("X-Butler-Actor", f.owner)
	request.Header.Set("X-Butler-Delivery-ID", "spoof")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatal("header supplied authority", response.Code)
	}
	request = httptest.NewRequest("GET", "/api/v3/owner-authority", nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatal("personal goal read unauthenticated", response.Code)
	}
	request = httptest.NewRequest("POST", "/api/v3/owner-goals", strings.NewReader(body+` {"goal":{}}`))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 40))
	request.Header.Set("X-Butler-Delivery-ID", "trailing-request")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatal("multiple owner decisions accepted as one delivery", response.Code)
	}
	req := goal.GoalRequest{Goal: actionauth.GoalPolicy{Kind: "preference", Status: "active", Title: "Prefer verified outcomes", Purpose: f.g.Purpose, ExpiresAt: f.g.ExpiresAt}}
	r, err := f.manager.PutGoal(f.actor("same-delivery"), req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.manager.PutGoal(f.actor("same-delivery"), req)
	if err != nil || again.EventID != r.EventID {
		t.Fatal("decision replay changed identity", again, err)
	}
	req.Goal.Title = "Different intent"
	if _, err = f.manager.PutGoal(f.actor("same-delivery"), req); !errors.Is(err, goal.ErrConflict) {
		t.Fatal("same delivery changed decision", err)
	}
	v, err := f.manager.View(f.actor("view"))
	if err != nil || len(v.Goals) != 2 || v.Goals[0].Source.Author != f.owner {
		t.Fatal("owner view/provenance", v, err)
	}
}

func TestAssumptionCannotPromoteOrGrantIntegration(t *testing.T) {
	f := setup(t)
	r := f.create(t, "task")
	_, runCtx := f.run(t, r, "model_running")
	proposal, err := f.manager.ProposeAssumption(runCtx, "An agent hypothesis", "service_maintenance", "observation:hypothesis")
	if err != nil {
		t.Fatal(err)
	}
	candidate := f.m
	candidate.ID = ""
	candidate.GoalID = proposal.SubjectID
	if _, err = f.manager.PutMandate(f.actor("invalid-assumption-grant"), goal.MandateRequest{Mandate: candidate}); err == nil {
		t.Fatal("assumption granted authority")
	}
	g := f.g
	g.ID = proposal.SubjectID
	g.Kind = "objective"
	if _, err = f.manager.PutGoal(f.actor("implicit-promotion"), goal.GoalRequest{Goal: g, ExpectedRevision: 1}); !errors.Is(err, goal.ErrConflict) {
		t.Fatal("assumption silently promoted", err)
	}
	g.ID = ""
	confirmed, err := f.manager.PutGoal(f.actor("explicit-confirmation"), goal.GoalRequest{Goal: g, ParentRef: proposal.SubjectID})
	if err != nil || confirmed.SubjectID == proposal.SubjectID {
		t.Fatal("explicit confirmed objective missing", confirmed, err)
	}
	view, err := f.manager.View(f.actor("read"))
	if err != nil {
		t.Fatal(err)
	}
	proposed, linked := false, false
	for _, g := range view.Goals {
		if g.ID == proposal.SubjectID {
			proposed = g.Kind == "assumption" && g.Status == "proposed"
		}
		if g.ID == confirmed.SubjectID {
			linked = g.Source.ParentRef == proposal.SubjectID
		}
	}
	if !proposed || !linked {
		t.Fatal("proposal/history distinction lost")
	}
}

func TestBroadAssignmentChildrenAmbiguityAndImmutableBudgetIntegration(t *testing.T) {
	f := setup(t)
	root := f.create(t, "root")
	child, err := f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.key, OwnerID: f.owner, ParentTaskID: root.TaskID, Goal: "Test recovery", CompletionCriteria: []string{"tests pass"}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := actionauth.GetTaskAssignment(f.ctx, f.pool, child.TaskID)
	if err != nil || a.MandateID != f.m.ID || a.GoalRevision != 1 || a.InitiativeReason == "" || a.ObservationRef == "" {
		t.Fatal("child authority missing", a, err)
	}
	second := f.m
	second.ID = ""
	second.Revision = 0
	receipt, err := f.manager.PutMandate(f.actor("second-mandate"), goal.MandateRequest{Mandate: second})
	if err != nil {
		t.Fatal(err)
	}
	_ = receipt
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	_, err = f.manager.CheckAdmissionTx(f.ctx, tx, goal.AdmissionRequest{OwnerID: f.owner, GoalID: f.g.ID, Purpose: f.g.Purpose, ObservationRef: "observation:ambiguous", InitiativeReason: "Test explicit selection"}, time.Now())
	if !errors.Is(err, goal.ErrConflict) {
		t.Fatal("ambiguous grants selected implicitly", err)
	}
	tx.Rollback(f.ctx)
	changed := f.m
	changed.Budget.ModelTurns++
	if _, err = f.manager.PutMandate(f.actor("replenish"), goal.MandateRequest{Mandate: changed, ExpectedRevision: 1}); !errors.Is(err, goal.ErrConflict) {
		t.Fatal("revision replenished quota", err)
	}
	changed = f.m
	changed.Status = "paused"
	if _, err = f.manager.PutMandate(f.actor("pause"), goal.MandateRequest{Mandate: changed, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.key, OwnerID: f.owner, ParentTaskID: child.TaskID, Goal: "Stale grandchild", CompletionCriteria: []string{"tests pass"}}); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("stale child admitted", err)
	}
	if _, err = f.controller.Apply(f.actor("resume-stale"), task.ControlRequest{TaskID: root.TaskID, ExpectedRevision: root.Revision, Kind: task.ControlAmend, Instruction: "Resume with stale mandate"}); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("owner task control bypassed stale mandate", err)
	}
}

func TestRevocationDeniesBeforeFailedPropagationAndRestartIntegration(t *testing.T) {
	f := setup(t)
	r := f.create(t, "root")
	child, err := f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.key, OwnerID: f.owner, ParentTaskID: r.TaskID, Goal: "Child tests", CompletionCriteria: []string{"verified"}})
	if err != nil {
		t.Fatal(err)
	}
	runID, runCtx := f.run(t, r, "finalizing")
	revoked := f.m
	revoked.Status = "revoked"
	receipt, err := f.manager.PutMandate(f.actor("revoke"), goal.MandateRequest{Mandate: revoked, ExpectedRevision: 1})
	if err != nil || receipt.Propagation != "pending" {
		t.Fatal("revocation was not durable", receipt, err)
	}
	// A late provider final cannot write any required result/outbox even though
	// asynchronous branch cancellation has not yet revoked the worker lease.
	coordinator := completion.New(f.pool)
	if _, err = coordinator.Save(runCtx, completion.Request{RunID: runID, SessionKey: f.key, TaskID: r.TaskID, TaskRevision: r.Revision, ConstraintsRevision: r.ConstraintsRevision, Content: "Late final", AnswerState: "final"}); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("late result persisted", err)
	}
	if _, err = f.tasks.CheckpointRun(runCtx, runID, r.WorkingRevision, task.WorkingState{}); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("late task state persisted", err)
	}
	if _, err = f.pool.Exec(f.ctx, `CREATE FUNCTION fail_owner_sweep() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture propagation failure'; END $$; CREATE TRIGGER fail_owner_sweep BEFORE UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION fail_owner_sweep()`); err != nil {
		t.Fatal(err)
	}
	if err = f.manager.Sweep(f.ctx); err == nil {
		t.Fatal("fault injection did not fail propagation")
	}
	var state string
	var attempts int
	if err = f.pool.QueryRow(f.ctx, `SELECT state,attempts FROM owner_authority_jobs WHERE event_id=$1`, receipt.EventID).Scan(&state, &attempts); err != nil || state != "pending" || attempts < 1 {
		t.Fatal("failed job disappeared", state, attempts, err)
	}
	if _, err = f.pool.Exec(f.ctx, `DROP TRIGGER fail_owner_sweep ON tasks; DROP FUNCTION fail_owner_sweep()`); err != nil {
		t.Fatal(err)
	}
	restarted := goal.New(f.pool, f.tasks, f.controller)
	if err = restarted.Sweep(f.ctx); err != nil {
		t.Fatal("restart sweep", err)
	}
	for _, id := range []string{r.TaskID, child.TaskID} {
		record, err := f.tasks.Get(f.ctx, id)
		if err != nil || record.Status != "cancelled" {
			t.Fatal("branch survived revocation", record, err)
		}
	}
	if err = f.pool.QueryRow(f.ctx, `SELECT state FROM owner_authority_jobs WHERE event_id=$1`, receipt.EventID).Scan(&state); err != nil || state != "applied" {
		t.Fatal("job completion not saved", state, err)
	}
	for _, table := range []string{"run_final_results", "delivery_outbox", "task_results"} {
		var n int
		if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM `+table+` WHERE run_id=$1`, runID).Scan(&n); err != nil || n != 0 {
			t.Fatal("revocation invented a final result", table, n, err)
		}
	}
}

func TestGoalAmendmentImmediatelyInvalidatesMandatesIntegration(t *testing.T) {
	f := setup(t)
	r := f.create(t, "task")
	updated := f.g
	updated.Constraints = []string{"Prepare diagnostics only; no production recovery"}
	receipt, err := f.manager.PutGoal(f.actor("amend-goal"), goal.GoalRequest{Goal: updated, ExpectedRevision: 1})
	if err != nil || receipt.Revision != 2 || receipt.Propagation != "pending" {
		t.Fatal(receipt, err)
	}
	if _, _, _, err = f.manager.ReadTaskAuthority(f.ctx, r.TaskID); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("old goal remained authority", err)
	}
	view, err := f.manager.View(f.actor("view"))
	if err != nil || len(view.Obligations) != 1 || view.Obligations[0].AuthorityCurrent || !strings.Contains(strings.Join(view.Goals[0].Constraints, " "), "Prepare diagnostics only") {
		data, _ := json.Marshal(view)
		t.Fatal(string(data), err)
	}
}

func TestConfirmedPurposeConstraintsFenceOldTasksAndProtectFreshContextIntegration(t *testing.T) {
	f := setup(t)
	old := f.create(t, "old-task")
	constraint := actionauth.GoalPolicy{Kind: "constraint", Status: "active", Title: "Only prepare recovery plan", Purpose: f.g.Purpose, Constraints: []string{"Never submit production changes"}, ExpiresAt: f.g.ExpiresAt}
	receipt, err := f.manager.PutGoal(f.actor("new-constraint"), goal.GoalRequest{Goal: constraint})
	if err != nil || receipt.Propagation != "pending" {
		t.Fatal(receipt, err)
	}
	if _, _, _, err = f.manager.ReadTaskAuthority(f.ctx, old.TaskID); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("new confirmed constraint ignored by old task", err)
	}
	fresh := f.create(t, "fresh-task")
	a, _, g, err := f.manager.ReadTaskAuthority(f.ctx, fresh.TaskID)
	if err != nil || a.PolicyRevision != 1 || len(g.Policies) != 1 || g.Policies[0].Constraints[0] != "Never submit production changes" {
		t.Fatal("full confirmed policy missing", a, g, err)
	}
	if err = f.manager.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	oldAfter, err := f.tasks.Get(f.ctx, old.TaskID)
	if err != nil || oldAfter.Status != "paused" {
		t.Fatal("old branch not paused", oldAfter, err)
	}
	freshAfter, err := f.tasks.Get(f.ctx, fresh.TaskID)
	if err != nil || freshAfter.Status != "active" {
		t.Fatal("current task incorrectly cancelled by old job", freshAfter, err)
	}
}
