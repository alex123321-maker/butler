package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/delegation"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
)

// Only model events are scripted. A separate production Broker and ordinary
// core/input workers own every receipt, source grant, effect, transcript and
// final row. This does not reproduce the unavailable old native parent body.
type receiptPathProvider struct {
	*nativeScriptedObservationProvider
	mu             sync.Mutex
	envelopes      []string
	staleTurnFinal bool
	missingFinal   bool
}

func (p *receiptPathProvider) StartRun(ctx context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	stream, err := p.nativeScriptedObservationProvider.StartRun(ctx, request)
	if err != nil {
		return stream, err
	}
	return p.withToolCommentary(request.Context.RunID, stream, false), nil
}

func (p *receiptPathProvider) withToolCommentary(runID string, stream transport.EventStream, stale bool) transport.EventStream {
	events := []transport.TransportEvent{transport.NewAssistantDeltaEvent(runID, p.Name(), transport.AssistantDelta{Content: "Intermediate admission commentary; this is not the final receipt projection.", SequenceNo: 1})}
	if stale {
		events = append(events, transport.NewAssistantFinalEvent(runID, p.Name(), transport.AssistantFinal{Content: "Stale answer from the previous tool turn", FinishReason: "max_tokens"}))
	}
	for event := range stream {
		events = append(events, event)
	}
	return nativeScriptedStream(events...)
}

func (p *receiptPathProvider) SubmitToolResult(ctx context.Context, request transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.mu.Lock()
	p.envelopes = append(p.envelopes, request.ToolResultJSON)
	count := len(p.envelopes)
	p.mu.Unlock()
	stream, err := p.nativeScriptedObservationProvider.SubmitToolResult(ctx, request)
	if err != nil {
		return stream, err
	}
	if count == 1 {
		return p.withToolCommentary(request.RunID, stream, p.staleTurnFinal), nil
	}
	if p.missingFinal {
		// A deliberately incomplete last continuation must not commit an answer
		// observed before the second real Broker dispatch.
		return nativeScriptedStream(transport.NewAssistantDeltaEvent(request.RunID, p.Name(), transport.AssistantDelta{Content: "Last continuation ended without an assistant final", SequenceNo: 1})), nil
	}
	return stream, nil
}

func TestDelegationReceiptPathRetainsExactBrokerResultsAndRequiresFreshFinalIntegration(t *testing.T) {
	if os.Getenv("BUTLER_DELEGATION_BROKER_BINARY") == "" {
		t.Skip("actual private Broker binary required")
	}
	for _, scenario := range []string{"commentary_then_exact_final", "stale_final_before_tool_then_exact_final", "stale_final_before_tool_then_missing_final"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			f := newNativeScenarioFixture(t, ctx)
			admission, broker := nativeDiagnosticBroker(t, f, 1)
			key := "receipt-path-" + scenario
			if _, _, err := f.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: nativeFixtureOwner, Channel: "web", MetadataJSON: `{}`}); err != nil {
				t.Fatal("parent session denied")
			}
			parent, err := f.goals.CreateOwnerTask(nativeOwner(ctx, key, key), goal.TaskRequest{SessionKey: key, GoalID: f.goalID, MandateID: f.mandateID, Purpose: "native_research", Goal: "Project the two actual admission receipts", CompletionCriteria: []string{"Synthetic event boundary check only"}, InitiativeReason: "isolated zero-inference proof", ObservationRef: "fixture:receipt-path"})
			if err != nil {
				t.Fatal("bounded parent task denied")
			}
			event, err := ingress.NormalizeEvent(ingress.NormalizeRequest{EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, EventID: key, SessionKey: key, Source: "web", Payload: map[string]any{"user_id": nativeFixtureOwner, "task_id": parent.TaskID, "text": "Admit the two exact bounded sources, then project only their actual task IDs as JSON."}, DeliveryID: key})
			if err != nil {
				t.Fatal("parent event invalid")
			}
			receipt, _, err := f.inputs.Accept(nativeOwner(ctx, key, key), event, nativeFixtureOwner, parent.TaskID)
			if err != nil {
				t.Fatal("parent input denied")
			}
			provider := &receiptPathProvider{nativeScriptedObservationProvider: &nativeScriptedObservationProvider{args: f.args, receipts: map[string][]string{}}, staleTurnFinal: scenario != "commentary_then_exact_final", missingFinal: strings.HasSuffix(scenario, "missing_final")}
			reader := &nativeDiagnosticContextReader{sources: f.sources, manager: admission.Manager}
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			runs := runservice.NewService(runservice.NewPostgresRepository(f.pool), log)
			worker := durableinput.NewService(f.inputs, f.sessions, nativeDiagnosisCore(f, provider, broker, reader, f.inputs), runs, log)
			workerCtx, stop := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() { done <- worker.Run(workerCtx) }()
			defer func() {
				stop()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("owning worker did not stop")
				}
			}()
			var state, runID string
			nativeWait(t, ctx, 15*time.Second, func() bool {
				return f.pool.QueryRow(ctx, `SELECT status,run_id FROM accepted_inputs WHERE input_id=$1`, receipt.InputID).Scan(&state, &runID) == nil && (state == "completed" || state == "failed" || state == "blocked")
			})
			provider.mu.Lock()
			envelopes := append([]string(nil), provider.envelopes...)
			provider.mu.Unlock()
			if len(envelopes) != 2 {
				t.Fatal("not exactly two actual Broker continuations")
			}
			var ids []string
			for _, raw := range envelopes {
				var envelope struct {
					Status string                      `json:"status"`
					Result delegation.AdmissionReceipt `json:"result"`
				}
				if json.Unmarshal([]byte(raw), &envelope) != nil || envelope.Status != "completed" || envelope.Result.TaskID == "" || envelope.Result.ContractRevision != 1 {
					t.Fatal("receipt lost its envelope or identity")
				}
				var actualState, actualInput string
				if err := f.pool.QueryRow(ctx, `SELECT state,input_id FROM task_delegations WHERE task_id=$1 AND parent_task_id=$2`, envelope.Result.TaskID, parent.TaskID).Scan(&actualState, &actualInput); err != nil {
					t.Fatal("provider received a receipt without committed admission", err)
				}
				if envelope.Result.State == "waiting" && envelope.Result.InputID != "" || envelope.Result.State == "queued" && envelope.Result.InputID == "" {
					t.Fatal("waiting/queued receipt fields changed")
				}
				if envelope.Result.State == "queued" && actualInput != envelope.Result.InputID {
					t.Fatal("queued receipt input identity differs from its actual admission")
				}
				ids = append(ids, envelope.Result.TaskID)
			}
			var effects, finals int
			if f.pool.QueryRow(ctx, `SELECT count(*) FROM action_effects WHERE state='effect_confirmed'`).Scan(&effects) != nil || f.pool.QueryRow(ctx, `SELECT count(*) FROM run_final_results WHERE run_id=$1`, runID).Scan(&finals) != nil || effects != 2 {
				t.Fatal("actual admission observations lost")
			}
			if provider.missingFinal {
				if state == "completed" || finals != 0 {
					t.Fatal("stale pre-tool answer became a successful final after incomplete continuation")
				}
				return
			}
			if state != "completed" || finals != 1 {
				t.Fatal("fresh exact final was not persisted")
			}
			var finalText, answerState string
			if err := f.pool.QueryRow(ctx, `SELECT a.content_text,f.answer_state FROM run_final_results f JOIN artifacts a USING(artifact_id) WHERE f.run_id=$1`, runID).Scan(&finalText, &answerState); err != nil || verifyNativeAdmissionReceipts(finalText, ids) != nil {
				t.Fatal("tool commentary contaminated the actual immutable final", err)
			}
			if answerState != "final" {
				t.Fatal("earlier tool-turn finish reason contaminated the actual answer state")
			}
			history, err := transcript.NewStore(f.pool).GetRunTranscript(ctx, runID)
			if err != nil || len(history.ToolCalls) != 2 {
				t.Fatal("durable receipt history missing", err)
			}
			var storedIDs []string
			for _, call := range history.ToolCalls {
				var stored delegation.AdmissionReceipt
				if call.Status != "completed" || json.Unmarshal([]byte(call.ResultJSON), &stored) != nil {
					t.Fatal("durable tool-result body was not the Broker receipt")
				}
				storedIDs = append(storedIDs, stored.TaskID)
			}
			if !reflect.DeepEqual(ids, storedIDs) {
				t.Fatal("durable result identities differ from live continuation")
			}
			t.Log("zero_inference=true actual_broker_receipts=2 exact_parent_projection=true tool_phase_text_not_final=true")
		})
	}
}
