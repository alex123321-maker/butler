package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/ownership"
)

func TestExtractionLeaseExpiresDuringDedupWaitIntegration(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "first_publication"
		if existing {
			name = "exact_retry"
		}
		t.Run(name, func(t *testing.T) {
			s, owner := fixture(t)
			live, cancel := context.WithTimeout(seedRun(t, s), 10*time.Second)
			defer cancel()
			src := put(t, s, owner, "source", "source")
			p := ClaimRequest{SourceID: src.ID, SourceRevision: src.Revision, Key: "language", Value: json.RawMessage(`"ru"`), Confidence: 1, DedupKey: "actual-effect-id"}
			var prior Claim
			if existing {
				tx, err := s.pool.Begin(owner)
				if err != nil {
					t.Fatal(err)
				}
				prior, err = s.ProposeTx(owner, tx, p)
				if err != nil || tx.Commit(owner) != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.pool.Exec(live, `UPDATE session_ownership SET expires_at=clock_timestamp()+INTERVAL '2 seconds' WHERE session_key='web:owner'`); err != nil {
				t.Fatal(err)
			}
			worker, err := s.pool.Begin(live)
			if err != nil {
				t.Fatal(err)
			}
			defer worker.Rollback(context.Background())
			blocker, err := s.pool.Begin(live)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(context.Background())
			if existing {
				_, err = blocker.Exec(live, `SELECT 1 FROM memory_extraction_dedup WHERE dedup_key=$1 FOR UPDATE`, p.DedupKey)
			} else {
				_, err = blocker.Exec(live, `INSERT INTO memory_extraction_dedup(owner_id,source_id,source_revision,dedup_key,request_hash,claim_id) VALUES('owner',$1,$2,$3,$4,'uncommitted-claim')`, src.ID, src.Revision, p.DedupKey, hash([]any{p.Key, json.RawMessage(p.Value), p.Summary, p.Confidence}))
			}
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			workerPID := worker.Conn().PgConn().PID()
			go func() {
				_, err := s.ProposeTx(live, worker, p)
				result <- err
			}()
			// Both connections are occupied. Observe the real waiter and database
			// clock through the blocker, without a third pool connection.
			for {
				var waiting bool
				if err = blocker.QueryRow(live, `SELECT COALESCE(wait_event_type='Lock',FALSE) FROM pg_stat_activity WHERE pid=$1`, workerPID).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case err = <-result:
					t.Fatal("worker did not wait on deduplication", err)
				default:
				}
				time.Sleep(10 * time.Millisecond)
			}
			for {
				var expired bool
				if err = blocker.QueryRow(live, `SELECT expires_at<=clock_timestamp() FROM session_ownership WHERE session_key='web:owner'`).Scan(&expired); err != nil {
					t.Fatal(err)
				}
				if expired {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err = blocker.Rollback(live); err != nil {
				t.Fatal(err)
			}
			if err = <-result; !errors.Is(err, ErrDenied) {
				t.Fatal("expired extraction returned/published candidate", err)
			}
			if err = worker.Rollback(live); err != nil {
				t.Fatal(err)
			}
			var count int
			want := 0
			if existing {
				want = 1
			}
			if err = s.pool.QueryRow(owner, `SELECT count(*) FROM memory_claims`).Scan(&count); err != nil || count != want {
				t.Fatal("late claim persisted", count, err)
			}
			if _, err = s.pool.Exec(owner, `UPDATE session_ownership SET lease_id='fresh',fence_epoch=2,expires_at=clock_timestamp()+INTERVAL '1 hour' WHERE session_key='web:owner'`); err != nil {
				t.Fatal(err)
			}
			fresh := ownership.WithToken(context.Background(), ownership.NewAuthority(s.pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "fresh", FenceEpoch: 2})
			tx, err := s.pool.Begin(fresh)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			claim, err := s.ProposeTx(fresh, tx, p)
			if err != nil || claim.Status != "candidate" || existing && claim.ID != prior.ID {
				t.Fatal("fresh extraction lost neutral/exact identity", claim, err)
			}
			if err = tx.Commit(fresh); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExtractionLeaseExpiresDuringSourceWaitIntegration(t *testing.T) {
	s, owner := fixture(t)
	live, cancel := context.WithTimeout(seedRun(t, s), 10*time.Second)
	defer cancel()
	src := put(t, s, owner, "source", "source")
	if _, err := s.pool.Exec(live, `UPDATE session_ownership SET expires_at=clock_timestamp()+INTERVAL '2 seconds' WHERE session_key='web:owner'`); err != nil {
		t.Fatal(err)
	}
	worker, err := s.pool.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Rollback(context.Background())
	blocker, err := s.pool.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err = blocker.Exec(live, `SELECT 1 FROM memory_sources WHERE source_id=$1 FOR UPDATE`, src.ID); err != nil {
		t.Fatal(err)
	}
	pid := worker.Conn().PgConn().PID()
	result := make(chan error, 1)
	go func() {
		_, err := s.AccessTx(live, worker, []SourceRef{ref(src)}, "memory_read")
		result <- err
	}()
	for {
		var waiting bool
		if err = blocker.QueryRow(live, `SELECT COALESCE(wait_event_type='Lock',FALSE) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err = <-result:
			t.Fatal("reader did not wait on source", err)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	for {
		var expired bool
		if err = blocker.QueryRow(live, `SELECT expires_at<=clock_timestamp() FROM session_ownership WHERE session_key='web:owner'`).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if expired {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = blocker.Rollback(live); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, ErrDenied) {
		t.Fatal("expired reader released source body", err)
	}
	if err = worker.Rollback(live); err != nil {
		t.Fatal(err)
	}
}
