package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/transport"
)

type streamSkillCatalog struct {
	SkillCatalog
	calls   atomic.Int64
	revoked atomic.Bool
	block   atomic.Bool
}

func (c *streamSkillCatalog) Pinned(ctx context.Context, _ string) ([]skills.Package, error) {
	c.calls.Add(1)
	if c.block.Load() {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if c.revoked.Load() {
		return nil, errors.New("private revocation detail")
	}
	return []skills.Package{{Digest: "loaded"}}, nil
}

func TestSkillStreamFragmentsDoNotReloadCatalog(t *testing.T) {
	c := &streamSkillCatalog{}
	provider := &skillTestProvider{stream: make(chan transport.TransportEvent, 601)}
	for i := 0; i < 600; i++ {
		provider.stream <- transport.TransportEvent{RunID: "run", EventType: transport.EventTypeAssistantDelta, AssistantDelta: &transport.AssistantDelta{DeltaType: "text", Content: "fragment"}}
	}
	provider.stream <- transport.TransportEvent{RunID: "run", EventType: transport.EventTypeAssistantFinal}
	close(provider.stream)
	p := &skillProtectedProvider{ModelProvider: provider, catalog: c}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream, err := p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for event := range stream {
		if event.TransportError != nil {
			t.Fatal("unexpected rejection")
		}
		n++
	}
	if n != 601 || c.calls.Load() < 2 || c.calls.Load() >= 60 {
		t.Fatalf("lost events or per-fragment proof: events=%d proofs=%d", n, c.calls.Load())
	}
}

func TestSkillStreamFreshProofBeforeNonDeltaRelease(t *testing.T) {
	for _, kind := range []transport.EventType{transport.EventTypeAssistantFinal, transport.EventTypeToolCallRequested, transport.EventTypeToolCallBatchRequested, transport.EventTypeRunCompleted} {
		t.Run(string(kind), func(t *testing.T) {
			c := &streamSkillCatalog{}
			provider := &skillTestProvider{stream: make(chan transport.TransportEvent)}
			p := &skillProtectedProvider{ModelProvider: provider, catalog: c}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			stream, err := p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
			if err != nil {
				t.Fatal(err)
			}
			c.revoked.Store(true)
			provider.stream <- transport.TransportEvent{RunID: "run", EventType: kind}
			select {
			case event := <-stream:
				if event.TransportError == nil || event.TransportError.Type != skillInvalidReason {
					t.Fatal("released event after revocation")
				}
			case <-ctx.Done():
				t.Fatal("missing rejection")
			}
		})
	}
}

func TestSkillStreamRevocationDuringContinuousDeltas(t *testing.T) {
	c := &streamSkillCatalog{}
	provider := &skillTestProvider{stream: make(chan transport.TransportEvent)}
	p := &skillProtectedProvider{ModelProvider: provider, catalog: c}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream, err := p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			select {
			case provider.stream <- transport.TransportEvent{RunID: "run", EventType: transport.EventTypeAssistantDelta, AssistantDelta: &transport.AssistantDelta{DeltaType: "text", Content: "fragment"}}:
			case <-ctx.Done():
				return
			}
		}
	}()
	if event := <-stream; event.EventType != transport.EventTypeAssistantDelta {
		t.Fatal("missing initial delta")
	}
	c.revoked.Store(true)
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				t.Fatal("closed without rejection")
			}
			if event.TransportError != nil {
				if event.TransportError.Type != skillInvalidReason {
					t.Fatal("wrong rejection")
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("hot stream postponed revocation")
		}
	}
}

func TestSkillStreamCancellationInterruptsCatalogCheck(t *testing.T) {
	c := &streamSkillCatalog{}
	provider := &skillTestProvider{stream: make(chan transport.TransportEvent)}
	p := &skillProtectedProvider{ModelProvider: provider, catalog: c}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	c.block.Store(true)
	provider.stream <- transport.TransportEvent{RunID: "run", EventType: transport.EventTypeAssistantFinal}
	cancel()
	deadline := time.After(time.Second)
	for {
		select {
		case event, ok := <-stream:
			if !ok {
				return
			}
			if event.TransportError == nil {
				t.Fatal("released a held final")
			}
		case <-deadline:
			t.Fatal("catalog check outlived cancellation")
		}
	}
}

func TestSkillStreamMixedDeltaCannotReuseProof(t *testing.T) {
	c := &streamSkillCatalog{}
	provider := &skillTestProvider{stream: make(chan transport.TransportEvent)}
	p := &skillProtectedProvider{ModelProvider: provider, catalog: c}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream, err := p.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	c.revoked.Store(true)
	provider.stream <- transport.TransportEvent{RunID: "run", EventType: transport.EventTypeAssistantDelta,
		AssistantDelta: &transport.AssistantDelta{DeltaType: "text"}, AssistantFinal: &transport.AssistantFinal{Content: "held"}}
	select {
	case event := <-stream:
		if event.TransportError == nil || event.TransportError.Type != skillInvalidReason {
			t.Fatal("mixed final/delta bypassed fresh proof")
		}
	case <-ctx.Done():
		t.Fatal("missing mixed-event rejection")
	}
}
