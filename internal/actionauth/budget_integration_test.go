package actionauth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestClaimAndBudgetAdmissionAreOneTransactionIntegration(t *testing.T) {
	s, store, reader, call, contract, receiver, server := receiptFixture(t)
	pool := store.pool
	ctx := context.Background()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "037_execution_budgets.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);INSERT INTO sessions VALUES('web:owner','owner');ALTER TABLE runs ADD COLUMN session_key TEXT DEFAULT 'web:owner';ALTER TABLE tasks ADD COLUMN parent_task_id TEXT,ADD COLUMN session_key TEXT DEFAULT 'web:owner',ADD COLUMN created_at TIMESTAMPTZ DEFAULT clock_timestamp()`)
	if err != nil {
		t.Fatal(err)
	}
	p := budget.DefaultPolicy()
	p.Task.ExternalCalls = 0
	store.SetBudgets(budget.NewController(pool, p))
	g := logicalGrant(t, s, reader, call, contract, "budget-first")
	err = store.ClaimGrant(ctx, g.ID)
	var exhausted *budget.ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Dimension != "external_calls" {
		t.Fatal("claim bypassed allowance", err)
	}
	stored, err := store.GetGrant(ctx, g.ID)
	if err != nil || stored.State != "authorized" || receiver.effects.Load() != 0 {
		t.Fatal("denial spent grant or invoked recipient", stored, err)
	}
	effect, err := s.EffectForGrant(ctx, g)
	if err != nil || effect.State != StateAllowed {
		t.Fatal("denial changed effect", effect, err)
	}
	var reservations int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM execution_budget_reservations`).Scan(&reservations); err != nil || reservations != 0 {
		t.Fatal("denial retained partial budget", reservations, err)
	}
	// Failed outer claim created no scope, so one explicit deployment policy is
	// established by the first successful reservation; later attempts can't reset it.
	p.Task.ExternalCalls = 1
	store.SetBudgets(budget.NewController(pool, p))
	if err = store.ClaimGrant(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	result := postReceipt(t, server, call, effect)
	if _, err = s.Outcome(ctx, g, result, false); err != nil {
		t.Fatal(err)
	}
	call.ToolCallId = "second-budget-call"
	second := logicalGrant(t, s, reader, call, contract, "budget-second")
	err = store.ClaimGrant(ctx, second.ID)
	if !errors.As(err, &exhausted) {
		t.Fatal("second external dispatch escaped shared scope", err)
	}
	if receiver.effects.Load() != 1 {
		t.Fatal("external side effect duplicated")
	}
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM execution_budget_reservations`).Scan(&reservations); err != nil || reservations != 1 {
		t.Fatal("reservation and dispatch not atomic", reservations, err)
	}
}

func TestBudgetUsesCurrentAssignedMandateAcrossIndependentTasksIntegration(t *testing.T) {
	base := actionDatabase(t)
	ctx := context.Background()
	cfg := base.Config()
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "037_execution_budgets.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);INSERT INTO sessions VALUES('web:owner','owner');ALTER TABLE runs ADD COLUMN session_key TEXT DEFAULT 'web:owner',ADD COLUMN current_state TEXT DEFAULT 'model_running';ALTER TABLE tasks ADD COLUMN parent_task_id TEXT,ADD COLUMN session_key TEXT DEFAULT 'web:owner',ADD COLUMN created_at TIMESTAMPTZ DEFAULT clock_timestamp();INSERT INTO tasks(task_id,owner_id,status,revision,constraints_revision) VALUES('second-task','owner','active',7,2);INSERT INTO runs(run_id,task_id,task_revision) VALUES('second-run','second-task',7)`)
	if err != nil {
		t.Fatal(err)
	}
	m := testOwnerMandate(time.Now().UTC().Truncate(time.Microsecond))
	m.OwnerID = "owner"
	m.Budget.ModelTurns = 1
	m.Budget.ProviderInputBytes = 2 * 8388608
	goal := GoalPolicy{ID: m.GoalID, OwnerID: m.OwnerID, Kind: "objective", Status: "active", Revision: 1, Purpose: m.Purpose, ExpiresAt: m.ExpiresAt}
	encoded, err := json.Marshal(goal)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES($1,$2,'objective','active',1,$3,$4,'{}')`, goal.ID, goal.OwnerID, goal.ExpiresAt, encoded)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture seeds the authenticated goal service's durable projection.
	if _, err = pool.Exec(ctx, `INSERT INTO owner_purpose_policies(owner_id,purpose,revision) VALUES($1,$2,0)`, goal.OwnerID, goal.Purpose); err != nil {
		t.Fatal(err)
	}
	// Broker's legacy PutMandate deliberately cannot grant broad owner authority.
	if err = ValidateMandate(m, time.Now()); err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status) VALUES($1,$2,'',1,$3,$4,'owner_goal',$5,1,'active')`, m.ID, m.OwnerID, encoded, m.ExpiresAt, m.GoalID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"task", "second-task"} {
		_, err = pool.Exec(ctx, `INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref) VALUES($1,$2,$3,1,$4,1,$5,'owner-request','synthetic-observation')`, id, m.OwnerID, m.GoalID, m.ID, m.Purpose)
		if err != nil {
			t.Fatal(err)
		}
	}
	p := budget.DefaultPolicy()
	p.Models["fixture:fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	c := budget.NewController(pool, p)
	c.SetAuthorityResolver(ResolveBudgetAuthority)
	modelCtx := func(run string) context.Context {
		return ownership.WithToken(ctx, nil, ownership.Token{SessionKey: "web:owner", RunID: run, LeaseID: "lease", FenceEpoch: 4})
	}
	if _, err = c.AdmitModel(modelCtx("run"), "run", "", "fixture", "fixture", 20); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE session_ownership SET run_id='second-run'`); err != nil {
		t.Fatal(err)
	}
	_, err = c.AdmitModel(modelCtx("second-run"), "second-run", "", "fixture", "fixture", 20)
	var exhausted *budget.ExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Kind != "mandate" {
		t.Fatal("new independent task reset assigned mandate", err)
	}
	m.Privacy = "local_only"
	m.Revision = 2
	encoded, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE action_mandates SET revision=2,mandate_data=$2 WHERE mandate_id=$1`, m.ID, encoded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE task_authority_bindings SET mandate_revision=2`); err != nil {
		t.Fatal(err)
	}
	_, err = c.AdmitModel(modelCtx("second-run"), "second-run", "", "fixture", "fixture", 20)
	var stopped *budget.LoopError
	if !errors.As(err, &stopped) || stopped.Reason != "mandate_model_privacy" {
		t.Fatal("local mandate permitted cloud inference", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE owner_goals SET status='paused'`); err != nil {
		t.Fatal(err)
	}
	if _, err = c.AdmitModel(modelCtx("second-run"), "second-run", "", "fixture", "fixture", 20); !errors.Is(err, ErrOwnerAuthority) {
		t.Fatal("paused authority admitted inference before sweep", err)
	}
	var attempts int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM execution_model_attempts`).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("rejected request spent model attempt", attempts, err)
	}
}
