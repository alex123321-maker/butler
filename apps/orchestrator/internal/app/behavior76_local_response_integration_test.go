package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
)

type behavior76PublicResponseFailure struct {
	*initiativeProjectScriptedProvider
	before   func(context.Context, string)
	starts   int
	measured bool
}

func (p *behavior76PublicResponseFailure) Name() string { return "antigravity" }
func (p *behavior76PublicResponseFailure) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	transport.ObserveProviderStage(ctx, transport.ProviderStageStream)
	if p.before != nil {
		p.before(ctx, q.Context.RunID)
	}
	events := behavior76ResponsePublicEvents(q.Context.RunID, "stream_finish_rejected")
	if p.measured {
		thinking := 3
		events[len(events)-1].Usage = &transport.Usage{InputTokens: 5, OutputTokens: 4, TotalTokens: 12, ThinkingTokens: &thinking}
		v := "gemini-public-v1"
		events[len(events)-1].ProviderModelVersion = &v
	}
	return initiativeProjectStream(events...), nil
}

func behavior76ResponsePublicEvents(run, reason string) []transport.TransportEvent {
	start := transport.NewEvent(run, transport.EventTypeRunStarted, "public")
	d1 := transport.NewEvent(run, transport.EventTypeAssistantDelta, "public")
	d1.AssistantDelta = &transport.AssistantDelta{DeltaType: "text", Content: "Partial ", SequenceNo: 1}
	d2 := transport.NewEvent(run, transport.EventTypeAssistantDelta, "public")
	d2.AssistantDelta = &transport.AssistantDelta{DeltaType: "text", Content: "answer", SequenceNo: 1}
	e := transport.NewEvent(run, transport.EventTypeTransportError, "public")
	e.TransportError = &transport.Error{Type: transport.ErrorTypeProviderProtocolError, SafeReason: reason}
	return []transport.TransportEvent{start, d1, d2, e}
}

func TestBehavior76LocalResponseIsolationIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	for _, measured := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown", true: "measured"}[measured], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			var global atomic.Bool
			f := newBehavior76ResponseFixture(t, ctx, &global)
			episode := behavior76DevelopmentEpisode()
			f.seed(episode)
			base := &behavior76PublicResponseFailure{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, measured: measured}
			p := behavior76PublicWrapped(t, f, &global, base)
			result, err := f.evaluate(p, episode.Steps[0])
			if err != nil || result.State != "fixture_local_response_failure" || global.Load() || base.starts != 1 {
				t.Fatal("response did not isolate", result, err, global.Load())
			}
			var exact bool
			if err = f.pool.QueryRow(ctx, `SELECT cause='model_response_rejected' AND effects='[]'::jsonb AND native_evidence->>'stage'='response_stream' AND native_evidence->>'run_id'=$1 AND native_evidence->'owner_inputs'->0->>'input_id'=$2 AND (native_evidence->>'native_admission_cutoff_n')::int=1 AND jsonb_array_length(native_evidence->'events')=4 FROM behavior76_fixture_stops`, result.OwnerRunID, result.OwnerInputID).Scan(&exact); err != nil || !exact {
				t.Fatal("durable response identity missing", err)
			}
			var event ingress.InputEvent
			if err = f.pool.QueryRow(ctx, `SELECT event_id,session_key FROM accepted_inputs WHERE input_id=$1`, result.OwnerInputID).Scan(&event.EventID, &event.SessionKey); err != nil {
				t.Fatal(err)
			}
			normalized := &transport.Error{Type: transport.ErrorTypeProviderProtocolError, Message: "provider_protocol_error"}
			if !p.localModelExecutionError(event, normalized) {
				t.Fatal("actual stored proof rejected")
			}
			if _, err = p.journal.Exec(ctx, `UPDATE public.behavior76_attempts SET milliseconds=milliseconds+1 WHERE window_id=$1`, p.window); err != nil {
				t.Fatal(err)
			}
			if p.localModelExecutionError(event, normalized) {
				t.Fatal("consistently modified post-barrier root admitted")
			}
			if _, err = p.journal.Exec(ctx, `UPDATE public.behavior76_attempts SET milliseconds=milliseconds-1 WHERE window_id=$1`, p.window); err != nil {
				t.Fatal(err)
			}
			witness, ok := p.localFailureProofs.LoadAndDelete(result.OwnerRunID)
			if !ok || p.localModelExecutionError(event, normalized) {
				t.Fatal("missing actual observation witness admitted")
			}
			p.localFailureProofs.Store(result.OwnerRunID, witness)
			var in, out, total, thinking *int64
			var version *string
			if err = p.journal.QueryRow(ctx, `SELECT input_tokens,output_tokens,total_tokens,thinking_tokens,model_version FROM public.behavior76_attempts WHERE window_id=$1`, p.window).Scan(&in, &out, &total, &thinking, &version); err != nil {
				t.Fatal(err)
			}
			if measured {
				if in == nil || *in != 5 || out == nil || *out != 4 || total == nil || *total != 12 || thinking == nil || *thinking != 3 || version == nil || *version != "gemini-public-v1" {
					t.Fatal("measured metadata lost")
				}
			} else if in != nil || out != nil || total != nil || thinking != nil || version != nil {
				t.Fatal("unknown metadata fabricated")
			}
			if _, err = p.journal.Exec(ctx, `UPDATE public.behavior76_attempts SET input_tokens=987 WHERE window_id=$1`, p.window); err != nil {
				t.Fatal(err)
			}
			if p.localModelExecutionError(event, normalized) {
				t.Fatal("forged stored usage admitted")
			}
			if _, err = p.journal.Exec(ctx, `UPDATE public.behavior76_attempts SET input_tokens=$2,model_version='' WHERE window_id=$1`, p.window, in); err != nil {
				t.Fatal(err)
			}
			if p.localModelExecutionError(event, normalized) {
				t.Fatal("unrepresentable empty version admitted")
			}
			if _, err = p.journal.Exec(ctx, `UPDATE public.behavior76_attempts SET model_version=$2 WHERE window_id=$1`, p.window, version); err != nil {
				t.Fatal(err)
			}
			tail, err := f.evaluate(p, episode.Steps[1])
			if err != nil || tail.State != "not_executed_after_fixture_local_response_failure" || tail.OwnerInputID != "" || base.starts != 1 {
				t.Fatal("response tail executed", tail, err)
			}
			if _, err = f.quiesce(ctx); err != nil {
				t.Fatal(err)
			}
			next := newBehavior76ResponseFixture(t, ctx, &global)
			next.seed(episode)
			nextBase := &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
			outcome, err := next.evaluate(behavior76PublicWrapped(t, next, &global, nextBase), episode.Steps[0])
			if err != nil || outcome.State != "completed" || global.Load() {
				t.Fatal("next original arm failed", outcome, err)
			}
			if _, err = next.quiesce(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBehavior76LocalResponseExactStream(t *testing.T) {
	reasons := []string{"stream_json_invalid", "stream_usage_ambiguous", "stream_usage_invalid", "stream_usage_conflict", "stream_model_version_invalid", "stream_candidate_unsupported", "stream_candidate_unexpected", "stream_finish_rejected", "stream_part_invalid", "stream_part_unsupported", "stream_thought_invalid", "stream_function_ambiguous", "stream_content_missing"}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) { behavior76CheckResponseStream(t, reason, "exact", true) })
	}
	for _, reason := range []string{"stream_terminal_missing", "stream_envelope_invalid", "stream_function_truncated", "stream_frame_limit", "stream_automatic_function_execution", "stream_unknown"} {
		t.Run(reason, func(t *testing.T) { behavior76CheckResponseStream(t, reason, "exact", false) })
	}
	for _, fault := range []string{"v9_strict", "wrong_status", "wrong_type", "late", "unclosed", "foreign_run", "empty_version", "invalid_version", "wrong_sequence", "zero_sequence", "high_sequence", "empty_delta", "thought_delta", "extra_usage", "final", "tool", "usage_reason_conflict", "version_reason_conflict", "zero_total_positive_counts"} {
		t.Run(fault, func(t *testing.T) { behavior76CheckResponseStream(t, "stream_finish_rejected", fault, false) })
	}
	for _, mode := range []string{"measured", "zero_usage", "valid_version", "independent_usage", "independent_version", "no_delta"} {
		t.Run(mode, func(t *testing.T) { behavior76CheckResponseStream(t, "stream_finish_rejected", mode, true) })
	}
}

func behavior76CheckResponseStream(t *testing.T, reason, mode string, eligible bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 75*time.Millisecond)
	defer cancel()
	var global atomic.Bool
	scope := &behavior76FixtureScope{localEmpty: true, localHTTP: true, localResponse: mode != "v9_strict"}
	p := &behavior76NativeProvider{scope: scope, sealed: &global}
	events := behavior76ResponsePublicEvents("r", reason)
	last := &events[len(events)-1]
	switch mode {
	case "wrong_status":
		last.TransportError.HTTPStatus = 500
	case "wrong_type":
		last.TransportError.Type = transport.ErrorTypeTransportConnectionError
	case "foreign_run":
		events[1].RunID = "foreign"
	case "empty_version", "invalid_version", "valid_version", "independent_version":
		v := "gemini-public-v1"
		if mode == "empty_version" {
			v = ""
		}
		if mode == "invalid_version" {
			v = "bad"
		}
		last.ProviderModelVersion = &v
		if mode == "independent_version" {
			last.TransportError.SafeReason = "stream_usage_invalid"
		}
	case "wrong_sequence":
		events[2].AssistantDelta.SequenceNo = 2
	case "zero_sequence":
		events[1].AssistantDelta.SequenceNo = 0
	case "high_sequence":
		events[1].AssistantDelta.SequenceNo = 5
	case "empty_delta":
		events[1].AssistantDelta.Content = ""
	case "thought_delta":
		events[1].AssistantDelta.DeltaType = "thought"
	case "extra_usage":
		events[1].Usage = &transport.Usage{}
	case "zero_total_positive_counts":
		last.Usage = &transport.Usage{InputTokens: 5, OutputTokens: 4, TotalTokens: 0}
	case "usage_reason_conflict", "measured", "zero_usage", "independent_usage":
		last.Usage = &transport.Usage{}
		if mode == "measured" || mode == "independent_usage" {
			last.Usage = &transport.Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5}
		}
		if mode == "usage_reason_conflict" {
			last.TransportError.SafeReason = "stream_usage_invalid"
		}
		if mode == "independent_usage" {
			last.TransportError.SafeReason = "stream_model_version_invalid"
		}
	case "version_reason_conflict":
		v := "gemini-public-v1"
		last.ProviderModelVersion = &v
		last.TransportError.SafeReason = "stream_model_version_invalid"
	case "final":
		events[1] = transport.NewAssistantFinalEvent("r", "public", transport.AssistantFinal{Content: "not accepted"})
	case "tool":
		events[1] = transport.NewEvent("r", transport.EventTypeToolCallRequested, "public")
	case "no_delta":
		events = []transport.TransportEvent{events[0], *last}
	}
	if mode == "late" {
		events = append(events, transport.NewEvent("r", transport.EventTypeRunCompleted, "public"))
	}
	stream := make(chan transport.TransportEvent, len(events))
	for _, event := range events {
		stream <- event
	}
	if mode != "unclosed" {
		close(stream)
	}
	called, finished := false, false
	for range p.observeStream(ctx, stream, func(_ string, _ []map[string]any, _ *transport.Usage, _ string) { finished = true }, func(chain []map[string]any, u *transport.Usage, v, run string) bool {
		called = true
		cause, stage, code := p.localFailureClass(chain, u, v)
		return cause == "model_response_rejected" && stage == "response_stream" && code == "provider_protocol_error" && run == "r"
	}) {
	}
	if !finished || global.Load() == eligible || (eligible && !called) {
		t.Fatal("response eligibility/EOF/global mismatch", eligible, called, finished, global.Load())
	}
}

func TestBehavior76LocalResponseStoredShape(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"DeltaType":"text","Content":"a","sequenceno":1}`, `{"DeltaType":"text","Content":null,"SequenceNo":1}`, `{"DeltaType":"text","Content":"a","SequenceNo":1,"extra":0}`} {
		if _, ok := behavior76ExactStoredDelta(json.RawMessage(raw)); ok {
			t.Fatal("malformed stored delta admitted", raw)
		}
	}
	d, ok := behavior76ExactStoredDelta(json.RawMessage(`{"DeltaType":"text","Content":"a","SequenceNo":1}`))
	if !ok || d.Content != "a" {
		t.Fatal("exact delta rejected")
	}
	events := []map[string]any{{"type": transport.EventTypeRunStarted}, {"type": transport.EventTypeAssistantDelta, "text_delta": &transport.AssistantDelta{DeltaType: "text", Content: strings.Repeat("a", 16*1024*1024+1), SequenceNo: 1}}, {"type": transport.EventTypeTransportError, "error_code": "provider_protocol_error", "safe_reason": "stream_finish_rejected", "http_status": 0}}
	if behavior76ExactResponseEvents(events) {
		t.Fatal("oversized partial text admitted")
	}
}

func TestBehavior76LocalResponseJoinsAdmittedAutomaticIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76ResponseFixture(t, ctx, &global)
	e := behavior76DevelopmentEpisode()
	e.Initial.Tasks[0].ExecutionPolicy = "normal"
	other := e.Initial.Tasks[0]
	other.Key = "B"
	e.Initial.Tasks = append(e.Initial.Tasks, other)
	f.seed(e)
	if _, err := f.apply(behavior76Step{ID: "public-http-auto-amend", Event: behavior76Event{Type: "owner_control", Kind: "amend", TaskKey: "A", Instruction: "Continue the bounded read"}}); err != nil {
		t.Fatal(err)
	}
	step := e.Steps[0]
	key := "B"
	step.RunTaskKey = &key
	step.Event.TargetTaskKey = &key
	base := &behavior76PublicConcurrent{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, f: f, entered: make(chan struct{}), responseFailure: true}
	p := behavior76PublicWrapped(t, f, &global, base)
	result, err := f.evaluate(p, step)
	if err != nil || result.State != "fixture_local_response_failure" || global.Load() || !base.finished.Load() || len(result.AutomaticRunIDs) != 1 {
		t.Fatal("response barrier cancelled/lost admitted automatic work", result, err)
	}
	var completed, blocked int
	if err = f.pool.QueryRow(ctx, `SELECT count(*)FILTER(WHERE status='completed'),count(*)FILTER(WHERE status='blocked') FROM accepted_inputs WHERE run_id=ANY($1::text[])`, result.RunIDs).Scan(&completed, &blocked); err != nil || completed != 1 || blocked != 1 {
		t.Fatal("ordinary process workers were not joined", completed, blocked, err)
	}
	var count, measured, nulls int
	if err = p.journal.QueryRow(ctx, `SELECT count(*),count(total_tokens),count(*)FILTER(WHERE total_tokens IS NULL) FROM public.behavior76_attempts WHERE window_id=$1`, p.window).Scan(&count, &measured, &nulls); err != nil || count != 3 || measured != 2 || nulls != 1 {
		t.Fatal("in-flight usage/admission populations changed", count, measured, nulls, err)
	}
	if _, err = f.quiesce(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestBehavior76LocalResponsePersistenceAuthorityIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	for _, fault := range []string{"foreign_journal_identity", "missing_stop_table", "ownership_revoked", "wrong_stage", "wrong_binding"} {
		t.Run(fault, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
			defer cancel()
			var global atomic.Bool
			f := newBehavior76ResponseFixture(t, ctx, &global)
			e := behavior76DevelopmentEpisode()
			f.seed(e)
			base := &behavior76PublicResponseFailure{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
			p := behavior76PublicWrapped(t, f, &global, base)
			var injected error
			base.before = func(c context.Context, run string) {
				switch fault {
				case "foreign_journal_identity":
					_, injected = p.journal.Exec(c, `UPDATE public.behavior76_attempts SET run_id='foreign-http-run' WHERE window_id=$1`, p.window)
				case "missing_stop_table":
					_, injected = f.pool.Exec(c, `DROP TABLE behavior76_fixture_stops`)
				case "ownership_revoked":
					_, injected = f.pool.Exec(c, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE run_id=$1`, run)
				case "wrong_stage":
					transport.ObserveProviderStage(c, transport.ProviderStageAuthentication)
				case "wrong_binding":
					f.scope.mu.Lock()
					f.scope.binding.Mode = "fixture-local-model-v8"
					f.scope.mu.Unlock()
				}
			}
			_, _ = f.evaluate(p, e.Steps[0])
			if injected != nil || !global.Load() || f.scope.localStopped() {
				t.Fatal("response authority/journal/profile fault acquired local exception", injected, global.Load(), f.scope.localStopped())
			}
		})
	}
}

func TestBehavior76LocalResponseAutomaticPreservesOwnerIntegration(t *testing.T) {
	behavior76AutomaticFailurePreservesOwner(t, false, true)
}

func TestBehavior76LocalResponsePriorBranchesIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	for _, branch := range []string{"empty", "http"} {
		t.Run(branch, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
			defer cancel()
			var global atomic.Bool
			f := newBehavior76ResponseFixture(t, ctx, &global)
			e := behavior76DevelopmentEpisode()
			f.seed(e)
			var base transport.ModelProvider
			want, cause := "fixture_local_model_failure", "model_empty_answer"
			if branch == "empty" {
				base = &behavior76PublicEmptyModel{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
			} else {
				base = &behavior76PublicHTTPFailure{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
				want, cause = "fixture_local_http_failure", "model_http_no_response"
			}
			result, err := f.evaluate(behavior76PublicWrapped(t, f, &global, base), e.Steps[0])
			if err != nil || result.State != want || global.Load() {
				t.Fatal("prior exact branch changed", result, err)
			}
			var actual string
			if err = f.pool.QueryRow(ctx, `SELECT cause FROM behavior76_fixture_stops`).Scan(&actual); err != nil || actual != cause {
				t.Fatal("prior cause renamed", actual, err)
			}
			if _, err = f.quiesce(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
