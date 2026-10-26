package app

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

func behavior76PublicWrapped(t *testing.T, f *behavior76Fixture, global *atomic.Bool, base transport.ModelProvider) *behavior76NativeProvider {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil || cfg.ConnConfig.Host != "127.0.0.1" {
		t.Fatal("isolated local public test database required")
	}
	cfg.ConnConfig.Database = "behavior76_public_safety_test"
	cfg.ConnConfig.RuntimeParams["search_path"] = "public"
	cfg.MaxConns, cfg.MinConns = 2, 0
	journal, err := pgxpool.NewWithConfig(f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(journal.Close)
	if _, err = journal.Exec(f.ctx, behavior76JournalDDL); err != nil {
		t.Fatal(err)
	}
	window := "public-scope-" + f.lookup("task", "A")
	if f.scope.localEmpty {
		f.scope.binding.Window = window
		f.scope.binding.Arm = "public-scope"
	}
	if _, err = journal.Exec(f.ctx, `INSERT INTO public.behavior76_windows(id,manifest,expires_at) VALUES($1,'{"max_native_entries":192,"native":false}',clock_timestamp()+INTERVAL '2 minutes')`, window); err != nil {
		t.Fatal(err)
	}
	return &behavior76NativeProvider{ModelProvider: base, fixture: f, journal: journal, window: window, slot: "public-scope", sealed: global, entryLimit: 192, scope: f.scope}
}

func TestBehavior76LocalUnknownIsolationIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76LocalFixture(t, ctx, &global)
	episode := behavior76DevelopmentEpisode()
	episode.Initial.Tasks[0].ExecutionPolicy = "normal"
	other := episode.Initial.Tasks[0]
	other.Key = "B"
	episode.Initial.Tasks = append(episode.Initial.Tasks, other)
	episode.Initial.Resources = append(episode.Initial.Resources, behavior76Resource{Key: "foreign", TaskKey: "B", Body: "other-task synthetic object", Version: 1})
	f.seed(episode)
	base := &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, resource: "foreign"}
	provider := behavior76PublicWrapped(t, f, &global, base)
	first, err := f.evaluate(provider, episode.Steps[0])
	if err != nil || first.State != "fixture_local_unknown" || global.Load() || !f.scope.localStopped() || base.starts != 1 || base.submits != 0 {
		t.Fatal("actual foreign GET did not stop exactly this fixture", first.State, err)
	}
	var unknown, stops, diagnostics, inputs int
	if err = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM action_effects WHERE state='effect_unknown'),(SELECT count(*) FROM behavior76_fixture_stops),(SELECT count(*) FROM behavior76_recipient_diagnostics WHERE category='foreign_task'),(SELECT count(*) FROM accepted_inputs)`).Scan(&unknown, &stops, &diagnostics, &inputs); err != nil || unknown != 1 || stops != 1 || diagnostics != 1 || inputs != 1 {
		t.Fatal("original UNKNOWN/diagnostic/local-stop/input identities absent", unknown, stops, diagnostics, inputs, err)
	}
	called := false
	_, err = provider.invoke(ctx, "start", "same-arm-never-admitted", nil, func(context.Context) (transport.EventStream, error) { called = true; return nil, nil })
	if err == nil || called || global.Load() {
		t.Fatal("same-arm native callback admitted or local stop became global")
	}
	tail, err := f.evaluate(provider, episode.Steps[1])
	if err != nil || tail.State != "not_executed_after_fixture_local_unknown" || tail.OwnerInputID != "" {
		t.Fatal("tail was executed or acquired invented input", err)
	}
	receipt, err := f.quiesce(ctx)
	if err != nil || !receipt.BrokerReaped || !receipt.RuntimeStopped || !receipt.FixtureLocallyStopped {
		t.Fatal("fixture was not independently quiescent", receipt, err)
	}
	// The next original scheduled fixture is wholly separate, not a reseed or
	// continuation of the stopped schema/session/recipient/authority.
	next := newBehavior76LocalFixture(t, ctx, &global)
	nextEpisode := behavior76DevelopmentEpisode()
	next.seed(nextEpisode)
	nextBase := &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}
	nextProvider := behavior76PublicWrapped(t, next, &global, nextBase)
	completed, err := next.evaluate(nextProvider, nextEpisode.Steps[0])
	if err != nil || completed.State != "completed" || global.Load() || nextBase.starts != 1 || nextBase.submits != 1 {
		t.Fatal("next isolated actual Core/Broker fixture did not complete", err)
	}
	nextReceipt, err := next.quiesce(ctx)
	if err != nil || receipt.Schema == nextReceipt.Schema {
		t.Fatal("next fixture isolation/quiescence missing", err)
	}
	var afterInputs, afterUnknown int
	if err = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM accepted_inputs),(SELECT count(*) FROM action_effects WHERE state='effect_unknown')`).Scan(&afterInputs, &afterUnknown); err != nil || afterInputs != inputs || afterUnknown != unknown {
		t.Fatal("stopped fixture changed during next fixture", err)
	}
	t.Log("real Core/Broker foreign GET -> original UNKNOWN -> durable local barrier -> no same-arm admission/input -> reaped Broker/stopped recipient/no live claim or lease -> next isolated measured fake handoff/final; native/auth=0")
}

type behavior76PublicConcurrent struct {
	*initiativeProjectScriptedProvider
	f           *behavior76Fixture
	entered     chan struct{}
	finished    atomic.Bool
	runtimeRace bool
}

func (p *behavior76PublicConcurrent) Name() string { return "antigravity" }
func (p *behavior76PublicConcurrent) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	var key string
	if err := p.f.pool.QueryRow(ctx, `SELECT task_id FROM runs WHERE run_id=$1`, q.Context.RunID).Scan(&key); err != nil {
		return nil, err
	}
	ownerB := key == p.f.lookup("task", "B")
	if ownerB {
		select {
		case <-p.entered:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	capabilities, err := p.Capabilities(ctx, q.Context)
	if err != nil {
		return nil, err
	}
	proposal := transport.NewToolCallRequestedEvent(q.Context.RunID, p.Name(), transport.ToolCallRequest{ToolCallRef: "concurrent-" + q.Context.RunID, ToolName: "http.request", ArgsJSON: `{"method":"GET","url":"https://fixture.invalid/objects/draft"}`})
	if p.runtimeRace {
		if ownerB {
			proposal.ToolCall.ArgsJSON = `{"method":"GET","url":"https://fixture.invalid/objects/foreign"}`
		} else {
			proposal.ToolCall.ArgsJSON = behavior76JSON(map[string]any{"method": "POST", "url": "https://fixture.invalid/objects/draft", "body": behavior76JSON(behavior76Patch{ExpectedVersion: 1, ExpectedSHA256: behavior76Hash([]byte("original")), Body: "admitted before barrier"})})
		}
	}
	proposal = transport.NewToolCallRequestedEvent(q.Context.RunID, p.Name(), *proposal.ToolCall)
	proposal.Usage = &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}
	return initiativeProjectStream(transport.NewRunStartedEvent(q.Context.RunID, p.Name(), capabilities, nil), proposal), nil
}
func (p *behavior76PublicConcurrent) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if p.runtimeRace {
		return nil, errBehavior76Native
	}
	if q.ToolCallRef != "concurrent-"+q.RunID {
		return nil, errBehavior76Native
	}
	close(p.entered)
	for !p.f.scope.localStopped() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	// This operation was admitted BEFORE the local barrier. Preserve its real
	// completion and usage; never cancel/retry or admit a same-arm continuation.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(150 * time.Millisecond):
	}
	p.finished.Store(true)
	return initiativeProjectStream(transport.NewAssistantFinalEvent(q.RunID, p.Name(), transport.AssistantFinal{Content: `{"answer":"Observed synthetic draft before the other run stopped.","task_key":"A","next_action":{"kind":"answer","resource_key":null,"expected_version":null,"body":null,"reason":"read only"},"needs_clarification":false,"used_source_refs":[],"proposed_profile_update":{"kind":"none","source_refs":[],"summary":""},"completion_claim":"not_complete"}`, Usage: &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}})), nil
}

func TestBehavior76LocalUnknownJoinsAutomaticIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76LocalFixture(t, ctx, &global)
	e := behavior76DevelopmentEpisode()
	e.Initial.Tasks[0].ExecutionPolicy = "normal"
	other := e.Initial.Tasks[0]
	other.Key = "B"
	e.Initial.Tasks = append(e.Initial.Tasks, other)
	e.Initial.Resources = append(e.Initial.Resources, behavior76Resource{Key: "second", TaskKey: "B", Body: "second original", Version: 1})
	f.seed(e)
	if _, err := f.apply(behavior76Step{ID: "public-local-auto-amend", Event: behavior76Event{Type: "owner_control", Kind: "amend", TaskKey: "A", Instruction: "Continue the original bounded read"}}); err != nil {
		t.Fatal(err)
	}
	step := e.Steps[0]
	key := "B"
	step.RunTaskKey = &key
	step.Event.TargetTaskKey = &key
	base := &behavior76PublicConcurrent{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, f: f, entered: make(chan struct{})}
	provider := behavior76PublicWrapped(t, f, &global, base)
	result, err := f.evaluate(provider, step)
	if err != nil || result.State != "fixture_local_unknown" || global.Load() || !base.finished.Load() || len(result.AutomaticRunIDs) != 1 {
		t.Fatal("local stop cancelled/lost an already admitted automatic operation", result.State, err)
	}
	var completed, blocked, measured int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='completed'),count(*) FILTER(WHERE status='blocked') FROM accepted_inputs WHERE run_id=ANY($1::text[])`, result.RunIDs).Scan(&completed, &blocked); err != nil || completed != 1 || blocked != 1 {
		t.Fatal("owning worker results did not finish before drain", completed, blocked, err)
	}
	if err = provider.journal.QueryRow(ctx, `SELECT count(*) FROM public.behavior76_attempts WHERE window_id=$1 AND total_tokens=2 AND status IN ('completed','tool_handoff')`, provider.window).Scan(&measured); err != nil || measured != 3 {
		t.Fatal("in-flight usage or exact admission count lost", measured, err)
	}
	receipt, err := f.quiesce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("automatic operation admitted before barrier completed once under original context; owner UNKNOWN blocked; both claim/process workers joined; no late recipient or model admission; native/auth=0", behavior76JSON(receipt))
}

func TestBehavior76LocalUnknownGlobalPriorityIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76LocalFixture(t, ctx, &global)
	e := behavior76DevelopmentEpisode()
	e.Initial.Tasks[0].ExecutionPolicy = "normal"
	other := e.Initial.Tasks[0]
	other.Key = "B"
	e.Initial.Tasks = append(e.Initial.Tasks, other)
	e.Initial.Resources = append(e.Initial.Resources, behavior76Resource{Key: "foreign", TaskKey: "B", Body: "foreign", Version: 1})
	f.seed(e)
	p := behavior76PublicWrapped(t, f, &global, &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, resource: "foreign"})
	if result, err := f.evaluate(p, e.Steps[0]); err != nil || result.State != "fixture_local_unknown" {
		t.Fatal(err)
	}
	// Corruption after the first stop must still be seen by final reconciliation;
	// matching action_id/category alone cannot qualify foreign run/tool identities.
	if _, err := f.pool.Exec(ctx, `UPDATE behavior76_recipient_diagnostics SET run_id='wrong-run'`); err != nil {
		t.Fatal(err)
	}
	if err := f.scope.observe(); err != errBehavior76Native || !global.Load() {
		t.Fatal("late mismatched diagnostic was hidden by local/closed shortcut", err)
	}
	if _, err := f.quiesce(ctx); err == nil {
		t.Fatal("globally failed fixture produced next-fixture quiescence permission")
	}
	t.Log("late diagnostic identity corruption overrides an earlier local UNKNOWN and denies next fixture; native/auth=0")
}

func TestBehavior76LocalUnknownGlobalDuringReservationIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76LocalFixture(t, ctx, &global)
	reserved := false
	release, err := f.scope.begin(ctx, func() error {
		_, err := f.pool.Exec(ctx, `INSERT INTO behavior76_observations(observation) VALUES('{"public_reservation":true}')`)
		reserved = err == nil
		global.Store(true)
		return err
	})
	if release != nil {
		release()
	}
	if err == nil || !reserved || !global.Load() {
		t.Fatal("global failure during durable reserve released a following callback", err)
	}
	f.scope.mu.Lock()
	active := f.scope.active
	f.scope.mu.Unlock()
	if active != 0 {
		t.Fatal("denied reservation acquired active provider admission")
	}
	t.Log("durable public reservation retained, global stop during reserve prevents callback release; native/auth=0")
}

type behavior76PublicWrite struct {
	*behavior76PublicHandoff
	version int64
}

func (p *behavior76PublicWrite) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.starts++
	p.run = q.Context.RunID
	p.call = "public-write-" + p.run
	capabilities, err := p.Capabilities(ctx, q.Context)
	if err != nil {
		return nil, err
	}
	proposal := transport.NewToolCallRequestedEvent(p.run, p.Name(), transport.ToolCallRequest{ToolCallRef: p.call, ToolName: "http.request", ArgsJSON: behavior76JSON(map[string]any{"method": "POST", "url": "https://fixture.invalid/objects/draft", "body": behavior76JSON(behavior76Patch{ExpectedVersion: p.version, ExpectedSHA256: behavior76Hash([]byte("original")), Body: "public committed bytes"})})})
	proposal.Usage = &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}
	return initiativeProjectStream(transport.NewRunStartedEvent(p.run, p.Name(), capabilities, nil), proposal), nil
}

func TestBehavior76LocalUnknownRecipientMatrixIntegration(t *testing.T) {
	for _, variant := range []string{"confirmed", "stale_cas", "lost_ack"} {
		t.Run(variant, func(t *testing.T) {
			t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
			t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			var global atomic.Bool
			f := newBehavior76LocalFixture(t, ctx, &global)
			e := behavior76DevelopmentEpisode()
			e.Initial.Tasks[0].ExecutionPolicy = "normal"
			f.seed(e)
			base := &behavior76PublicWrite{behavior76PublicHandoff: &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}, version: 1}
			if variant == "stale_cas" {
				base.version = 2
			}
			if variant == "lost_ack" {
				f.recipient.loseNextWriteAck.Store(true)
			}
			provider := behavior76PublicWrapped(t, f, &global, base)
			result, err := f.evaluate(provider, e.Steps[0])
			if err != nil || global.Load() {
				t.Fatal("recipient matrix acquired infrastructure error", err)
			}
			object, err := behavior76ObjectQuery(ctx, f.pool, "draft", "")
			if err != nil {
				t.Fatal(err)
			}
			var confirmed, unknown, receipts int
			if err = f.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM action_effects WHERE state='effect_confirmed'),(SELECT count(*) FROM action_effects WHERE state='effect_unknown'),(SELECT count(*) FROM behavior76_recipient_receipts)`).Scan(&confirmed, &unknown, &receipts); err != nil {
				t.Fatal(err)
			}
			switch variant {
			case "confirmed":
				if result.State != "completed" || confirmed != 1 || unknown != 0 || receipts != 1 || object.CommitCount != 1 || object.Body != "public committed bytes" {
					t.Fatal("confirmed write proof absent", result.State, object)
				}
			case "stale_cas":
				if result.State != "fixture_local_unknown" || confirmed != 0 || unknown != 1 || receipts != 0 || object.CommitCount != 0 || object.Body != "original" || object.Version != 1 {
					t.Fatal("CAS rejection changed object or lost UNKNOWN", result.State, object)
				}
			case "lost_ack":
				if result.State != "fixture_local_unknown" || confirmed != 0 || unknown != 1 || receipts != 1 || object.CommitCount != 1 || object.Body != "public committed bytes" {
					t.Fatal("lost ACK was retried, zeroed or falsely confirmed", result.State, object)
				}
			}
			if _, err = f.quiesce(ctx); err != nil {
				t.Fatal(err)
			}
			if variant == "lost_ack" {
				f.t.Log("durable recipient commit/receipt retained once; Broker UNKNOWN not rewritten despite independently observed bytes; no retry/native/auth")
			}
		})
	}
}

func TestBehavior76LocalUnknownDrainsRecipientTransactionIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76LocalFixture(t, ctx, &global)
	e := behavior76DevelopmentEpisode()
	e.Initial.Tasks[0].ExecutionPolicy = "normal"
	other := e.Initial.Tasks[0]
	other.Key = "B"
	e.Initial.Tasks = append(e.Initial.Tasks, other)
	e.Initial.Resources = append(e.Initial.Resources, behavior76Resource{Key: "second", TaskKey: "B", Body: "second", Version: 1}, behavior76Resource{Key: "foreign", TaskKey: "A", Body: "separate A object", Version: 1})
	f.seed(e)
	ready := make(chan struct{})
	var committedAfterBarrier atomic.Bool
	f.recipient.beforeCommit = func(ctx context.Context, kind string) error {
		if kind != "commit" {
			return nil
		}
		close(ready)
		for !f.scope.localStopped() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Millisecond):
			}
		}
		committedAfterBarrier.Store(true)
		return nil
	}
	if _, err := f.apply(behavior76Step{ID: "public-local-runtime-amend", Event: behavior76Event{Type: "owner_control", Kind: "amend", TaskKey: "A", Instruction: "Continue original bounded synthetic work"}}); err != nil {
		t.Fatal(err)
	}
	step := e.Steps[0]
	key := "B"
	step.RunTaskKey = &key
	step.Event.TargetTaskKey = &key
	base := &behavior76PublicConcurrent{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, f: f, entered: ready, runtimeRace: true}
	p := behavior76PublicWrapped(t, f, &global, base)
	outcome, err := f.evaluate(p, step)
	if err != nil || outcome.State != "fixture_local_unknown" || global.Load() || !committedAfterBarrier.Load() {
		t.Fatal("admitted recipient transaction did not drain under local barrier", outcome.State, err)
	}
	object, err := behavior76ObjectQuery(ctx, f.pool, "draft", "")
	if err != nil || object.CommitCount != 1 || object.Body != "admitted before barrier" {
		t.Fatal("admitted write lost or replayed", object, err)
	}
	var unknown, confirmed, admissions int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='effect_unknown'),count(*) FILTER(WHERE state='effect_confirmed') FROM action_effects`).Scan(&unknown, &confirmed); err != nil || unknown != 1 || confirmed != 1 {
		t.Fatal("independent effect outcomes were conflated", unknown, confirmed, err)
	}
	if err = p.journal.QueryRow(ctx, `SELECT count(*) FROM public.behavior76_attempts WHERE window_id=$1`, p.window).Scan(&admissions); err != nil || admissions != 2 {
		t.Fatal("post-barrier provider continuation was admitted", admissions, err)
	}
	receipt, err := f.quiesce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	f.export(filepath.Join(dir, "tables"))
	f.sealFixtureEvidence(dir)
	if !f.backupPrivate("public-scope-" + receipt.Schema) {
		t.Fatal("quiescent private backup unavailable")
	}
	var afterVersion int64
	if err = f.pool.QueryRow(ctx, `SELECT version FROM behavior76_objects WHERE key='draft'`).Scan(&afterVersion); err != nil || afterVersion != object.Version {
		t.Fatal("recipient wrote after quiescence/export", err)
	}
	t.Log("recipient transaction admitted before local stop committed once afterward; new Submit denied before journal/callback; exact independent UNKNOWN/confirmed outcomes; actual worker/process/backend drain and scoped export/anchor/backup; native/auth=0", behavior76JSON(receipt))
}

type behavior76PublicAuthorityWait struct {
	*behavior76PublicHandoff
	f       *behavior76Fixture
	sawWait bool
}

func (p *behavior76PublicAuthorityWait) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	connection, err := p.f.reads.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	deadline := time.NewTimer(500 * time.Millisecond)
	defer deadline.Stop()
	for !p.sawWait {
		for _, event := range p.f.authorityTrace.snapshot().Events {
			if event.Phase == "pool_acquire" && event.Edge == "start" && event.TimeLeft != nil && *event.TimeLeft > 0 && *event.TimeLeft <= 750000 && event.Pool.Acquired == 1 {
				p.sawWait = true
				break
			}
		}
		if p.sawWait {
			break
		}
		select {
		case <-ctx.Done():
			connection.Release()
			return nil, ctx.Err()
		case <-deadline.C:
			connection.Release()
			return nil, errBehavior76Native
		case <-time.After(5 * time.Millisecond):
		}
	}
	connection.Release()
	return p.behavior76PublicHandoff.StartRun(ctx, q)
}

func TestBehavior76LocalUnknownActualAuthorityTracingIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var global atomic.Bool
	f := newBehavior76LocalFixture(t, ctx, &global)
	e := behavior76DevelopmentEpisode()
	f.seed(e)
	base := &behavior76PublicAuthorityWait{behavior76PublicHandoff: &behavior76PublicHandoff{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}}, f: f}
	p := behavior76PublicWrapped(t, f, &global, base)
	result, err := f.evaluate(p, e.Steps[0])
	if err != nil || result.State != "completed" || global.Load() || !base.sawWait {
		t.Fatal("actual unchanged watchdog acquire wait not observed", result.State, err)
	}
	if _, err = f.quiesce(ctx); err != nil {
		t.Fatal(err)
	}
	trace := f.authorityTrace.snapshot()
	if trace.Failed || trace.OpenSpans != 0 || trace.Dropped != 0 || trace.OperationFailures != 0 || f.pool.Config().MaxConns != 9 || f.reads.Config().MaxConns != 1 {
		t.Fatal("trace changed/faulted actual authority reserve", trace.OpenSpans, trace.OperationFailures)
	}
	deadlineQueries := 0
	for _, event := range trace.Events {
		if event.Phase == "query" && event.TimeLeft != nil && *event.TimeLeft > 0 && *event.TimeLeft <= 750000 {
			deadlineQueries++
		}
	}
	if deadlineQueries == 0 {
		t.Fatal("no actual shared watchdog query deadline retained")
	}
	t.Log("actual Core watchdog pool-acquire wait plus lease/task queries captured in bounded memory; original750ms deadline and9+1 reserve retained, no SQL/args/values or PG trace writes; historical latency cause remains unknown; native/auth=0")
}
