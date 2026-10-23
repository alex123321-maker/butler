package app

import (
	"context"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/memorytest"
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
	if db.Pool().Config().MaxConns != 3 || db.AuthorityReadPool().Config().MaxConns != 1 || db.Pool() == db.AuthorityReadPool() {
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
	defer func() {
		for _, c := range occupied {
			c.Release()
		}
	}()
	for range 3 {
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
	read, stopRead := context.WithTimeout(ctx, 500*time.Millisecond)
	defer stopRead()
	if _, err = leases.GetLease(read, live.LeaseID); err != nil {
		t.Fatal(err)
	}
	if err = tasks.ValidateRunAuthority(read, "capacity-run"); err != nil {
		t.Fatal(err)
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
}

func TestCommunicationAuthorityReserveRejectsInsufficientCapacity(t *testing.T) {
	if _, err := postgresstore.OpenWithAuthorityReserve(t.Context(), postgresstore.Config{URL: "postgres://localhost/unused", MaxConns: 2}, nil); err == nil {
		t.Fatal("startup accepted no capacity for live authority observation")
	}
}
