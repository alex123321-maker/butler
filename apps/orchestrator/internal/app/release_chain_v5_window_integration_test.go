package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5"
)

func TestReleaseChainV5WindowSharedCeilingRestartAndExpiryIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	w, err := createReleaseChainV5Window(ctx, pool, strings.Repeat("a", 64), strings.Repeat("f", 64), 7, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	guard := func(context.Context, pgx.Tx, string) (bool, error) { return true, nil }
	var calls atomic.Int32
	var workers sync.WaitGroup
	for i := 0; i < 7+releaseChainV5NewCap+1; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			if _, _, e := w.reserve(ctx, releaseChainV5Scenarios()[i%5], []string{"start", "continue", "submit"}[i%3], "synthetic-root", "synthetic-window-row", i, guard); e == nil {
				calls.Add(1)
			}
		}(i)
	}
	workers.Wait()
	if calls.Load() != releaseChainV5NewCap {
		t.Fatal("shared concurrent ceiling lost", calls.Load())
	}
	var attempts, rows int
	var expiry time.Time
	if pool.QueryRow(ctx, `SELECT attempts,expires_at FROM fixture_chain_v5_high_window`).Scan(&attempts, &expiry) != nil || attempts != 7+releaseChainV5NewCap || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_chain_v5_high_requests`).Scan(&rows) != nil || rows != releaseChainV5NewCap {
		t.Fatal("shared spending rows lost")
	}
	reopened, e := createReleaseChainV5Window(ctx, pool, strings.Repeat("a", 64), strings.Repeat("f", 64), 7, 60*time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	var retained time.Time
	if pool.QueryRow(ctx, `SELECT expires_at FROM fixture_chain_v5_high_window`).Scan(&retained) != nil || !retained.Equal(expiry) {
		t.Fatal("restart extended deadline")
	}
	if _, _, e = reopened.reserve(ctx, "restart_sent", "start", "new-root", "new-run", nil, guard); e == nil {
		t.Fatal("new root refilled spent allowance")
	}
	for _, q := range []string{`UPDATE fixture_chain_v5_high_window SET attempts=7`, `UPDATE fixture_chain_v5_high_window SET attempts=attempts+1,expires_at=expires_at+interval '1 hour'`, `DELETE FROM fixture_chain_v5_high_requests`, `UPDATE fixture_chain_v5_high_requests SET input_hash=repeat('b',64)`} {
		if _, e = pool.Exec(ctx, q); e == nil {
			t.Fatal("sealed history changed")
		}
	}
}

func TestReleaseChainV5WindowChecksDeadlineAfterCounterWaitIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	w, e := createReleaseChainV5Window(ctx, pool, strings.Repeat("b", 64), strings.Repeat("f", 64), 7, 150*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `SELECT attempts FROM fixture_chain_v5_high_window FOR UPDATE`); e != nil {
		t.Fatal(e)
	}
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, _, e := w.reserve(ctx, "restart_sent", "start", "held-root", "held", nil, func(context.Context, pgx.Tx, string) (bool, error) {
			select {
			case <-entered:
			default:
				close(entered)
			}
			return true, nil
		})
		done <- e
	}()
	<-entered
	time.Sleep(200 * time.Millisecond)
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e == nil {
		t.Fatal("expired window admitted after lock wait")
	}
	var attempts, rows int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_chain_v5_high_window`).Scan(&attempts) != nil || attempts != 7 || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_chain_v5_high_requests`).Scan(&rows) != nil || rows != 0 {
		t.Fatal("denied entry was charged")
	}
}

func TestReleaseChainV5WindowSharesCounterAcrossPrivateTaskSchemasIntegration(t *testing.T) {
	first, second := memorytest.Database(t), memorytest.Database(t)
	ctx := context.Background()
	var firstSchema string
	if first.QueryRow(ctx, `SELECT current_schema()`).Scan(&firstSchema) != nil {
		t.Fatal("schema absent")
	}
	shared := firstSchema + "_chain"
	t.Cleanup(func() { _, _ = first.Exec(ctx, `DROP SCHEMA IF EXISTS `+pgx.Identifier{shared}.Sanitize()+` CASCADE`) })
	w1, e := createReleaseChainV5WindowInSchema(ctx, first, strings.Repeat("d", 64), strings.Repeat("f", 64), 7, shared, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	w2, e := createReleaseChainV5WindowInSchema(ctx, second, strings.Repeat("d", 64), strings.Repeat("f", 64), 7, shared, 60*time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	guard := func(context.Context, pgx.Tx, string) (bool, error) { return true, nil }
	for i := 0; i < releaseChainV5NewCap; i++ {
		w := w1
		if i%2 != 0 {
			w = w2
		}
		if _, _, e = w.reserve(ctx, "restart_sent", "start", "separate-root", "separate-run", i, guard); e != nil {
			t.Fatal("cross-schema shared spending denied", i)
		}
	}
	if _, _, e = w2.reserve(ctx, "lost_ack", "submit", "later-root", "later-run", nil, guard); e == nil {
		t.Fatal("private task schema reset the shared native counter")
	}
	var attempts, rows int
	if first.QueryRow(ctx, `SELECT attempts FROM `+w1.table("fixture_chain_v5_high_window")).Scan(&attempts) != nil || second.QueryRow(ctx, `SELECT count(*) FROM `+w2.table("fixture_chain_v5_high_requests")).Scan(&rows) != nil || attempts != 7+releaseChainV5NewCap || rows != releaseChainV5NewCap {
		t.Fatal("shared cross-root counter inconsistent")
	}
}

type releaseChainV5FailureSpy struct {
	*converterWorkerProvider
	calls atomic.Int32
}

type releaseChainV5DeadlineSpy struct {
	*converterWorkerProvider
	deadline    time.Time
	hasDeadline bool
	remaining   time.Duration
}

func (p *releaseChainV5DeadlineSpy) StartRun(ctx context.Context, _ transport.StartRunRequest) (transport.EventStream, error) {
	p.deadline, p.hasDeadline = ctx.Deadline()
	p.remaining = time.Until(p.deadline)
	return nil, errors.New("synthetic provider invocation after auth preflight")
}

func TestReleaseChainV5WholeEntryDeadlineIncludesProviderPreflightIntegration(t *testing.T) {
	for _, longWindow := range []bool{false, true} {
		name := "global_deadline_earlier"
		if longWindow {
			name = "entry_maximum120seconds"
		}
		t.Run(name, func(t *testing.T) {
			spy := &releaseChainV5DeadlineSpy{converterWorkerProvider: &converterWorkerProvider{}}
			p, live, req, _ := releaseChainV5OwnedProviderFixture(t, spy)
			if longWindow {
				schema := p.window.schema + "_entry"
				pool := p.window.pool
				t.Cleanup(func() {
					_, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
				})
				var err error
				p.window, err = createReleaseChainV5WindowInSchema(live, p.window.pool, p.window.pin, p.window.history, p.window.baseline, schema, 60*time.Minute)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := p.StartRun(live, req); err == nil {
				t.Fatal("synthetic provider failure lost")
			}
			var global time.Time
			if p.window.pool.QueryRow(live, `SELECT expires_at FROM `+p.window.table("fixture_chain_v5_high_window")).Scan(&global) != nil {
				t.Fatal("owning global deadline unavailable")
			}
			var admitted time.Time
			if p.window.pool.QueryRow(live, `SELECT admitted_at FROM `+p.window.table("fixture_chain_v5_high_requests")+` WHERE ordinal=8`).Scan(&admitted) != nil {
				t.Fatal("owning admission clock unavailable")
			}
			if !spy.hasDeadline || spy.remaining <= 0 || spy.remaining > 120*time.Second || spy.deadline.After(global) || longWindow && spy.remaining < 119*time.Second || !longWindow && !spy.deadline.Equal(global) {
				t.Fatal("provider auth/invoke lacked the earlier entry/global/owner deadline")
			}
			if longWindow && !spy.deadline.Equal(admitted.Add(120*time.Second)) {
				t.Fatal("post-admission work extended the120second entry allowance")
			}
		})
	}
}

func (p *releaseChainV5FailureSpy) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	return nil, errors.New("synthetic failed start")
}
func (p *releaseChainV5FailureSpy) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	return nil, errors.New("synthetic failed continue")
}
func (p *releaseChainV5FailureSpy) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	return nil, errors.New("synthetic failed submit")
}

func releaseChainV5OwnedProviderFixture(t *testing.T, provider transport.ModelProvider) (*releaseChainV5Provider, context.Context, transport.StartRunRequest, context.Context) {
	t.Helper()
	pool := memorytest.Database(t)
	ctx := context.Background()
	const key = "chain-high:synthetic-owner"
	if _, e := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'owner','web')`, key); e != nil {
		t.Fatal(e)
	}
	actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", SessionKey: key, Channel: "web", DeliveryID: "source-fixture"})
	store := task.NewStore(pool)
	root, e := store.Create(actor, task.CreateRequest{SessionKey: key, OwnerID: "owner", Goal: "Synthetic original proof", CompletionCriteria: []string{"original"}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,task_id,task_revision,input_event_id,status,autonomy_mode,current_state,model_provider,lease_id,started_at,updated_at) VALUES('chain-run',$1,$2,1,'input','model_running','mode_2','model_running','antigravity','chain-lease',clock_timestamp(),clock_timestamp())`, key, root.TaskID); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES($1,'chain-run','chain-lease',1,clock_timestamp()+interval '1 hour')`, key); e != nil {
		t.Fatal(e)
	}
	token := ownership.Token{SessionKey: key, RunID: "chain-run", LeaseID: "chain-lease", FenceEpoch: 1}
	live := ownership.WithToken(ctx, nil, token)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions.log"))
	if e = manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(pool, manifest)
	if e = sources.Reconcile(ctx); e != nil {
		t.Fatal(e)
	}
	refs := []catalog.SourceRef{}
	for _, id := range []string{"document", "need", "spec"} {
		s, e := sources.Put(actor, catalog.PutSource{ExternalKey: id, Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Synthetic DATA", Body: "exact " + id})
		if e != nil {
			t.Fatal(e)
		}
		refs = append(refs, catalog.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash})
	}
	if e = sources.PinTranscript(live, key, 0); e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(live)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sources.AccessTx(live, tx, refs, "memory_read"); e != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	w, e := createReleaseChainV5Window(ctx, pool, strings.Repeat("c", 64), strings.Repeat("f", 64), 7, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	p := &releaseChainV5Provider{ModelProvider: provider, window: w, scenario: "restart_sent", expectedVersion: "gemini-3.8-flash-n", checker: &releaseChainEntryChecker{pool: pool, sources: sources, root: root.TaskID, document: refs[0], need: refs[1], spec: refs[2]}}
	req := transport.StartRunRequest{Context: transport.TransportRunContext{RunID: token.RunID, SessionKey: key, ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel}}
	return p, live, req, actor
}

func TestReleaseChainV5AllFailedEntriesSpentButSourceDenialZeroIntegration(t *testing.T) {
	spy := &releaseChainV5FailureSpy{converterWorkerProvider: &converterWorkerProvider{}}
	p, live, req, actor := releaseChainV5OwnedProviderFixture(t, spy)
	ctx, pool, sources, ref := context.Background(), p.window.pool, p.checker.sources, p.checker.document
	var e error
	if _, e = p.StartRun(live, req); e == nil {
		t.Fatal("synthetic failure lost")
	}
	if _, e = p.ContinueRun(live, transport.ContinueRunRequest{RunID: req.Context.RunID}); e == nil {
		t.Fatal("synthetic continuation failure lost")
	}
	if _, e = p.SubmitToolResult(live, transport.SubmitToolResultRequest{RunID: req.Context.RunID}); e == nil {
		t.Fatal("synthetic submit failure lost")
	}
	if _, e = sources.Correct(actor, catalog.Correction{SourceID: ref.ID, ExpectedRevision: ref.Revision, Title: "Current changed DATA", Body: "changed"}); e != nil {
		t.Fatal(e)
	}
	if _, e = p.StartRun(live, req); e == nil {
		t.Fatal("changed original source reached provider")
	}
	var attempts, failed, unknown int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_chain_v5_high_window`).Scan(&attempts) != nil || pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE outcome='transport_error'),count(*) FILTER(WHERE input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL) FROM fixture_chain_v5_high_requests`).Scan(&failed, &unknown) != nil || attempts != 7+3 || failed != 3 || unknown != 3 || spy.calls.Load() != 3 {
		t.Fatal("failed entries refunded or denial charged", attempts, failed, unknown, spy.calls.Load())
	}
}

type releaseChainV5StreamSpy struct {
	*converterWorkerProvider
	events []transport.TransportEvent
	calls  atomic.Int32
}

func (p *releaseChainV5StreamSpy) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	stream := make(chan transport.TransportEvent, len(p.events))
	for _, event := range p.events {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func TestReleaseChainV5TerminalMetadataRetainedAndReleaseGatedIntegration(t *testing.T) {
	known := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 205}
	changed := transport.Usage{InputTokens: 3, OutputTokens: 3, TotalTokens: 206}
	zero := transport.Usage{}
	legacy := transport.Usage{InputTokens: 3, OutputTokens: 2}
	overbound := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 40000}
	inputOverbound := transport.Usage{InputTokens: 1048577, OutputTokens: 2, TotalTokens: 1048579}
	outputOverbound := transport.Usage{InputTokens: 3, OutputTokens: 32769, TotalTokens: 32772}
	version := "gemini-3.8-flash-n"
	errorEvent := func(usage *transport.Usage, version *string) transport.TransportEvent {
		e := transport.NewTransportErrorEvent("chain-run", antigravity.Name, errors.New("synthetic private error body"))
		e.Usage, e.ProviderModelVersion = usage, version
		return e
	}
	toolEvent := func(usage *transport.Usage, version *string) transport.TransportEvent {
		e := transport.NewToolCallRequestedEvent("chain-run", antigravity.Name, transport.ToolCallRequest{ToolName: "synthetic.read", ArgsJSON: `{}`})
		e.Usage, e.ProviderModelVersion = usage, version
		return e
	}
	finalEvent := func(usage *transport.Usage, version *string) transport.TransportEvent {
		e := transport.NewAssistantFinalEvent("chain-run", antigravity.Name, transport.AssistantFinal{Content: "synthetic final", FinishReason: "STOP", Usage: usage})
		e.ProviderModelVersion = version
		return e
	}
	untrustedError := errorEvent(&known, &version)
	untrustedError.TransportError = &transport.Error{Type: transport.ErrorType("PRIVATE_TYPE"), Message: "PRIVATE_MESSAGE", ProviderCode: "PRIVATE_CODE", ProviderDetails: map[string]any{"PRIVATE_KEY": "PRIVATE_VALUE"}}
	provisionalVersion := transport.TransportEvent{RunID: "chain-run", EventType: transport.EventTypeAssistantDelta, ProviderModelVersion: &version}
	batch := transport.NewToolCallBatchRequestedEvent("chain-run", antigravity.Name, transport.ToolCallBatch{ToolCalls: []transport.ToolCallRequest{{ToolName: "synthetic.read", ArgsJSON: `{}`}, {ToolName: "synthetic.write", ArgsJSON: `{}`}}})
	batch.Usage, batch.ProviderModelVersion = &known, &version
	emptyBatch := transport.NewToolCallBatchRequestedEvent("chain-run", antigravity.Name, transport.ToolCallBatch{})
	emptyBatch.Usage, emptyBatch.ProviderModelVersion = &known, &version
	truncated := finalEvent(&known, &version)
	truncated.AssistantFinal.FinishReason = "MAX_TOKENS"
	missingFinal := transport.NewTerminalEvent("chain-run", transport.EventTypeRunCompleted, antigravity.Name)
	missingFinal.Usage, missingFinal.ProviderModelVersion = &known, &version
	foreignFinal := finalEvent(&known, &version)
	foreignFinal.RunID = "foreign-run"
	foreignTerminal := transport.NewTerminalEvent("foreign-run", transport.EventTypeRunCompleted, antigravity.Name)
	foreignTerminal.Usage, foreignTerminal.ProviderModelVersion = &changed, &version
	for _, tc := range []struct {
		name    string
		events  []transport.TransportEvent
		usage   *transport.Usage
		version bool
		total   bool
		outcome string
		tools   int
		finals  int
	}{
		{"failed_terminal_known", []transport.TransportEvent{errorEvent(&known, &version)}, &known, true, true, "transport_error", 0, 0},
		{"untrusted_diagnostic_type", []transport.TransportEvent{untrustedError}, &known, true, true, "transport_error", 0, 0},
		{"known_batch_cost_retained_for_ordinary_core", []transport.TransportEvent{batch}, &known, true, true, "completed", 1, 0},
		{"empty_batch_denied", []transport.TransportEvent{emptyBatch}, &known, true, true, "transport_error", 0, 0},
		{"batch_after_final_denied", []transport.TransportEvent{finalEvent(&known, &version), batch}, &known, true, true, "transport_error", 0, 0},
		{"conflicting_complete_usage", []transport.TransportEvent{finalEvent(&known, &version), errorEvent(&changed, &version)}, nil, true, false, "transport_error", 0, 0},
		{"true_zero_triple", []transport.TransportEvent{toolEvent(&zero, &version)}, &zero, true, true, "completed", 1, 0},
		{"missing_total_legacy_pair", []transport.TransportEvent{toolEvent(&legacy, &version)}, &legacy, true, false, "transport_error", 0, 0},
		{"missing_metadata", []transport.TransportEvent{toolEvent(nil, nil)}, nil, false, false, "transport_error", 0, 0},
		{"missing_version", []transport.TransportEvent{finalEvent(&known, nil)}, &known, false, true, "transport_error", 0, 0},
		{"provisional_version", []transport.TransportEvent{provisionalVersion, toolEvent(&known, nil)}, &known, false, true, "transport_error", 0, 0},
		{"missing_terminal_known", []transport.TransportEvent{finalEvent(&known, &version)}, &known, true, true, "transport_error", 0, 0},
		{"duplicate_final_known", []transport.TransportEvent{finalEvent(&known, &version), finalEvent(&known, &version)}, &known, true, true, "transport_error", 0, 0},
		{"truncated_final_known", []transport.TransportEvent{truncated}, &known, true, true, "transport_error", 0, 0},
		{"terminal_without_final_known", []transport.TransportEvent{missingFinal}, &known, true, true, "transport_error", 0, 0},
		{"generation_bound_known", []transport.TransportEvent{toolEvent(&overbound, &version)}, &overbound, true, true, "transport_error", 0, 0},
		{"input_bound_known", []transport.TransportEvent{toolEvent(&inputOverbound, &version)}, &inputOverbound, true, true, "transport_error", 0, 0},
		{"output_bound_known", []transport.TransportEvent{toolEvent(&outputOverbound, &version)}, &outputOverbound, true, true, "transport_error", 0, 0},
		{"conflicting_finish_reasons_known", []transport.TransportEvent{finalEvent(&known, &version), truncated}, &known, true, true, "transport_error", 0, 0},
		{"completed_final", []transport.TransportEvent{finalEvent(&known, &version), transport.NewTerminalEvent("chain-run", transport.EventTypeRunCompleted, antigravity.Name)}, &known, true, true, "completed", 0, 1},
		{"foreign_metadata_never_supplies_cost_or_version", []transport.TransportEvent{foreignFinal}, nil, false, false, "transport_error", 0, 0},
		{"own_complete_cost_survives_foreign_terminal", []transport.TransportEvent{finalEvent(&known, &version), foreignTerminal}, &known, true, true, "transport_error", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spy := &releaseChainV5StreamSpy{converterWorkerProvider: &converterWorkerProvider{}, events: tc.events}
			p, live, req, _ := releaseChainV5OwnedProviderFixture(t, spy)
			stream, e := p.StartRun(live, req)
			if e != nil {
				t.Fatal("current actual source/lease preflight denied synthetic stream", e)
			}
			tools, finals := 0, 0
			for event := range stream {
				if event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested || event.EventType == transport.EventTypeAssistantFinal {
					var durable bool
					if p.window.pool.QueryRow(context.Background(), `SELECT outcome='completed' AND total_tokens IS NOT NULL AND model_version IS NOT NULL FROM fixture_chain_v5_high_requests WHERE ordinal=8`).Scan(&durable) != nil || !durable {
						t.Fatal("actionable release preceded durable native measurement")
					}
				}
				if event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested {
					tools++
				}
				if tc.name == "known_batch_cost_retained_for_ordinary_core" && (event.EventType != transport.EventTypeToolCallBatchRequested || event.ToolCallBatch == nil || event.ToolCallBatch.BatchRef != batch.ToolCallBatch.BatchRef || len(event.ToolCallBatch.ToolCalls) != 2 || event.ToolCallBatch.ToolCalls[0] != batch.ToolCallBatch.ToolCalls[0] || event.ToolCallBatch.ToolCalls[1] != batch.ToolCallBatch.ToolCalls[1] || event.Usage == nil || *event.Usage != known || event.ProviderModelVersion == nil || *event.ProviderModelVersion != version) {
					t.Fatal("valid complete transport batch was transformed before ordinary Core")
				}
				if event.EventType == transport.EventTypeAssistantFinal {
					finals++
				}
			}
			var outcome string
			var in, out, total *int64
			var observedVersion *string
			var admitted, finished time.Time
			var elapsed int64
			var stage, code *string
			if e = p.window.pool.QueryRow(context.Background(), `SELECT outcome,input_tokens,output_tokens,total_tokens,model_version,admitted_at,finished_at,elapsed_ms,failure_stage,diagnostic_code FROM fixture_chain_v5_high_requests WHERE ordinal=8`).Scan(&outcome, &in, &out, &total, &observedVersion, &admitted, &finished, &elapsed, &stage, &code); e != nil {
				t.Fatal(e)
			}
			if outcome != tc.outcome || tools != tc.tools || finals != tc.finals || spy.calls.Load() != 1 {
				t.Fatal("terminal release/outcome differs", outcome, tools, finals, spy.calls.Load())
			}
			if finished.Before(admitted) || elapsed < 0 || tc.outcome == "completed" && (stage != nil || code != nil) || tc.outcome != "completed" && (stage == nil || *stage != "stream" || code == nil || *code != string(transport.ErrorTypeInternalTransportError) && *code != string(transport.ErrorTypeProviderProtocolError)) {
				t.Fatal("elapsed time or bounded diagnostic not retained")
			}
			if tc.usage == nil {
				if in != nil || out != nil || total != nil {
					t.Fatal("unknown or conflicting usage became known")
				}
			} else if in == nil || out == nil || *in != int64(tc.usage.InputTokens) || *out != int64(tc.usage.OutputTokens) {
				t.Fatal("known failed/zero usage was lost")
			}
			if (total != nil) != tc.total || total != nil && *total != int64(tc.usage.TotalTokens) || (observedVersion != nil) != tc.version || observedVersion != nil && *observedVersion != version {
				t.Fatal("complete total/version truth differs")
			}
			original := &releaseChainV5Observations{}
			for _, event := range tc.events {
				original.observe(event)
			}
			var finalCount, terminalCount, stopCount, maxCount int
			var finishReason *string
			var finishUncertain bool
			if e = p.window.pool.QueryRow(context.Background(), `SELECT final_count,terminal_count,stop_final_count,max_tokens_final_count,finish_reason,finish_observation_uncertain FROM fixture_chain_v5_high_requests WHERE ordinal=8`).Scan(&finalCount, &terminalCount, &stopCount, &maxCount, &finishReason, &finishUncertain); e != nil || finalCount != original.finals || terminalCount != original.terminals || stopCount != original.stopFinals || maxCount != original.maxTokensFinals || finishUncertain != original.finishUncertain || (finishReason != nil) != (original.finishReason != nil) || finishReason != nil && *finishReason != *original.finishReason {
				t.Fatal("owning SQL did not retain exact original lifecycle/finish facts")
			}
			var attempts, rows int
			if p.window.pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_chain_v5_high_window`).Scan(&attempts) != nil || p.window.pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_chain_v5_high_requests`).Scan(&rows) != nil || attempts != 8 || rows != 1 {
				t.Fatal("failed terminal refunded or duplicated the entry")
			}
			// Terminal observation is immutable, including a known zero triple.
			p.finish(8, "completed", &changed, &version, "stream", nil, nil)
			var retainedOutcome string
			var retainedTotal *int64
			if p.window.pool.QueryRow(context.Background(), `SELECT outcome,total_tokens FROM fixture_chain_v5_high_requests WHERE ordinal=8`).Scan(&retainedOutcome, &retainedTotal) != nil || retainedOutcome != outcome || (retainedTotal != nil) != (total != nil) || retainedTotal != nil && *retainedTotal != *total {
				t.Fatal("terminal observation overwritten")
			}
		})
	}
}

func TestReleaseChainV5MeasurementWriteFailureDeniesReleaseIntegration(t *testing.T) {
	usage := &transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 205}
	version := "gemini-3.8-flash-n"
	event := transport.NewToolCallRequestedEvent("chain-run", antigravity.Name, transport.ToolCallRequest{ToolName: "synthetic.read", ArgsJSON: `{}`})
	event.Usage, event.ProviderModelVersion = usage, &version
	spy := &releaseChainV5StreamSpy{converterWorkerProvider: &converterWorkerProvider{}, events: []transport.TransportEvent{event}}
	p, live, req, _ := releaseChainV5OwnedProviderFixture(t, spy)
	if _, err := p.window.pool.Exec(live, `CREATE FUNCTION fixture_chain_v5_reject_measurement() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic persistence outage'; END $$; CREATE TRIGGER fixture_chain_v5_reject_measurement BEFORE UPDATE ON fixture_chain_v5_high_requests FOR EACH ROW EXECUTE FUNCTION fixture_chain_v5_reject_measurement()`); err != nil {
		t.Fatal(err)
	}
	stream, err := p.StartRun(live, req)
	if err != nil {
		t.Fatal("synthetic provider admission denied")
	}
	errorsSeen := 0
	for observed := range stream {
		if observed.EventType == transport.EventTypeToolCallRequested || observed.EventType == transport.EventTypeAssistantFinal {
			t.Fatal("failed persistence released model actions")
		}
		if observed.EventType == transport.EventTypeTransportError {
			errorsSeen++
		}
	}
	var counter int
	var unknown bool
	if errorsSeen != 1 || p.window.pool.QueryRow(live, `SELECT attempts FROM fixture_chain_v5_high_window`).Scan(&counter) != nil || counter != 8 || p.window.pool.QueryRow(live, `SELECT outcome='unavailable' AND input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL AND model_version IS NULL FROM fixture_chain_v5_high_requests WHERE ordinal=8`).Scan(&unknown) != nil || !unknown {
		t.Fatal("failed persistence refunded or invented a measurement")
	}
}

func TestReleaseChainV5CancelledTimedOutOriginalStateAndCostIntegration(t *testing.T) {
	known := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 205}
	changed := transport.Usage{InputTokens: 3, OutputTokens: 3, TotalTokens: 206}
	zero := transport.Usage{}
	legacy := transport.Usage{InputTokens: 3, OutputTokens: 2}
	version, foreignVersion := "gemini-3.8-flash-n", "gemini-3.8-flash-other"
	terminal := func(kind transport.EventType, usage *transport.Usage, v *string) transport.TransportEvent {
		e := transport.NewTerminalEvent("chain-run", kind, antigravity.Name)
		e.Usage, e.ProviderModelVersion = usage, v
		return e
	}
	final := transport.NewAssistantFinalEvent("chain-run", antigravity.Name, transport.AssistantFinal{Content: "private synthetic pending final", FinishReason: "STOP", Usage: &known})
	final.ProviderModelVersion = &version
	provisionalVersion := transport.TransportEvent{RunID: "chain-run", EventType: transport.EventTypeAssistantDelta, ProviderModelVersion: &version}
	provisionalUsage := transport.TransportEvent{RunID: "chain-run", EventType: transport.EventTypeAssistantDelta, Usage: &known}
	for _, tc := range []struct {
		name                             string
		events                           []transport.TransportEvent
		kind                             transport.EventType
		outcome, code                    string
		usage                            *transport.Usage
		total, version                   bool
		finals                           int
		usageUncertain, versionUncertain bool
	}{
		{"cancelled_known", []transport.TransportEvent{terminal(transport.EventTypeRunCancelled, &known, &version)}, transport.EventTypeRunCancelled, "cancelled", "transport_connection_error", &known, true, true, 0, false, false},
		{"timed_out_known", []transport.TransportEvent{terminal(transport.EventTypeRunTimedOut, &known, &version)}, transport.EventTypeRunTimedOut, "timed_out", "provider_timeout", &known, true, true, 0, false, false},
		{"cancelled_after_final", []transport.TransportEvent{final, terminal(transport.EventTypeRunCancelled, &known, &version)}, transport.EventTypeRunCancelled, "cancelled", "transport_connection_error", &known, true, true, 1, false, false},
		{"timed_out_after_final", []transport.TransportEvent{final, terminal(transport.EventTypeRunTimedOut, &known, &version)}, transport.EventTypeRunTimedOut, "timed_out", "provider_timeout", &known, true, true, 1, false, false},
		{"cancelled_true_zero", []transport.TransportEvent{terminal(transport.EventTypeRunCancelled, &zero, &version)}, transport.EventTypeRunCancelled, "cancelled", "transport_connection_error", &zero, true, true, 0, false, false},
		{"timed_out_legacy_pair", []transport.TransportEvent{terminal(transport.EventTypeRunTimedOut, &legacy, &version)}, transport.EventTypeRunTimedOut, "timed_out", "provider_timeout", &legacy, false, true, 0, false, false},
		{"cancelled_missing_version", []transport.TransportEvent{terminal(transport.EventTypeRunCancelled, &known, nil)}, transport.EventTypeRunCancelled, "cancelled", "transport_connection_error", &known, true, false, 0, false, false},
		{"cancelled_conflicting_usage", []transport.TransportEvent{final, terminal(transport.EventTypeRunCancelled, &changed, &version)}, transport.EventTypeRunCancelled, "cancelled", "provider_protocol_error", nil, false, true, 1, true, false},
		{"timed_out_conflicting_version", []transport.TransportEvent{final, terminal(transport.EventTypeRunTimedOut, &known, &foreignVersion)}, transport.EventTypeRunTimedOut, "timed_out", "provider_protocol_error", &known, true, false, 1, false, true},
		{"cancelled_provisional_version", []transport.TransportEvent{provisionalVersion, terminal(transport.EventTypeRunCancelled, &known, &version)}, transport.EventTypeRunCancelled, "cancelled", "transport_connection_error", &known, true, false, 0, false, true},
		{"timed_out_provisional_usage", []transport.TransportEvent{provisionalUsage, terminal(transport.EventTypeRunTimedOut, &known, &version)}, transport.EventTypeRunTimedOut, "timed_out", "provider_timeout", nil, false, true, 0, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spy := &releaseChainV5StreamSpy{converterWorkerProvider: &converterWorkerProvider{}, events: tc.events}
			p, live, req, _ := releaseChainV5OwnedProviderFixture(t, spy)
			stream, err := p.StartRun(live, req)
			if err != nil {
				t.Fatal("owning terminal fixture denied", err)
			}
			terminals := 0
			for e := range stream {
				if e.EventType == transport.EventTypeToolCallRequested || e.EventType == transport.EventTypeToolCallBatchRequested || e.EventType == transport.EventTypeAssistantFinal {
					t.Fatal("non-success terminal released pending actions or final")
				}
				if e.EventType == transport.EventTypeRunCancelled || e.EventType == transport.EventTypeRunTimedOut {
					terminals++
					if e.EventType != tc.kind || (e.ProviderModelVersion != nil) != tc.version || (e.Usage != nil) != (tc.usage != nil) || e.Usage != nil && *e.Usage != *tc.usage {
						t.Fatal("owning terminal metadata/state changed before normal Core")
					}
					var durable bool
					if p.window.pool.QueryRow(context.Background(), `SELECT outcome=$1 AND terminal_count=1 FROM fixture_chain_v5_high_requests WHERE ordinal=8`, tc.outcome).Scan(&durable) != nil || !durable {
						t.Fatal("terminal release preceded owning durable measurement")
					}
				}
			}
			var outcome, stage, code string
			var input, output, total *int64
			var observedVersion, finishReason *string
			var finals, terminalCount, completed, cancelled, timedOut, transportErrors int
			var usageUncertain, versionUncertain bool
			if err = p.window.pool.QueryRow(context.Background(), `SELECT outcome,failure_stage,diagnostic_code,input_tokens,output_tokens,total_tokens,model_version,final_count,terminal_count,completed_count,cancelled_count,timed_out_count,transport_error_count,finish_reason,usage_observation_uncertain,version_observation_uncertain FROM fixture_chain_v5_high_requests WHERE ordinal=8`).Scan(&outcome, &stage, &code, &input, &output, &total, &observedVersion, &finals, &terminalCount, &completed, &cancelled, &timedOut, &transportErrors, &finishReason, &usageUncertain, &versionUncertain); err != nil {
				t.Fatal(err)
			}
			wantCancelled, wantTimedOut := 1, 0
			if tc.kind == transport.EventTypeRunTimedOut {
				wantCancelled, wantTimedOut = 0, 1
			}
			if terminals != 1 || spy.calls.Load() != 1 || outcome != tc.outcome || stage != "stream" || code != tc.code || finals != tc.finals || terminalCount != 1 || completed != 0 || cancelled != wantCancelled || timedOut != wantTimedOut || transportErrors != 0 || usageUncertain != tc.usageUncertain || versionUncertain != tc.versionUncertain {
				t.Fatal("exact original terminal or uncertainty state differs")
			}
			if tc.usage == nil {
				if input != nil || output != nil || total != nil {
					t.Fatal("uncertain usage became a quote or known zero")
				}
			} else if input == nil || output == nil || *input != int64(tc.usage.InputTokens) || *output != int64(tc.usage.OutputTokens) {
				t.Fatal("known failed terminal dimensions lost")
			}
			if (total != nil) != tc.total || total != nil && *total != int64(tc.usage.TotalTokens) || (observedVersion != nil) != tc.version || observedVersion != nil && *observedVersion != version || (finishReason != nil) != (tc.finals == 1) || finishReason != nil && *finishReason != "STOP" {
				t.Fatal("exact total/version/finish observation differs")
			}
			var attempts, rows int
			if p.window.pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_chain_v5_high_window`).Scan(&attempts) != nil || attempts != 8 || p.window.pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_chain_v5_high_requests`).Scan(&rows) != nil || rows != 1 {
				t.Fatal("failed terminal refunded, retried or duplicated its admission")
			}
			for _, query := range []string{`UPDATE fixture_chain_v5_high_requests SET outcome='completed'`, `UPDATE fixture_chain_v5_high_requests SET terminal_count=0`, `UPDATE fixture_chain_v5_high_requests SET model_version='invented'`, `UPDATE fixture_chain_v5_high_requests SET finish_reason='private reason'`} {
				if _, err := p.window.pool.Exec(context.Background(), query); err == nil {
					t.Fatal("sealed terminal witness changed")
				}
			}
		})
	}
}

type releaseChainV5ContextEndingSpy struct {
	*converterWorkerProvider
	started chan struct{}
}

func (p *releaseChainV5ContextEndingSpy) StartRun(ctx context.Context, _ transport.StartRunRequest) (transport.EventStream, error) {
	close(p.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestReleaseChainV5ContextDeadlineAndCancelAreDistinctIntegration(t *testing.T) {
	for _, name := range []string{"parent_deadline", "parent_cancel"} {
		t.Run(name, func(t *testing.T) {
			spy := &releaseChainV5ContextEndingSpy{converterWorkerProvider: &converterWorkerProvider{}, started: make(chan struct{})}
			p, live, req, _ := releaseChainV5OwnedProviderFixture(t, spy)
			call, cancel := context.WithCancel(live)
			if name == "parent_deadline" {
				cancel()
				call, cancel = context.WithTimeout(live, 2*time.Second)
			} else {
				go func() { <-spy.started; cancel() }()
			}
			defer cancel()
			if _, err := p.StartRun(call, req); err == nil {
				t.Fatal("owning context failure disappeared")
			}
			var outcome, code, stage string
			var unknown bool
			var finals, terminals int
			if p.window.pool.QueryRow(context.Background(), `SELECT outcome,diagnostic_code,failure_stage,input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL AND model_version IS NULL,final_count,terminal_count FROM fixture_chain_v5_high_requests WHERE ordinal=8`).Scan(&outcome, &code, &stage, &unknown, &finals, &terminals) != nil {
				t.Fatal("spent owning context witness unavailable")
			}
			wantOutcome, wantCode := "cancelled", "transport_connection_error"
			if name == "parent_deadline" {
				wantOutcome, wantCode = "timed_out", "provider_timeout"
			}
			if outcome != wantOutcome || code != wantCode || stage != "entry" || !unknown || finals != 0 || terminals != 0 {
				t.Fatal("context deadline/cancel/state/counts or unknown cost differ")
			}
		})
	}
}

type releaseChainV5OpenStreamSpy struct {
	*converterWorkerProvider
	event     transport.TransportEvent
	cancel    func()
	nilStream bool
	calls     atomic.Int32
}

func (p *releaseChainV5OpenStreamSpy) StartRun(ctx context.Context, _ transport.StartRunRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	if p.nilStream {
		return nil, nil
	}
	stream := make(chan transport.TransportEvent)
	go func() {
		defer close(stream)
		select {
		case stream <- p.event:
			if p.cancel != nil {
				p.cancel()
			}
		case <-ctx.Done():
			return
		}
		<-ctx.Done()
	}()
	return stream, nil
}

func TestReleaseChainV5ContextFailureAfterCompleteFinalAndNilStreamIntegration(t *testing.T) {
	for _, name := range []string{"stream_deadline_known_cost", "stream_cancel_known_cost", "nil_stream_unknown_cost"} {
		t.Run(name, func(t *testing.T) {
			known := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 205}
			version := "gemini-3.8-flash-n"
			final := transport.NewAssistantFinalEvent("chain-run", antigravity.Name, transport.AssistantFinal{Content: "synthetic pending final", FinishReason: "STOP", Usage: &known})
			final.ProviderModelVersion = &version
			spy := &releaseChainV5OpenStreamSpy{converterWorkerProvider: &converterWorkerProvider{}, event: final, nilStream: name == "nil_stream_unknown_cost"}
			p, live, req, _ := releaseChainV5OwnedProviderFixture(t, spy)
			call, cancel := context.WithTimeout(live, 2*time.Second)
			defer cancel()
			if name == "stream_cancel_known_cost" {
				spy.cancel = cancel
			}
			stream, err := p.StartRun(call, req)
			if spy.nilStream {
				if err == nil || stream != nil {
					t.Fatal("nil provider stream was admitted as a usable lifecycle")
				}
			} else {
				if err != nil {
					t.Fatal("owning stream fixture denied", err)
				}
				for event := range stream {
					if event.EventType == transport.EventTypeAssistantFinal || event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested {
						t.Fatal("incomplete lifecycle released pending action")
					}
				}
			}
			var outcome, stage, code string
			var input, output, total *int64
			var observedVersion *string
			var finals, terminals, stop int
			if p.window.pool.QueryRow(context.Background(), `SELECT outcome,failure_stage,diagnostic_code,input_tokens,output_tokens,total_tokens,model_version,final_count,terminal_count,stop_final_count FROM fixture_chain_v5_high_requests WHERE ordinal=8`).Scan(&outcome, &stage, &code, &input, &output, &total, &observedVersion, &finals, &terminals, &stop) != nil {
				t.Fatal("owning context witness missing")
			}
			wantOutcome, wantCode, wantStage := "timed_out", "provider_timeout", "stream"
			if name == "stream_cancel_known_cost" {
				wantOutcome, wantCode = "cancelled", "transport_connection_error"
			}
			if spy.nilStream {
				wantOutcome, wantCode, wantStage = "transport_error", "provider_protocol_error", "entry"
			}
			if outcome != wantOutcome || code != wantCode || stage != wantStage || terminals != 0 || spy.calls.Load() != 1 {
				t.Fatal("context/nil-stream outcome or actual counts differ")
			}
			if spy.nilStream {
				if input != nil || output != nil || total != nil || observedVersion != nil || finals != 0 || stop != 0 {
					t.Fatal("nil stream invented original counts or known measurement")
				}
			} else if input == nil || output == nil || total == nil || observedVersion == nil || *input != 3 || *output != 2 || *total != 205 || *observedVersion != version || finals != 1 || stop != 1 {
				t.Fatal("complete own metadata was erased after context failure")
			}
			var attempts, rows int
			if p.window.pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_chain_v5_high_window`).Scan(&attempts) != nil || attempts != 8 || p.window.pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_chain_v5_high_requests`).Scan(&rows) != nil || rows != 1 {
				t.Fatal("context/nil-stream failure refunded or retried admission")
			}
		})
	}
}
