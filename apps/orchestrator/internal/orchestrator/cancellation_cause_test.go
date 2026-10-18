package orchestrator

import (
	"context"
	"errors"
	"strings"
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
			if strings.Contains(err.Error(), "private storage sentinel") {
				t.Fatal("private authority-read details escaped through cancellation")
			}
			want := "execution_authority_check_failed"
			if mode == "changed" {
				want = "execution_ownership_changed"
			}
			if !strings.Contains(err.Error(), want) || !errors.Is(err, context.Canceled) {
				t.Fatal("fixed owning cause and cancelled classification required")
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

func TestOwnedCancellationKeepsIndependentCauses(t *testing.T) {
	owned, cancelOwned := context.WithCancelCause(t.Context())
	cancelOwned(&ownedCancellation{reason: "execution_ownership_changed", failure: ownership.ErrStale})
	transportFailure := &transport.Error{Type: transport.ErrorTypeProviderUnavailable, Message: "provider_unavailable"}
	if preserveOwnedCancellation(owned, transportFailure) != transportFailure || preserveOwnedCancellation(owned, context.DeadlineExceeded) != context.DeadlineExceeded {
		t.Fatal("an independent provider failure or deadline was relabelled")
	}
	for _, cause := range []error{context.Canceled, errors.New("independent parent cancellation")} {
		parent, cancelParent := context.WithCancelCause(t.Context())
		cancelParent(cause)
		if preserveOwnedCancellation(parent, context.Canceled) != context.Canceled {
			t.Fatal("parent cancellation acquired a fabricated ownership cause")
		}
	}
}

func TestExecuteRetainsOwnedRenewalCancellationCause(t *testing.T) {
	leases := &memoryLeaseManager{renewErr: errors.New("private renewal sentinel")}
	provider := &causeControlProvider{mockProvider: &mockProvider{}}
	service := newFixtureService(&memorySessionRepo{}, leases, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{ProviderName: "openai", LeaseTTL: 1}, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, err := service.Execute(ctx, causeControlEvent())
	if !errors.Is(err, errLeaseRenewalFailed) || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "execution_lease_renewal_failed") || strings.Contains(err.Error(), "private renewal sentinel") {
		t.Fatalf("bounded owning renewal cause was lost or exposed private details: %v", err)
	}
	if leases.releaseCount != 1 {
		t.Fatal("renewal failure must still release the lease exactly once")
	}
}

func TestExecuteParentCancellationStaysParent(t *testing.T) {
	leases := &memoryLeaseManager{}
	provider := &causeControlProvider{mockProvider: &mockProvider{}, started: make(chan struct{})}
	service := newFixtureService(&memorySessionRepo{}, leases, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{ProviderName: "openai", LeaseTTL: 120}, nil)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := service.Execute(ctx, causeControlEvent()); done <- err }()
	select {
	case <-provider.started:
	case <-time.After(3 * time.Second):
		t.Fatal("parent cancellation control did not reach its model stream")
	}
	cancel()
	select {
	case err := <-done:
		var owned *ownedCancellation
		if !errors.Is(err, context.Canceled) || errors.As(err, &owned) || errors.Is(err, ownership.ErrStale) || leases.releaseCount != 1 {
			t.Fatal("independent parent cancellation acquired an owning cause or lost cleanup")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("parent cancellation was not honored")
	}
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
	started   chan struct{}
}

func (p *causeControlProvider) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	if p.started != nil {
		close(p.started)
	}
	// Deliberately silent even after cancellation. The owning consumer must stop
	// without depending on a cooperative provider terminal/stream close race.
	return make(chan transport.TransportEvent), nil
}
func (p *causeControlProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.cancelled = true
	return nil, nil
}
