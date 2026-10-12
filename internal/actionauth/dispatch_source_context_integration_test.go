package actionauth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// Plain authenticated Broker RPC context has no core ownership token. Only the
// actual already locked PG lease may become the same-TX source verifier's proof.
func TestClaimRunDataGuardDerivesOnlyCurrentDatabaseOwnershipIntegration(t *testing.T) {
	for _, fault := range []string{"plain_rpc", "forged_caller", "expired_lease", "superseded_lease", "stale_task", "current_source_denied"} {
		t.Run(fault, func(t *testing.T) {
			pool := actionDatabase(t)
			store := NewPostgresStore(pool)
			service, _, reader, _, call, contract := fixture(t)
			service.store = store
			grant := prepareApproved(t, service, reader, call, contract)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			caller := ctx
			if fault == "forged_caller" {
				caller = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "forged-owner", Channel: "web", DeliveryID: "forged-decision"})
				caller = ownership.WithToken(caller, nil, ownership.Token{SessionKey: "forged-session", RunID: "forged-run", LeaseID: "forged-lease", FenceEpoch: 999})
			}
			called := 0
			denied := errors.New("fixed controlled current source denial")
			store.SetRunDataGuard(func(proofCtx context.Context, tx pgx.Tx, runID string) error {
				called++
				token, ok := ownership.TokenFromContext(proofCtx)
				if !ok || token != (ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "lease", FenceEpoch: 4}) || runID != token.RunID {
					return errors.New("same-TX source guard lacks exact current database ownership")
				}
				if _, ok := owneridentity.FromContext(proofCtx); ok {
					return errors.New("caller owner actor reached private source proof")
				}
				// This is read proof, not a reusable write authority. A guard can
				// read using its existing tx but cannot borrow/create a new fence.
				if _, release, err := ownership.Begin(proofCtx); err == nil {
					release()
					return errors.New("private read proof minted general write authority")
				}
				var exact bool
				if err := tx.QueryRow(proofCtx, `SELECT EXISTS(SELECT 1 FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp())`, token.SessionKey, token.RunID, token.LeaseID, token.FenceEpoch).Scan(&exact); err != nil || !exact {
					return errors.New("private source proof is not backed by caller transaction")
				}
				if fault == "current_source_denied" {
					return denied
				}
				return nil
			})
			switch fault {
			case "expired_lease":
				if _, err := pool.Exec(ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()+interval '50 milliseconds'`); err != nil {
					t.Fatal(err)
				}
				time.Sleep(80 * time.Millisecond)
			case "superseded_lease":
				if _, err := pool.Exec(ctx, `UPDATE session_ownership SET lease_id='new-owner-lease',fence_epoch=fence_epoch+1`); err != nil {
					t.Fatal(err)
				}
			case "stale_task":
				if _, err := pool.Exec(ctx, `UPDATE tasks SET revision=revision+1,constraints_revision=constraints_revision+1`); err != nil {
					t.Fatal(err)
				}
			}
			err := store.ClaimGrant(caller, grant.ID)
			valid := fault == "plain_rpc" || fault == "forged_caller"
			if valid && (err != nil || called != 1) {
				t.Fatal("current plain-RPC grant did not reach exact same-TX source check", err, called)
			}
			if !valid && err == nil {
				t.Fatal("changed owning/source proof dispatched")
			}
			if fault == "current_source_denied" && (!errors.Is(err, denied) || called != 1) {
				t.Fatal("current source guard was bypassed", err, called)
			}
			if !valid && fault != "current_source_denied" && called != 0 {
				t.Fatal("stale ownership/task reached private source proof")
			}
			var state, effect string
			var dispatches int
			if pool.QueryRow(ctx, `SELECT state FROM action_authorizations WHERE authorization_id=$1`, grant.ID).Scan(&state) != nil || pool.QueryRow(ctx, `SELECT e.state FROM action_effects e JOIN action_intents i ON i.effect_id=e.action_id WHERE i.action_id=$1`, grant.Binding.ActionID).Scan(&effect) != nil || pool.QueryRow(ctx, `SELECT count(*) FROM action_effect_dispatches`).Scan(&dispatches) != nil {
				t.Fatal("actual dispatch/effect ledger unavailable")
			}
			if valid && (state != "inflight" || effect != "executing" || dispatches != 1) {
				t.Fatal("valid source proof did not commit one dispatch")
			}
			if !valid && (state != "authorized" || effect != "allowed" || dispatches != 0) {
				t.Fatal("failed source proof partially spent grant/effect")
			}
			// The private derivative never escapes to caller context. Forged
			// metadata remains untrusted outside this one verifier invocation.
			if token, ok := ownership.TokenFromContext(caller); (fault == "forged_caller" && (!ok || token.RunID != "forged-run")) || (fault != "forged_caller" && ok) {
				t.Fatal("claim leaked private read proof into caller")
			}
			if valid {
				if err := store.ClaimGrant(caller, grant.ID); err == nil {
					t.Fatal("same-TX proof permitted an already claimed grant replay")
				}
				if pool.QueryRow(ctx, `SELECT count(*) FROM action_effect_dispatches`).Scan(&dispatches) != nil || dispatches != 1 {
					t.Fatal("claim replay duplicated permanent dispatch")
				}
			}
		})
	}
}

func TestClaimGrantExpiryDuringPrivateSourceReadProofRollsBackIntegration(t *testing.T) {
	pool := actionDatabase(t)
	store := NewPostgresStore(pool)
	service, _, reader, _, call, contract := fixture(t)
	service.store, service.now = store, time.Now
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", "037_execution_budgets.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(data)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);INSERT INTO sessions VALUES('web:owner','owner');ALTER TABLE runs ADD COLUMN session_key TEXT DEFAULT 'web:owner';ALTER TABLE tasks ADD COLUMN parent_task_id TEXT,ADD COLUMN session_key TEXT DEFAULT 'web:owner',ADD COLUMN created_at TIMESTAMPTZ DEFAULT clock_timestamp()`); err != nil {
		t.Fatal(err)
	}
	store.SetBudgets(budget.NewController(pool, budget.DefaultPolicy()))
	prepared, err := service.Prepare(ctx, call, contract)
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().UTC().Truncate(time.Microsecond).Add(350 * time.Millisecond)
	reader.approved = ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: prepared.Binding, ExpiresAt: until, ResolvedAt: time.Now()}
	grant, err := service.Authorize(ctx, call, contract, prepared.Binding.ActionID, "approval", "")
	if err != nil || !grant.ExpiresAt.Equal(until) {
		t.Fatal("actual finite exact approval grant was not retained", err)
	}
	called := false
	store.SetRunDataGuard(func(proofCtx context.Context, tx pgx.Tx, id string) error {
		called = true
		token, ok := ownership.TokenFromContext(proofCtx)
		if !ok || token != (ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "lease", FenceEpoch: 4}) || id != "run" {
			return errors.New("expired-grant check did not receive current PG read proof")
		}
		var live bool
		if tx.QueryRow(proofCtx, `SELECT expires_at>clock_timestamp() FROM action_authorizations WHERE authorization_id=$1`, grant.ID).Scan(&live) != nil || !live {
			return errors.New("grant was not live before owning verifier wait")
		}
		// The actual DB wall clock crosses the immutable grant while its same
		// transaction/read proof stays current. No forged clocks or expiry UPDATE.
		_, err := tx.Exec(proofCtx, `SELECT pg_sleep(0.45)`)
		return err
	})
	if err = store.ClaimGrant(ctx, grant.ID); err == nil || !called {
		t.Fatal("private read proof widened expired dispatch permission")
	}
	var state, effect string
	var dispatches, reservations, scopes int
	if pool.QueryRow(ctx, `SELECT state FROM action_authorizations WHERE authorization_id=$1`, grant.ID).Scan(&state) != nil || pool.QueryRow(ctx, `SELECT e.state FROM action_effects e JOIN action_intents i ON i.effect_id=e.action_id WHERE i.action_id=$1`, grant.Binding.ActionID).Scan(&effect) != nil || pool.QueryRow(ctx, `SELECT count(*) FROM action_effect_dispatches`).Scan(&dispatches) != nil || pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations) != nil || pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_scopes`).Scan(&scopes) != nil {
		t.Fatal("expiry rollback ledger unavailable")
	}
	if state != "authorized" || effect != "allowed" || dispatches != 0 || reservations != 0 || scopes != 0 {
		t.Fatal("post-verifier grant expiry partially spent permission/budget")
	}
}
