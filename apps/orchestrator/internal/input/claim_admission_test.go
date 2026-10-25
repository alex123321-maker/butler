package input

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/memorytest"
)

// This is public worker lifecycle coverage. No model, Broker, research holdout,
// credential source, task authority override or native accounting is involved.
type claimFenceDelayedExecutor struct {
	started chan context.Context
	release chan struct{}
	exited  chan error
	calls   atomic.Int32
}

func (e *claimFenceDelayedExecutor) Execute(ctx context.Context, _ ingress.InputEvent) (*flow.ExecutionResult, error) {
	e.calls.Add(1)
	select {
	case e.started <- ctx:
	case <-ctx.Done():
		e.exited <- ctx.Err()
		return nil, ctx.Err()
	}
	select {
	case <-e.release:
	case <-ctx.Done():
		e.exited <- ctx.Err()
		return nil, ctx.Err()
	}
	e.exited <- ctx.Err()
	return &flow.ExecutionResult{CurrentState: commonv1.RunState_RUN_STATE_COMPLETED}, nil
}

func claimFenceEvent(id, key string) ingress.InputEvent {
	return ingress.InputEvent{
		EventID: id, IdempotencyKey: id, SessionKey: key, Source: "web",
		EventType:   runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		PayloadJSON: `{"text":"public input lifecycle","user_id":"claim-fence-owner"}`,
		CreatedAt:   time.Now().UTC(),
	}
}

func TestClaimAdmissionDrainOwnsDelayedProcessIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	store := NewStore(pool)
	executor := &claimFenceDelayedExecutor{
		started: make(chan context.Context, 4), release: make(chan struct{}), exited: make(chan error, 4),
	}
	var mu sync.Mutex
	closed := false
	denied := make(chan struct{})
	var deniedOnce sync.Once
	admit := func(context.Context) (func(), error) {
		mu.Lock()
		if closed {
			mu.Unlock()
			deniedOnce.Do(func() { close(denied) })
			return nil, ErrClaimAdmissionClosed
		}
		return mu.Unlock, nil
	}
	service, err := NewServiceWithClaimAdmission(store, session.NewPostgresRepository(pool), executor, nil, nil, admit)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Execute(ctx, claimFenceEvent("public-owning-first", "claim-fence:owning"))
	if err != nil {
		t.Fatal(err)
	}
	workCtx, stop := context.WithCancel(ctx)
	runDone := make(chan error, 1)
	go func() { runDone <- service.Run(workCtx) }()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(executor.release) }) }
	var joined bool
	t.Cleanup(func() {
		release()
		stop()
		if !joined {
			select {
			case <-runDone:
			case <-time.After(5 * time.Second):
				t.Error("public Input workers did not join during cleanup")
			}
		}
	})
	var owningCtx context.Context
	select {
	case owningCtx = <-executor.started:
	case <-ctx.Done():
		t.Fatal("first claimed process did not start")
	}
	claimed, err := store.Get(ctx, first.InputID)
	if err != nil || claimed.Status != "running" || claimed.ClaimID == "" || claimed.Attempts != 1 {
		t.Fatal("delayed executor lacks its real owning claim", err)
	}
	// This successor already exists before closure, but the owning FIFO head is
	// still executing. The second worker has no claimable work in this session.
	queued, err := service.Execute(ctx, claimFenceEvent("public-queued-successor", "claim-fence:owning"))
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	closed = true
	mu.Unlock()
	// Acceptance and claiming are different contracts. A new ready-session row
	// arriving after closure must also remain unclaimed, as can late automatic
	// work admitted by another component while an old process is finishing.
	late, err := service.Execute(ctx, claimFenceEvent("public-late-other-session", "claim-fence:other"))
	if err != nil {
		t.Fatal(err)
	}
	queuedBefore, err := store.Get(ctx, queued.InputID)
	if err != nil {
		t.Fatal(err)
	}
	lateBefore, err := store.Get(ctx, late.InputID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-denied:
	case <-ctx.Done():
		t.Fatal("idle second worker did not observe closed admission")
	}
	// The executor is held by a channel, so this is a deterministic negative
	// drain check: the bounded wait must expire, never report a completed join.
	probe, stopProbe := context.WithTimeout(ctx, 100*time.Millisecond)
	err = service.WaitClaimWorkers(probe)
	stopProbe()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("claim workers reported drained before the owning process finished", err)
	}
	if owningCtx.Err() != nil || workCtx.Err() != nil {
		t.Fatal("closing claim admission cancelled an already claimed process")
	}
	release()
	if err = service.WaitClaimWorkers(ctx); err != nil {
		t.Fatal("owning process failed to finish before claim-worker join", err)
	}
	if err = <-executor.exited; err != nil {
		t.Fatal("owning executor completed with a cancelled context", err)
	}
	completed, err := store.Get(ctx, first.InputID)
	if err != nil || completed.Status != "completed" || completed.ClaimID != "" || completed.ClaimExpiresAt != nil || completed.Attempts != 1 {
		t.Fatal("worker join preceded ordinary durable claim finish", err)
	}
	for _, before := range []Record{queuedBefore, lateBefore} {
		after, err := store.Get(ctx, before.InputID)
		if err != nil || after.Status != "queued" || after.Attempts != 0 || after.RunID != "" || after.ClaimID != "" || after.ClaimExpiresAt != nil || after.Event.PayloadJSON != before.Event.PayloadJSON || !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Fatal("closed fence claimed, expired or rewrote a queued input", err)
		}
	}
	if executor.calls.Load() != 1 {
		t.Fatal("closed admission allowed a queued executor callback")
	}
	if workCtx.Err() != nil {
		t.Fatal("claim-worker drain cancelled the usual Run context")
	}
	stop()
	if err = <-runDone; err != nil {
		t.Fatal("Run failed after already drained claim workers", err)
	}
	joined = true
	t.Log("two actual Input workers: delayed owning process finishes its durable claim before drain; same-session successor and late independent row remain queued; no cancellation, expiry rewrite, model or replay")
}

func TestClaimAdmissionCallbackFailureVisibleIntegration(t *testing.T) {
	for _, name := range []string{"nil_release", "callback_failure"} {
		t.Run(name, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			store := NewStore(pool)
			failure := errors.New("public claim fence proof unavailable")
			admit := func(context.Context) (func(), error) {
				if name == "callback_failure" {
					return nil, failure
				}
				return nil, nil
			}
			executor := &claimFenceDelayedExecutor{started: make(chan context.Context, 2), release: make(chan struct{}), exited: make(chan error, 2)}
			service, err := NewServiceWithClaimAdmission(store, session.NewPostgresRepository(pool), executor, nil, nil, admit)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := service.Execute(ctx, claimFenceEvent("public-fence-error", "claim-fence:error"))
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.Get(ctx, receipt.InputID)
			if err != nil {
				t.Fatal(err)
			}
			workCtx, stop := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() { done <- service.Run(workCtx) }()
			joined := false
			t.Cleanup(func() {
				stop()
				if !joined {
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("failed claim worker did not join during cleanup")
					}
				}
			})
			drainErr := service.WaitClaimWorkers(ctx)
			if drainErr == nil || errors.Is(drainErr, ErrClaimAdmissionClosed) || (name == "callback_failure" && !errors.Is(drainErr, failure)) {
				t.Fatal("callback proof failure was hidden as normal admission closure", drainErr)
			}
			after, err := store.Get(ctx, receipt.InputID)
			if err != nil || after.Status != "queued" || after.Attempts != 0 || after.ClaimID != "" || !after.UpdatedAt.Equal(before.UpdatedAt) || executor.calls.Load() != 0 {
				t.Fatal("invalid claim hook still reached queue/executor mutation", err)
			}
			stop()
			select {
			case runErr := <-done:
				joined = true
				if runErr == nil || (name == "callback_failure" && !errors.Is(runErr, failure)) {
					t.Fatal("Run concealed its claim-worker proof failure", runErr)
				}
			case <-ctx.Done():
				t.Fatal("failed claim workers did not join")
			}
		})
	}
}

func TestClaimAdmissionConstructorIsExplicit(t *testing.T) {
	if service, err := NewServiceWithClaimAdmission(nil, nil, nil, nil, nil, nil); err == nil || service != nil {
		t.Fatal("optional claim lifecycle accepted a missing trusted callback")
	}
	ordinary := NewService(nil, nil, nil, nil, nil)
	if ordinary.ClaimAdmissionFailed() != nil || ordinary.ClaimWorkersDone() != nil {
		t.Fatal("ordinary construction acquired optional worker broadcasts")
	}
	if err := ordinary.WaitClaimWorkers(t.Context()); err == nil {
		t.Fatal("ordinary unfenced service acquired the optional drain lifecycle")
	}
}

func TestClaimAdmissionFailureBroadcastPrecedesOwningDrainIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	store := NewStore(pool)
	executor := &claimFenceDelayedExecutor{started: make(chan context.Context, 4), release: make(chan struct{}), exited: make(chan error, 4)}
	failure := errors.New("public second-worker admission failure")
	var fail atomic.Bool
	service, err := NewServiceWithClaimAdmission(store, session.NewPostgresRepository(pool), executor, nil, nil, func(context.Context) (func(), error) {
		if fail.Load() {
			return nil, failure
		}
		return func() {}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Execute(ctx, claimFenceEvent("public-broadcast-owner", "claim-fence:broadcast-owner"))
	if err != nil {
		t.Fatal(err)
	}
	workCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- service.Run(workCtx) }()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(executor.release) }) }
	joined := false
	t.Cleanup(func() {
		release()
		stop()
		if !joined {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("broadcast test workers did not join")
			}
		}
	})
	var owning context.Context
	select {
	case owning = <-executor.started:
	case <-ctx.Done():
		t.Fatal("owning worker did not enter delayed executor")
	}
	fail.Store(true)
	select {
	case <-service.ClaimAdmissionFailed():
	case <-ctx.Done():
		t.Fatal("idle worker failure was not broadcast while owner remained alive")
	}
	if !errors.Is(service.ClaimAdmissionError(), failure) || owning.Err() != nil {
		t.Fatal("broadcast lost the exact fixed error or cancelled owning execution")
	}
	select {
	case <-service.ClaimWorkersDone():
		t.Fatal("failure notification was confused with completion of all workers")
	default:
	}
	late, err := service.Execute(ctx, claimFenceEvent("public-broadcast-tail", "claim-fence:broadcast-other"))
	if err != nil {
		t.Fatal(err)
	}
	release()
	if err = service.WaitClaimWorkers(ctx); !errors.Is(err, failure) {
		t.Fatal("final drain did not retain the early failure", err)
	}
	completed, err := store.Get(ctx, first.InputID)
	if err != nil || completed.Status != "completed" || completed.ClaimID != "" || completed.ClaimExpiresAt != nil {
		t.Fatal("broadcast cancelled ordinary owning finish", err)
	}
	tail, err := store.Get(ctx, late.InputID)
	if err != nil || tail.Status != "queued" || tail.Attempts != 0 || tail.ClaimID != "" || executor.calls.Load() != 1 {
		t.Fatal("another claim was admitted after the failure broadcast", err)
	}
	stop()
	if err = <-done; !errors.Is(err, failure) {
		t.Fatal("Run concealed the broadcast failure", err)
	}
	joined = true
}

func TestClaimAdmissionClosedSkipsQueuedExpiryIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	store := NewStore(pool)
	service, err := NewServiceWithClaimAdmission(store, session.NewPostgresRepository(pool), nil, nil, nil, func(context.Context) (func(), error) {
		return nil, ErrClaimAdmissionClosed
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Execute(ctx, claimFenceEvent("public-expired-tail", "claim-fence:expired"))
	if err != nil {
		t.Fatal(err)
	}
	// Public setup fault on this new isolated test row only, not cleanup or a
	// claim reset: if ExpireQueued were wrongly reached, this row would change.
	if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, receipt.InputID); err != nil {
		t.Fatal(err)
	}
	before, err := store.Get(ctx, receipt.InputID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.claim(ctx); !errors.Is(err, ErrClaimAdmissionClosed) {
		t.Fatal("closed fence did not deny claim before expiry maintenance", err)
	}
	after, err := store.Get(ctx, receipt.InputID)
	if err != nil || after.Status != "queued" || after.Attempts != 0 || !after.UpdatedAt.Equal(before.UpdatedAt) || after.ErrorCode != before.ErrorCode {
		t.Fatal("closed fence still executed ExpireQueued or changed the input", err)
	}
}

func TestClaimAdmissionFailureWhileOtherCallbackWaitsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	store := NewStore(pool)
	executor := &claimFenceDelayedExecutor{started: make(chan context.Context, 2), release: make(chan struct{}), exited: make(chan error, 2)}
	entered, allowReturn := make(chan struct{}), make(chan struct{})
	var calls, released atomic.Int32
	failure := errors.New("public admission failure while other callback waits")
	service, err := NewServiceWithClaimAdmission(store, session.NewPostgresRepository(pool), executor, nil, nil, func(ctx context.Context) (func(), error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-allowReturn:
				return func() { released.Add(1) }, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		<-entered
		return nil, failure
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Execute(ctx, claimFenceEvent("public-at-fence", "claim-fence:at-fence"))
	if err != nil {
		t.Fatal(err)
	}
	// An expired public setup row also detects an erroneous ExpireQueued write,
	// independently of whether a later Store.Claim could start the executor.
	if _, err = pool.Exec(ctx, `UPDATE accepted_inputs SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE input_id=$1`, receipt.InputID); err != nil {
		t.Fatal(err)
	}
	before, err := store.Get(ctx, receipt.InputID)
	if err != nil {
		t.Fatal(err)
	}
	workCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- service.Run(workCtx) }()
	var once sync.Once
	open := func() { once.Do(func() { close(allowReturn) }) }
	joined := false
	t.Cleanup(func() {
		open()
		stop()
		if !joined {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("workers waiting at admission fence did not join")
			}
		}
	})
	select {
	case <-service.ClaimAdmissionFailed():
	case <-ctx.Done():
		t.Fatal("second callback did not broadcast failure")
	}
	if calls.Load() != 2 || released.Load() != 0 {
		t.Fatal("public interleaving was not held inside first callback")
	}
	open()
	if err = service.WaitClaimWorkers(ctx); !errors.Is(err, failure) {
		t.Fatal("drain lost callback failure", err)
	}
	after, err := store.Get(ctx, receipt.InputID)
	if err != nil || after.Status != "queued" || after.Attempts != 0 || after.ClaimID != "" || !after.UpdatedAt.Equal(before.UpdatedAt) || executor.calls.Load() != 0 || released.Load() != 1 {
		t.Fatal("callback admitted expiry/claim after failure broadcast, or failed to release fence", err)
	}
	stop()
	if err = <-done; !errors.Is(err, failure) {
		t.Fatal("Run concealed callback failure", err)
	}
	joined = true
}
