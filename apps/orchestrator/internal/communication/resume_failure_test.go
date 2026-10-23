package communication_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func answeredResume(t *testing.T, options ...fixtureOptions) (*fixture, string, context.Context) {
	t.Helper()
	opt := fixtureOptions{PoolConns: 4}
	if len(options) > 0 {
		opt = options[0]
		opt.PoolConns = 4
	}
	f := setup(t, opt)
	pub, err := f.publish(t, "question", "communication.widget", widget)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Respond(f.actor("web", "answer"), pub.InteractionRef, f.bindings["web"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.leases.ReleaseLease(f.ctx, f.lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	claim, err := f.s.Inputs.Claim(f.ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.p.SessionKey, RunID: f.p.RunID, OwnerID: "resume", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	f.lease = lease
	ctx := ownership.WithToken(input.WithClaim(context.Background(), claim), f.leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if _, err = run.NewService(run.NewPostgresRepository(f.pool), nil).RefreshExecutionLease(ctx, f.p.RunID, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	return f, pub.InteractionRef, ctx
}

func TestCommunicationResumePostWaitFencesIntegration(t *testing.T) {
	for _, phase := range []string{"replaced_claim", "expired_claim", "expired_lease", "source_stale", "restore", "storage", "cancel"} {
		t.Run(phase, func(t *testing.T) {
			f, id, live := answeredResume(t)
			ctx, cancel := context.WithTimeout(live, 3*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			f.s.SourceGuard = func(ctx context.Context, _ pgx.Tx, _ string) error {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				switch phase {
				case "source_stale":
					return catalog.ErrStale
				case "restore":
					return catalog.ErrRestore
				case "storage":
					return errors.New("private database/source/credential sentinel")
				}
				return nil
			}
			if phase == "expired_lease" {
				if _, err := f.pool.Exec(f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()+INTERVAL '100 milliseconds' WHERE lease_id=$1`, f.lease.LeaseID); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() {
				data, err := f.s.ResumeInteraction(ctx, f.p.RunID, id)
				if data != "" {
					done <- errors.New("canonical DATA escaped a failed fence")
					return
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("resume did not reach source proof")
			}
			want := "interaction_resume_source_"
			switch phase {
			case "replaced_claim":
				_, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, f.claim.InputID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.s.Inputs.Claim(f.ctx, time.Minute); err != nil {
					t.Fatal(err)
				}
				want = "interaction_resume_claim_lost"
			case "expired_claim":
				_, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, f.claim.InputID)
				if err != nil {
					t.Fatal(err)
				}
				want = "interaction_resume_claim_lost"
			case "expired_lease":
				for {
					var expired bool
					if err := f.pool.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() FROM session_ownership WHERE lease_id=$1`, f.lease.LeaseID).Scan(&expired); err != nil {
						t.Fatal(err)
					}
					if expired {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					case <-time.After(5 * time.Millisecond):
					}
				}
				want = "interaction_resume_ownership_denied"
			case "source_stale":
				want += "stale"
			case "restore":
				want += "restore"
			case "storage":
				want += "unavailable"
			case "cancel":
				cancel()
				want += "canceled"
			}
			close(release)
			err := <-done
			if err == nil || err.Error() != want || strings.Contains(err.Error(), "sentinel") {
				t.Fatalf("owning failure lost its safe phase: got %v, want %s", err, want)
			}
			if f.count(t, "communication_publications") != 1 || f.count(t, "messages") != 2 {
				t.Fatal("failed resume duplicated the question/answer")
			}
		})
	}
}
