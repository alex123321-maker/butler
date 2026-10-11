package orchestrator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

type idleMemoryProvider struct {
	mockProvider
	stream chan transport.TransportEvent
	ctx    context.Context
}

type slowOwnedMemorySources struct {
	*catalog.Store
	bound time.Duration
}

func (s slowOwnedMemorySources) MemorySourceCheckTimeout() time.Duration { return s.bound }
func (s slowOwnedMemorySources) CheckRunSources(ctx context.Context, id string) error {
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	return s.Store.CheckRunSources(ctx, id)
}

func TestOwningSourceVerificationBoundSlowCurrentAndRevocationIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "slow-check"})
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "manifest"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(pool, manifest)
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('run','web:owner','input','model_running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('web:owner','run','lease',1,clock_timestamp()+INTERVAL '1 hour')`); err != nil {
		t.Fatal(err)
	}
	live := ownership.WithToken(ctx, ownership.NewAuthority(pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
	src, err := sources.Put(owner, catalog.PutSource{ExternalKey: "slow-source", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "private original DATA"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sources.Get(live, src.ID); err != nil {
		t.Fatal(err)
	}
	if err = sources.PinTranscript(live, "web:owner", 0); err != nil {
		t.Fatal(err)
	}
	for _, bound := range []time.Duration{0, 3 * time.Second} {
		provider := &idleMemoryProvider{stream: make(chan transport.TransportEvent, 1)}
		guard := &sourceGuardedProvider{provider, slowOwnedMemorySources{sources, bound}}
		if _, err = guard.StartRun(live, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}}); err == nil || provider.ctx != nil {
			t.Fatal("invalid owning bound released provider")
		}
	}
	provider := &idleMemoryProvider{stream: make(chan transport.TransportEvent, 1)}
	guard := &sourceGuardedProvider{provider, slowOwnedMemorySources{sources, 2 * time.Second}}
	short, stop := context.WithTimeout(live, 50*time.Millisecond)
	if _, err = guard.StartRun(short, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}}); err == nil || provider.ctx != nil {
		t.Fatal("parent cancellation released provider")
	}
	stop()
	stream, err := guard.StartRun(live, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if err != nil {
		t.Fatal("current guard beyond200ms denied", err)
	}
	provider.stream <- transport.TransportEvent{RunID: "run", EventType: transport.EventTypeAssistantFinal}
	// Source deletion occurs while the second slow current check is pending.
	time.Sleep(50 * time.Millisecond)
	if _, err = sources.Delete(owner, src.ID, "delete", src.Revision); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-stream:
		if event.EventType != transport.EventTypeTransportError {
			t.Fatal("late final escaped slow revoke", event.EventType)
		}
	case <-ctx.Done():
		t.Fatal("source guard did not finish")
	}
	if _, ok := <-stream; ok {
		t.Fatal("source revoked final stream remained open")
	}
}

func (p *idleMemoryProvider) StartRun(ctx context.Context, _ transport.StartRunRequest) (transport.EventStream, error) {
	p.ctx = ctx
	return p.stream, nil
}
func (*idleMemoryProvider) InferenceLimits() (int64, int64) { return 1000, 1000 }

func TestIdleProviderCannotPublishAfterSourceDeletionIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture"})
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "manifest"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(pool, manifest)
	if _, e := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('run','web:owner','input','model_running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('web:owner','run','lease',1,clock_timestamp()+INTERVAL '1 hour')`); e != nil {
		t.Fatal(e)
	}
	live := ownership.WithToken(ctx, ownership.NewAuthority(pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
	src, e := sources.Put(owner, catalog.PutSource{ExternalKey: "private", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "original private data"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sources.Get(live, src.ID); e != nil {
		t.Fatal(e)
	}
	if e = sources.PinTranscript(live, "web:owner", 0); e != nil {
		t.Fatal("fresh transcript rejected", e)
	}
	provider := &idleMemoryProvider{stream: make(chan transport.TransportEvent, 1)}
	guard := &sourceGuardedProvider{ModelProvider: provider, sources: sources}
	if in, out := guard.InferenceLimits(); in != 1000 || out != 1000 {
		t.Fatal("inference limits lost through guard")
	}
	stream, e := guard.StartRun(live, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sources.Delete(owner, src.ID, "delete", src.Revision); e != nil {
		t.Fatal(e)
	}
	select {
	case event := <-stream:
		if event.EventType != transport.EventTypeTransportError || event.TransportError.Type != "memory_source_stale" {
			t.Fatal(event)
		}
	case <-ctx.Done():
		t.Fatal("idle provider did not stop")
	}
	select {
	case <-provider.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("provider context remained live")
	}
	// The transport may ignore cancellation and publish later; Butler has already
	// closed its stream and cannot accept this final as a saved result.
	provider.stream <- transport.TransportEvent{RunID: "run", EventType: transport.EventTypeAssistantFinal}
	if _, ok := <-stream; ok {
		t.Fatal("late provider final escaped")
	}
	if _, e = guard.StartRun(live, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}}); e == nil {
		t.Fatal("deleted source released again")
	}
}
