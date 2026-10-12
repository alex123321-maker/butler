package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
)

func TestHighRemainderProviderRoutesOnlyDeclaredWidth2ChildrenIntegration(t *testing.T) {
	testHighMixedProviderRoutesOnlyDeclaredWidth2Children(t, false)
}

func TestHighFinalProviderRoutesOnlyDeclaredWidth2ChildrenIntegration(t *testing.T) {
	testHighMixedProviderRoutesOnlyDeclaredWidth2Children(t, true)
}

func testHighMixedProviderRoutesOnlyDeclaredWidth2Children(t *testing.T, final bool) {
	if os.Getenv("BUTLER_DELEGATION_BROKER_BINARY") == "" {
		t.Skip("actual private Broker binary required")
	}
	t.Setenv("BUTLER_DELEGATION_HIGH_REMAINDER_WINDOW", "")
	t.Setenv("BUTLER_DELEGATION_HIGH_FINAL_WINDOW", "")
	baseline := 37
	if final {
		t.Setenv("BUTLER_DELEGATION_HIGH_FINAL_WINDOW", "1")
		baseline = 39
	} else {
		t.Setenv("BUTLER_DELEGATION_HIGH_REMAINDER_WINDOW", "1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	f := newNativeScenarioFixture(t, ctx)
	admission, broker := nativeDiagnosticBroker(t, f, 2)
	spy := &fullHighPreflightProvider{nativeScriptedObservationProvider: &nativeScriptedObservationProvider{args: f.args, receipts: map[string][]string{}}, f: f}
	provider, err := newHighRemainderProvider(f.pool, f.corpus, f.args, spy)
	if err != nil {
		t.Fatal("routing composition unavailable")
	}
	reader := &nativeDiagnosticContextReader{sources: f.sources, manager: admission.Manager}
	core := nativeDiagnosisCore(f, provider, broker, reader, f.inputs)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := durableinput.NewService(f.inputs, f.sessions, core, runservice.NewService(runservice.NewPostgresRepository(f.pool), log), log)
	worker.SetMaintenance((&delegationMaintenance{pool: f.pool, manager: admission.Manager}).Sweep)
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(workerCtx) }()
	defer func() { stop(); <-done }()
	// Both parents share the same actual schema, provider and Broker. A fresh
	// fixture per width would miss globally colliding scripted call references.
	for _, width := range []int{1, 2} {
		t.Run(string(rune('0'+width)), func(t *testing.T) {
			key := "native-root-width-" + string(rune('0'+width))
			if _, _, err := f.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: nativeFixtureOwner, Channel: "web", MetadataJSON: `{}`}); err != nil {
				t.Fatal("routing session unavailable")
			}
			parent, err := f.goals.CreateOwnerTask(nativeOwner(ctx, key, key), nativeScriptedTaskRequest(f, key))
			if err != nil {
				t.Fatal("routing task unavailable")
			}
			// An unknown context must not enter the selected native test-double.
			if _, err := provider.StartRun(ctx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "unknown-remainder-run", SessionKey: key}}); err == nil {
				t.Fatal("unknown context entered selected provider")
			}
			// Accept through the ordinary owner ingress API. Parent tool proposals
			// are scripted, while Broker receipts and child execution are real.
			event, err := ingress.NormalizeEvent(ingress.NormalizeRequest{EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, EventID: key, SessionKey: key, Source: "web", Payload: map[string]any{"user_id": nativeFixtureOwner, "task_id": parent.TaskID, "text": fullHighParentPrompt(f.args)}, DeliveryID: key})
			if err != nil {
				t.Fatal("routing event unavailable")
			}
			if _, _, err := f.inputs.Accept(nativeOwner(ctx, key, key), event, nativeFixtureOwner, parent.TaskID); err != nil {
				t.Fatal("routing input unavailable")
			}
			nativeWait(t, ctx, 25*time.Second, func() bool {
				var n int
				return f.pool.QueryRow(ctx, `SELECT count(*) FROM task_delegations WHERE parent_task_id=$1 AND state='result_ready'`, parent.TaskID).Scan(&n) == nil && n == 2
			})
			want := int32(0)
			if width == 2 {
				want = 2
			}
			var spent, contexts int
			if spy.parentStarts.Load() != 0 || spy.childEntries.Load() != want || provider.parentStarts.Load() != int32(width) || provider.continuations.Load() != int32(2*width) || f.pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&spent) != nil || spent != baseline || f.pool.QueryRow(ctx, `SELECT count(*) FROM fixture_native_contexts`).Scan(&contexts) != nil || contexts != 3*width {
				t.Fatal("scripted routing entered wrong lane or charged native calls")
			}
		})
	}
}
