package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The owner adapter in this fixture requires independently recorded evidence
// for the exact resource/lease/version. Production adapters must provide their
// own owning effect/artifact check; there is no default permissive adapter.
type evidenceGuard struct{}

func (evidenceGuard) CheckReconciliationTx(ctx context.Context, tx pgx.Tx, d Decision) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.OwnerID != d.OwnerID {
		return ErrDenied
	}
	var valid bool
	err := tx.QueryRow(ctx, `SELECT owner_id=$2 AND resource_id=$3 AND lease_id=$4 AND version=$5 FROM independent_resource_evidence WHERE evidence_id=$1 FOR SHARE`, d.EvidenceID, d.OwnerID, d.ResourceID, d.LeaseID, d.ExpectedVersion).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !valid {
		return ErrDenied
	}
	return err
}
func resourceDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid isolated database configuration")
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("requires isolated *_test database")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("resources_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	_, err = pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT NOT NULL);
 CREATE TABLE tasks(task_id TEXT PRIMARY KEY,parent_task_id TEXT,owner_id TEXT NOT NULL,session_key TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'active',revision BIGINT NOT NULL DEFAULT 1,constraints_revision BIGINT NOT NULL DEFAULT 1);
 CREATE TABLE runs(run_id TEXT PRIMARY KEY,session_key TEXT NOT NULL,task_id TEXT,task_revision BIGINT,current_state TEXT DEFAULT 'tool_running',status TEXT DEFAULT 'tool_running');
 CREATE TABLE session_ownership(session_key TEXT PRIMARY KEY,run_id TEXT NOT NULL,lease_id TEXT NOT NULL,fence_epoch BIGINT NOT NULL,expires_at TIMESTAMPTZ NOT NULL);
 CREATE TABLE memory_sources(source_id TEXT PRIMARY KEY);
 CREATE TABLE independent_resource_evidence(evidence_id TEXT PRIMARY KEY,owner_id TEXT,resource_id TEXT,lease_id TEXT,version BIGINT);
 INSERT INTO sessions VALUES('s1','owner'),('s2','owner');
 INSERT INTO tasks(task_id,owner_id,session_key) VALUES('t1','owner','s1'),('t2','owner','s2');
 INSERT INTO runs(run_id,session_key,task_id,task_revision) VALUES('r1','s1','t1',1),('r2','s2','t2',1);
 INSERT INTO session_ownership VALUES('s1','r1','owner-lease1',1,clock_timestamp()+INTERVAL '1 hour'),('s2','r2','owner-lease2',1,clock_timestamp()+INTERVAL '1 hour');`)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"031_action_authorization.up.sql", "035_external_effect_journal.up.sql", "037_execution_budgets.up.sql", "038_owner_goals_mandates.up.sql", "038_owner_purpose_policies.up.sql", "057_task_delegations.up.sql", "058_execution_resource_leases.up.sql"} {
		data, e := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(data)); e != nil {
			t.Fatalf("migration %s: %v", name, e)
		}
	}
	return pool
}
func worker(i int) (context.Context, Binding) {
	b := Binding{OwnerID: "owner", RunID: fmt.Sprintf("r%d", i), SessionKey: fmt.Sprintf("s%d", i), TaskID: fmt.Sprintf("t%d", i), TaskRevision: 1, ConstraintsRevision: 1, LeaseEpoch: 1, Deadline: time.Now().Add(10 * time.Minute)}
	ctx := ownership.WithToken(context.Background(), nil, ownership.Token{SessionKey: b.SessionKey, RunID: b.RunID, LeaseID: fmt.Sprintf("owner-lease%d", i), FenceEpoch: 1})
	return ctx, b
}
func newStore(t *testing.T) *Store {
	t.Helper()
	s, e := New(WorkerGuard{}, evidenceGuard{})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func transact(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func register(t *testing.T, pool *pgxpool.Pool, s *Store, ids ...string) {
	t.Helper()
	ctx, b := worker(1)
	for _, id := range ids {
		if err := transact(ctx, pool, func(tx pgx.Tx) error { return s.RegisterTx(ctx, tx, b, id) }); err != nil {
			t.Fatal(err)
		}
	}
}
func acquire(ctx context.Context, pool *pgxpool.Pool, s *Store, b Binding, reqs ...Request) (proofs []Proof, err error) {
	err = transact(ctx, pool, func(tx pgx.Tx) error { var e error; proofs, e = s.AcquireTx(ctx, tx, b, reqs, MaxLeaseTTL); return e })
	return
}
func release(t *testing.T, pool *pgxpool.Pool, s *Store, ctx context.Context, b Binding, p Proof) {
	t.Helper()
	if err := transact(ctx, pool, func(tx pgx.Tx) error { return s.ReleaseTx(ctx, tx, b, p) }); err != nil {
		t.Fatal(err)
	}
}

func TestResourceReadersAndExclusiveWriterAcrossSessionsIntegration(t *testing.T) {
	pool := resourceDB(t)
	s := newStore(t)
	other := newStore(t)
	register(t, pool, s, "browser-tab:one")
	c1, b1 := worker(1)
	c2, b2 := worker(2)
	p1, e := acquire(c1, pool, s, b1, Request{"browser-tab:one", Read, 1})
	if e != nil {
		t.Fatal(e)
	}
	p2, e := acquire(c2, pool, other, b2, Request{"browser-tab:one", Read, 1})
	if e != nil {
		t.Fatal("independent read denied:", e)
	}
	if _, e = acquire(c1, pool, s, b1, Request{"browser-tab:one", Write, 1}); !errors.Is(e, ErrConflict) {
		t.Fatalf("writer overlapped readers: %v", e)
	}
	release(t, pool, s, c1, b1, p1[0])
	release(t, pool, other, c2, b2, p2[0])
	p1, e = acquire(c1, pool, s, b1, Request{"browser-tab:one", Write, 1})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = acquire(c2, pool, other, b2, Request{"browser-tab:one", Write, 1}); !errors.Is(e, ErrConflict) {
		t.Fatalf("two writers admitted: %v", e)
	}
	if _, e = acquire(c2, pool, other, b2, Request{"browser-tab:one", Read, 1}); !errors.Is(e, ErrConflict) {
		t.Fatalf("read overlapped writer: %v", e)
	}
	forged := p1[0]
	forged.RunID = b2.RunID
	forged.SessionKey = b2.SessionKey
	forged.TaskID = b2.TaskID
	if e = transact(c2, pool, func(tx pgx.Tx) error { return other.CheckTx(c2, tx, b2, forged, Write) }); !errors.Is(e, ErrDenied) {
		t.Fatalf("cross-worker copied proof: %v", e)
	}
	if e = transact(context.Background(), pool, func(tx pgx.Tx) error { return s.CheckTx(context.Background(), tx, b1, p1[0], Write) }); !errors.Is(e, ErrDenied) {
		t.Fatalf("no ownership token accepted: %v", e)
	}
	encoded, _ := json.Marshal(p1[0])
	if string(encoded) != "{}" {
		t.Fatalf("private proof serialized: %s", encoded)
	}
}

func TestResourceSetAtomicFailureAndOppositeOrderIntegration(t *testing.T) {
	pool := resourceDB(t)
	s := newStore(t)
	register(t, pool, s, "workspace:a", "workspace:b")
	c1, b1 := worker(1)
	c2, b2 := worker(2)
	held, e := acquire(c2, pool, s, b2, Request{"workspace:b", Write, 1})
	if e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(c1)
	if e != nil {
		t.Fatal(e)
	}
	_, denied := s.AcquireTx(c1, tx, b1, []Request{{"workspace:b", Write, 1}, {"workspace:a", Write, 1}}, MaxLeaseTTL)
	if !errors.Is(denied, ErrConflict) {
		tx.Rollback(c1)
		t.Fatal("partial admission not denied:", denied)
	}
	if e = tx.Commit(c1); e != nil {
		t.Fatal(e)
	}
	var count, fence int64
	if e = pool.QueryRow(c1, `SELECT count(*) FROM execution_resource_leases WHERE resource_id='workspace:a'`).Scan(&count); e != nil || count != 0 {
		t.Fatalf("partial lease leaked after caller commit: %d %v", count, e)
	}
	if e = pool.QueryRow(c1, `SELECT fence_epoch FROM execution_resources WHERE resource_id='workspace:a'`).Scan(&fence); e != nil || fence != 1 {
		t.Fatalf("partial fence leaked: %d %v", fence, e)
	}
	release(t, pool, s, c2, b2, held[0])
	type outcome struct {
		ctx context.Context
		b   Binding
		p   []Proof
		err error
	}
	start := make(chan struct{})
	done := make(chan outcome, 2)
	for i := 1; i <= 2; i++ {
		go func(i int) {
			ctx, b := worker(i)
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			<-start
			reqs := []Request{{"workspace:a", Write, 1}, {"workspace:b", Write, 1}}
			if i == 2 {
				reqs[0], reqs[1] = reqs[1], reqs[0]
			}
			p, e := acquire(ctx, pool, newStore(t), b, reqs...)
			base, _ := worker(i)
			done <- outcome{base, b, p, e}
		}(i)
	}
	close(start)
	accepted := 0
	outcomes := []outcome{<-done, <-done}
	for _, o := range outcomes {
		if o.err == nil {
			accepted++
		} else if !errors.Is(o.err, ErrConflict) {
			t.Fatal("opposite order deadlocked/failed:", o.err)
		}
	}
	if accepted != 1 {
		t.Fatalf("exclusive set admission count: %d", accepted)
	}
	for _, o := range outcomes {
		if o.err == nil {
			for _, p := range o.p {
				release(t, pool, s, o.ctx, o.b, p)
			}
		}
	}
}

func TestResourceExpiredWriterRemainsBlockedUntilEvidenceReconciliationIntegration(t *testing.T) {
	pool := resourceDB(t)
	s := newStore(t)
	register(t, pool, s, "workspace:crash")
	c1, b1 := worker(1)
	c2, b2 := worker(2)
	p, e := acquire(c1, pool, s, b1, Request{"workspace:crash", Write, 1})
	if e != nil {
		t.Fatal(e)
	}
	if e = transact(c1, pool, func(tx pgx.Tx) error { return s.ClaimTx(c1, tx, b1, p[0]) }); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(c1, `UPDATE execution_resource_leases SET expires_at=clock_timestamp() WHERE lease_id=$1`, p[0].LeaseID); e != nil {
		t.Fatal(e)
	}
	restarted := newStore(t)
	if _, e = acquire(c2, pool, restarted, b2, Request{"workspace:crash", Write, 1}); !errors.Is(e, ErrConflict) {
		t.Fatalf("crash/expiry silently released claimed write: %v", e)
	}
	if e = transact(c1, pool, func(tx pgx.Tx) error { return restarted.CheckTx(c1, tx, b1, p[0], Write) }); !errors.Is(e, ErrDenied) {
		t.Fatalf("expired writer retained authority: %v", e)
	}
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "independent-check"})
	reconcile := func(ctx context.Context, evidence string) (version int64, err error) {
		err = transact(ctx, pool, func(tx pgx.Tx) error {
			var e error
			version, e = restarted.ReconcileTx(ctx, tx, "owner", "workspace:crash", p[0].LeaseID, evidence, 1)
			return e
		})
		return
	}
	if _, e = reconcile(c1, "actual-receipt"); !errors.Is(e, ErrDenied) {
		t.Fatalf("worker reconciled own unknown write: %v", e)
	}
	if _, e = reconcile(owner, "invented-receipt"); !errors.Is(e, ErrDenied) {
		t.Fatalf("unrecorded evidence accepted: %v", e)
	}
	if _, e = pool.Exec(owner, `INSERT INTO independent_resource_evidence VALUES('actual-receipt','owner','workspace:crash',$1,1)`, p[0].LeaseID); e != nil {
		t.Fatal(e)
	}
	version, e := reconcile(owner, "actual-receipt")
	if e != nil || version != 2 {
		t.Fatalf("exact owner evidence reconciliation: %d %v", version, e)
	}
	if _, e = acquire(c2, pool, restarted, b2, Request{"workspace:crash", Write, 1}); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale observation acquired after reconciliation: %v", e)
	}
	next, e := acquire(c2, pool, restarted, b2, Request{"workspace:crash", Write, 2})
	if e != nil || next[0].FenceEpoch <= p[0].FenceEpoch {
		t.Fatalf("fresh successor fence missing: %+v %v", next, e)
	}
}

func TestResourceCurrentTaskParentAndOwnerFencesIntegration(t *testing.T) {
	for _, change := range []string{"owner_epoch", "task_revision", "constraints", "parent"} {
		t.Run(change, func(t *testing.T) {
			pool := resourceDB(t)
			s := newStore(t)
			register(t, pool, s, "memory:private")
			ctx, b := worker(1)
			if change == "parent" {
				_, e := pool.Exec(ctx, `INSERT INTO tasks(task_id,owner_id,session_key) VALUES('parent','owner','parent-session');
 UPDATE tasks SET parent_task_id='parent' WHERE task_id='t1';INSERT INTO memory_sources VALUES('contract');
 INSERT INTO task_delegations(task_id,root_task_id,parent_task_id,parent_revision,parent_constraints_revision,contract_source_id,contract_json,budget_json,deadline,depth,state)
 VALUES('t1','parent','parent',1,1,'contract','{}','{}',clock_timestamp()+INTERVAL '1 hour',1,'executing');`)
				if e != nil {
					t.Fatal(e)
				}
			}
			p, e := acquire(ctx, pool, s, b, Request{"memory:private", Write, 1})
			if e != nil {
				t.Fatal(e)
			}
			statement := map[string]string{"owner_epoch": `UPDATE session_ownership SET fence_epoch=fence_epoch+1 WHERE session_key='s1'`, "task_revision": `UPDATE tasks SET revision=revision+1 WHERE task_id='t1'`, "constraints": `UPDATE tasks SET constraints_revision=constraints_revision+1 WHERE task_id='t1'`, "parent": `UPDATE tasks SET revision=revision+1 WHERE task_id='parent'`}[change]
			if _, e = pool.Exec(ctx, statement); e != nil {
				t.Fatal(e)
			}
			if e = transact(ctx, pool, func(tx pgx.Tx) error { return s.ClaimTx(ctx, tx, b, p[0]) }); e == nil {
				t.Fatal("stale authority claimed resource write")
			}
			var state string
			if e = pool.QueryRow(ctx, `SELECT state FROM execution_resource_leases WHERE lease_id=$1`, p[0].LeaseID).Scan(&state); e != nil || state != "held" {
				t.Fatalf("denial mutated lease: %s %v", state, e)
			}
		})
	}
}

func TestResourceConfirmedWriteConsumesLeaseAndInvalidatesObservationIntegration(t *testing.T) {
	pool := resourceDB(t)
	s := newStore(t)
	register(t, pool, s, "workspace:confirmed")
	ctx, b := worker(1)
	p, e := acquire(ctx, pool, s, b, Request{"workspace:confirmed", Write, 1})
	if e != nil {
		t.Fatal(e)
	}
	if e = transact(ctx, pool, func(tx pgx.Tx) error { return s.ClaimTx(ctx, tx, b, p[0]) }); e != nil {
		t.Fatal(e)
	}
	var version int64
	e = transact(ctx, pool, func(tx pgx.Tx) error {
		var err error
		version, err = s.CompleteTx(ctx, tx, b, p[0], "file-cas:confirmed")
		return err
	})
	if e != nil || version != 2 {
		t.Fatalf("confirmed version: %d %v", version, e)
	}
	if e = transact(ctx, pool, func(tx pgx.Tx) error { return s.CheckTx(ctx, tx, b, p[0], Write) }); !errors.Is(e, ErrDenied) {
		t.Fatalf("consumed lease replay: %v", e)
	}
	if _, e = acquire(ctx, pool, s, b, Request{"workspace:confirmed", Read, 1}); !errors.Is(e, ErrConflict) {
		t.Fatalf("old observation accepted: %v", e)
	}
	if _, e = acquire(ctx, pool, s, b, Request{"workspace:confirmed", Read, 2}); e != nil {
		t.Fatal(e)
	}
}

func TestResourceDeadlineIsRecheckedAfterContendedLockIntegration(t *testing.T) {
	pool := resourceDB(t)
	s := newStore(t)
	register(t, pool, s, "workspace:deadline")
	ctx, b := worker(1)
	b.Deadline = time.Now().Add(300 * time.Millisecond)
	blocker, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback(ctx)
	if _, e = blocker.Exec(ctx, `SELECT resource_id FROM execution_resources WHERE resource_id='workspace:deadline' FOR UPDATE`); e != nil {
		t.Fatal(e)
	}
	result := make(chan error, 1)
	go func() { _, e := acquire(ctx, pool, s, b, Request{"workspace:deadline", Write, 1}); result <- e }()
	// Wait for a database row-lock waiter, proving the expiry happened after
	// initial live authority checks rather than an already-expired input.
	until := time.Now().Add(time.Second)
	waiting := false
	for time.Now().Before(until) {
		// PostgreSQL caches activity snapshots within a transaction. Refresh the
		// snapshot so this observer can see the other process begin its wait.
		if _, e = blocker.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); e != nil {
			t.Fatal(e)
		}
		if e = blocker.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid<>pg_backend_pid() AND wait_event_type='Lock' AND query LIKE '%execution_resources%')`).Scan(&waiting); e != nil {
			t.Fatal(e)
		}
		if waiting {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !waiting {
		t.Fatal("acquisition did not reach the contended resource lock")
	}
	if _, e = blocker.Exec(ctx, `SELECT pg_sleep(0.35)`); e != nil {
		t.Fatal(e)
	}
	if e = blocker.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-result; !errors.Is(e, ErrDenied) {
		t.Fatalf("authority expired while waiting: %v", e)
	}
	var leases, fence int64
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM execution_resource_leases WHERE resource_id='workspace:deadline'`).Scan(&leases); e != nil || leases != 0 {
		t.Fatalf("late lease leaked: %d %v", leases, e)
	}
	if e = pool.QueryRow(ctx, `SELECT fence_epoch FROM execution_resources WHERE resource_id='workspace:deadline'`).Scan(&fence); e != nil || fence != 1 {
		t.Fatalf("late fence mutation leaked: %d %v", fence, e)
	}
}
