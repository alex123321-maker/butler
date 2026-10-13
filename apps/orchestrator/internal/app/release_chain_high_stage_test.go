package app

import (
	"context"
	"sync"
	"testing"

	"github.com/butler/butler/internal/transport"
)

// Request-local diagnostic DATA; no payload or authority. Freeze before the
// owning one terminal measurement, ignoring late callbacks after cancellation.
type releaseChainHighStageTrace struct {
	mu     sync.Mutex
	last   *transport.ProviderStage
	sealed bool
}

func (s *releaseChainHighStageTrace) observe(stage transport.ProviderStage) {
	if s == nil {
		return
	}
	switch stage {
	case transport.ProviderStageAuthentication, transport.ProviderStageSerialization, transport.ProviderStageHTTP, transport.ProviderStageStream:
	default:
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.sealed {
		copy := stage
		s.last = &copy
	}
}

func (s *releaseChainHighStageTrace) seal() *transport.ProviderStage {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sealed = true
	if s.last == nil {
		return nil
	}
	copy := *s.last
	return &copy
}

func (s *releaseChainHighStageTrace) current() *transport.ProviderStage {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		return nil
	}
	copy := *s.last
	return &copy
}

func TestReleaseChainV6ProviderStageRequestLocalAndSealed(t *testing.T) {
	first, second := &releaseChainHighStageTrace{}, &releaseChainHighStageTrace{}
	ctx := transport.WithProviderStageObserver(context.Background(), first.observe)
	transport.ObserveProviderStage(ctx, transport.ProviderStage("PRIVATE_UNKNOWN_STAGE"))
	transport.ObserveProviderStage(context.Background(), transport.ProviderStageHTTP)
	transport.ObserveProviderStage(ctx, transport.ProviderStageAuthentication)
	got := first.seal()
	if got == nil || *got != transport.ProviderStageAuthentication || second.seal() != nil {
		t.Fatal("request-local stage became guessed or shared")
	}
	transport.ObserveProviderStage(ctx, transport.ProviderStageStream)
	if after := first.seal(); after == nil || *after != transport.ProviderStageAuthentication {
		t.Fatal("late stage changed a terminal witness")
	}
	*got = transport.ProviderStageStream
	if after := first.seal(); after == nil || *after != transport.ProviderStageAuthentication {
		t.Fatal("consumer changed frozen stage identity")
	}
	concurrent := &releaseChainHighStageTrace{}
	concurrent.observe(transport.ProviderStageAuthentication)
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); concurrent.observe(transport.ProviderStageStream) }()
	}
	frozen := concurrent.seal()
	workers.Wait()
	after := concurrent.current()
	if frozen == nil || after == nil || *after != *frozen {
		t.Fatal("concurrent callback changed a frozen terminal stage")
	}
}
