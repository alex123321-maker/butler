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
