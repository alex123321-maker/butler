package app

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func behavior76AuthorityCode(code string) bool {
	switch code {
	case "execution_authority_check_failed", "execution_ownership_changed", "execution_authority_observation_timeout", "execution_lease_renewal_failed":
		return true
	}
	return false
}

func behavior76AuthorityFailure(err error) bool {
	return err != nil && behavior76AuthorityCode(transport.DiagnosticCode(err))
}

// This is a research-wide conservative stop. It grants no authority, resolves
// no uncertain effect and never resumes/rewrites the ordinary blocked Input.
func (p *behavior76NativeProvider) stopOnUnknownEffect() bool {
	if p.scope != nil {
		return p.scope.observe() != nil
	}
	if p.fixture == nil || p.fixture.pool == nil {
		p.sealed.Store(true)
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var unknown bool
	err := p.fixture.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effects WHERE state='effect_unknown')`).Scan(&unknown)
	if err != nil || unknown {
		p.sealed.Store(true)
		return true
	}
	return false
}

func (p *behavior76NativeProvider) observeCoreOutcome(result *flow.ExecutionResult, err error) {
	if behavior76AuthorityFailure(err) || (result != nil && behavior76AuthorityCode(result.ErrorCode)) {
		p.sealed.Store(true)
	}
	p.stopOnUnknownEffect()
}

func (p *behavior76NativeProvider) CancelRun(ctx context.Context, request transport.CancelRunRequest) (*transport.TransportEvent, error) {
	// This fixed reason comes from the owning Core watchdog, not model text.
	// Seal synchronously before forwarding provider cancellation.
	if request.Reason == "owner authority revoked or unavailable" || behavior76AuthorityCode(request.Reason) {
		p.sealed.Store(true)
	}
	return p.ModelProvider.CancelRun(ctx, request)
}

type behavior76PublicCancelProbe struct {
	*initiativeProjectScriptedProvider
	sealed *atomic.Bool
	calls  int
}

func (p *behavior76PublicCancelProbe) CancelRun(_ context.Context, request transport.CancelRunRequest) (*transport.TransportEvent, error) {
	p.calls++
	if request.Reason == "owner authority revoked or unavailable" && !p.sealed.Load() {
		return nil, errBehavior76Native
	}
	return &transport.TransportEvent{}, nil
}

func TestBehavior76GlobalStopCancellation(t *testing.T) {
	for _, reason := range []string{"owner authority revoked or unavailable", "execution_lease_renewal_failed", "model_call_limit", "durable_context_recovery"} {
		t.Run(reason, func(t *testing.T) {
			var sealed atomic.Bool
			base := &behavior76PublicCancelProbe{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, sealed: &sealed}
			provider := &behavior76NativeProvider{ModelProvider: base, sealed: &sealed}
			_, err := provider.CancelRun(t.Context(), transport.CancelRunRequest{RunID: "public-run", Reason: reason})
			want := reason == "owner authority revoked or unavailable" || reason == "execution_lease_renewal_failed"
			if err != nil || base.calls != 1 || sealed.Load() != want {
				t.Fatal("owning cancellation did not seal before forwarding, or ordinary recovery acquired a false authority failure", err)
			}
		})
	}
}

func TestBehavior76GlobalStopIncompleteStream(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed_without_terminal", true: "completed"}[complete], func(t *testing.T) {
			var sealed atomic.Bool
			provider := &behavior76NativeProvider{sealed: &sealed}
			stream := make(chan transport.TransportEvent, 2)
			if complete {
				stream <- transport.NewAssistantFinalEvent("public-run", "public-provider", transport.AssistantFinal{Content: "public response", Usage: &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}})
				stream <- transport.NewTerminalEvent("public-run", transport.EventTypeRunCompleted, "public-provider")
			}
			close(stream)
			var status string
			var usage *transport.Usage
			output := provider.observeStream(t.Context(), stream, func(state string, _ []map[string]any, observed *transport.Usage, _ string) {
				status, usage = state, observed
			})
			for range output {
			}
			if complete {
				if sealed.Load() || status != "completed" || usage == nil || usage.TotalTokens != 2 {
					t.Fatal("complete measured stream was changed")
				}
			} else {
				if !sealed.Load() || status != "stream_closed" || usage != nil {
					t.Fatal("incomplete stream did not seal with NULL usage")
				}
				called := false
				_, err := provider.invoke(t.Context(), "start", "next-public-run", nil, func(context.Context) (transport.EventStream, error) { called = true; return nil, nil })
				if err == nil || called {
					t.Fatal("sealed window admitted the next model operation")
				}
			}
		})
	}
}

func TestBehavior76GlobalStopMalformedStreams(t *testing.T) {
	for name, event := range map[string]transport.TransportEvent{
		"started_only":           {EventType: transport.EventTypeRunStarted},
		"terminal_without_final": {EventType: transport.EventTypeRunCompleted},
		"cancelled":              {EventType: transport.EventTypeRunCancelled},
		"timed_out":              {EventType: transport.EventTypeRunTimedOut},
		"missing_tool_payload":   {EventType: transport.EventTypeToolCallRequested},
		"missing_tool_ref":       transport.NewToolCallRequestedEvent("public", "public", transport.ToolCallRequest{ToolName: "http.request", ArgsJSON: `{}`}),
		"invalid_tool_json":      transport.NewToolCallRequestedEvent("public", "public", transport.ToolCallRequest{ToolCallRef: "ref", ToolName: "http.request", ArgsJSON: `{`}),
		"unsupported_batch":      transport.NewToolCallBatchRequestedEvent("public", "public", transport.ToolCallBatch{BatchRef: "batch", ToolCalls: []transport.ToolCallRequest{{ToolCallRef: "ref", ToolName: "http.request", ArgsJSON: `{}`}}}),
		"provider_error":         transport.NewTransportErrorEvent("public", "public", errBehavior76Native),
	} {
		t.Run(name, func(t *testing.T) {
			var sealed atomic.Bool
			p := &behavior76NativeProvider{sealed: &sealed}
			var usage *transport.Usage
			var status string
			for range p.observeStream(t.Context(), initiativeProjectStream(event), func(s string, _ []map[string]any, u *transport.Usage, _ string) { status, usage = s, u }) {
			}
			if !sealed.Load() || usage != nil || status == "completed" || status == "tool_handoff" {
				t.Fatal("malformed or unsupported stream acquired successful handoff/final or invented usage", status)
			}
		})
	}
}

type behavior76PublicForeignRead struct {
	*behavior76Scripted
	observer *behavior76NativeProvider
}

func (p *behavior76PublicForeignRead) StartRun(_ context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	p.run = request.Context.RunID
	return p.proposal(p.run, "public-foreign-read", "http.request", map[string]any{"method": "GET", "url": "https://fixture.invalid/objects/foreign"}), nil
}

func (p *behavior76PublicForeignRead) observeCoreOutcome(result *flow.ExecutionResult, err error) {
	p.observer.observeCoreOutcome(result, err)
}

func TestBehavior76UnknownEffectStopsBeforeNextAdmissionIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	f := newBehavior76Fixture(t, ctx)
	episode := behavior76DevelopmentEpisode()
	episode.Initial.Tasks[0].ExecutionPolicy = "normal"
	other := episode.Initial.Tasks[0]
	other.Key = "B"
	episode.Initial.Tasks = append(episode.Initial.Tasks, other)
	episode.Initial.Resources = append(episode.Initial.Resources, behavior76Resource{Key: "foreign", TaskKey: "B", Body: "private other-task synthetic bytes", Version: 1})
	f.seed(episode)
	var sealed atomic.Bool
	observer := &behavior76NativeProvider{fixture: f, sealed: &sealed}
	provider := &behavior76PublicForeignRead{behavior76Scripted: &behavior76Scripted{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}, observer: observer}
	outcome, err := f.evaluate(provider, episode.Steps[0])
	if err != nil || outcome.State != "failed" || !sealed.Load() {
		t.Fatal("actual unknown Broker outcome did not stop research admission", err)
	}
	var unknown, events int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM action_effects WHERE state='effect_unknown'`).Scan(&unknown); err != nil || unknown != 1 {
		t.Fatal("actual failed-read UNKNOWN was not preserved", err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM behavior76_object_events`).Scan(&events); err != nil || events != 0 {
		t.Fatal("foreign read mutated a synthetic resource", err)
	}
	called := false
	_, err = observer.invoke(t.Context(), "start", "never-admitted-public-run", nil, func(context.Context) (transport.EventStream, error) { called = true; return nil, nil })
	if err == nil || called {
		t.Fatal("UNKNOWN allowed a following native admission")
	}
	t.Log("real PG/Core/Broker rejection -> original UNKNOWN -> shared stop -> no following model callback; native/auth=0")
}

func TestBehavior76GlobalStopObserverPreservesWaitingIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	f := newBehavior76Fixture(t, ctx)
	episode := behavior76DevelopmentEpisode()
	f.seed(episode)
	firstStep := episode.Steps[0]
	firstStep.RunTaskKey = nil
	firstStep.Event.Intent = "task_message"
	const sessionKey = "behavior76:conversation"
	if _, _, err := session.NewPostgresRepository(f.pool).CreateSession(f.ctx, session.CreateSessionParams{SessionKey: sessionKey, UserID: behavior76Owner, Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	store := input.NewStore(f.pool)
	event := ingress.InputEvent{EventID: "behavior76:" + firstStep.ID, IdempotencyKey: "behavior76:" + firstStep.ID, SessionKey: sessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: behavior76JSON(map[string]any{"text": firstStep.Event.Text, "user_id": behavior76Owner}), CreatedAt: time.Now().UTC()}
	// Use the owning admission API, exactly as unresolved target routing does.
	// No UPDATE, fake approval, expiry or completion is used to create this wait.
	initial, created, err := store.AcceptWaiting(f.ctx, event, behavior76Owner)
	if err != nil || !created || initial.Status != "waiting_for_user" {
		t.Fatal("actual target-required waiting admission unavailable", err)
	}
	provider := &behavior76SetupProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, fixture: f}
	first, err := f.evaluate(provider, firstStep)
	if err != nil || first.State != "waiting_for_user" || first.OwnerInputID != initial.InputID || first.OwnerRunID != "" || len(provider.requests) != 0 {
		t.Fatal("waiting observation hung, executed a model or invented a final", err)
	}
	secondStep := episode.Steps[1]
	secondStep.RunTaskKey = nil
	secondStep.Event.Intent = "task_message"
	second, err := f.evaluate(provider, secondStep)
	if err != nil || second.State != "queued_after_stopped_input" || second.BlockingInputID != initial.InputID || second.OwnerRunID != "" || len(provider.requests) != 0 {
		t.Fatal("fenced FIFO observation hung or bypassed its predecessor", err)
	}
	retained, err := store.Get(f.ctx, initial.InputID)
	if err != nil || retained.Status != initial.Status || retained.ErrorCode != initial.ErrorCode || retained.Event.PayloadJSON != initial.Event.PayloadJSON {
		t.Fatal("observer modified the original unresolved input", err)
	}
	queued, err := store.Get(f.ctx, second.OwnerInputID)
	if err != nil || queued.Status != "queued" || queued.RunID != "" {
		t.Fatal("observer falsely ran or completed a queued successor", err)
	}
	t.Log("real waiting and queued FIFO successor retained unchanged; no model, approval, replay or timeout extension")
}

type behavior76PublicHandoff struct {
	*initiativeProjectScriptedProvider
	run, call       string
	resource        string
	starts, submits int
}

func (p *behavior76PublicHandoff) Name() string { return antigravity.Name }
func (p *behavior76PublicHandoff) StartRun(ctx context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	p.run, p.call = request.Context.RunID, "public-handoff-"+request.Context.RunID
	capabilities, err := p.Capabilities(ctx, request.Context)
	if err != nil {
		return nil, err
	}
	resource := p.resource
	if resource == "" {
		resource = "draft"
	}
	proposal := transport.NewToolCallRequestedEvent(p.run, p.Name(), transport.ToolCallRequest{ToolCallRef: p.call, ToolName: "http.request", ArgsJSON: behavior76JSON(map[string]any{"method": "GET", "url": "https://fixture.invalid/objects/" + resource})})
	proposal.Usage = &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}
	return initiativeProjectStream(transport.NewRunStartedEvent(p.run, p.Name(), capabilities, nil), proposal), nil
}

func (p *behavior76PublicHandoff) SubmitToolResult(_ context.Context, request transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if request.RunID != p.run || request.ToolCallRef != p.call || p.submits != 0 {
		return nil, errBehavior76Native
	}
	p.submits++
	return initiativeProjectStream(transport.NewAssistantFinalEvent(p.run, p.Name(), transport.AssistantFinal{Content: `{"answer":"Observed the public synthetic draft.","task_key":"A","next_action":{"kind":"answer","resource_key":null,"expected_version":null,"body":null,"reason":"read only"},"needs_clarification":false,"used_source_refs":[],"proposed_profile_update":{"kind":"none","source_refs":[],"summary":""},"completion_claim":"not_complete"}`, Usage: &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}})), nil
}

func TestBehavior76GlobalStopAllowsCorrelatedHandoffIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	f := newBehavior76Fixture(t, ctx)
	episode := behavior76DevelopmentEpisode()
	f.seed(episode)
	config, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil || config.ConnConfig.Host != "127.0.0.1" {
		t.Fatal("isolated local control database required")
	}
	// Fake accounting lives in a different database. Original A/B/C/fresh rows
	// are never extended, overwritten or refunded by this zero-native regression.
	config.ConnConfig.Database = "behavior76_public_safety_test"
	config.ConnConfig.RuntimeParams["search_path"] = "public"
	config.MaxConns, config.MinConns = 2, 0
	journal, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(journal.Close)
	if _, err = journal.Exec(f.ctx, behavior76JournalDDL); err != nil {
		t.Fatal(err)
	}
	window := "public-handoff-" + f.lookup("task", "A")
	if _, err = journal.Exec(f.ctx, `INSERT INTO public.behavior76_windows(id,manifest,expires_at) VALUES($1,'{"max_native_entries":192,"native":false}',clock_timestamp()+INTERVAL '1 minute')`, window); err != nil {
		t.Fatal(err)
	}
	var sealed atomic.Bool
	base := &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
	provider := &behavior76NativeProvider{ModelProvider: base, fixture: f, journal: journal, window: window, slot: "public-handoff", sealed: &sealed, entryLimit: 192}
	outcome, err := f.evaluate(provider, episode.Steps[0])
	if err != nil || outcome.State != "completed" || sealed.Load() || base.starts != 1 || base.submits != 1 {
		t.Fatal("normalized tool handoff was rejected or did not submit its exact result once", err)
	}
	var admissions, handoffs, finals int
	if err = journal.QueryRow(f.ctx, `SELECT count(*),count(*) FILTER(WHERE status='tool_handoff'),count(*) FILTER(WHERE status='completed') FROM public.behavior76_attempts WHERE window_id=$1`, window).Scan(&admissions, &handoffs, &finals); err != nil || admissions != 2 || handoffs != 1 || finals != 1 {
		t.Fatal("handoff/final accounting was conflated", admissions, handoffs, finals, err)
	}
	t.Log("real Core/Broker GET and wrapped normalized handoff -> exactly one correlated SubmitToolResult -> final-only completion; native/auth=0")
}
