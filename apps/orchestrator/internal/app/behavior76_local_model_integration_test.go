package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/transport"
)

type behavior76PublicEmptyModel struct {
	*initiativeProjectScriptedProvider
	usage   *transport.Usage
	version *string
	starts  int
	before  func(context.Context, string)
}

func (p *behavior76PublicEmptyModel) Name() string { return "antigravity" }
func (p *behavior76PublicEmptyModel) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	transport.ObserveProviderStage(ctx, transport.ProviderStageStream)
	errEvent := transport.NewEvent(q.Context.RunID, transport.EventTypeTransportError, p.Name())
	errEvent.TransportError = &transport.Error{Type: transport.ErrorTypeProviderProtocolError, SafeReason: "stream_empty_answer"}
	errEvent.Usage, errEvent.ProviderModelVersion = p.usage, p.version
	if p.before != nil {
		p.before(ctx, q.Context.RunID)
	}
	return initiativeProjectStream(transport.NewEvent(q.Context.RunID, transport.EventTypeRunStarted, p.Name()), errEvent), nil
}

func TestBehavior76LocalModelIsolationIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	for _, measured := range []bool{false, true} {
		t.Run(map[bool]string{false: "nullable", true: "measured"}[measured], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			var global atomic.Bool
			f := newBehavior76ModelFixture(t, ctx, &global)
			episode := behavior76DevelopmentEpisode()
			f.seed(episode)
			base := &behavior76PublicEmptyModel{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
			if measured {
				version := "public-version"
				thinking := 3
				base.usage = &transport.Usage{InputTokens: 5, OutputTokens: 4, TotalTokens: 12, ThinkingTokens: &thinking}
				base.version = &version
			}
			p := behavior76PublicWrapped(t, f, &global, base)
			outcome, err := f.evaluate(p, episode.Steps[0])
			if err != nil || outcome.State != "fixture_local_model_failure" || global.Load() || base.starts != 1 {
				t.Fatal("completed empty native response did not stop only its actual fixture", outcome, err, global.Load())
			}
			var cause string
			var exact bool
			if err = f.pool.QueryRow(ctx, `SELECT cause,native_evidence->>'run_id'=$1 AND jsonb_array_length(native_evidence->'owner_inputs')=1 AND (native_evidence->>'native_admission_cutoff_n')::int=1 AND (native_evidence->'total_tokens'='null'::jsonb)=$2 FROM behavior76_fixture_stops`, outcome.RunIDs[0], !measured).Scan(&cause, &exact); err != nil || cause != "model_empty_answer" || !exact {
				t.Fatal("actual run/Input/cutoff/nullable usage evidence missing", cause, exact, err)
			}
			if measured {
				if err = f.pool.QueryRow(ctx, `SELECT (native_evidence->>'input_tokens')::int=5 AND (native_evidence->>'output_tokens')::int=4 AND (native_evidence->>'total_tokens')::int=12 AND (native_evidence->>'thinking_tokens')::int=3 AND native_evidence->>'model_version'='public-version' AND native_evidence->'owner_inputs'->0->>'input_id'=$1 FROM behavior76_fixture_stops`, outcome.OwnerInputID).Scan(&exact); err != nil || !exact {
					t.Fatal("actual nullable metadata changed", err)
				}
			}
			var count int
			if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM accepted_inputs`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			tail, err := f.evaluate(p, episode.Steps[1])
			if err != nil || tail.State != "not_executed_after_fixture_local_model_failure" || tail.OwnerInputID != "" || base.starts != 1 {
				t.Fatal("same arm tail executed", tail, err)
			}
			if _, err = f.scope.begin(ctx, nil); !errors.Is(err, errBehavior76FixtureStopped) {
				t.Fatal("barrier admitted new work", err)
			}
			if _, err = f.quiesce(ctx); err != nil {
				t.Fatal("actual stop failed to quiesce", err)
			}
			next := newBehavior76LocalFixture(t, ctx, &global)
			next.seed(episode)
			nextBase := &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
			nextOutcome, err := next.evaluate(behavior76PublicWrapped(t, next, &global, nextBase), episode.Steps[0])
			if err != nil || nextOutcome.State != "completed" || global.Load() {
				t.Fatal("next isolated arm failed", nextOutcome, err)
			}
			if _, err = next.quiesce(ctx); err != nil {
				t.Fatal(err)
			}
			var after int
			if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM accepted_inputs`).Scan(&after); err != nil || after != count {
				t.Fatal("stopped arm changed", err)
			}
		})
	}
}

func TestBehavior76LocalModelExactStream(t *testing.T) {
	for _, test := range []struct {
		name                         string
		reason                       string
		code                         transport.ErrorType
		status                       int
		prefix, late, close, enabled bool
		local                        bool
	}{
		{"exact", "stream_empty_answer", transport.ErrorTypeProviderProtocolError, 0, true, false, true, true, true},
		{"legacy_strict", "stream_empty_answer", transport.ErrorTypeProviderProtocolError, 0, true, false, true, false, false},
		{"wrong_reason", "stream_terminal_missing", transport.ErrorTypeProviderProtocolError, 0, true, false, true, true, false},
		{"wrong_code", "stream_empty_answer", transport.ErrorTypeProviderUnavailable, 0, true, false, true, true, false},
		{"http", "stream_empty_answer", transport.ErrorTypeProviderProtocolError, 403, true, false, true, true, false},
		{"nonzero_http_masked_by_diagnostic", "stream_empty_answer", transport.ErrorTypeProviderProtocolError, 200, true, false, true, true, false},
		{"missing_start", "stream_empty_answer", transport.ErrorTypeProviderProtocolError, 0, false, false, true, true, false},
		{"late_event", "stream_empty_answer", transport.ErrorTypeProviderProtocolError, 0, true, true, true, true, false},
		{"unclosed", "stream_empty_answer", transport.ErrorTypeProviderProtocolError, 0, true, false, false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			var global atomic.Bool
			scope := &behavior76FixtureScope{localEmpty: test.enabled}
			p := &behavior76NativeProvider{scope: scope, sealed: &global}
			stream := make(chan transport.TransportEvent, 4)
			if test.prefix {
				stream <- transport.NewEvent("r", transport.EventTypeRunStarted, "public")
			}
			e := transport.NewEvent("r", transport.EventTypeTransportError, "public")
			e.TransportError = &transport.Error{Type: test.code, SafeReason: test.reason, HTTPStatus: test.status}
			stream <- e
			if test.late {
				stream <- transport.NewEvent("r", transport.EventTypeRunCompleted, "public")
			}
			if test.close {
				close(stream)
			}
			called, finished := false, false
			observed := p.observeStream(ctx, stream, func(_ string, _ []map[string]any, _ *transport.Usage, _ string) { finished = true }, func(events []map[string]any, _ *transport.Usage, _, run string) bool {
				called = true
				return run == "r" && !global.Load() && behavior76ExactEmptyEvents(events)
			})
			for range observed {
			}
			if !finished || test.local == global.Load() || (test.local && !called) {
				t.Fatal("local exception widened or EOF/global guard lost", called, finished, global.Load())
			}
		})
	}
}

func TestBehavior76LocalModelPersistenceAuthorityIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	for _, fault := range []string{"missing_journal_row", "foreign_journal_identity", "null_event_usage", "aliased_event_usage", "missing_stop_table", "ownership_revoked", "task_revision_changed", "wrong_binding", "wrong_stage"} {
		t.Run(fault, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
			defer cancel()
			var global atomic.Bool
			f := newBehavior76ModelFixture(t, ctx, &global)
			episode := behavior76DevelopmentEpisode()
			f.seed(episode)
			base := &behavior76PublicEmptyModel{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
			p := behavior76PublicWrapped(t, f, &global, base)
			var injected error
			base.before = func(c context.Context, run string) {
				switch fault {
				case "missing_journal_row":
					_, injected = p.journal.Exec(c, `DELETE FROM public.behavior76_attempts WHERE window_id=$1`, p.window)
				case "foreign_journal_identity":
					_, injected = p.journal.Exec(c, `UPDATE public.behavior76_attempts SET run_id='foreign-run' WHERE window_id=$1`, p.window)
				case "null_event_usage", "aliased_event_usage":
					name := "behavior76_corrupt_" + behavior76Hash([]byte(p.window))[:12]
					value := "null"
					if fault == "aliased_event_usage" {
						value = `{"inputtokens":0,"OutputTokens":0,"TotalTokens":0}`
					}
					_, injected = p.journal.Exec(c, `CREATE FUNCTION public.`+name+`() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN NEW.events=jsonb_set(NEW.events,'{1,usage}','`+value+`'::jsonb,true); RETURN NEW; END$$; CREATE TRIGGER `+name+` BEFORE UPDATE ON public.behavior76_attempts FOR EACH ROW EXECUTE FUNCTION public.`+name+`()`)
					t.Cleanup(func() {
						cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
						defer stop()
						if _, err := p.journal.Exec(cleanup, `DROP TRIGGER `+name+` ON public.behavior76_attempts; DROP FUNCTION public.`+name+`()`); err != nil {
							t.Error("public corruption trigger cleanup", err)
						}
					})
				case "missing_stop_table":
					_, injected = f.pool.Exec(c, `DROP TABLE behavior76_fixture_stops`)
				case "ownership_revoked":
					_, injected = f.pool.Exec(c, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE run_id=$1`, run)
				case "task_revision_changed":
					_, injected = f.pool.Exec(c, `UPDATE tasks SET revision=revision+1 WHERE task_id=(SELECT task_id FROM runs WHERE run_id=$1)`, run)
				case "wrong_binding":
					f.scope.mu.Lock()
					f.scope.binding.Window = "foreign-window"
					f.scope.mu.Unlock()
				case "wrong_stage":
					transport.ObserveProviderStage(c, transport.ProviderStageAuthentication)
				}
			}
			_, _ = f.evaluate(p, episode.Steps[0])
			if injected != nil || !global.Load() || f.scope.localStopped() {
				t.Fatal("persistence/authority/stage fault became local", injected, global.Load(), f.scope.localStopped())
			}
		})
	}
}

func TestBehavior76LocalModelExactStoredUsage(t *testing.T) {
	for _, value := range []string{`null`, `{}`, `{"InputTokens":null,"OutputTokens":0,"TotalTokens":0}`, `{"inputtokens":0,"OutputTokens":0,"TotalTokens":0}`, `{"InputTokens":0,"OutputTokens":0,"TotalTokens":0,"extra":0}`, `{"InputTokens":0.5,"OutputTokens":0,"TotalTokens":0}`, `{"InputTokens":1,"OutputTokens":1,"TotalTokens":2,"ThinkingTokens":1}`} {
		if _, ok := behavior76ExactStoredUsage(json.RawMessage(value)); ok {
			t.Fatal("malformed stored usage admitted", value)
		}
	}
	for _, value := range []string{`{"InputTokens":0,"OutputTokens":0,"TotalTokens":0}`, `{"InputTokens":5,"OutputTokens":4,"TotalTokens":12,"ThinkingTokens":3}`} {
		if _, ok := behavior76ExactStoredUsage(json.RawMessage(value)); !ok {
			t.Fatal("exact actual usage rejected", value)
		}
	}
}

type behavior76PublicAutomaticEmpty struct {
	*initiativeProjectScriptedProvider
	f               *behavior76Fixture
	entered         chan struct{}
	httpFailure     bool
	responseFailure bool
}

func (p *behavior76PublicAutomaticEmpty) Name() string { return "antigravity" }
func (p *behavior76PublicAutomaticEmpty) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	var task string
	if err := p.f.pool.QueryRow(ctx, `SELECT task_id FROM runs WHERE run_id=$1`, q.Context.RunID).Scan(&task); err != nil {
		return nil, err
	}
	if task == p.f.lookup("task", "A") {
		close(p.entered)
		for {
			var completed bool
			if err := p.f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs WHERE task_id=$1 AND current_state='completed')`, p.f.lookup("task", "B")).Scan(&completed); err != nil {
				return nil, err
			}
			if completed {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
		}
		if p.httpFailure {
			return (&behavior76PublicHTTPFailure{initiativeProjectScriptedProvider: p.initiativeProjectScriptedProvider}).StartRun(ctx, q)
		}
		if p.responseFailure {
			return (&behavior76PublicResponseFailure{initiativeProjectScriptedProvider: p.initiativeProjectScriptedProvider}).StartRun(ctx, q)
		}
		base := &behavior76PublicEmptyModel{initiativeProjectScriptedProvider: p.initiativeProjectScriptedProvider}
		return base.StartRun(ctx, q)
	}
	select {
	case <-p.entered:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return initiativeProjectStream(transport.NewAssistantFinalEvent(q.Context.RunID, p.Name(), transport.AssistantFinal{Content: `{"answer":"The actual owner answer remains available.","task_key":"B","next_action":{"kind":"answer","resource_key":null,"expected_version":null,"body":null,"reason":"answered"},"needs_clarification":false,"used_source_refs":[],"proposed_profile_update":{"kind":"none","source_refs":[],"summary":""},"completion_claim":"not_complete"}`, Usage: &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}})), nil
}

func TestBehavior76LocalModelAutomaticPreservesOwnerIntegration(t *testing.T) {
	behavior76AutomaticFailurePreservesOwner(t, false)
}

func behavior76AutomaticFailurePreservesOwner(t *testing.T, httpFailure bool, response ...bool) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	var global atomic.Bool
	var f *behavior76Fixture
	responseFailure := len(response) == 1 && response[0]
	if responseFailure {
		f = newBehavior76ResponseFixture(t, ctx, &global)
	} else if httpFailure {
		f = newBehavior76HTTPFixture(t, ctx, &global)
	} else {
		f = newBehavior76ModelFixture(t, ctx, &global)
	}
	e := behavior76DevelopmentEpisode()
	e.Initial.Tasks[0].ExecutionPolicy = "normal"
	other := e.Initial.Tasks[0]
	other.Key = "B"
	e.Initial.Tasks = append(e.Initial.Tasks, other)
	f.seed(e)
	if _, err := f.apply(behavior76Step{ID: "public-model-auto-amend", Event: behavior76Event{Type: "owner_control", Kind: "amend", TaskKey: "A", Instruction: "Continue the bounded synthetic task"}}); err != nil {
		t.Fatal(err)
	}
	step := e.Steps[0]
	key := "B"
	step.RunTaskKey, step.Event.TargetTaskKey = &key, &key
	base := &behavior76PublicAutomaticEmpty{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, f: f, entered: make(chan struct{}), httpFailure: httpFailure, responseFailure: responseFailure}
	p := behavior76PublicWrapped(t, f, &global, base)
	result, err := f.evaluate(p, step)
	want, code := "fixture_local_model_failure", "provider_protocol_error"
	if httpFailure {
		want, code = "fixture_local_http_failure", "transport_connection_error"
	}
	if responseFailure {
		want = "fixture_local_response_failure"
	}
	if err != nil || result.State != want || global.Load() || len(result.AutomaticRunIDs) != 1 || result.OwnerRunID == "" {
		t.Fatal("automatic failure lost original identities", result, err)
	}
	var ownerCompleted, automaticFailed bool
	if err = f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs WHERE input_id=$1 AND status='completed'),EXISTS(SELECT 1 FROM accepted_inputs a JOIN runs r ON r.run_id=a.run_id WHERE a.run_id=$2 AND a.status='blocked' AND a.error_code=$3 AND r.current_state='failed')`, result.OwnerInputID, result.AutomaticRunIDs[0], code).Scan(&ownerCompleted, &automaticFailed); err != nil || !ownerCompleted || !automaticFailed {
		t.Fatal("automatic empty rewrote owner final or original blocked Input/failed run", ownerCompleted, automaticFailed, err)
	}
	var failedRun string
	if err = f.pool.QueryRow(ctx, `SELECT native_evidence->>'run_id' FROM behavior76_fixture_stops`).Scan(&failedRun); err != nil || failedRun != result.AutomaticRunIDs[0] || failedRun == result.OwnerRunID {
		t.Fatal("automatic failure mislabelled as owner", err)
	}
	if _, err = f.quiesce(ctx); err != nil {
		t.Fatal(err)
	}
}
