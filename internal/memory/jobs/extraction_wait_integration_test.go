package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
)

func TestExtractionJobExpiresDuringExactRetryWaitIntegration(t *testing.T) {
	s, c, owner, src := fixture(t)
	p := catalog.ClaimRequest{SourceID: src.ID, SourceRevision: src.Revision, Key: "candidate", Value: json.RawMessage(`"data"`), Confidence: 1, DedupKey: "trusted-job-checkpoint"}
	tx, err := s.pool.Begin(owner)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := c.ProposeTx(owner, tx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(owner); err != nil {
		t.Fatal(err)
	}
	r := request(src)
	r.Kind, r.Purpose = "extract", "memory_extract"
	r.Deadline = time.Now().Add(2 * time.Second)
	if _, err = s.Enqueue(owner, r); err != nil {
		t.Fatal(err)
	}
	j := claim(t, s)
	ctx, cancel := context.WithTimeout(s.Context(context.Background(), j), 10*time.Second)
	defer cancel()
	worker, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Rollback(context.Background())
	blocker, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err = blocker.Exec(ctx, `SELECT 1 FROM memory_extraction_dedup WHERE dedup_key=$1 FOR UPDATE`, p.DedupKey); err != nil {
		t.Fatal(err)
	}
	pid := worker.Conn().PgConn().PID()
	result := make(chan error, 1)
	go func() {
		_, err := c.ProposeTx(ctx, worker, p)
		result <- err
	}()
	for {
		var waiting bool
		if err = blocker.QueryRow(ctx, `SELECT COALESCE(wait_event_type='Lock',FALSE) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err = <-result:
			t.Fatal("job did not wait on exact retry", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	for {
		var expired bool
		if err = blocker.QueryRow(ctx, `SELECT clock_timestamp()>=$1::timestamptz`, j.Deadline).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, ErrClaim) {
		t.Fatal("expired independent job returned candidate", err)
	}
	if err = worker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	claims, err := c.Claims(owner)
	if err != nil || len(claims) != 1 || claims[0].ID != prior.ID || claims[0].Status != "candidate" {
		t.Fatal("retry accepted/replaced/duplicated candidate", claims, err)
	}
}
