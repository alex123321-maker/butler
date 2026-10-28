package app

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCommunicationAuthorityReserveWhileBusinessPoolSaturatedIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_COMMUNICATION_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated communication PostgreSQL required")
	}
	t.Setenv("BUTLER_TEST_MEMORY_POSTGRES_URL", dsn)
	fixture := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	var schema string
	if err := fixture.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	db, err := postgresstore.OpenWithAuthorityReserve(ctx, postgresstore.Config{URL: u.String(), MaxConns: 4, MinConns: 4}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.Pool().Config().MaxConns != 2 || db.AuthorityReadPool().Config().MaxConns != 1 || db.SourceReadPool().Config().MaxConns != 1 || db.Pool() == db.AuthorityReadPool() || db.SourceReadPool() == db.AuthorityReadPool() || db.SourceReadPool() == db.Pool() {
		t.Fatal("reserve increased total connection allowance or shared business capacity")
	}
	if _, err = db.Pool().Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:capacity','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('capacity-run','web:capacity','input','running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	tasks := task.NewStoreWithAuthorityReads(db.Pool(), db.AuthorityReadPool())
	job, err := tasks.Create(ctx, task.CreateRequest{SessionKey: "web:capacity", OwnerID: "owner", Goal: "capacity proof", CompletionCriteria: []string{"current authority remains observable under saturated business capacity"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool().Exec(ctx, `UPDATE runs SET task_id=$1,task_revision=$2 WHERE run_id='capacity-run'`, job.TaskID, job.Revision); err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManagerWithAuthorityReads(db.Pool(), db.AuthorityReadPool())
	live, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: "web:capacity", RunID: "capacity-run", OwnerID: "worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	var occupied []*pgxpool.Conn
	manifestPath := filepath.Join(t.TempDir(), "deletions")
	manifest := catalog.NewFileManifest(manifestPath)
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.NewWithRunObservationPool(db.Pool(), db.SourceReadPool(), manifest)
	owned := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: live.SessionKey, RunID: live.RunID, LeaseID: live.LeaseID, FenceEpoch: live.FenceEpoch})
	defer func() {
		for _, c := range occupied {
			c.Release()
		}
	}()
	for range 2 {
		c, err := db.Pool().Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		occupied = append(occupied, c)
	}
	// Same business-pool lookup reproduces the original inability to observe.
	blocked, stopBlocked := context.WithTimeout(ctx, 50*time.Millisecond)
	_, err = session.NewPostgresLeaseManager(db.Pool()).GetLease(blocked, live.LeaseID)
	stopBlocked()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("unreserved observer unexpectedly had capacity", err)
	}
	// A source proof may legitimately last longer than the lease observation
	// window. Reproduce the old shared-reserve timeout without a sleep/SQL delay.
	busySource, err := db.SourceReadPool().Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer busySource.Release()
	oldShared, stopOld := context.WithTimeout(ctx, 50*time.Millisecond)
	_, err = session.NewPostgresLeaseManagerWithAuthorityReads(db.Pool(), db.SourceReadPool()).GetLease(oldShared, live.LeaseID)
	stopOld()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("shared source reserve did not reproduce lease starvation", err)
	}
	var acquisition *session.LeaseObservationError
	if !errors.As(err, &acquisition) || acquisition.Phase != "pool_acquire" || acquisition.Acquired != 1 || acquisition.Idle != 0 {
		t.Fatal("lease queue failure has no safe acquisition diagnostic")
	}
	read, stopRead := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stopRead()
	if _, err = leases.GetLease(read, live.LeaseID); err != nil {
		t.Fatal(err)
	}
	if err = tasks.ValidateRunAuthority(read, "capacity-run"); err != nil {
		t.Fatal(err)
	}
	blockedSource, stopBlockedSource := context.WithTimeout(owned, 50*time.Millisecond)
	err = sources.CheckRunSources(blockedSource, "capacity-run")
	stopBlockedSource()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("source proof escaped its occupied pool or accepted cached authority", err)
	}
	busySource.Release()
	// A blocked SQL read is a separate failure from pool starvation. This lock
	// exists only in the disposable fixture schema, never in the live database.
	lock, err := fixture.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `LOCK TABLE session_ownership IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	blockedSQL, stopSQL := context.WithTimeout(ctx, 50*time.Millisecond)
	_, err = leases.GetLease(blockedSQL, live.LeaseID)
	stopSQL()
	var query *session.LeaseObservationError
	if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &query) || query.Phase != "query" {
		t.Fatal("SQL wait confused with lease queue or accepted unavailable authority", err)
	}
	if err = lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	sourceRead, stopSourceRead := context.WithTimeout(owned, 2*time.Second)
	defer stopSourceRead()
	if err = sources.CheckRunSources(sourceRead, "capacity-run"); err != nil {
		t.Fatal("source observer starved behind business work", err)
	}
	lessons, err := experience.New(sources, strings.Repeat("a", 64), experienceToolsFixture{contract: communicationcontrol.Contract("communication.message")})
	if err != nil {
		t.Fatal(err)
	}
	composed := experienceRunSources{sources: converterSources{Store: sources, pool: db.Pool()}, catalog: sources, store: lessons}
	if err = composed.CheckRunSources(sourceRead, "capacity-run"); err != nil {
		t.Fatal("composed converter/experience proof starved behind business work", err)
	}
	compound, stopCompound := context.WithTimeout(owneridentity.WithActor(owned, owneridentity.Actor{OwnerID: "owner", Channel: "web", SessionKey: "web:capacity", DeliveryID: "compound"}), 50*time.Millisecond)
	err = sources.CheckRunSources(compound, "capacity-run")
	stopCompound()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("compound owner context consumed the pure observer reserve", err)
	}
	// Current PG revocation must still deny; the reserve is no cached authority.
	if _, err = occupied[0].Exec(ctx, `UPDATE tasks SET status='paused' WHERE task_id=$1`, job.TaskID); err != nil {
		t.Fatal(err)
	}
	if err = tasks.ValidateRunAuthority(read, "capacity-run"); !errors.Is(err, task.ErrConflict) {
		t.Fatal("paused Task retained authority", err)
	}
	if _, err = occupied[0].Exec(ctx, `UPDATE session_ownership SET expires_at=clock_timestamp() WHERE lease_id=$1`, live.LeaseID); err != nil {
		t.Fatal(err)
	}
	if _, err = leases.GetLease(read, live.LeaseID); !errors.Is(err, session.ErrLeaseNotFound) {
		t.Fatal("expired lease retained authority", err)
	}
	if err = sources.CheckRunSources(sourceRead, "capacity-run"); err == nil {
		t.Fatal("source observer released expired ownership")
	}
	if err = os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err = sources.CheckRunSources(sourceRead, "capacity-run"); !errors.Is(err, catalog.ErrRestore) {
		t.Fatal("source observer bypassed missing deletion anchor", err)
	}
}

func TestCommunicationAuthorityReserveRejectsInsufficientCapacity(t *testing.T) {
	if _, err := postgresstore.OpenWithAuthorityReserve(t.Context(), postgresstore.Config{URL: "postgres://localhost/unused", MaxConns: 2}, nil); err == nil {
		t.Fatal("startup accepted no capacity for live authority observation")
	}
}
