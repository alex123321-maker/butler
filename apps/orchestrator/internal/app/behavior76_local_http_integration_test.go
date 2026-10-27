package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/transport"
)

type behavior76PublicHTTPFailure struct {
	*initiativeProjectScriptedProvider
	before func(context.Context, string)
	starts int
}

func TestBehavior76LocalHTTPAutomaticPreservesOwnerIntegration(t *testing.T) {
	behavior76AutomaticFailurePreservesOwner(t, true)
}

func (p *behavior76PublicHTTPFailure) Name() string { return "antigravity" }
func (p *behavior76PublicHTTPFailure) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	transport.ObserveProviderStage(ctx, transport.ProviderStageHTTP)
	if p.before != nil {
		p.before(ctx, q.Context.RunID)
	}
	e := transport.NewEvent(q.Context.RunID, transport.EventTypeTransportError, p.Name())
	e.TransportError = &transport.Error{Type: transport.ErrorTypeTransportConnectionError, SafeReason: "http_do_failed_without_response"}
	return initiativeProjectStream(transport.NewEvent(q.Context.RunID, transport.EventTypeRunStarted, p.Name()), e), nil
}

func TestBehavior76LocalHTTPIsolationIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76HTTPFixture(t, ctx, &global)
	episode := behavior76DevelopmentEpisode()
	f.seed(episode)
	base := &behavior76PublicHTTPFailure{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
	p := behavior76PublicWrapped(t, f, &global, base)
	result, err := f.evaluate(p, episode.Steps[0])
	if err != nil || result.State != "fixture_local_http_failure" || global.Load() || base.starts != 1 {
		t.Fatal("exact HTTP failure did not isolate original arm", result, err, global.Load())
	}
	var exact bool
	if err = f.pool.QueryRow(ctx, `SELECT cause='model_http_no_response' AND effects='[]'::jsonb AND native_evidence->>'stage'='http_round_trip' AND native_evidence->'input_tokens'='null'::jsonb AND native_evidence->'output_tokens'='null'::jsonb AND native_evidence->'total_tokens'='null'::jsonb AND native_evidence->'thinking_tokens'='null'::jsonb AND native_evidence->'model_version'='null'::jsonb AND native_evidence->>'run_id'=$1 AND native_evidence->'owner_inputs'->0->>'input_id'=$2 AND (native_evidence->>'native_admission_cutoff_n')::int=1 FROM behavior76_fixture_stops`, result.OwnerRunID, result.OwnerInputID).Scan(&exact); err != nil || !exact {
		t.Fatal("exact durable HTTP/null/input proof missing", err)
	}
	tail, err := f.evaluate(p, episode.Steps[1])
	if err != nil || tail.State != "not_executed_after_fixture_local_http_failure" || tail.OwnerInputID != "" || base.starts != 1 {
		t.Fatal("HTTP tail executed", tail, err)
	}
	if _, err = f.quiesce(ctx); err != nil {
		t.Fatal("HTTP arm not quiescent", err)
	}
	next := newBehavior76HTTPFixture(t, ctx, &global)
	next.seed(episode)
	nextBase := &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
	out, err := next.evaluate(behavior76PublicWrapped(t, next, &global, nextBase), episode.Steps[0])
	if err != nil || out.State != "completed" || global.Load() {
		t.Fatal("next original isolated arm failed", out, err)
	}
	if _, err = next.quiesce(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestBehavior76LocalHTTPExactStream(t *testing.T) {
	for _, name := range []string{"exact", "legacy_strict", "old_v8", "wrong_reason", "timeout_type", "auth_reason", "wrong_http", "delta", "late", "unclosed", "usage", "version", "empty_observed_version"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			var global atomic.Bool
			scope := &behavior76FixtureScope{localEmpty: name != "legacy_strict", localHTTP: name != "legacy_strict" && name != "old_v8"}
			p := &behavior76NativeProvider{scope: scope, sealed: &global}
			stream := make(chan transport.TransportEvent, 5)
			start := transport.NewEvent("r", transport.EventTypeRunStarted, "public")
			if name == "empty_observed_version" {
				v := ""
				start.ProviderModelVersion = &v
			}
			stream <- start
			if name == "delta" {
				stream <- transport.NewEvent("r", transport.EventTypeAssistantDelta, "public")
			}
			e := transport.NewEvent("r", transport.EventTypeTransportError, "public")
			e.TransportError = &transport.Error{Type: transport.ErrorTypeTransportConnectionError, SafeReason: "http_do_failed_without_response"}
			switch name {
			case "wrong_reason":
				e.TransportError.SafeReason = ""
			case "timeout_type":
				e.TransportError.Type = transport.ErrorTypeProviderTimeout
			case "auth_reason":
				e.TransportError.SafeReason = "upstream_unauthenticated"
			case "wrong_http":
				e.TransportError.HTTPStatus = 403
			case "usage":
				e.Usage = &transport.Usage{}
			case "version":
				v := "public-version"
				e.ProviderModelVersion = &v
			}
			stream <- e
			if name == "late" {
				stream <- transport.NewEvent("r", transport.EventTypeRunCompleted, "public")
			}
			if name != "unclosed" {
				close(stream)
			}
			called, finished := false, false
			for range p.observeStream(ctx, stream, func(_ string, _ []map[string]any, _ *transport.Usage, _ string) { finished = true }, func(events []map[string]any, u *transport.Usage, v, run string) bool {
				called = true
				return !global.Load() && run == "r" && u == nil && v == "" && behavior76ExactHTTPEvents(events)
			}) {
			}
			if !finished || ((name == "exact") == global.Load()) || (name == "exact" && !called) {
				t.Fatal("HTTP exception widened or EOF/global lost", called, finished, global.Load())
			}
		})
	}
}

func TestBehavior76LocalHTTPJoinsAdmittedAutomaticIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76HTTPFixture(t, ctx, &global)
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
	base := &behavior76PublicConcurrent{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, f: f, entered: make(chan struct{}), httpFailure: true}
	p := behavior76PublicWrapped(t, f, &global, base)
	result, err := f.evaluate(p, step)
	if err != nil || result.State != "fixture_local_http_failure" || global.Load() || !base.finished.Load() || len(result.AutomaticRunIDs) != 1 {
		t.Fatal("HTTP barrier cancelled/lost admitted automatic work", result, err)
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

func TestBehavior76LocalHTTPPersistenceAuthorityIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	for _, fault := range []string{"foreign_journal_identity", "missing_stop_table", "ownership_revoked", "wrong_stage", "wrong_binding"} {
		t.Run(fault, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
			defer cancel()
			var global atomic.Bool
			f := newBehavior76HTTPFixture(t, ctx, &global)
			e := behavior76DevelopmentEpisode()
			f.seed(e)
			base := &behavior76PublicHTTPFailure{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
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
				t.Fatal("HTTP authority/journal/profile fault acquired local exception", injected, global.Load(), f.scope.localStopped())
			}
		})
	}
}
