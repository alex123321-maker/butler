package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLocalPostgresFiniteWindowSurvivesLostAttempt(t *testing.T) {
	dsn := os.Getenv("BUTLER_EFFORT_LAB_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated local research PostgreSQL required")
	}
	if dsn != "postgres://postgres@127.0.0.1:55449/effort97_test?sslmode=disable" {
		t.Fatal("unexpected fixture database")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	if _, e = pool.Exec(ctx, schema); e != nil {
		t.Fatal(e)
	}
	id := fmt.Sprintf("fixture-%d", time.Now().UnixNano())
	_, e = pool.Exec(ctx, `INSERT INTO effort_windows(id,manifest,manifest_sha256,expires_at,max_calls,max_reserved)VALUES($1,'frozen','hash',clock_timestamp()+interval '1 minute',3,30)`, id)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM effort_attempts WHERE window_id=$1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM effort_windows WHERE id=$1`, id)
	})
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := reserveAttempt(ctx, pool, id, "fixture", "synthetic", []byte(`{}`), 10); e == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 3 {
		t.Fatal(successes.Load())
	}
	var calls, reserved, unknown int
	if e = pool.QueryRow(ctx, `SELECT calls,reserved,(SELECT count(*) FROM effort_attempts WHERE window_id=$1 AND status='unknown' AND input_tokens IS NULL AND total_tokens IS NULL) FROM effort_windows WHERE id=$1`, id).Scan(&calls, &reserved, &unknown); e != nil || calls != 3 || reserved != 30 || unknown != 3 {
		t.Fatal(calls, reserved, unknown, e)
	}
	// A restarted collector cannot spend again or turn lost receipts into zero.
	pool.Reset()
	if _, e = reserveAttempt(ctx, pool, id, "restart", "synthetic", []byte(`{}`), 1); e == nil {
		t.Fatal("lost attempt was replayed")
	}
	_, e = pool.Exec(ctx, `UPDATE effort_windows SET closed=true,max_calls=20,max_reserved=200 WHERE id=$1`, id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = reserveAttempt(ctx, pool, id, "closed", "synthetic", []byte(`{}`), 1); e == nil {
		t.Fatal("closed window admitted")
	}
}
