package orchestrator

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/transport"
)

type terminalWatchLeases struct {
	*memoryLeaseManager
	lease            session.LeaseRecord
	entered, release chan struct{}
	once             sync.Once
	err              error
}

func (l *terminalWatchLeases) GetLease(ctx context.Context, _ string) (session.LeaseRecord, error) {
	l.once.Do(func() { close(l.entered) })
	select {
	case <-ctx.Done():
		return session.LeaseRecord{}, ctx.Err()
	case <-l.release:
		return l.lease, l.err
	}
}

type terminalWatchTasks struct {
	task.Manager
	check func(context.Context, string) error
}

func (t *terminalWatchTasks) ValidateRunAuthority(ctx context.Context, run string) error {
	return t.check(ctx, run)
}

type terminalWatchProvider struct {
	mockProvider
	cancelled atomic.Int32
}

func (p *terminalWatchProvider) CancelRun(_ context.Context, q transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.cancelled.Add(1)
	e := transport.NewEvent(q.RunID, transport.EventTypeRunCancelled, p.Name())
	return &e, nil
}

// Request quiescence while a real supervisor check is in flight. A successful
// check may finish without cancelling the original execution context; actual
// in-flight failures must still cancel and retain their original classification.
func TestTerminalWatchQuiescePreservesInflightChecks(t *testing.T) {
	for _, fault := range []string{"none", "lease_unavailable", "task_revoked", "task_deadline", "wrong_lease", "parent_cancel"} {
		t.Run(fault, func(t *testing.T) {
			parent, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			lease := session.LeaseRecord{LeaseID: "lease", RunID: "run", SessionKey: "session", FenceEpoch: 1, ExpiresAt: time.Now().Add(time.Minute)}
			reader := &terminalWatchLeases{memoryLeaseManager: &memoryLeaseManager{}, lease: lease, entered: make(chan struct{}), release: make(chan struct{})}
			if fault == "lease_unavailable" {
				reader.err = errors.New("synthetic authority store unavailable")
			}
			if fault == "wrong_lease" {
				reader.lease.RunID = "other-run"
			}
			guard := &terminalWatchTasks{check: func(ctx context.Context, run string) error {
				if run != "run" {
					return errors.New("foreign run")
				}
				if fault == "task_revoked" {
					return task.ErrConflict
				}
				if fault == "task_deadline" {
					<-ctx.Done()
					return ctx.Err()
				}
				return nil
			}}
			provider := &terminalWatchProvider{}
			s := &Service{leases: reader, provider: provider, config: Config{LeaseTTL: 60, Tasks: guard}}
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			runCtx, stop, errs := s.startLeaseRenewer(parent, lease, log)
			defer stop()
			select {
			case <-reader.entered:
			case <-parent.Done():
				t.Fatal("original observer did not enter")
			}
			joined := make(chan struct{})
			go func() { quiesceLeaseRenewer(runCtx); close(joined) }()
			control, ok := runCtx.Value(leaseRenewerKey{}).(*leaseRenewerControl)
			if !ok {
				t.Fatal("owning supervisor control missing")
			}
			select {
			case <-control.requested:
			case <-parent.Done():
				t.Fatal("quiescence request missing")
			}
			select {
			case <-joined:
				t.Fatal("in-flight check abandoned")
			default:
			}
			if runCtx.Err() != nil {
				t.Fatal("quiescence itself cancelled execution")
			}
			if fault == "parent_cancel" {
				cancel()
			}
			close(reader.release)
			select {
			case <-joined:
			case <-parent.Done():
				if fault != "parent_cancel" {
					t.Fatal("supervisor did not join")
				}
				<-joined
			}
			got := drainRenewError(errs)
			if fault == "none" {
				if runCtx.Err() != nil || got != nil || provider.cancelled.Load() != 0 {
					t.Fatal("ordinary terminal cleanup acquired cancellation", got)
				}
				quiesceLeaseRenewer(runCtx) // idempotent without closing the context
				if runCtx.Err() != nil {
					t.Fatal("second quiescence cancelled context")
				}
			} else if fault == "parent_cancel" {
				if !errors.Is(runCtx.Err(), context.Canceled) || got != nil || provider.cancelled.Load() != 0 {
					t.Fatal("parent cancellation relabelled", got)
				}
			} else {
				var owned *ownedCancellation
				if !errors.As(got, &owned) || context.Cause(runCtx) != owned || provider.cancelled.Load() != 1 {
					t.Fatal("real in-flight fault hidden", got, context.Cause(runCtx))
				}
			}
			stop()
			if runCtx.Err() == nil {
				t.Fatal("ordinary final stop did not cancel execution")
			}
		})
	}
}

func TestTerminalWatchAbsentControlDoesNotCreateAuthority(t *testing.T) {
	ctx := context.Background()
	quiesceLeaseRenewer(ctx)
	if ctx.Err() != nil {
		t.Fatal("unowned context changed")
	}
}
