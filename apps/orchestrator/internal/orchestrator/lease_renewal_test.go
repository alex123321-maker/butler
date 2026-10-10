package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
)

func TestExecuteStopsInFlightRenewalBeforeReleasingLease(t *testing.T) {
	t.Parallel()
	leases := &blockingRenewalLeaseManager{started: make(chan struct{}), done: make(chan struct{})}
	provider := &renewalCoordinatedProvider{
		mockProvider: &mockProvider{startEvents: []transport.TransportEvent{
			transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "done", FinishReason: "completed"}),
			transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
		}},
		started: leases.started,
	}
	service := NewService(&memorySessionRepo{}, leases, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{ProviderName: "openai", LeaseTTL: 1}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := service.Execute(ctx, ingress.InputEvent{
		EventID: "event-1", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey: "telegram:chat:1", Source: "telegram", PayloadJSON: `{"text":"hello"}`,
		CreatedAt: time.Now().UTC(), IdempotencyKey: "event-1",
	})
	if err != nil {
		t.Fatalf("Execute returned error on orderly renewal cancellation: %v", err)
	}
	if leases.releaseBeforeStop || leases.releaseCount != 1 {
		t.Fatalf("lease release must follow renewal shutdown exactly once: early=%v count=%d", leases.releaseBeforeStop, leases.releaseCount)
	}
}

type blockingRenewalLeaseManager struct {
	memoryLeaseManager
	started           chan struct{}
	done              chan struct{}
	releaseBeforeStop bool
}

func (m *blockingRenewalLeaseManager) RenewLease(ctx context.Context, _ string, _ time.Duration) (session.LeaseRecord, error) {
	close(m.started)
	defer close(m.done)
	<-ctx.Done()
	return session.LeaseRecord{}, ctx.Err()
}

func (m *blockingRenewalLeaseManager) ReleaseLease(ctx context.Context, leaseID string) (bool, error) {
	select {
	case <-m.done:
	default:
		m.releaseBeforeStop = true
	}
	return m.memoryLeaseManager.ReleaseLease(ctx, leaseID)
}

type renewalCoordinatedProvider struct {
	*mockProvider
	started <-chan struct{}
}

func (p *renewalCoordinatedProvider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	select {
	case <-p.started:
		return p.mockProvider.StartRun(ctx, req)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
