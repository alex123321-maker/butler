package communication_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
)

type resumeFailingLeases struct {
	*session.PostgresLeaseManager
	fail atomic.Bool
}

func (l *resumeFailingLeases) GetLease(ctx context.Context, id string) (session.LeaseRecord, error) {
	if l.fail.Load() {
		return session.LeaseRecord{}, errors.New("private watchdog database sentinel")
	}
	return l.PostgresLeaseManager.GetLease(ctx, id)
}

type resumeCancelProvider struct {
	resumedProvider
	leases *resumeFailingLeases
	phase  string
}

func (p *resumeCancelProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.leases.fail.Store(true)
	if p.phase == "start" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return make(chan transport.TransportEvent), nil
}

func TestCommunicationResumeRetainsWatchdogCauseIntegration(t *testing.T) {
	for _, phase := range []string{"answer_read", "start", "stream"} {
		t.Run(phase, func(t *testing.T) {
			f, _, live := answeredResume(t)
			if _, err := f.leases.ReleaseLease(live, f.lease.LeaseID); err != nil {
				t.Fatal(err)
			}
			leases := &resumeFailingLeases{PostgresLeaseManager: f.leases}
			if phase == "answer_read" {
				f.s.SourceGuard = func(ctx context.Context, _ pgx.Tx, _ string) error {
					leases.fail.Store(true)
					<-ctx.Done()
					return ctx.Err()
				}
			}
			provider := &resumeCancelProvider{leases: leases, phase: phase}
			core := flow.NewService(session.NewPostgresRepository(f.pool), leases, run.NewService(run.NewPostgresRepository(f.pool), nil), transcript.NewStore(f.pool), provider, flow.Config{Communication: f.s, Checkpoints: f.s.Inputs, OwnerID: "resume-cause", LeaseTTL: 30, ProviderName: "fixture", ModelName: "fixture"}, nil)
			ctx, cancel := context.WithTimeout(live, 3*time.Second)
			defer cancel()
			_, err := core.Resume(ctx, f.p.RunID, ingress.InputEvent{SessionKey: f.p.SessionKey, Source: "fixture", PayloadJSON: `{"text":"ask owner"}`})
			if err == nil || err.Error() != "execution_authority_check_failed" || !errors.Is(err, context.Canceled) || !errors.Is(err, ownership.ErrStale) || strings.Contains(err.Error(), "sentinel") {
				t.Fatalf("recovery lost its owning cancellation cause at %s: %v", phase, err)
			}
			if ctx.Err() != nil {
				t.Fatal("watchdog cancellation delayed until parent timeout")
			}
		})
	}
}
