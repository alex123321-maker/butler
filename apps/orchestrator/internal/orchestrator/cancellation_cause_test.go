package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

// A failed authority read must still cancel immediately. Its owning cause must
// survive the stream's context.Canceled error without exposing storage details.
func TestExecuteRetainsOwnedAuthorityCancellationCause(t *testing.T) {
	for _, mode := range []string{"unavailable", "changed"} {
		t.Run(mode, func(t *testing.T) {
			leases := &causeControlLeases{mode: mode}
			provider := &causeControlProvider{mockProvider: &mockProvider{}}
			service := newFixtureService(&memorySessionRepo{}, leases, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{ProviderName: "openai", LeaseTTL: 120}, nil)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			_, err := service.Execute(ctx, causeControlEvent())
			if !errors.Is(err, ownership.ErrStale) {
				t.Fatalf("owning authority cancellation was replaced by a generic stream failure: %v", err)
			}
			if errors.Is(err, context.DeadlineExceeded) || time.Until(ctxDeadline(ctx)) <= 0 {
				t.Fatal("authority cancellation was delayed until the caller deadline")
			}
			if !provider.cancelled || leases.releaseCount != 1 {
				t.Fatal("authority cancellation must still cancel the provider and release its lease exactly once")
			}
		})
	}
}

func ctxDeadline(ctx context.Context) time.Time {
	deadline, _ := ctx.Deadline()
	return deadline
}

func causeControlEvent() ingress.InputEvent {
	return ingress.InputEvent{EventID: "authority-cause", IdempotencyKey: "authority-cause", SessionKey: "web:authority-cause", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"bounded read"}`, CreatedAt: time.Now().UTC()}
}

type causeControlLeases struct {
	memoryLeaseManager
	mode string
	run  session.LeaseRecord
}

func (m *causeControlLeases) AcquireLease(ctx context.Context, p session.AcquireLeaseParams) (session.LeaseRecord, error) {
	r, err := m.memoryLeaseManager.AcquireLease(ctx, p)
	r.FenceEpoch = 1
	m.run = r
	return r, err
}
func (m *causeControlLeases) GetLease(context.Context, string) (session.LeaseRecord, error) {
	if m.mode == "unavailable" {
		return session.LeaseRecord{}, errors.New("private storage sentinel must not become a diagnostic")
	}
	r := m.run
	r.FenceEpoch++
	return r, nil
}

type causeControlProvider struct {
	*mockProvider
	cancelled bool
}

func (p *causeControlProvider) StartRun(ctx context.Context, _ transport.StartRunRequest) (transport.EventStream, error) {
	out := make(chan transport.TransportEvent)
	go func() { <-ctx.Done(); close(out) }()
	return out, nil
}
func (p *causeControlProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.cancelled = true
	return nil, nil
}
