package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/butler/butler/internal/ownership"
)

func TestExtractionConcurrentWorkerCandidatesNeedOwnerHeadCASIntegration(t *testing.T) {
	s, owner := fixture(t)
	first := seedRun(t, s)
	if _, err := s.pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner:other','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at) VALUES('other-run','web:owner:other','other-input','model_running','model_running','mode_1','fixture',clock_timestamp(),clock_timestamp()); INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('web:owner:other','other-run','other-lease',1,clock_timestamp()+INTERVAL '1 hour')`); err != nil {
		t.Fatal(err)
	}
	second := ownership.WithToken(context.Background(), ownership.NewAuthority(s.pool), ownership.Token{SessionKey: "web:owner:other", RunID: "other-run", LeaseID: "other-lease", FenceEpoch: 1})
	src := put(t, s, owner, "source", "Пиши по-русски")
	original := propose(t, s, owner, src, `"ru"`)
	original, err := s.Decide(owner, Decision{ClaimID: original.ID})
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		claim Claim
		err   error
	}
	results := make(chan result, 2)
	for i, ctx := range []context.Context{first, second} {
		go func() {
			tx, err := s.pool.Begin(ctx)
			if err != nil {
				results <- result{err: err}
				return
			}
			defer tx.Rollback(context.Background())
			value, key := `"en"`, "first-actual-effect"
			if i == 1 {
				value, key = `"de"`, "second-actual-effect"
			}
			claim, err := s.ProposeTx(ctx, tx, ClaimRequest{SourceID: src.ID, SourceRevision: src.Revision, Key: "language", Value: json.RawMessage(value), Confidence: 1, DedupKey: key})
			if err == nil {
				err = tx.Commit(ctx)
			}
			results <- result{claim, err}
		}()
	}
	claims := []Claim{}
	for range 2 {
		r := <-results
		if r.err != nil || r.claim.Status != "candidate" {
			t.Fatal("worker bypassed candidate admission", r.err, r.claim)
		}
		claims = append(claims, r.claim)
	}
	var active string
	if err = s.pool.QueryRow(owner, `SELECT active_claim_id FROM memory_fact_heads WHERE fact_key='language'`).Scan(&active); err != nil || active != original.ID {
		t.Fatal("last worker replaced accepted preference", active, err)
	}
	if _, err = s.Decide(first, Decision{ClaimID: claims[0].ID, ExpectedActiveID: original.ID}); !errors.Is(err, ErrDenied) {
		t.Fatal("worker accepted its own claim", err)
	}
	for _, claim := range claims {
		go func() {
			c, err := s.Decide(owner, Decision{ClaimID: claim.ID, ExpectedActiveID: original.ID})
			results <- result{c, err}
		}()
	}
	winners, stale := 0, 0
	for range 2 {
		r := <-results
		if r.err == nil && r.claim.Status == "confirmed" {
			winners++
		} else if errors.Is(r.err, ErrStale) {
			stale++
		} else {
			t.Fatal("unexpected owner decision", r.err)
		}
	}
	if winners != 1 || stale != 1 {
		t.Fatal("competing owner decisions overwrote a current head", winners, stale)
	}
	var activeProfiles int
	if err = s.pool.QueryRow(owner, `SELECT count(*) FROM memory_profile WHERE status='active'`).Scan(&activeProfiles); err != nil || activeProfiles != 1 {
		t.Fatal("concurrent accepted profiles", activeProfiles, err)
	}
}
