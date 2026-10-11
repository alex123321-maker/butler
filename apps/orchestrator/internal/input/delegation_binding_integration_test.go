package input

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

type delegatedBindingFixture struct {
	ctx       context.Context
	pool      *pgxpool.Pool
	store     *Store
	claim     Record
	event     ingress.InputEvent
	token     ownership.Token
	authority *ownership.Authority
	source    delegation.SourceRef
}

// This fixture exercises only the queue's identity verifier. Its source reference
// is synthetic; the app test separately uses real Catalog/Manager/Broker checks.
func newDelegatedBindingFixture(t *testing.T, claimTTL, inputTTL time.Duration) *delegatedBindingFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	pool := memorytest.Database(t)
	f := &delegatedBindingFixture{pool: pool, store: NewStore(pool), source: delegation.SourceRef{ID: "fixture-contract", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)}}
	repo := session.NewPostgresRepository(pool)
	if _, _, err := repo.CreateSession(ctx, session.CreateSessionParams{SessionKey: "binding-session", UserID: "binding-owner", Channel: "internal", MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	tasks := task.NewStore(pool)
	c, err := tasks.Create(ctx, task.CreateRequest{SessionKey: "binding-session", OwnerID: "binding-owner", Goal: "Identity verification fixture", CompletionCriteria: []string{"No model response"}})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT session_key FROM sessions WHERE session_key=$1 FOR UPDATE`, c.SessionKey); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.AcceptBoundedTx(ctx, tx, delegation.InputRequest{OwnerID: c.OwnerID, SessionKey: c.SessionKey, TaskID: c.TaskID, EventID: "delegation:" + c.TaskID, ContractSource: f.source, Deadline: time.Now().Add(inputTTL).UTC(), MaxAttempts: 2}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	f.claim, err = f.store.Claim(ctx, claimTTL)
	if err != nil {
		t.Fatal(err)
	}
	f.event = f.claim.Event
	f.event.IdempotencyKey = f.claim.InputID
	runs := run.NewService(run.NewPostgresRepository(pool), slog.New(slog.NewTextHandler(io.Discard, nil)))
	r, err := runs.CreateRun(ctx, &sessionv1.CreateRunRequest{SessionKey: c.SessionKey, InputEvent: f.event.ToProto(), ModelProvider: "zero-inference", AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2})
	if err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManager(pool)
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: c.SessionKey, RunID: r.RunId, OwnerID: "binding-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	f.token = ownership.Token{SessionKey: c.SessionKey, RunID: r.RunId, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch}
	f.authority = leases.Authority()
	f.ctx = ownership.WithToken(WithClaim(ctx, f.claim), f.authority, f.token)
	if _, err = runs.RefreshExecutionLease(f.ctx, r.RunId, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	if err = tasks.AttachRun(f.ctx, c.TaskID, c.Revision, r.RunId, c.SessionKey); err != nil {
		t.Fatal(err)
	}
	if err = f.store.SaveCheckpoint(f.ctx, r.RunId, "acquired", `{}`); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestDelegatedInputVerifierCurrentClaimAndFenceIntegration(t *testing.T) {
	f := newDelegatedBindingFixture(t, time.Minute, time.Minute)
	if err := f.store.VerifyDelegatedInput(f.ctx, f.event, f.claim.TaskID, f.source); err != nil {
		t.Fatal("current queue identity denied", err)
	}
	for _, name := range []string{"owner", "claim", "fence", "session", "run", "source", "no-claim", "no-token"} {
		t.Run(name, func(t *testing.T) {
			ctx, pin := f.ctx, f.source
			claim, token := f.claim, f.token
			switch name {
			case "owner":
				claim.OwnerID = "other-owner"
			case "claim":
				claim.ClaimID = "old-claim"
			case "fence":
				token.FenceEpoch++
			case "session":
				token.SessionKey = "other-session"
			case "run":
				token.RunID = "other-run"
			case "source":
				pin.AccessRevision++
			}
			ctx = ownership.WithToken(WithClaim(ctx, claim), f.authority, token)
			if name == "no-claim" {
				ctx = ownership.WithToken(context.Background(), f.authority, token)
			}
			if name == "no-token" {
				ctx = WithClaim(context.Background(), claim)
			}
			if err := f.store.VerifyDelegatedInput(ctx, f.event, f.claim.TaskID, pin); !errors.Is(err, delegation.ErrDenied) {
				t.Fatal("invalid private identity accepted", name, err)
			}
		})
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE runs SET idempotency_key='wrong-run-key' WHERE run_id=$1`, f.token.RunID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.VerifyDelegatedInput(f.ctx, f.event, f.claim.TaskID, f.source); !errors.Is(err, delegation.ErrDenied) {
		t.Fatal("different persisted run dedup key accepted", err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE runs SET idempotency_key=$2 WHERE run_id=$1`, f.token.RunID, f.claim.InputID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id=$1`, f.claim.TaskID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.VerifyDelegatedInput(f.ctx, f.event, f.claim.TaskID, f.source); !errors.Is(err, delegation.ErrDenied) {
		t.Fatal("stale task pin accepted", err)
	}
}

func TestDelegatedInputVerifierExpiryDuringLockWaitIntegration(t *testing.T) {
	for _, name := range []string{"claim", "input"} {
		t.Run(name, func(t *testing.T) {
			claimTTL, inputTTL := time.Minute, time.Minute
			if name == "claim" {
				claimTTL = time.Second
			} else {
				inputTTL = time.Second
			}
			f := newDelegatedBindingFixture(t, claimTTL, inputTTL)
			if err := f.store.VerifyDelegatedInput(f.ctx, f.event, f.claim.TaskID, f.source); err != nil {
				t.Fatal("current claim denied before wait", err)
			}
			locker, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer locker.Rollback(context.Background())
			if _, err = locker.Exec(f.ctx, `SELECT task_id FROM tasks WHERE task_id=$1 FOR UPDATE`, f.claim.TaskID); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- f.store.VerifyDelegatedInput(f.ctx, f.event, f.claim.TaskID, f.source) }()
			// Observe a real PostgreSQL lock wait using the locker's existing connection;
			// neither path borrows a third connection from this two-connection pool.
			deadline := time.Now().Add(time.Second)
			for {
				var blocked bool
				if err = locker.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pg_backend_pid()=ANY(pg_blocking_pids(pid)))`).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("verifier did not reach real task lock wait")
				}
				time.Sleep(5 * time.Millisecond)
			}
			expires := *f.claim.ClaimExpiresAt
			if name == "input" {
				expires = f.claim.ExpiresAt
			}
			timer := time.NewTimer(time.Until(expires) + 25*time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-f.ctx.Done():
				t.Fatal("fixture deadline before claim expiry")
			}
			if err = locker.Rollback(f.ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-done:
				if !errors.Is(err, delegation.ErrDenied) {
					t.Fatal("expired claim survived wait", err)
				}
			case <-f.ctx.Done():
				t.Fatal("verifier lock wait did not terminate")
			}
		})
	}
}
