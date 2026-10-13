package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5"
)

func TestReleaseChainV9LanePolicyRetainsOldWindows(t *testing.T) {
	policy := releaseChainV9Policy()
	if !policy.valid() || !policy.SerializedEntries || !policy.ProposalRetention || policy.attemptMillis() != 300000 || policy.caseMillis() != 2400000 || policy.globalMillis() != 7200000 || releaseChainHighEntryLaneFor(policy) == nil {
		t.Fatal("unfixed new provider lane")
	}
	for _, old := range []releaseChainHighWindowPolicy{{}, releaseChainV7Policy(), releaseChainV8Policy()} {
		if old.SerializedEntries || strings.Contains(string(old.json()), "provider_entries_serialized") || releaseChainHighEntryLaneFor(old) != nil {
			t.Fatal("old window serialization changed")
		}
	}
}

func TestReleaseChainV9ReleasedHolderDoesNotDenyWaitingChild(t *testing.T) {
	lane := releaseChainHighEntryLaneFor(releaseChainV9Policy())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	parent := ownership.WithToken(ctx, nil, ownership.Token{RunID: "parent", SessionKey: "parent-session", LeaseID: "parent-lease", FenceEpoch: 1})
	child := ownership.WithToken(ctx, nil, ownership.Token{RunID: "child", SessionKey: "child-session", LeaseID: "child-lease", FenceEpoch: 1})
	var checks atomic.Int32
	entered, finished := make(chan struct{}), make(chan struct{})
	release, err := lane.acquire(parent, func(guard context.Context) error {
		token, ok := ownership.TokenFromContext(guard)
		if !ok || token.RunID != "parent" {
			return errors.New("wrong holder proof")
		}
		if checks.Add(1) > 2 {
			close(entered)
			select {
			case <-finished:
				return errors.New("completed old holder")
			case <-guard.Done():
				return guard.Err()
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result := make(chan error, 1)
	go func() {
		free, e := lane.acquire(child, func(guard context.Context) error {
			token, ok := ownership.TokenFromContext(guard)
			if !ok || token.RunID != "child" {
				return errors.New("holder authority leaked into queued caller")
			}
			return nil
		})
		if free != nil {
			free()
		}
		result <- e
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("queued holder check not entered")
	}
	release()
	close(finished)
	select {
	case e := <-result:
		if e != nil {
			t.Fatal("released old holder denied live child", e)
		}
	case <-ctx.Done():
		t.Fatal("live child did not acquire after release")
	}
}

// Synthetic counter rows and two independent callers isolate the guard/CAS
// race. The actual production lineage checks are exercised below. Native calls0.
func TestReleaseChainV9PendingHistoryAndAtomicAdmissionIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var schema string
	if pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema) != nil {
		t.Fatal("schema missing")
	}
	w, err := createReleaseChainHighWindowWithPolicy(ctx, pool, strings.Repeat("a", 64), strings.Repeat("b", 64), 544, schema, time.Now().UTC().Add(time.Minute), 71, true, releaseChainV9Policy())
	if err != nil {
		t.Fatal(err)
	}
	entered, unblock, secondEntered := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var firstOnce, secondOnce sync.Once
	guard := func(ctx context.Context, tx pgx.Tx, run string) (bool, error) {
		if run == "first" {
			firstOnce.Do(func() {
				close(entered)
				select {
				case <-unblock:
				case <-ctx.Done():
				}
			})
		} else {
			secondOnce.Do(func() { close(secondEntered) })
		}
		return false, releaseChainV6HistoryAllowsAdmission(ctx, tx, w, "normal_sent", "start", "root", run, "synthetic-operation")
	}
	type admission struct {
		ordinal int
		err     error
	}
	first, second := make(chan admission, 1), make(chan admission, 1)
	go func() {
		n, _, e := w.reserve(ctx, "normal_sent", "start", "root", "first", map[string]bool{"synthetic": true}, guard)
		first <- admission{n, e}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first guard not reached")
	}
	go func() {
		n, _, e := w.reserve(ctx, "normal_sent", "start", "root", "second", map[string]bool{"synthetic": true}, guard)
		second <- admission{n, e}
	}()
	select {
	case <-secondEntered:
		t.Fatal("second checked history before singleton lock")
	case <-time.After(75 * time.Millisecond):
	}
	close(unblock)
	one, two := <-first, <-second
	if one.err != nil || one.ordinal != 545 || !errors.Is(two.err, errNativeFixtureDenied) || two.ordinal != 0 {
		t.Fatal("pending row did not deny concurrent admission", one, two)
	}
	// Exact reproduction of the old pending-history denial, followed by a
	// successfully retained packet. No policy permits an unfinished request.
	version, phase := "gemini-3.8-flash-n", transport.ProviderStageStream
	p := &releaseChainV5Provider{window: w}
	if err = p.finish(545, "completed", &transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 205}, &version, "stream", nil, &releaseChainV5Observations{providerStage: &phase}); err != nil {
		t.Fatal(err)
	}
	n, _, err := w.reserve(ctx, "normal_sent", "start", "root", "second", map[string]bool{"synthetic": true}, guard)
	if err != nil || n != 546 {
		t.Fatal("measured parent packet did not unblock following admission", n, err)
	}
	if err = p.finish(546, "timed_out", nil, nil, "stream", transport.NormalizeError(context.DeadlineExceeded, "antigravity"), &releaseChainV5Observations{providerStage: &phase}); err != nil {
		t.Fatal(err)
	}
	if n, _, err = w.reserve(ctx, "normal_sent", "start", "root", "third", map[string]bool{"synthetic": true}, guard); !errors.Is(err, errNativeFixtureDenied) || n != 0 {
		t.Fatal("closed unproved unknown timeout admitted another request", n, err)
	}
}

func TestReleaseChainV9QueuedLaneRechecksCurrentProofIntegration(t *testing.T) {
	for _, kind := range []string{"owner_stop", "source_changed", "lease_expired"} {
		t.Run(kind, func(t *testing.T) {
			spy := &releaseChainV5FailureSpy{converterWorkerProvider: &converterWorkerProvider{}}
			p, live, _, actor := releaseChainV5OwnedProviderFixture(t, spy)
			lane := releaseChainHighEntryLaneFor(releaseChainV9Policy())
			current := func(ctx context.Context) error { _, e := p.checker.guard(ctx, "chain-run"); return e }
			release, err := lane.acquire(live, current)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			waiting := make(chan error, 1)
			go func() {
				free, e := lane.acquire(live, current)
				if free != nil {
					free()
				}
				waiting <- e
			}()
			select {
			case e := <-waiting:
				t.Fatal("queued caller escaped occupied lane", e)
			case <-time.After(75 * time.Millisecond):
			}
			switch kind {
			case "owner_stop":
				leases := session.NewPostgresLeaseManager(p.window.pool)
				controller := task.NewController(task.NewStore(p.window.pool), leases.Authority(), leases, run.NewPostgresRepository(p.window.pool), approvals.NewPostgresRepository(p.window.pool), durableinput.NewStore(p.window.pool), transcript.NewStore(p.window.pool))
				if _, err = controller.Apply(actor, task.ControlRequest{TaskID: p.checker.root, ExpectedRevision: 1, Kind: task.ControlStop}); err != nil {
					t.Fatal(err)
				}
			case "source_changed":
				ref := p.checker.document
				if _, err = p.checker.sources.Correct(actor, catalog.Correction{SourceID: ref.ID, ExpectedRevision: ref.Revision, Title: "Current changed DATA", Body: "changed"}); err != nil {
					t.Fatal(err)
				}
			case "lease_expired":
				if _, err = p.window.pool.Exec(context.Background(), `UPDATE session_ownership SET expires_at=clock_timestamp()-interval '1 second' WHERE run_id='chain-run'`); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case e := <-waiting:
				if e == nil {
					t.Fatal("revoked queued proof accepted")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("queued proof was not rechecked")
			}
			var attempts int
			if p.window.pool.QueryRow(context.Background(), `SELECT attempts FROM `+p.window.table("fixture_chain_v5_high_window")).Scan(&attempts) != nil || attempts != p.window.baseline || spy.calls.Load() != 0 {
				t.Fatal("waiting/revoked entry charged native counter or called provider")
			}
		})
	}
}

func TestReleaseChainV9DistinctParentChildLaneIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	child, childCtx, _ := f.testedChild(t)
	if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='model_running',current_state='model_running' WHERE run_id IN('converter-parent-run','converter-child-run')`); err != nil {
		t.Fatal(err)
	}
	var parentToken ownership.Token
	if f.f.pool.QueryRow(f.f.ctx, `SELECT session_key,run_id,lease_id,fence_epoch FROM session_ownership WHERE run_id=$1`, f.parentRun).Scan(&parentToken.SessionKey, &parentToken.RunID, &parentToken.LeaseID, &parentToken.FenceEpoch) != nil {
		t.Fatal("parent ownership missing")
	}
	parentCtx := ownership.WithToken(f.f.ctx, f.f.leases.Authority(), parentToken)
	childToken, ok := ownership.TokenFromContext(childCtx)
	if !ok || childToken.RunID == parentToken.RunID || childToken.SessionKey == parentToken.SessionKey {
		t.Fatal("test did not create distinct current ownership")
	}
	tx, err := f.f.pool.Begin(parentCtx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.f.sources.AccessTx(parentCtx, tx, []catalog.SourceRef{f.document, f.needRef, f.specRef}, "memory_read")
	if err != nil {
		_ = tx.Rollback(parentCtx)
		t.Fatal(err)
	}
	if err = tx.Commit(parentCtx); err != nil {
		t.Fatal(err)
	}
	checker := &releaseChainEntryChecker{pool: f.f.pool, sources: f.f.sources, root: f.f.parent.TaskID, document: f.document, need: f.needRef, spec: f.specRef}
	lane := releaseChainHighEntryLaneFor(releaseChainV9Policy())
	release, err := lane.acquire(parentCtx, func(ctx context.Context) error { _, e := checker.guard(ctx, parentToken.RunID); return e })
	if err != nil {
		t.Fatal("current parent cannot hold lane", err)
	}
	defer release()
	var schema string
	if f.f.pool.QueryRow(f.f.ctx, `SELECT current_schema()`).Scan(&schema) != nil {
		t.Fatal("schema unavailable")
	}
	w, err := createReleaseChainHighWindowWithPolicy(f.f.ctx, f.f.pool, strings.Repeat("a", 64), strings.Repeat("b", 64), 544, schema, time.Now().UTC().Add(time.Minute), 71, true, releaseChainV9Policy())
	if err != nil {
		t.Fatal(err)
	}
	version := "gemini-3.8-flash-n"
	usage := transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 205}
	event := transport.NewToolCallRequestedEvent(childToken.RunID, antigravity.Name, transport.ToolCallRequest{ToolCallRef: "synthetic-child-discovery", ToolName: "converters.discover", ArgsJSON: `{}`})
	event.Usage, event.ProviderModelVersion = &usage, &version
	spy := &releaseChainV6StageSpy{releaseChainV5StreamSpy: &releaseChainV5StreamSpy{converterWorkerProvider: &converterWorkerProvider{}, events: []transport.TransportEvent{event}}, stage: transport.ProviderStageStream}
	p := &releaseChainV5Provider{ModelProvider: spy, checker: checker, window: w, scenario: "normal_sent", expectedVersion: version, entryLane: lane, proposals: releaseChainProposalSinkFor(true, t.TempDir(), "normal_sent")}
	type entered struct {
		stream transport.EventStream
		err    error
	}
	result := make(chan entered, 1)
	req := transport.StartRunRequest{Context: transport.TransportRunContext{RunID: childToken.RunID, SessionKey: child.SessionKey, ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel}}
	go func() { s, e := p.StartRun(childCtx, req); result <- entered{s, e} }()
	select {
	case r := <-result:
		t.Fatal("child did not wait with its own proof", r.err)
	case <-time.After(100 * time.Millisecond):
	}
	if spy.calls.Load() != 0 {
		t.Fatal("queued child invoked provider")
	}
	release()
	var actual entered
	select {
	case actual = <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("live child did not acquire released lane")
	}
	if actual.err != nil {
		t.Fatal("distinct child proof rejected after parent release", actual.err)
	}
	tools := 0
	for e := range actual.stream {
		if e.EventType == transport.EventTypeToolCallRequested {
			tools++
		}
	}
	var ownChild bool
	if f.f.pool.QueryRow(f.f.ctx, `SELECT outcome='completed' AND run_id=$1 AND NOT is_parent AND input_tokens=3 AND output_tokens=2 AND total_tokens=205 FROM `+w.table("fixture_chain_v5_high_requests")+` WHERE ordinal=545`, childToken.RunID).Scan(&ownChild) != nil || !ownChild || tools != 1 || spy.calls.Load() != 1 {
		t.Fatal("queued child's original owning entry or measured packet changed")
	}
}
