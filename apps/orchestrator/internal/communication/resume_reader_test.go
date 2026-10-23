package communication_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// The real owner feed holds session/run read proofs until its page is assembled.
// Reading a canonical answer must coexist with that reader; it grants no effect
// and need not wait for the whole historical feed to release an exclusive row.
func TestCommunicationResumeCoexistsWithOpenWebFeedIntegration(t *testing.T) {
	for _, answerChannel := range []string{"web", "telegram"} {
		t.Run(answerChannel, func(t *testing.T) {
			f := setup(t, fixtureOptions{PoolConns: 4})
			f.s.WebAvailable = true
			pub, err := f.publish(t, "reader-question", "communication.widget", widget)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.Respond(f.actor(answerChannel, "answer"), pub.InteractionRef, f.bindings[answerChannel], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); err != nil {
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
			live := ownership.WithToken(input.WithClaim(context.Background(), claim), f.leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			if _, err = run.NewService(run.NewPostgresRepository(f.pool), nil).RefreshExecutionLease(live, f.p.RunID, lease.LeaseID); err != nil {
				t.Fatal(err)
			}
			readerEntered := make(chan struct{})
			releaseReader := make(chan struct{})
			var once sync.Once
			f.s.SourceGuard = func(ctx context.Context, tx pgx.Tx, id string) error {
				var yes bool
				if err := tx.QueryRow(ctx, `SELECT TRUE FROM runs WHERE run_id=$1 FOR SHARE`, id).Scan(&yes); err != nil {
					return err
				}
				if actor, ok := owneridentity.FromContext(ctx); ok && actor.DeliveryID == "held-feed" {
					once.Do(func() { close(readerEntered) })
					select {
					case <-releaseReader:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			}
			w := &communication.WebReader{Core: f.s, Binding: communication.Binding{ID: f.bindings["web"], Version: 1, Channel: "web", Recipient: "web:verified"}, Resolve: func(context.Context, pgx.Tx, string, communication.Binding, string) (artifacts.Media, error) {
				return artifacts.Media{}, communication.ErrDenied
			}}
			readerDone := make(chan error, 1)
			go func() {
				_, err := w.Feed(f.actor("web", "held-feed"), 0, 50)
				readerDone <- err
			}()
			select {
			case <-readerEntered:
			case err := <-readerDone:
				t.Fatal("feed never held its read proof", err)
			case <-time.After(3 * time.Second):
				t.Fatal("feed did not start")
			}
			ctx, cancel := context.WithTimeout(live, time.Second)
			answer, resumeErr := f.s.ResumeInteraction(ctx, f.p.RunID, pub.InteractionRef)
			cancel()
			close(releaseReader)
			if err := <-readerDone; err != nil {
				t.Fatal("feed after release", err)
			}
			if resumeErr != nil || answer == "" {
				t.Fatalf("canonical answer waited for unrelated page assembly: %v", resumeErr)
			}
		})
	}
}
