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

func TestReleaseChainV4WindowSharedCeilingRestartAndExpiryIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	w, err := createReleaseChainV4Window(ctx, pool, strings.Repeat("a", 64), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	guard := func(context.Context, pgx.Tx, string) (bool, error) { return true, nil }
	var calls atomic.Int32
	var workers sync.WaitGroup
	for i := 0; i < 68; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			if _, _, e := w.reserve(ctx, []string{"restart_sent", "explicit_absence", "lost_ack", "owner_stop"}[i%4], []string{"start", "continue", "submit"}[i%3], "synthetic-root", "synthetic-window-row", i, guard); e == nil {
				calls.Add(1)
			}
		}(i)
	}
	workers.Wait()
	if calls.Load() != releaseChainV4NewCap {
		t.Fatal("shared concurrent ceiling lost", calls.Load())
	}
	var attempts, rows int
	var expiry time.Time
	if pool.QueryRow(ctx, `SELECT attempts,expires_at FROM fixture_chain_v4_high_window`).Scan(&attempts, &expiry) != nil || attempts != releaseChainV4Ceiling || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_chain_v4_high_requests`).Scan(&rows) != nil || rows != releaseChainV4NewCap {
		t.Fatal("shared spending rows lost")
	}
	reopened, e := createReleaseChainV4Window(ctx, pool, strings.Repeat("a", 64), 60*time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	var retained time.Time
	if pool.QueryRow(ctx, `SELECT expires_at FROM fixture_chain_v4_high_window`).Scan(&retained) != nil || !retained.Equal(expiry) {
		t.Fatal("restart extended deadline")
	}
	if _, _, e = reopened.reserve(ctx, "restart_sent", "start", "new-root", "new-run", nil, guard); e == nil {
		t.Fatal("new root refilled spent allowance")
	}
	for _, q := range []string{`UPDATE fixture_chain_v4_high_window SET attempts=520`, `UPDATE fixture_chain_v4_high_window SET attempts=attempts+1,expires_at=expires_at+interval '1 hour'`, `DELETE FROM fixture_chain_v4_high_requests`, `UPDATE fixture_chain_v4_high_requests SET input_hash=repeat('b',64)`} {
		if _, e = pool.Exec(ctx, q); e == nil {
			t.Fatal("sealed history changed")
		}
	}
}

func TestReleaseChainV4WindowChecksDeadlineAfterCounterWaitIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	w, e := createReleaseChainV4Window(ctx, pool, strings.Repeat("b", 64), 150*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `SELECT attempts FROM fixture_chain_v4_high_window FOR UPDATE`); e != nil {
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
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_chain_v4_high_window`).Scan(&attempts) != nil || attempts != releaseChainV4Baseline || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_chain_v4_high_requests`).Scan(&rows) != nil || rows != 0 {
		t.Fatal("denied entry was charged")
	}
}

func TestReleaseChainV4WindowSharesCounterAcrossPrivateTaskSchemasIntegration(t *testing.T) {
	first, second := memorytest.Database(t), memorytest.Database(t)
	ctx := context.Background()
	var firstSchema string
	if first.QueryRow(ctx, `SELECT current_schema()`).Scan(&firstSchema) != nil {
		t.Fatal("schema absent")
	}
	shared := firstSchema + "_chain"
	t.Cleanup(func() { _, _ = first.Exec(ctx, `DROP SCHEMA IF EXISTS `+pgx.Identifier{shared}.Sanitize()+` CASCADE`) })
	w1, e := createReleaseChainV4WindowInSchema(ctx, first, strings.Repeat("d", 64), shared, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	w2, e := createReleaseChainV4WindowInSchema(ctx, second, strings.Repeat("d", 64), shared, 60*time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	guard := func(context.Context, pgx.Tx, string) (bool, error) { return true, nil }
	for i := 0; i < releaseChainV4NewCap; i++ {
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
	if first.QueryRow(ctx, `SELECT attempts FROM `+w1.table("fixture_chain_v4_high_window")).Scan(&attempts) != nil || second.QueryRow(ctx, `SELECT count(*) FROM `+w2.table("fixture_chain_v4_high_requests")).Scan(&rows) != nil || attempts != releaseChainV4Ceiling || rows != releaseChainV4NewCap {
		t.Fatal("shared cross-root counter inconsistent")
	}
}

type releaseChainV4FailureSpy struct {
	*converterWorkerProvider
	calls atomic.Int32
}

type releaseChainV4DeadlineSpy struct {
	*converterWorkerProvider
	deadline    time.Time
	hasDeadline bool
	remaining   time.Duration
}

func (p *releaseChainV4DeadlineSpy) StartRun(ctx context.Context, _ transport.StartRunRequest) (transport.EventStream, error) {
	p.deadline, p.hasDeadline = ctx.Deadline()
	p.remaining = time.Until(p.deadline)
	return nil, errors.New("synthetic provider invocation after auth preflight")
}

func TestReleaseChainV4WholeEntryDeadlineIncludesProviderPreflightIntegration(t *testing.T) {
	for _, longWindow := range []bool{false, true} {
		name := "global_deadline_earlier"
		if longWindow {
			name = "entry_maximum120seconds"
		}
		t.Run(name, func(t *testing.T) {
			spy := &releaseChainV4DeadlineSpy{converterWorkerProvider: &converterWorkerProvider{}}
			p, live, req, _ := releaseChainV4OwnedProviderFixture(t, spy)
			if longWindow {
				schema := p.window.schema + "_entry"
				pool := p.window.pool
				t.Cleanup(func() {
					_, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
				})
				var err error
				p.window, err = createReleaseChainV4WindowInSchema(live, p.window.pool, p.window.pin, schema, 60*time.Minute)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := p.StartRun(live, req); err == nil {
				t.Fatal("synthetic provider failure lost")
			}
			var global time.Time
			if p.window.pool.QueryRow(live, `SELECT expires_at FROM `+p.window.table("fixture_chain_v4_high_window")).Scan(&global) != nil {
				t.Fatal("owning global deadline unavailable")
			}
			var admitted time.Time
			if p.window.pool.QueryRow(live, `SELECT admitted_at FROM `+p.window.table("fixture_chain_v4_high_requests")+` WHERE ordinal=521`).Scan(&admitted) != nil {
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

func (p *releaseChainV4FailureSpy) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	return nil, errors.New("synthetic failed start")
}
func (p *releaseChainV4FailureSpy) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	return nil, errors.New("synthetic failed continue")
}
func (p *releaseChainV4FailureSpy) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	return nil, errors.New("synthetic failed submit")
}

func releaseChainV4OwnedProviderFixture(t *testing.T, provider transport.ModelProvider) (*releaseChainV4Provider, context.Context, transport.StartRunRequest, context.Context) {
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
	w, e := createReleaseChainV4Window(ctx, pool, strings.Repeat("c", 64), time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	p := &releaseChainV4Provider{ModelProvider: provider, window: w, scenario: "restart_sent", expectedVersion: "gemini-3.8-flash-n", checker: &releaseChainEntryChecker{pool: pool, sources: sources, root: root.TaskID, document: refs[0], need: refs[1], spec: refs[2]}}
	req := transport.StartRunRequest{Context: transport.TransportRunContext{RunID: token.RunID, SessionKey: key, ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel}}
	return p, live, req, actor
}

func TestReleaseChainV4AllFailedEntriesSpentButSourceDenialZeroIntegration(t *testing.T) {
	spy := &releaseChainV4FailureSpy{converterWorkerProvider: &converterWorkerProvider{}}
	p, live, req, actor := releaseChainV4OwnedProviderFixture(t, spy)
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
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_chain_v4_high_window`).Scan(&attempts) != nil || pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE outcome='transport_error'),count(*) FILTER(WHERE input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL) FROM fixture_chain_v4_high_requests`).Scan(&failed, &unknown) != nil || attempts != 523 || failed != 3 || unknown != 3 || spy.calls.Load() != 3 {
		t.Fatal("failed entries refunded or denial charged", attempts, failed, unknown, spy.calls.Load())
	}
}

type releaseChainV4StreamSpy struct {
	*converterWorkerProvider
	events []transport.TransportEvent
	calls  atomic.Int32
}

func (p *releaseChainV4StreamSpy) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	stream := make(chan transport.TransportEvent, len(p.events))
	for _, event := range p.events {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func TestReleaseChainV4TerminalMetadataRetainedAndReleaseGatedIntegration(t *testing.T) {
	known := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 205}
	changed := transport.Usage{InputTokens: 3, OutputTokens: 3, TotalTokens: 206}
	zero := transport.Usage{}
	legacy := transport.Usage{InputTokens: 3, OutputTokens: 2}
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
		e := transport.NewAssistantFinalEvent("chain-run", antigravity.Name, transport.AssistantFinal{Content: "synthetic final", Usage: usage})
		e.ProviderModelVersion = version
		return e
	}
	untrustedError := errorEvent(&known, &version)
	untrustedError.TransportError = &transport.Error{Type: transport.ErrorType("PRIVATE_TYPE"), Message: "PRIVATE_MESSAGE", ProviderCode: "PRIVATE_CODE", ProviderDetails: map[string]any{"PRIVATE_KEY": "PRIVATE_VALUE"}}
	provisionalVersion := transport.TransportEvent{RunID: "chain-run", EventType: transport.EventTypeAssistantDelta, ProviderModelVersion: &version}
	batch := transport.NewToolCallBatchRequestedEvent("chain-run", antigravity.Name, transport.ToolCallBatch{ToolCalls: []transport.ToolCallRequest{{ToolName: "synthetic.read", ArgsJSON: `{}`}, {ToolName: "synthetic.write", ArgsJSON: `{}`}}})
	batch.Usage, batch.ProviderModelVersion = &known, &version
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
		{"known_batch_cost_denied", []transport.TransportEvent{batch}, &known, true, true, "transport_error", 0, 0},
		{"conflicting_complete_usage", []transport.TransportEvent{finalEvent(&known, &version), errorEvent(&changed, &version)}, nil, true, false, "transport_error", 0, 0},
		{"true_zero_triple", []transport.TransportEvent{toolEvent(&zero, &version)}, &zero, true, true, "completed", 1, 0},
		{"missing_total_legacy_pair", []transport.TransportEvent{toolEvent(&legacy, &version)}, &legacy, true, false, "transport_error", 0, 0},
		{"missing_metadata", []transport.TransportEvent{toolEvent(nil, nil)}, nil, false, false, "transport_error", 0, 0},
		{"missing_version", []transport.TransportEvent{finalEvent(&known, nil)}, &known, false, true, "transport_error", 0, 0},
		{"provisional_version", []transport.TransportEvent{provisionalVersion, toolEvent(&known, nil)}, nil, false, false, "transport_error", 0, 0},
		{"missing_terminal_known", []transport.TransportEvent{finalEvent(&known, &version)}, &known, true, true, "transport_error", 0, 0},
		{"completed_final", []transport.TransportEvent{finalEvent(&known, &version), transport.NewTerminalEvent("chain-run", transport.EventTypeRunCompleted, antigravity.Name)}, &known, true, true, "completed", 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spy := &releaseChainV4StreamSpy{converterWorkerProvider: &converterWorkerProvider{}, events: tc.events}
			p, live, req, _ := releaseChainV4OwnedProviderFixture(t, spy)
			stream, e := p.StartRun(live, req)
			if e != nil {
				t.Fatal("current actual source/lease preflight denied synthetic stream", e)
			}
			tools, finals := 0, 0
			for event := range stream {
				if event.EventType == transport.EventTypeToolCallRequested || event.EventType == transport.EventTypeToolCallBatchRequested {
					tools++
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
			if e = p.window.pool.QueryRow(context.Background(), `SELECT outcome,input_tokens,output_tokens,total_tokens,model_version,admitted_at,finished_at,elapsed_ms,failure_stage,diagnostic_code FROM fixture_chain_v4_high_requests WHERE ordinal=521`).Scan(&outcome, &in, &out, &total, &observedVersion, &admitted, &finished, &elapsed, &stage, &code); e != nil {
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
			var attempts, rows int
			if p.window.pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_chain_v4_high_window`).Scan(&attempts) != nil || p.window.pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_chain_v4_high_requests`).Scan(&rows) != nil || attempts != 521 || rows != 1 {
				t.Fatal("failed terminal refunded or duplicated the entry")
			}
			// Terminal observation is immutable, including a known zero triple.
			p.finish(521, "completed", &changed, &version, "stream", nil)
			var retainedOutcome string
			var retainedTotal *int64
			if p.window.pool.QueryRow(context.Background(), `SELECT outcome,total_tokens FROM fixture_chain_v4_high_requests WHERE ordinal=521`).Scan(&retainedOutcome, &retainedTotal) != nil || retainedOutcome != outcome || (retainedTotal != nil) != (total != nil) || retainedTotal != nil && *retainedTotal != *total {
				t.Fatal("terminal observation overwritten")
			}
		})
	}
}
