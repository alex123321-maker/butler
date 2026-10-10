package initiative

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	postgres "github.com/butler/butler/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	store    *Store
	goals    *goal.Manager
	promoter *Promoter
	source   Source
	mandate  actionauth.Mandate
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_INITIATIVE_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_INITIATIVE_POSTGRES_URL is not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("isolated *_test database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.ConnectConfig(ctx, cfg.ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("initiative_%d", time.Now().UnixNano())
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
		if _, e := admin.Exec(cleanup, `DROP SCHEMA `+schema+` CASCADE`); e != nil {
			t.Error(e)
		}
		admin.Close(cleanup)
	})
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	f := &fixture{ctx: ctx, pool: db.Pool(), source: testSource()}
	f.store = NewStore(f.pool)
	tasks := task.NewStore(f.pool)
	f.goals = goal.New(f.pool, tasks, nil)
	now := time.Now().UTC()
	f.source.ActiveStart, f.source.ActiveEnd = now.Hour(), (now.Hour()+23)%24
	if _, err = f.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,$2,'web')`, f.source.SessionKey, f.source.OwnerID); err != nil {
		t.Fatal(err)
	}
	r, err := f.goals.PutGoal(f.actor("goal-create"), goal.GoalRequest{Goal: actionauth.GoalPolicy{Kind: "objective", Status: "active", Title: "Maintain fixture project", Purpose: f.source.Purpose, ExpiresAt: now.Add(48 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	f.source.GoalID = r.SubjectID
	p := budget.DefaultPolicy()
	p.Models["test:test"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	u := p.Mandate
	f.mandate = actionauth.Mandate{GoalID: r.SubjectID, GoalRevision: r.Revision, Status: "active", Purpose: f.source.Purpose, Tools: []string{"http.request"}, ResourceScopes: []actionauth.ResourceScope{{Origin: "https://example.invalid", PathPrefix: "/fixture"}}, Effects: []actionauth.Effect{actionauth.EffectRead, actionauth.EffectPrepare}, Privacy: "cloud_allowed", Budget: actionauth.MandateBudget{ModelTurns: u.ModelTurns, Tokens: u.Tokens, CostMicros: u.CostMicros, ExternalCalls: u.ExternalCalls, Children: u.Children, ActiveMillis: u.ActiveMillis, EgressBytes: u.EgressBytes, ProviderInputBytes: u.ProviderInputBytes}, ExpiresAt: now.Add(24 * time.Hour), AllowInitiative: true, Intervention: []string{"effect_unknown", "budget_exhausted"}, Reporting: "exceptions"}
	r, err = f.goals.PutMandate(f.actor("mandate-create"), goal.MandateRequest{Mandate: f.mandate})
	if err != nil {
		t.Fatal(err)
	}
	f.source.MandateID = r.SubjectID
	f.mandate, err = actionauth.NewPostgresStore(f.pool).GetMandate(ctx, r.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SaveSource(f.actor("source-create"), f.source, 0); err != nil {
		t.Fatal(err)
	}
	f.promoter = &Promoter{Store: f.store, Goals: f.goals, Tasks: tasks, Inputs: durableinput.NewStore(f.pool), Budgets: budget.NewController(f.pool, p), Allowance: budget.ModelAllowance{Provider: "test", Model: "test", InputTokens: 1024, OutputTokens: 8192}}
	return f
}

func (f *fixture) actor(delivery string) context.Context {
	return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.source.OwnerID, Channel: "web", DeliveryID: delivery})
}
func (f *fixture) errors(t *testing.T) Candidate {
	t.Helper()
	var c Candidate
	for i := 1; i <= 3; i++ {
		e := testEvent(time.Now().UTC())
		e.ID = fmt.Sprintf("failure:%d", i)
		e.EvidenceDigest = hash(i)
		var err error
		c, _, err = f.store.Observe(f.ctx, f.source.OwnerID, e, f.source.Revision, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
	}
	if c.Status != "ready" || c.Occurrences != 3 {
		t.Fatalf("candidate not ready: %+v", c)
	}
	return c
}

func TestRepeatedErrorsAdmitExactlyOnePreparedTaskIntegration(t *testing.T) {
	f := setup(t)
	c := f.errors(t)
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			promoted, err := f.promoter.Promote(f.ctx, c.ID, f.source.OwnerID)
			if err != nil {
				failures <- err
				return
			}
			if promoted.Status != "promoted" || promoted.TaskID == "" || promoted.InputID == "" {
				failures <- fmt.Errorf("missing atomic admission")
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var taskCount, inputCount, charges int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM tasks),(SELECT count(*) FROM accepted_inputs),(SELECT count(*) FROM execution_budget_reservations)`).Scan(&taskCount, &inputCount, &charges); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 || inputCount != 1 || charges != 0 {
		t.Fatalf("unexpected task/input/model charge: %d/%d/%d", taskCount, inputCount, charges)
	}
	c, err := f.store.Get(f.ctx, c.ID, f.source.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := f.promoter.Tasks.Get(f.ctx, c.TaskID)
	if err != nil || record.ExecutionPolicy != task.PolicyPrepareOnly {
		t.Fatal("initiative widened execution policy", err)
	}
	assignment, err := actionauth.GetTaskAssignment(f.ctx, f.pool, c.TaskID)
	if err != nil || assignment.MandateID != f.source.MandateID || assignment.ObservationRef != c.ObservationRef {
		t.Fatal("task lost its pinned authority", err)
	}
}

func TestDistinctEvidenceAndDeliveryDeduplicationIntegration(t *testing.T) {
	f := setup(t)
	e := testEvent(time.Now().UTC())
	c, created, err := f.store.Observe(f.ctx, f.source.OwnerID, e, 1, time.Now().UTC())
	if err != nil || !created || c.Occurrences != 1 {
		t.Fatal(err)
	}
	dup, created, err := f.store.Observe(f.ctx, f.source.OwnerID, e, 1, time.Now().UTC())
	if err != nil || created || dup.ID != c.ID {
		t.Fatal("delivery repeated", err)
	}
	e.ID = "another-delivery"
	dup, _, err = f.store.Observe(f.ctx, f.source.OwnerID, e, 1, time.Now().UTC())
	if err != nil || dup.Occurrences != 1 || dup.Status != "observing" {
		t.Fatal("one failure counted as independent failures", err)
	}
	e.Confidence++
	if _, _, err = f.store.Observe(f.ctx, f.source.OwnerID, e, 1, time.Now().UTC()); !errors.Is(err, ErrConflict) {
		t.Fatal("changed delivery payload accepted", err)
	}
}

func TestCurrentConditionAuthorityAndBudgetDenyPromotionIntegration(t *testing.T) {
	for _, kind := range []string{"healthy", "source_changed", "mandate_revoked", "budget_exhausted", "unknown_cost", "attention_exhausted"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			c := f.errors(t)
			switch kind {
			case "healthy":
				e := testEvent(time.Now().UTC())
				e.ID = "healthy"
				e.Kind = "project_healthy"
				if _, _, err := f.store.Observe(f.ctx, f.source.OwnerID, e, 1, time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
			case "source_changed":
				f.source.TaskGoal = "Revised task"
				if _, err := f.store.SaveSource(f.actor("change-source"), f.source, 1); err != nil {
					t.Fatal(err)
				}
			case "mandate_revoked":
				if _, err := f.pool.Exec(f.ctx, `UPDATE action_mandates SET revoked_at=clock_timestamp(),status='revoked' WHERE mandate_id=$1`, f.source.MandateID); err != nil {
					t.Fatal(err)
				}
			case "budget_exhausted":
				f.promoter.Budgets.Policy.Task.ModelTurns = 0
			case "unknown_cost":
				f.promoter.Budgets.Policy.Models = map[string]budget.ModelPrice{}
			case "attention_exhausted":
				if _, err := f.pool.Exec(f.ctx, `INSERT INTO initiative_attention_days(owner_id,day,admitted_tasks) VALUES($1,(clock_timestamp() AT TIME ZONE 'UTC')::date,20)`, f.source.OwnerID); err != nil {
					t.Fatal(err)
				}
			}
			got, err := f.promoter.Promote(f.ctx, c.ID, f.source.OwnerID)
			if err != nil || (got.Status != "rejected" && got.Status != "proposed" && got.Status != "deferred") {
				t.Fatalf("denied condition became runnable: %+v %v", got, err)
			}
			var count int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM tasks`).Scan(&count); err != nil || count != 0 {
				t.Fatal("denial created task", err)
			}
		})
	}
}

func TestTaskAndReceiptRollbackTogetherIntegration(t *testing.T) {
	f := setup(t)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	var owner string
	if err = tx.QueryRow(f.ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, f.source.SessionKey).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	record, err := f.promoter.Tasks.CreateTx(f.ctx, tx, task.CreateRequest{SessionKey: f.source.SessionKey, OwnerID: owner, Goal: "Rollback fixture", CompletionCriteria: []string{"No partial admission"}})
	if err != nil {
		t.Fatal(err)
	}
	event := ingress.InputEvent{EventID: "rollback", SessionKey: f.source.SessionKey, Source: "initiative", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_SCHEDULED_INTERNAL_EVENT, PayloadJSON: `{"text":"rollback"}`}
	if _, created, err := f.promoter.Inputs.AcceptTx(f.ctx, tx, event, owner, record.TaskID); err != nil || !created {
		t.Fatal(err)
	}
	if err = tx.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	var tasks, inputs int
	if err = f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM tasks),(SELECT count(*) FROM accepted_inputs)`).Scan(&tasks, &inputs); err != nil || tasks != 0 || inputs != 0 {
		t.Fatal("partial admission survived rollback", tasks, inputs, err)
	}
}

func TestEmptyScheduledChecksStayQuietIntegration(t *testing.T) {
	f := setup(t)
	now := time.Now().UTC()
	q := Schedule{ID: "heartbeat", SourceID: f.source.ID, TimeZone: "UTC", Kind: "interval", IntervalSeconds: 3600, MissedPolicy: "coalesce", Enabled: true}
	if _, err := f.store.SaveSchedule(f.actor("heartbeat"), q, 0, now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	w := &Worker{Store: f.store, Promoter: f.promoter}
	if err := w.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := w.Sweep(f.ctx); err != nil {
		t.Fatal(err)
	}
	v, err := f.store.View(f.ctx, f.source.OwnerID)
	if err != nil || len(v.Events) != 1 || v.Events[0].Verdict != "empty_check" || len(v.Candidates) != 0 || v.AttentionUsed != 0 {
		t.Fatal("empty heartbeat became work or attention", v, err)
	}
	var charges, outbox int
	if err = f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM execution_budget_reservations),(SELECT count(*) FROM delivery_outbox)`).Scan(&charges, &outbox); err != nil || charges != 0 || outbox != 0 {
		t.Fatal("empty heartbeat spent model or notified owner", charges, outbox, err)
	}
}

func TestChangedScheduleCannotRecordOldTickIntegration(t *testing.T) {
	f := setup(t)
	now := time.Now().UTC()
	q := Schedule{ID: "heartbeat", SourceID: f.source.ID, TimeZone: "UTC", Kind: "interval", IntervalSeconds: 3600, MissedPolicy: "coalesce", Enabled: true}
	if _, err := f.store.SaveSchedule(f.actor("heartbeat"), q, 0, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AdvanceDue(f.ctx, now, 10); err != nil {
		t.Fatal(err)
	}
	tick, err := f.store.ClaimTick(f.ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	q.Enabled = false
	if _, err = f.store.SaveSchedule(f.actor("heartbeat-disable"), q, 1, now); err != nil {
		t.Fatal(err)
	}
	e := testEvent(now)
	e.ID = "tick-fixture"
	if _, _, err = f.store.ObserveTick(f.ctx, f.source.OwnerID, e, tick, now); !errors.Is(err, ErrConflict) {
		t.Fatal("old schedule retained observation authority", err)
	}
}

func TestScheduleDowntimeLeaseRecoveryAndAcknowledgementIntegration(t *testing.T) {
	f := setup(t)
	now := time.Now().UTC()
	q := Schedule{ID: "hourly", SourceID: f.source.ID, TimeZone: "UTC", Kind: "interval", IntervalSeconds: 3600, MissedPolicy: "coalesce", Enabled: true}
	if _, err := f.store.SaveSchedule(f.actor("schedule-create"), q, 0, now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n, err := f.store.AdvanceDue(f.ctx, now, 10); err != nil || n != 1 {
		t.Fatal("downtime did not coalesce", n, err)
	}
	if n, err := f.store.AdvanceDue(f.ctx, now, 10); err != nil || n != 0 {
		t.Fatal("pending check duplicated", n, err)
	}
	first, err := f.store.ClaimTick(f.ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ClaimTick(f.ctx, time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatal("active lease stolen", err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE initiative_ticks SET claim_expires_at=clock_timestamp()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	second, err := f.store.ClaimTick(f.ctx, time.Minute)
	if err != nil || second.ClaimID == first.ClaimID || second.SlotKey != first.SlotKey {
		t.Fatal("lost acknowledgement produced new work", err)
	}
	if err = f.store.FinishTick(f.ctx, first, "checked", "empty_check"); !errors.Is(err, ErrConflict) {
		t.Fatal("late worker acknowledged new lease", err)
	}
	if err = f.store.FinishTick(f.ctx, second, "checked", "empty_check"); err != nil {
		t.Fatal(err)
	}
	var ticks, missed, candidates int
	if err = f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM initiative_ticks),(SELECT missed_count FROM initiative_ticks LIMIT 1),(SELECT count(*) FROM initiatives)`).Scan(&ticks, &missed, &candidates); err != nil {
		t.Fatal(err)
	}
	if ticks != 1 || missed != 24 || candidates != 0 {
		t.Fatalf("downtime avalanche or empty task: %d/%d/%d", ticks, missed, candidates)
	}
}
