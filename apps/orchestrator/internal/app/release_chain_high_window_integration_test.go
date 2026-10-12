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

func TestReleaseChainHighWindowSharedCeilingRestartAndExpiryIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	w, err := createChainHighWindow(ctx, pool, strings.Repeat("a", 64), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	guard := func(context.Context, pgx.Tx, string) (bool, error) { return true, nil }
	var calls atomic.Int32
	var workers sync.WaitGroup
	for i := 0; i < 48; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			if _, _, e := w.reserve(ctx, []string{"restart_sent", "lost_ack", "owner_stop"}[i%3], []string{"start", "continue", "submit"}[i%3], "synthetic-root", "synthetic-window-row", i, guard); e == nil {
				calls.Add(1)
			}
		}(i)
	}
	workers.Wait()
	if calls.Load() != chainHighNewCap {
		t.Fatal("shared concurrent ceiling lost", calls.Load())
	}
	var attempts, rows int
	var expiry time.Time
	if pool.QueryRow(ctx, `SELECT attempts,expires_at FROM fixture_chain_high_window`).Scan(&attempts, &expiry) != nil || attempts != chainHighCeiling || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_chain_high_requests`).Scan(&rows) != nil || rows != chainHighNewCap {
		t.Fatal("shared spending rows lost")
	}
	reopened, e := createChainHighWindow(ctx, pool, strings.Repeat("a", 64), 45*time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	var retained time.Time
	if pool.QueryRow(ctx, `SELECT expires_at FROM fixture_chain_high_window`).Scan(&retained) != nil || !retained.Equal(expiry) {
		t.Fatal("restart extended deadline")
	}
	if _, _, e = reopened.reserve(ctx, "restart_sent", "start", "new-root", "new-run", nil, guard); e == nil {
		t.Fatal("new root refilled spent allowance")
	}
	for _, q := range []string{`UPDATE fixture_chain_high_window SET attempts=480`, `UPDATE fixture_chain_high_window SET attempts=attempts+1,expires_at=expires_at+interval '1 hour'`, `DELETE FROM fixture_chain_high_requests`, `UPDATE fixture_chain_high_requests SET input_hash=repeat('b',64)`} {
		if _, e = pool.Exec(ctx, q); e == nil {
			t.Fatal("sealed history changed")
		}
	}
}

func TestReleaseChainHighWindowChecksDeadlineAfterCounterWaitIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	w, e := createChainHighWindow(ctx, pool, strings.Repeat("b", 64), 150*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `SELECT attempts FROM fixture_chain_high_window FOR UPDATE`); e != nil {
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
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_chain_high_window`).Scan(&attempts) != nil || attempts != chainHighBaseline || pool.QueryRow(ctx, `SELECT count(*) FROM fixture_chain_high_requests`).Scan(&rows) != nil || rows != 0 {
		t.Fatal("denied entry was charged")
	}
}

func TestReleaseChainHighWindowSharesCounterAcrossPrivateTaskSchemasIntegration(t *testing.T) {
	first, second := memorytest.Database(t), memorytest.Database(t)
	ctx := context.Background()
	var firstSchema string
	if first.QueryRow(ctx, `SELECT current_schema()`).Scan(&firstSchema) != nil {
		t.Fatal("schema absent")
	}
	shared := firstSchema + "_chain"
	t.Cleanup(func() { _, _ = first.Exec(ctx, `DROP SCHEMA IF EXISTS `+pgx.Identifier{shared}.Sanitize()+` CASCADE`) })
	w1, e := createChainHighWindowInSchema(ctx, first, strings.Repeat("d", 64), shared, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	w2, e := createChainHighWindowInSchema(ctx, second, strings.Repeat("d", 64), shared, 45*time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	guard := func(context.Context, pgx.Tx, string) (bool, error) { return true, nil }
	for i := 0; i < chainHighNewCap; i++ {
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
	if first.QueryRow(ctx, `SELECT attempts FROM `+w1.table("fixture_chain_high_window")).Scan(&attempts) != nil || second.QueryRow(ctx, `SELECT count(*) FROM `+w2.table("fixture_chain_high_requests")).Scan(&rows) != nil || attempts != chainHighCeiling || rows != chainHighNewCap {
		t.Fatal("shared cross-root counter inconsistent")
	}
}

type chainHighFailureSpy struct {
	*converterWorkerProvider
	calls atomic.Int32
}

func (p *chainHighFailureSpy) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	return nil, errors.New("synthetic failed start")
}
func (p *chainHighFailureSpy) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	return nil, errors.New("synthetic failed continue")
}
func (p *chainHighFailureSpy) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.calls.Add(1)
	return nil, errors.New("synthetic failed submit")
}

func TestReleaseChainHighAllFailedEntriesSpentButSourceDenialZeroIntegration(t *testing.T) {
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
	w, e := createChainHighWindow(ctx, pool, strings.Repeat("c", 64), time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	spy := &chainHighFailureSpy{converterWorkerProvider: &converterWorkerProvider{}}
	p := &chainHighProvider{ModelProvider: spy, window: w, scenario: "restart_sent", checker: &releaseChainEntryChecker{pool: pool, sources: sources, root: root.TaskID, document: refs[0], need: refs[1], spec: refs[2]}}
	req := transport.StartRunRequest{Context: transport.TransportRunContext{RunID: token.RunID, SessionKey: key, ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel}}
	if _, e = p.StartRun(live, req); e == nil {
		t.Fatal("synthetic failure lost")
	}
	if _, e = p.ContinueRun(live, transport.ContinueRunRequest{RunID: token.RunID}); e == nil {
		t.Fatal("synthetic continuation failure lost")
	}
	if _, e = p.SubmitToolResult(live, transport.SubmitToolResultRequest{RunID: token.RunID}); e == nil {
		t.Fatal("synthetic submit failure lost")
	}
	if _, e = sources.Correct(actor, catalog.Correction{SourceID: refs[0].ID, ExpectedRevision: refs[0].Revision, Title: "Current changed DATA", Body: "changed"}); e != nil {
		t.Fatal(e)
	}
	if _, e = p.StartRun(live, req); e == nil {
		t.Fatal("changed original source reached provider")
	}
	var attempts, failed, unknown int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_chain_high_window`).Scan(&attempts) != nil || pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE outcome='transport_error'),count(*) FILTER(WHERE input_tokens IS NULL AND output_tokens IS NULL AND total_tokens IS NULL) FROM fixture_chain_high_requests`).Scan(&failed, &unknown) != nil || attempts != 483 || failed != 3 || unknown != 3 || spy.calls.Load() != 3 {
		t.Fatal("failed entries refunded or denial charged", attempts, failed, unknown, spy.calls.Load())
	}
}
