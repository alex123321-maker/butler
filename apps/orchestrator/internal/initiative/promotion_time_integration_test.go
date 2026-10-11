package initiative

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This clock fault lives only in the disposable test schema. A real shared
// budget row blocks promotion while the observer advances the schema-local DB
// clock. Production retains PostgreSQL's clock and has no injected clock seam.
func promotionClock(t *testing.T, f *fixture, at time.Time) *pgx.Conn {
	t.Helper()
	cfg := f.pool.Config()
	cfg.ConnConfig.RuntimeParams["search_path"] = strings.Replace(cfg.ConnConfig.RuntimeParams["search_path"], ",public", ",pg_catalog,public", 1)
	cfg.ConnConfig.RuntimeParams["application_name"] = fmt.Sprintf("initiative_time_%d", time.Now().UnixNano())
	pool, err := pgxpool.NewWithConfig(f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	observer, err := pgx.ConnectConfig(f.ctx, cfg.ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = observer.Close(ctx)
	})
	if _, err = observer.Exec(f.ctx, `CREATE TABLE initiative_test_clock(moment timestamptz NOT NULL);
	 CREATE FUNCTION clock_timestamp() RETURNS timestamptz LANGUAGE SQL VOLATILE AS $$ SELECT moment FROM initiative_test_clock $$`); err != nil {
		t.Fatal(err)
	}
	if _, err = observer.Exec(f.ctx, `INSERT INTO initiative_test_clock VALUES($1)`, at); err != nil {
		t.Fatal(err)
	}
	f.pool, f.store = pool, NewStore(pool)
	f.promoter.Store = f.store
	f.promoter.Tasks = task.NewStore(pool)
	f.goals = goal.New(pool, f.promoter.Tasks, nil)
	f.promoter.Goals = f.goals
	f.promoter.Inputs = durableinput.NewStore(pool)
	f.promoter.Budgets = budget.NewController(pool, f.promoter.Budgets.Policy)
	return observer
}

func TestPromotionRechecksTimeAfterSharedBudgetWaitIntegration(t *testing.T) {
	for _, kind := range []string{"active_hours_closed", "observation_expired", "utc_day_changed"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			day := time.Now().UTC().Truncate(24 * time.Hour)
			before := day.Add(10*time.Hour + 59*time.Minute + 59*time.Second)
			f.source.ActiveStart, f.source.ActiveEnd = 10, 11
			wantStatus, wantReason := "deferred", "outside_active_hours"
			if kind == "observation_expired" {
				before = day.Add(12 * time.Hour)
				f.source.ActiveStart, f.source.ActiveEnd = 8, 20
				wantStatus, wantReason = "rejected", "condition_obsolete"
			} else if kind == "utc_day_changed" {
				before = day.Add(24*time.Hour - time.Second)
				f.source.ActiveStart, f.source.ActiveEnd = 22, 2
				wantReason = "admission_day_changed"
			}
			after := before.Add(2 * time.Second)
			observer := promotionClock(t, f, before)
			receipt, err := f.store.SaveSource(f.actor("bounded-hours"), f.source, f.source.Revision)
			if err != nil {
				t.Fatal(err)
			}
			f.source.Revision = receipt.Revision
			observed := before
			if kind == "observation_expired" {
				observed = before.Add(-24*time.Hour + time.Second)
			}
			var candidate Candidate
			for i := 1; i <= 3; i++ {
				event := testEvent(observed)
				event.ID, event.EvidenceDigest = fmt.Sprintf("failure:%d", i), hash(i)
				candidate, _, err = f.store.Observe(f.ctx, f.source.OwnerID, event, f.source.Revision, before)
				if err != nil {
					t.Fatal(err)
				}
			}
			if candidate.Status != "ready" {
				t.Fatal("candidate not ready before contention", candidate)
			}
			b := f.mandate.Budget
			scope, err := budget.NewLedger(f.pool).Ensure(f.ctx, budget.ScopeSpec{Kind: "mandate", OwnerID: f.source.OwnerID, SubjectID: f.mandate.ID, Deadline: f.mandate.BudgetDeadline,
				Limits: budget.Units{ModelTurns: b.ModelTurns, Tokens: b.Tokens, CostMicros: b.CostMicros, ExternalCalls: b.ExternalCalls, Children: b.Children, ActiveMillis: b.ActiveMillis, EgressBytes: b.EgressBytes, ProviderInputBytes: b.ProviderInputBytes}})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "utc_day_changed" {
				if _, err = observer.Exec(f.ctx, `INSERT INTO initiative_attention_days(owner_id,day,admitted_tasks) VALUES($1,$2,20)`, f.source.OwnerID, after.UTC().Truncate(24*time.Hour)); err != nil {
					t.Fatal(err)
				}
			}
			blocker, err := observer.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(f.ctx)
			if _, err = blocker.Exec(f.ctx, `SELECT scope_id FROM execution_budget_scopes WHERE scope_id=$1 FOR UPDATE`, scope.ID); err != nil {
				t.Fatal(err)
			}
			type result struct {
				candidate Candidate
				err       error
			}
			finished := make(chan result, 1)
			go func() {
				got, e := f.promoter.Promote(f.ctx, candidate.ID, f.source.OwnerID)
				finished <- result{got, e}
			}()
			wait, cancel := context.WithTimeout(f.ctx, 3*time.Second)
			defer cancel()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				var waiting bool
				if err = blocker.QueryRow(wait, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND pg_backend_pid()=ANY(pg_blocking_pids(pid)))`, f.pool.Config().ConnConfig.RuntimeParams["application_name"]).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case got := <-finished:
					t.Fatal("promotion bypassed the held budget boundary", got)
				case <-wait.Done():
					t.Fatal("promotion did not reach the held budget boundary")
				case <-ticker.C:
				}
			}
			if _, err = blocker.Exec(f.ctx, `UPDATE initiative_test_clock SET moment=$1`, after); err != nil {
				t.Fatal(err)
			}
			if err = blocker.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			var got result
			select {
			case got = <-finished:
			case <-f.ctx.Done():
				t.Fatal("promotion did not finish after budget release")
			}
			if got.err != nil || got.candidate.Status != wantStatus || got.candidate.Reason != wantReason || got.candidate.TaskID != "" || got.candidate.InputID != "" {
				t.Fatalf("stale admission after shared budget wait: %+v %v", got.candidate, got.err)
			}
			if kind == "active_hours_closed" && !got.candidate.NotBefore.Equal(day.Add(34*time.Hour)) {
				t.Fatal("did not defer until the next configured active hour", got.candidate.NotBefore)
			}
			if kind == "utc_day_changed" {
				// Reconstruct the service before retry. The next admission must read
				// the new day's exhausted counter, never increment the old day.
				restarted := *f.promoter
				restarted.Store = NewStore(f.pool)
				retry, e := restarted.Promote(f.ctx, candidate.ID, f.source.OwnerID)
				if e != nil || retry.Status != "deferred" || retry.Reason != "attention_budget_exhausted" {
					t.Fatal("restart reused the previous day's attention", retry, e)
				}
			}
			var tasks, inputs, charges, attention int
			if err = f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM tasks),(SELECT count(*) FROM accepted_inputs),(SELECT count(*) FROM execution_budget_reservations),COALESCE((SELECT admitted_tasks FROM initiative_attention_days WHERE owner_id=$1 AND day=$2),0)`, f.source.OwnerID, day).Scan(&tasks, &inputs, &charges, &attention); err != nil || tasks != 0 || inputs != 0 || charges != 0 || attention != 0 {
				t.Fatal("denial committed task, input, model spend or old-day attention", tasks, inputs, charges, attention, err)
			}
		})
	}
}
