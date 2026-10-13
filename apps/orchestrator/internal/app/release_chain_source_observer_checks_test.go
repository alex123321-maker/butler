package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReleaseChainV11SourceObserverPrivateDataAndFirstFailure(t *testing.T) {
	o := &releaseChainSourceObserver{}
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	ctx = budget.WithModelOperation(ctx, "synthetic-operation")
	ctx = ownership.WithToken(ctx, nil, ownership.Token{RunID: "owned-run", SessionKey: "owned-session", LeaseID: "owned-lease", FenceEpoch: 7})
	private := "PRIVATE_OBSERVER_VALUE"
	original := &pgconn.PgError{Code: "55P03", Message: private, Detail: private, Hint: private}
	err := o.observe(ctx, nil, "task_authority_watchdog", "owned-run", 750*time.Millisecond, func(marked context.Context) error {
		got, ok := ownership.TokenFromContext(marked)
		op, hasOp := budget.ModelOperationFromContext(marked)
		if !ok || got.RunID != "owned-run" || got.FenceEpoch != 7 || !hasOp || op != "synthetic-operation" {
			t.Fatal("observation changed owning context")
		}
		acquired := o.TraceAcquireStart(marked, nil, pgxpool.TraceAcquireStartData{})
		o.TraceAcquireEnd(acquired, nil, pgxpool.TraceAcquireEndData{})
		q := o.TraceQueryStart(marked, nil, pgx.TraceQueryStartData{SQL: "SELECT task_id FROM runs WHERE run_id=$1", Args: []any{private}})
		o.TraceQueryEnd(q, nil, pgx.TraceQueryEndData{Err: original})
		// Cleanup cannot overwrite the first actual failing query/category.
		q = o.TraceQueryStart(marked, nil, pgx.TraceQueryStartData{SQL: "rollback", Args: []any{private}})
		o.TraceQueryEnd(q, nil, pgx.TraceQueryEndData{Err: context.Canceled})
		return original
	})
	if err != original {
		t.Fatal("observer changed source error")
	}
	snapshot := o.snapshot()
	if len(snapshot.Failed) != 1 {
		t.Fatal("failure observation missing")
	}
	r := snapshot.Failed[0]
	if r.Phase != "query" || r.Family == nil || *r.Family != "runs" || r.QuerySHA == nil || r.Failure != "postgres_error" || r.SQLState == nil || *r.SQLState != "55P03" || r.Maximum == nil || *r.Maximum != 750 || r.TimeLeft == nil || *r.TimeLeft > 750 || r.Operation == nil || r.Acquisitions != 1 {
		t.Fatal("source failure phase or declared/current bound changed")
	}
	raw, _ := json.Marshal(snapshot)
	if strings.Contains(string(raw), private) || strings.Contains(string(raw), "SELECT") || strings.Contains(string(raw), "rollback") {
		t.Fatal("private data or SQL escaped")
	}
}

func TestReleaseChainV11SourceObserverUnmarkedHealthyAndPrequote(t *testing.T) {
	o, other := &releaseChainSourceObserver{}, &releaseChainSourceObserver{}
	ctx := context.Background()
	if o.TraceAcquireStart(ctx, nil, pgxpool.TraceAcquireStartData{}) != ctx || o.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SECRET", Args: []any{"SECRET"}}) != ctx {
		t.Fatal("unmarked query context modified")
	}
	o.TraceAcquireEnd(ctx, nil, pgxpool.TraceAcquireEndData{Err: errors.New("SECRET")})
	o.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: errors.New("SECRET")})
	if err := o.observe(ctx, nil, "source_check", "", 2*time.Second, func(marked context.Context) error {
		// A tracer belonging to another fixture cannot collect this context.
		other.TraceQueryStart(marked, nil, pgx.TraceQueryStartData{SQL: "SELECT run_id FROM runs"})
		other.TraceQueryEnd(marked, nil, pgx.TraceQueryEndData{Err: context.DeadlineExceeded})
		return nil
	}); err != nil || len(o.snapshot().Failed) != 0 || len(other.snapshot().Failed) != 0 {
		t.Fatal("healthy/unmarked/wrong observer data retained")
	}
	err := errors.New("PRIVATE_CHECK_ERROR")
	if got := o.observe(ctx, nil, "source_check", "", 2*time.Second, func(context.Context) error { return err }); got != err {
		t.Fatal("unquoted error changed")
	}
	r := o.snapshot().Failed[0]
	if r.Operation != nil || r.TimeLeft != nil || r.RunID != nil || r.Family != nil || r.QuerySHA != nil || r.Phase != "check_started" {
		t.Fatal("missing context measurements invented")
	}
}

// Real current source and watchdog delegates on a disposable two-connection
// pool. Deliberate connection exhaustion models a pre-query wait, not proof of
// the original557 cause. It never invokes a model, tool or delivery.
func TestReleaseChainV11SourceObserverPoolContentionIntegration(t *testing.T) {
	if os.Getenv("BUTLER_TEST_ACTIONAUTH_POSTGRES_URL") == "" {
		t.Skip("isolated actionauth PostgreSQL required")
	}
	o := &releaseChainSourceObserver{}
	pool, _ := e2eDatabaseWithTracer(t, o)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('observer-session','observer-owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('observer-run','observer-session','observer-input','model_running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp()); INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('observer-session','observer-run','observer-lease',1,clock_timestamp()+INTERVAL '1 hour')`); err != nil {
		t.Fatal("disposable owned setup unavailable")
	}
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "manifest"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(pool, manifest)
	live := ownership.WithToken(ctx, ownership.NewAuthority(pool), ownership.Token{SessionKey: "observer-session", RunID: "observer-run", LeaseID: "observer-lease", FenceEpoch: 1})
	live = budget.WithModelOperation(live, "observer-operation")
	observed := releaseChainObservedSources{converterSources: converterSources{sources, pool}, observer: o}
	if err := observed.PinTranscript(live, "observer-session", 0); err != nil {
		t.Fatal("current transcript pin unavailable", err)
	}
	if err := observed.CheckRunSources(live, "observer-run"); err != nil {
		t.Fatal("healthy current source rejected", err)
	}
	leases := releaseChainObservedLeases{PostgresLeaseManager: session.NewPostgresLeaseManager(pool), pool: pool, observer: o}
	if _, err := leases.GetLease(live, "observer-lease"); err != nil {
		t.Fatal("healthy current lease rejected", err)
	}
	tasks := releaseChainObservedTasks{Store: task.NewStore(pool), pool: pool, observer: o}
	if err := tasks.ValidateRunAuthority(live, "observer-run"); err != nil || len(o.snapshot().Failed) != 0 {
		t.Fatal("healthy current authority rejected or failure invented", err)
	}
	first, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	second, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
	for _, role := range []string{"source_check", "lease_watchdog", "task_authority_watchdog"} {
		bounded, stop := context.WithTimeout(live, 80*time.Millisecond)
		var err error
		switch role {
		case "source_check":
			err = observed.CheckRunSources(bounded, "observer-run")
		case "lease_watchdog":
			_, err = leases.GetLease(bounded, "observer-lease")
		case "task_authority_watchdog":
			err = tasks.ValidateRunAuthority(bounded, "observer-run")
		}
		stop()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("actual bounded pool contention was not retained")
		}
	}
	snapshot := o.snapshot()
	if len(snapshot.Failed) != 3 || snapshot.Dropped != 0 {
		t.Fatal("failure-only trace inventory changed")
	}
	for i, r := range snapshot.Failed {
		maximum := int64(750)
		if i == 0 {
			maximum = 2000
		}
		if r.Phase != "pool_acquire" || r.Family != nil || r.QuerySHA != nil || r.Failure != "deadline_exceeded" || r.Acquisitions != 1 || r.AcquireMillis < 60 || r.Before.Acquired != 2 || r.Before.Max != 2 || r.Maximum == nil || *r.Maximum != maximum || r.TimeLeft == nil || *r.TimeLeft > 80 || r.Operation == nil || *r.Operation != "observer-operation" || r.RunID == nil || *r.RunID != "observer-run" {
			t.Fatal("actual pre-query wait not identified safely")
		}
	}
}
