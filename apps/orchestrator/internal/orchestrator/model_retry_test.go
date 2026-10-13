package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func TestModelRetryDeadlineOrigin(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	for _, tc := range []struct {
		name   string
		scope  bool
		parent time.Duration
		cancel bool
		want   bool
	}{
		{"nominal_model_turn", false, 2 * time.Minute, false, true}, {"scope_or_authority", true, 2 * time.Minute, false, false}, {"parent_global_deadline", false, time.Second, false, false}, {"parent_cancel", false, 2 * time.Minute, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tc.parent)
			defer cancel()
			if tc.cancel {
				cancel()
			}
			if modelTurnRetryBound(ctx, budget.ModelAdmission{Deadline: deadline, ModelTurnLimited: !tc.scope}) != tc.want {
				t.Fatal("nominal expiry and current authority were conflated")
			}
		})
	}
}

func TestModelRetryEquivalentEffectGuard(t *testing.T) {
	finished := time.Now()
	current := &sessionv1.RunRecord{RunId: "run", SessionKey: "session", CurrentState: commonv1.RunState_RUN_STATE_MODEL_RUNNING}
	for _, tc := range []struct {
		name, tool, prior, next        string
		retry, read, unknown, disabled bool
		want                           bool
	}{
		{"new_call_id", "http.request", `{"method":"POST","url":"https://fixture.invalid","amount":1,"body":{"a":1,"b":2}}`, `{"body":{"b":2,"a":1},"amount":1.00,"url":"https://fixture.invalid","method":"POST"}`, true, false, false, false, false},
		{"numeric_exponent", "http.request", `{"method":"POST","amount":1}`, `{"amount":1e0,"method":"POST"}`, true, false, false, false, false},
		{"normal_explicit_repeat", "http.request", `{"method":"POST"}`, `{"method":"POST"}`, false, false, false, false, true},
		{"distinct_arguments", "http.request", `{"method":"POST","amount":1}`, `{"method":"POST","amount":2}`, true, false, false, false, true},
		{"current_read_only", "workspace.read", `{"path":"x"}`, `{"path":"x"}`, true, true, false, false, true},
		{"untrusted_read_name", "unknown.read", `{}`, `{}`, true, true, false, false, false},
		{"http_write_under_read_metadata", "http.request", `{"method":"POST"}`, `{"method":"POST"}`, true, true, false, false, false},
		{"disabled_read", "workspace.read", `{}`, `{}`, true, true, false, true, false},
		{"unknown_effect", "workspace.read", `{}`, `{}`, true, true, true, false, false},
		{"duplicate_nested_key", "http.request", `{"method":"POST","body":{"x":1}}`, `{"method":"POST","body":{"x":1,"x":1}}`, true, false, false, false, false},
		{"extra_json_value", "http.request", `{"method":"POST"}`, `{"method":"POST"} {}`, true, false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := "completed"
			if tc.unknown {
				status = "effect_unknown"
			}
			store := &memoryTranscriptStore{toolCalls: []transcript.ToolCall{{RunID: "run", ToolCallID: "already-confirmed", ToolName: tc.tool, ArgsJSON: tc.prior, Status: status, ResultJSON: `{"receipt":"one"}`, FinishedAt: &finished}}}
			contractStatus := "active"
			if tc.disabled {
				contractStatus = "disabled"
			}
			safety := "unsafe"
			if tc.read {
				safety = "read_only"
			}
			catalog := &recoveryCatalog{contracts: []*toolbrokerv1.ToolContract{{ToolName: tc.tool, Status: contractStatus, EffectContract: &toolbrokerv1.EffectContract{RetrySafety: safety}}}}
			s := &Service{transcript: store, config: Config{ToolCatalog: catalog}}
			ctx := context.WithValue(context.Background(), modelRetryRecoveryKey{}, tc.retry)
			requested := &transport.ToolCallRequest{ToolCallRef: "NEW-ID", ToolName: tc.tool, ArgsJSON: tc.next}
			err := s.guardModelRetryProposal(ctx, current, requested)
			if (err == nil) != tc.want {
				t.Fatal("known non-read replay or normal intentional repeat classification changed", err)
			}
			if !tc.want {
				// Missing run manager/authorizer/executor deliberately panic if the
				// guard moves after any transition or action preparation.
				if _, _, err = s.handleActionToolCall(ctx, slog.Default(), current, "lease", requested, ""); err == nil {
					t.Fatal("duplicate effect reached preparation")
				}
			}
		})
	}
}

func TestModelRetryReplayDataAndUnknownDenial(t *testing.T) {
	finished := time.Now()
	call := transcript.ToolCall{RunID: "run", ToolCallID: "confirmed", ToolName: "http.request", ArgsJSON: `{"method":"POST"}`, Status: "completed", ResultJSON: `{"receipt":"one"}`, FinishedAt: &finished}
	s := &Service{transcript: &memoryTranscriptStore{toolCalls: []transcript.ToolCall{call}}}
	current := &sessionv1.RunRecord{RunId: "run", SessionKey: "session"}
	items, err := s.modelRetryRecordedInputs(context.Background(), current, []transport.InputItem{content.CurrentOwnerMessage("continue", "input")})
	if err != nil || len(items) != 2 || items[1].Role != "user" || content.ItemOrigin(items[1]).Kind != content.ToolOutput || !strings.Contains(items[1].Content, `"already_executed":true`) {
		t.Fatal("known receipt became instruction, live functionResponse or a lost observation")
	}
	for _, tc := range []struct {
		name   string
		change func(*transcript.ToolCall)
	}{{"inflight", func(c *transcript.ToolCall) { c.FinishedAt = nil }}, {"unknown", func(c *transcript.ToolCall) { c.Status = "effect_unknown" }}, {"absent_result", func(c *transcript.ToolCall) { c.ResultJSON = ""; c.ErrorJSON = "" }}} {
		t.Run(tc.name, func(t *testing.T) {
			next := call
			tc.change(&next)
			s.transcript = &memoryTranscriptStore{toolCalls: []transcript.ToolCall{next}}
			if _, err := s.modelRetryRecordedInputs(context.Background(), current, nil); !errors.Is(err, ErrActionOutcomeUnknown) {
				t.Fatal("uncertain action replayed")
			}
		})
	}
}

type modelRetryUnitStore struct{ claims int }

func (*modelRetryUnitStore) SaveCheckpoint(context.Context, string, string, string) error { return nil }
func (*modelRetryUnitStore) LoadCheckpoint(context.Context, string) (string, string, error) {
	return "", "", nil
}
func (s *modelRetryUnitStore) ClaimModelRetry(context.Context, string, string) error {
	s.claims++
	if s.claims > 1 {
		return errors.New("spent")
	}
	return nil
}
func (*modelRetryUnitStore) ValidateModelRetry(context.Context, string, string) error    { return nil }
func (*modelRetryUnitStore) MarkModelRetryStarted(context.Context, string, string) error { return nil }

type modelRetryUnitSources struct{}

func (modelRetryUnitSources) PinTranscript(context.Context, string, int64) error { return nil }
func (modelRetryUnitSources) CheckRunSources(context.Context, string) error      { return nil }

func TestModelRetryEligibilityRequiresTrustedTimeout(t *testing.T) {
	store := &modelRetryUnitStore{}
	s := newFixtureService(nil, nil, nil, &memoryTranscriptStore{}, &mockProvider{}, Config{Checkpoints: store, MemorySources: modelRetryUnitSources{}, Budgets: budget.NewController(nil, budget.DefaultPolicy())}, nil)
	current := &sessionv1.RunRecord{RunId: "run", SessionKey: "session"}
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"raw_timeout_text", errors.New("provider timeout")}, {"provider_retryable_bit", &transport.Error{Type: transport.ErrorTypeProviderTimeout, Retryable: true}}, {"auth_unavailable", &transport.Error{Type: transport.ErrorTypeProviderUnavailable, Retryable: true}}, {"schema", &transport.Error{Type: transport.ErrorTypeProviderProtocolError}}, {"source", &transport.Error{Type: transport.ErrorTypeMemorySourceTimeout}}, {"cancel", context.Canceled}, {"parent_deadline", context.DeadlineExceeded}, {"unknown_effect", ErrActionOutcomeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if s.scheduleModelRetry(context.Background(), current, tc.err) {
				t.Fatal("untrusted or unsafe error claimed automatic inference")
			}
		})
	}
	if store.claims != 0 {
		t.Fatal("denied eligibility mutated the durable count")
	}
	if !s.scheduleModelRetry(context.Background(), current, &modelTurnTimeout{operationID: "operation-owned"}) || s.scheduleModelRetry(context.Background(), current, &modelTurnTimeout{operationID: "operation-next"}) {
		t.Fatal("private timeout did not use one durable claim")
	}
}

func TestModelRetryStageEligibilityAndObserverComposition(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage transport.ProviderStage
		want  bool
	}{
		{"absent", "", false}, {"authentication", transport.ProviderStageAuthentication, false}, {"serialization", transport.ProviderStageSerialization, false}, {"http", transport.ProviderStageHTTP, true}, {"stream", transport.ProviderStageStream, true}, {"unknown", transport.ProviderStage("PRIVATE_STAGE"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var forwarded []transport.ProviderStage
			parent := transport.WithProviderStageObserver(context.Background(), func(stage transport.ProviderStage) { forwarded = append(forwarded, stage) })
			ctx, trace := withModelRetryStages(parent)
			transport.ObserveProviderStage(ctx, tc.stage)
			if trace.inferencePhase() != tc.want {
				t.Fatal("auth/serialization/unobserved deadline became model retry")
			}
			before := len(forwarded)
			transport.ObserveProviderStage(ctx, transport.ProviderStageStream)
			if trace.inferencePhase() != tc.want || len(forwarded) != before+1 {
				t.Fatal("late phase changed frozen eligibility or outer observer was replaced")
			}
		})
	}
}

func TestModelRetryProviderTimeoutClassification(t *testing.T) {
	usage := &transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}
	for _, tc := range []struct {
		name            string
		kind            transport.EventType
		code            transport.ErrorType
		nominal, active bool
		want            transport.ErrorType
	}{
		{"provider_timeout", transport.EventTypeTransportError, transport.ErrorTypeProviderTimeout, true, true, "model_turn_timeout"},
		{"provider_scope_bound", transport.EventTypeTransportError, transport.ErrorTypeProviderTimeout, false, true, transport.ErrorTypeProviderTimeout},
		{"parent_cancel", transport.EventTypeTransportError, transport.ErrorTypeProviderTimeout, true, false, transport.ErrorTypeProviderTimeout},
		{"terminal_timeout", transport.EventTypeRunTimedOut, "", true, true, "model_turn_timeout"},
		{"terminal_scope_deadline", transport.EventTypeRunTimedOut, "", false, true, "execution_budget_deadline"},
		{"forged_private_timeout", transport.EventTypeTransportError, "model_turn_timeout", true, true, transport.ErrorTypeProviderProtocolError},
		{"cancel_never_retry", transport.EventTypeRunCancelled, "", true, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := transport.TransportEvent{EventType: tc.kind, Usage: usage}
			if tc.code != "" {
				event.TransportError = &transport.Error{Type: tc.code, Retryable: true}
			}
			got := modelRetryTimeoutEvent(event, tc.nominal, tc.active)
			if got.Usage != usage || (got.TransportError == nil) != (tc.want == "") || got.TransportError != nil && got.TransportError.Type != tc.want {
				t.Fatal("private eligibility forged, authority timeout retried, or actual usage lost")
			}
		})
	}
}
