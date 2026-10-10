package goal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func expiringPurpose(t *testing.T, f *fixture, after time.Duration) time.Time {
	t.Helper()
	var at time.Time
	if err := f.pool.QueryRow(f.ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	expiry := at.Add(after).UTC().Truncate(time.Microsecond)
	_, err := f.manager.PutGoal(f.actor("short-purpose-policy"), goal.GoalRequest{Goal: actionauth.GoalPolicy{Kind: "constraint", Status: "active", Title: "Synthetic maintenance permission window", Purpose: f.g.Purpose, ExpiresAt: expiry}})
	if err != nil {
		t.Fatal(err)
	}
	return expiry
}

func crossDBDeadline(t *testing.T, ctx context.Context, p *pgx.Conn, expiry time.Time) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var crossed bool
		if err := p.QueryRow(ctx, `SELECT clock_timestamp()>$1::timestamptz`, expiry).Scan(&crossed); err != nil {
			t.Fatal(err)
		}
		if crossed {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("DB deadline was not crossed", ctx.Err())
		case <-ticker.C:
		}
	}
}

func authorityBudgets(p *pgxpool.Pool) *budget.Controller {
	policy := budget.DefaultPolicy()
	policy.Models["fixture:synthetic"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	c := budget.NewController(p, policy)
	c.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	return c
}

func TestBudgetWaitCrossingPurposeDeadlineRollsBackEveryAdmissionIntegration(t *testing.T) {
	for _, kind := range []string{"dispatch", "model", "child"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			expiry := expiringPurpose(t, f, 5*time.Second)
			worker, name := processPool(t, f, "deadline_"+kind)
			var act allowedAction
			var admit func(context.Context) error
			if kind == "dispatch" {
				act = newAllowedAction(t, f, worker, false)
				if !act.grant.ExpiresAt.Equal(expiry) {
					t.Fatal("grant outlives purpose policy", act.grant.ExpiresAt, expiry)
				}
				admit = func(ctx context.Context) error { return act.store.ClaimGrant(ctx, act.grant.ID) }
			} else {
				r := f.create(t, "deadline-root")
				c := authorityBudgets(worker)
				if kind == "model" {
					id, owned := f.run(t, r, "model_running")
					admit = func(ctx context.Context) error {
						// Preserve the trusted ownership value while bounding this operation.
						owned, cancel := context.WithTimeout(owned, 8*time.Second)
						defer cancel()
						_, err := c.AdmitModel(owned, id, "", "fixture", "synthetic", 10)
						return err
					}
				} else {
					store := task.NewStore(worker)
					store.SetBudgets(c, "")
					admit = func(ctx context.Context) error {
						_, err := store.Create(ctx, task.CreateRequest{SessionKey: f.key, OwnerID: f.owner, ParentTaskID: r.TaskID, Goal: "Synthetic child", CompletionCriteria: []string{"reviewed"}})
						return err
					}
				}
			}
			o := observer(t, f)
			release := holdWrite(t, f, o, "execution_budget_reservations", "INSERT", "TRUE")
			ctx, cancel := context.WithTimeout(f.ctx, 8*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- admit(ctx) }()
			awaitLock(t, ctx, o, name, done)
			crossDBDeadline(t, ctx, o, expiry)
			release()
			if err := await(t, ctx, done); !errors.Is(err, actionauth.ErrOwnerAuthority) {
				t.Fatal("expired authority committed after budget wait", kind, err)
			}
			for _, table := range []string{"execution_budget_reservations", "execution_model_attempts", "action_effect_dispatches"} {
				var n int
				if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
					t.Fatal("expired admission retained authority write", kind, table, n, err)
				}
			}
			var children int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE parent_task_id IS NOT NULL`).Scan(&children); err != nil || children != 0 {
				t.Fatal("expired admission created child", children, err)
			}
			if kind == "dispatch" {
				g, err := act.store.GetGrant(ctx, act.grant.ID)
				if err != nil || g.State != "authorized" || act.posts.Load() != 0 {
					t.Fatal("expired permission spent/reached recipient", g, act.posts.Load(), err)
				}
				e, err := act.store.GetEffect(ctx, act.effect.ActionID)
				if err != nil || e.State != actionauth.StateAllowed {
					t.Fatal("expired permission changed journal", e, err)
				}
			}
		})
	}
}

func TestModelDeadlineClampsPurposeWhileBudgetPeriodStaysImmutableIntegration(t *testing.T) {
	f := setup(t)
	expiry := expiringPurpose(t, f, 30*time.Second)
	r := f.create(t, "model-deadline")
	id, owned := f.run(t, r, "model_running")
	c := authorityBudgets(f.pool)
	a, err := c.AdmitModel(owned, id, "", "fixture", "synthetic", 10)
	if err != nil || !a.Deadline.Equal(expiry) {
		t.Fatal("model deadline widened purpose window", a, expiry, err)
	}
	scope, err := c.Ledger.Get(f.ctx, budget.ScopeID("mandate", f.owner, f.m.ID))
	if err != nil || !scope.Deadline.Equal(f.m.BudgetDeadline) {
		t.Fatal("transient release bound replaced immutable quota", scope, err)
	}
}

func TestOwnerShortensAuthorityWithoutReplenishingOriginalBudgetIntegration(t *testing.T) {
	f := setup(t)
	old := f.create(t, "old-deadline-root")
	shorter := f.m
	shorter.ExpiresAt = time.Now().Add(30 * time.Second).UTC().Truncate(time.Microsecond)
	r, err := f.manager.PutMandate(f.actor("shorten-owner-deadline"), goal.MandateRequest{Mandate: shorter, ExpectedRevision: f.m.Revision})
	if err != nil {
		t.Fatal("owner could not reduce authority deadline", err)
	}
	updated, err := actionauth.NewPostgresStore(f.pool).GetMandate(f.ctx, r.SubjectID)
	if err != nil || !updated.ExpiresAt.Equal(shorter.ExpiresAt) || !updated.BudgetDeadline.Equal(f.m.BudgetDeadline) || updated.Budget != f.m.Budget {
		t.Fatal("shorter authority reset allowance", updated, err)
	}
	if _, err = f.tasks.Create(f.ctx, task.CreateRequest{SessionKey: f.key, OwnerID: f.owner, ParentTaskID: old.TaskID, Goal: "Stale child", CompletionCriteria: []string{"reviewed"}}); !errors.Is(err, actionauth.ErrOwnerAuthority) {
		t.Fatal("old branch adopted amended authority", err)
	}
	f.m = updated
	fresh := f.create(t, "shorter-current-root")
	id, owned := f.run(t, fresh, "model_running")
	c := authorityBudgets(f.pool)
	a, err := c.AdmitModel(owned, id, "", "fixture", "synthetic", 10)
	if err != nil || !a.Deadline.Equal(shorter.ExpiresAt) {
		t.Fatal("fresh assignment widened shortened deadline", a, err)
	}
	scope, err := c.Ledger.Get(f.ctx, budget.ScopeID("mandate", f.owner, f.m.ID))
	if err != nil || !scope.Deadline.Equal(updated.BudgetDeadline) {
		t.Fatal("shortened authority changed quota period", scope, err)
	}
}
