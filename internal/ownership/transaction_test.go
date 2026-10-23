package ownership_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

func transactionFixture(t *testing.T) (context.Context, *pgxpool.Pool, *ownership.Authority) {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_COMMUNICATION_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated communication PostgreSQL required")
	}
	t.Setenv("BUTLER_TEST_MEMORY_POSTGRES_URL", dsn)
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:proof','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('proof-run','web:proof','input','running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO session_ownership(session_key,lease_id,run_id,owner_id,fence_epoch,acquired_at,expires_at) VALUES('web:proof','proof-lease','proof-run','worker',1,clock_timestamp(),clock_timestamp()+INTERVAL '1 minute')`); err != nil {
		t.Fatal(err)
	}
	authority := ownership.NewAuthority(pool)
	live := ownership.WithToken(ctx, authority, ownership.Token{SessionKey: "web:proof", RunID: "proof-run", LeaseID: "proof-lease", FenceEpoch: 1})
	return live, pool, authority
}

func TestCommunicationFencedTransactionLeavesObserverCapacityIntegration(t *testing.T) {
	live, pool, authority := transactionFixture(t)
	ctx := live
	_, tx, err := ownership.BeginTransaction(live, pool)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if pool.Stat().AcquiredConns() != 1 {
		t.Fatal("one owning write consumed its observer connection")
	}
	read, cancelRead := context.WithTimeout(ctx, 250*time.Millisecond)
	var observed bool
	err = pool.QueryRow(read, `SELECT lease_id='proof-lease' AND run_id='proof-run' AND expires_at>clock_timestamp() FROM session_ownership WHERE session_key='web:proof'`).Scan(&observed)
	cancelRead()
	if err != nil || !observed {
		t.Fatal("watchdog could not read the actual PG lease while the write was open", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE runs SET metadata_json='{"proof":"committed"}' WHERE run_id='proof-run'`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Commit and deferred Rollback must not leak or release the same slot twice.
	release, err := authority.BeginMutation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE session_ownership SET lease_id='',expires_at=clock_timestamp() WHERE session_key='web:proof'`)
	release()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = ownership.BeginTransaction(live, pool); !errors.Is(err, ownership.ErrStale) {
		t.Fatal("superseded lease admitted a write", err)
	}
}

func TestCommunicationFencedTransactionExpiryRollsBackIntegration(t *testing.T) {
	ctx, pool, _ := transactionFixture(t)
	if _, err := pool.Exec(ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()+INTERVAL '150 milliseconds' WHERE session_key='web:proof'`); err != nil {
		t.Fatal(err)
	}
	_, tx, err := ownership.BeginTransaction(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `UPDATE runs SET metadata_json='{"expired_write":true}' WHERE run_id='proof-run'`); err != nil {
		t.Fatal(err)
	}
	for {
		var expired bool
		if err = pool.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() FROM session_ownership WHERE session_key='web:proof'`).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err = tx.Commit(ctx); !errors.Is(err, ownership.ErrStale) {
		t.Fatal("expired authority committed", err)
	}
	var changed bool
	if err = pool.QueryRow(ctx, `SELECT metadata_json ? 'expired_write' FROM runs WHERE run_id='proof-run'`).Scan(&changed); err != nil || changed {
		t.Fatal("expired transaction left a business mutation", err, changed)
	}
}

func TestCommunicationFencedTransactionBlocksTakeoverUntilRollbackIntegration(t *testing.T) {
	ctx, pool, authority := transactionFixture(t)
	_, tx, err := ownership.BeginTransaction(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	wait, cancel := context.WithCancel(ctx)
	defer cancel()
	started, finished := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		release, err := authority.BeginMutation(wait)
		if err == nil {
			_, err = pool.Exec(wait, `UPDATE session_ownership SET lease_id='replacement',fence_epoch=2 WHERE session_key='web:proof'`)
			release()
		}
		finished <- err
	}()
	<-started
	select {
	case err := <-finished:
		t.Fatal("takeover escaped live write fence", err)
	case <-time.After(30 * time.Millisecond):
	}
	if pool.Stat().AcquiredConns() != 1 {
		t.Fatal("takeover consumed observer connection while waiting for fenced writer")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, _, err = ownership.BeginTransaction(ctx, pool); !errors.Is(err, ownership.ErrStale) {
		t.Fatal("replaced fence admitted a write", err)
	}
}
