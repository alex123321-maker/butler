package orchestrator

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

// Delay only the current proof after provider admission. The actual Catalog
// still decides source/ownership truth; neither channel supplies a verdict.
type heldCompoundMemorySources struct {
	*catalog.Store
	checks  atomic.Int32
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

type explicitCompoundMemorySources struct{ *heldCompoundMemorySources }

func (explicitCompoundMemorySources) MemorySourceCheckTimeout() time.Duration { return 2 * time.Second }
func (s *heldCompoundMemorySources) CheckRunSources(ctx context.Context, run string) error {
	if s.checks.Add(1) > 1 {
		s.once.Do(func() { close(s.entered) })
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.Store.CheckRunSources(ctx, run)
}

func TestOwningCompoundSourceHoldsEventsForCurrentProofIntegration(t *testing.T) {
	testOwningHeldEvents(t, true)
}

func TestOwningDefaultSourceHoldsEventsForCurrentProofIntegration(t *testing.T) {
	testOwningHeldEvents(t, false)
}

func testOwningHeldEvents(t *testing.T, explicit bool) {
	t.Helper()
	for _, kind := range []transport.EventType{transport.EventTypeAssistantFinal, transport.EventTypeToolCallRequested, transport.EventTypeToolCallBatchRequested} {
		for _, control := range []string{"current", "privacy", "cancel"} {
			t.Run(string(kind)+"/"+control, func(t *testing.T) {
				testOwningCompoundHeldEvent(t, kind, control, explicit)
			})
		}
	}
	for _, control := range []string{"expired_lease", "deadline"} {
		t.Run("assistant_final/"+control, func(t *testing.T) {
			testOwningCompoundHeldEvent(t, transport.EventTypeAssistantFinal, control, explicit)
		})
	}
}

func testOwningCompoundHeldEvent(t *testing.T, kind transport.EventType, control string, explicit bool) {
	t.Helper()
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "compound"})
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "manifest"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(pool, manifest)
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('run','web:owner','input','model_running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('web:owner','run','lease',1,clock_timestamp()+INTERVAL '1 hour')`); err != nil {
		t.Fatal(err)
	}
	live := ownership.WithToken(ctx, ownership.NewAuthority(pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
	src, err := sources.Put(owner, catalog.PutSource{ExternalKey: "compound-source", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "private original data"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sources.Get(live, src.ID); err != nil {
		t.Fatal(err)
	}
	if err = sources.PinTranscript(live, "web:owner", 0); err != nil {
		t.Fatal(err)
	}
	live, stop := context.WithCancel(live)
	defer stop()
	probe := &heldCompoundMemorySources{Store: sources, entered: make(chan struct{}), release: make(chan struct{})}
	provider := &idleMemoryProvider{stream: make(chan transport.TransportEvent, 1)}
	guard := &sourceGuardedProvider{ModelProvider: provider, sources: probe}
	if explicit {
		guard.sources = explicitCompoundMemorySources{probe}
	}
	out, err := guard.StartRun(live, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	usage := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}
	call := transport.ToolCallRequest{ToolName: "PRIVATE_TOOL", ArgsJSON: `{"text":"PRIVATE_BODY"}`}
	event := transport.NewAssistantFinalEvent("run", "fixture", transport.AssistantFinal{Content: "PRIVATE_FINAL", Usage: &usage})
	if kind == transport.EventTypeToolCallRequested {
		event = transport.NewToolCallRequestedEvent("run", "fixture", call)
	}
	if kind == transport.EventTypeToolCallBatchRequested {
		event = transport.NewToolCallBatchRequestedEvent("run", "fixture", transport.ToolCallBatch{ToolCalls: []transport.ToolCallRequest{call}})
	}
	event.Usage = &usage
	provider.stream <- event
	close(provider.stream)
	select {
	case <-probe.entered:
	case <-ctx.Done():
		t.Fatal("current compound proof did not start")
	}
	select {
	case <-out:
		t.Fatal("event escaped while its current proof was held")
	default:
	}
	started := time.Now()
	switch control {
	case "current":
		// A legitimate compound proof may exceed the single-Catalog deadline.
		timer := time.NewTimer(300 * time.Millisecond)
		select {
		case <-out:
			timer.Stop()
			t.Fatal("event escaped before current proof completed")
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			t.Fatal("current proof did not finish")
		}
		close(probe.release)
	case "privacy":
		if _, err = sources.Delete(owner, src.ID, "delete", src.Revision); err != nil {
			t.Fatal(err)
		}
		close(probe.release)
	case "expired_lease":
		if _, err = pool.Exec(ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE session_key='web:owner'`); err != nil {
			t.Fatal(err)
		}
		close(probe.release)
	case "cancel":
		// This is the parent cancellation used by Stop, not a verifier override.
		stop()
	case "deadline":
		// Do not release: the declared two-second current proof must expire.
	default:
		t.Fatal("unknown control")
	}
	if control == "cancel" {
		for e := range out {
			if e.EventType != transport.EventTypeTransportError {
				t.Fatal("cancelled proof released DATA", e.EventType)
			}
		}
	} else {
		select {
		case e, ok := <-out:
			if !ok {
				t.Fatal("current source decision disappeared")
			}
			if control == "current" {
				if e.EventType != kind || e.PayloadJSON != event.PayloadJSON || e.Usage == nil || *e.Usage != usage {
					t.Fatal("current event or usage changed")
				}
			} else {
				want := transport.ErrorTypeMemorySourceStale
				if control == "deadline" {
					want = transport.ErrorTypeMemorySourceTimeout
				}
				if e.EventType != transport.EventTypeTransportError || e.TransportError == nil || e.TransportError.Type != want || e.ToolCall != nil || e.ToolCallBatch != nil || e.AssistantFinal != nil || strings.Contains(e.PayloadJSON, "PRIVATE_") || e.Usage == nil || *e.Usage != usage {
					t.Fatal("denied proof released DATA or lost complete usage", e.EventType)
				}
			}
		case <-ctx.Done():
			t.Fatal("bounded source decision did not finish")
		}
		if _, ok := <-out; ok {
			t.Fatal("closed source decision released a second event")
		}
	}
	select {
	case <-provider.ctx.Done():
	case <-ctx.Done():
		t.Fatal("provider survived the closed decision")
	}
	if control == "deadline" && (time.Since(started) < time.Second || time.Since(started) > 3*time.Second) {
		t.Fatal("compound proof did not honor its explicit two-second ceiling")
	}
}
