package app

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
)

type behavior76TerminalAuthorityObservation struct {
	run string
	err error
}

// This public diagnostic embeds the ordinary prepared/real Store, including
// its optional authority interfaces. It neither changes SQL nor grants power.
type behavior76TerminalHoldTasks struct {
	*behavior76PreparedTasks
	parent       context.Context
	committed    chan string
	release      chan struct{}
	observations chan behavior76TerminalAuthorityObservation
	once         sync.Once
	committedRun atomic.Value
}

func (s *behavior76TerminalHoldTasks) SaveRunOutcome(ctx context.Context, run, state, note string) error {
	if err := s.Store.SaveRunOutcome(ctx, run, state, note); err != nil {
		return err
	}
	if state != "failed" {
		return nil
	}
	s.once.Do(func() { s.committedRun.Store(run); s.committed <- run })
	select {
	case <-s.release:
		return nil
	case <-s.parent.Done():
		return s.parent.Err()
	}
}

func (s *behavior76TerminalHoldTasks) ValidateRunAuthority(ctx context.Context, run string) error {
	err := s.Store.ValidateRunAuthority(ctx, run)
	if committed, ok := s.committedRun.Load().(string); ok && committed == run {
		select {
		case s.observations <- behavior76TerminalAuthorityObservation{run: run, err: err}:
		default:
		}
	}
	return err
}

type behavior76TerminalFailureProvider struct {
	*initiativeProjectScriptedProvider
	started atomic.Int32
	cancel  chan transport.CancelRunRequest
}

func (p *behavior76TerminalFailureProvider) StartRun(_ context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.started.Add(1)
	return initiativeProjectStream(
		transport.NewEvent(q.Context.RunID, transport.EventTypeRunStarted, p.Name()),
		transport.NewTransportErrorEvent(q.Context.RunID, p.Name(), &transport.Error{Type: transport.ErrorTypeTransportConnectionError, Message: "public synthetic transport failure"}),
	), nil
}

func (p *behavior76TerminalFailureProvider) CancelRun(_ context.Context, q transport.CancelRunRequest) (*transport.TransportEvent, error) {
	select {
	case p.cancel <- q:
	default:
	}
	event := transport.NewTerminalEvent(q.RunID, transport.EventTypeRunCancelled, p.Name())
	return &event, nil
}

// This opt-in, zero-native diagnostic documents the OLD bug. Its success means
// the causal self-revocation was observed, not that acceptance passed. It is
// excluded from ordinary Public selectors and must not survive as a product
// regression requiring the bug after its production repair.
func TestBehavior76TerminalObserverLegacyDiagnosticIntegration(t *testing.T) {
	if os.Getenv("BUTLER_BEHAVIOR76_TERMINAL_OBSERVER_DIAGNOSTIC") != "reproduce-old-self-revocation-v1" {
		t.Skip("explicit old-core public diagnostic only")
	}
	if os.Getenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE") != "disabled" {
		t.Fatal("native must be disabled")
	}
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	f := newBehavior76Fixture(t)
	episode := behavior76DevelopmentEpisode()
	episode.Initial.Tasks[0].ExecutionPolicy = "normal"
	f.seed(episode)
	taskID, sessionKey := f.lookup("task", "A"), f.lookup("session", "A")
	before, err := f.tasks.Get(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	tasks := &behavior76TerminalHoldTasks{behavior76PreparedTasks: newBehavior76PreparedTasks(f), parent: ctx, committed: make(chan string, 1), release: make(chan struct{}), observations: make(chan behavior76TerminalAuthorityObservation, 4)}
	var release sync.Once
	unblock := func() { release.Do(func() { close(tasks.release) }) }
	defer unblock()
	provider := &behavior76TerminalFailureProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, cancel: make(chan transport.CancelRunRequest, 1)}
	core := behavior76CoreWithAssembler(f, provider, f.mandate("A"), tasks)
	inputs := input.NewStore(f.pool)
	worker := input.NewService(inputs, session.NewPostgresRepository(f.pool), core, run.NewService(run.NewPostgresRepository(f.pool), nil), nil)
	accepted, err := worker.Execute(ctx, ingress.InputEvent{EventID: "public-terminal-observer-diagnostic", IdempotencyKey: "public-terminal-observer-diagnostic", SessionKey: sessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: behavior76JSON(map[string]any{"text": "Public synthetic failure; no external effect.", "user_id": behavior76Owner, "task_id": taskID}), CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	live, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(live) }()
	defer func() {
		unblock()
		stop()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("public worker not joined")
		}
	}()
	var runID string
	select {
	case runID = <-tasks.committed:
	case <-ctx.Done():
		t.Fatal("ordinary failed outcome did not commit", ctx.Err())
	}
	var status, runState string
	var revision int64
	if err = f.pool.QueryRow(ctx, `SELECT t.status,t.revision,r.current_state FROM tasks t JOIN runs r ON r.task_id=t.task_id WHERE r.run_id=$1`, runID).Scan(&status, &revision, &runState); err != nil || status != "blocked" || revision != before.Revision+1 || runState != "model_running" {
		t.Fatal("not held after own task commit before failed run transition", status, revision, runState, err)
	}
	select {
	case observed := <-tasks.observations:
		if observed.run != runID || !errors.Is(observed.err, task.ErrConflict) {
			t.Fatal("actual watcher did not observe own blocked/revision conflict", observed.run, observed.err)
		}
	case <-ctx.Done():
		t.Fatal("actual watcher did not run after held commit", ctx.Err())
	}
	select {
	case request := <-provider.cancel:
		if request.RunID != runID || request.Reason != "owner authority revoked or unavailable" {
			t.Fatal("wrong authority cancellation identity", request)
		}
	case <-ctx.Done():
		t.Fatal("authority cancellation not observed before release", ctx.Err())
	}
	if provider.started.Load() != 1 {
		t.Fatal("unexpected provider admissions", provider.started.Load())
	}
	t.Logf("CAUSAL_REPRO own_task_status=%s task_revision_delta=1 pre_transition_run_state=%s guard_error=task.ErrConflict authority_CancelRun_before_release=true provider_calls=1 accepted_input_present=%t native_calls=0", status, runState, accepted.InputID != "")
	unblock()
}

type behavior76TerminalWriteWitness struct {
	run      string
	ctxErr   error
	token    ownership.Token
	hasToken bool
	fenceErr error
}

type behavior76TerminalRegressionTasks struct {
	*behavior76PreparedTasks
	parent    context.Context
	checked   chan struct{}
	checkOnce sync.Once
	committed chan behavior76TerminalWriteWitness
	release   chan struct{}
}

func (s *behavior76TerminalRegressionTasks) ValidateRunAuthority(ctx context.Context, run string) error {
	err := s.Store.ValidateRunAuthority(ctx, run)
	if err == nil {
		s.checkOnce.Do(func() { close(s.checked) })
	}
	return err
}

func (s *behavior76TerminalRegressionTasks) SaveRunOutcome(ctx context.Context, run, state, note string) error {
	if err := s.Store.SaveRunOutcome(ctx, run, state, note); err != nil {
		return err
	}
	if state != "failed" {
		return nil
	}
	token, ok := ownership.TokenFromContext(ctx)
	_, release, err := ownership.Begin(ctx)
	if release != nil {
		release()
	}
	w := behavior76TerminalWriteWitness{run: run, ctxErr: ctx.Err(), token: token, hasToken: ok, fenceErr: err}
	select {
	case s.committed <- w:
	case <-s.parent.Done():
		return s.parent.Err()
	}
	select {
	case <-s.release:
		return nil
	case <-s.parent.Done():
		return s.parent.Err()
	}
}

type behavior76TerminalRegressionProvider struct {
	*behavior76TerminalFailureProvider
	checked    <-chan struct{}
	entered    chan string
	revocation bool
}

func (p *behavior76TerminalRegressionProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.entered <- q.Context.RunID
	if p.revocation {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	select {
	case <-p.checked:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return p.behavior76TerminalFailureProvider.StartRun(ctx, q)
}

type behavior76TerminalCapturedExecutor struct {
	core   *flow.Service
	errors chan error
}

func (e behavior76TerminalCapturedExecutor) Execute(ctx context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	result, err := e.core.Execute(ctx, event)
	e.errors <- err
	return result, err
}

// Actual Input/Core/Store regression: own terminal task changes must not be
// observed as revocation; genuine preterminal lease revocation must still stop.
func TestBehavior76GlobalStopTerminalObserverIntegration(t *testing.T) {
	t.Setenv(prompt.SettingKeyBasePrompt, prompt.DefaultBasePrompt)
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	for _, revocation := range []bool{false, true} {
		name := "self_failure_preserves_live_fence"
		if revocation {
			name = "preterminal_real_lease_revocation"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			f := newBehavior76Fixture(t)
			episode := behavior76DevelopmentEpisode()
			episode.Initial.Tasks[0].ExecutionPolicy = "normal"
			f.seed(episode)
			taskID, sessionKey := f.lookup("task", "A"), f.lookup("session", "A")
			tasks := &behavior76TerminalRegressionTasks{behavior76PreparedTasks: newBehavior76PreparedTasks(f), parent: ctx, checked: make(chan struct{}), committed: make(chan behavior76TerminalWriteWitness, 1), release: make(chan struct{})}
			var release sync.Once
			unblock := func() { release.Do(func() { close(tasks.release) }) }
			defer unblock()
			provider := &behavior76TerminalRegressionProvider{behavior76TerminalFailureProvider: &behavior76TerminalFailureProvider{initiativeProjectScriptedProvider: &initiativeProjectScriptedProvider{}, cancel: make(chan transport.CancelRunRequest, 4)}, checked: tasks.checked, entered: make(chan string, 1), revocation: revocation}
			executor := behavior76TerminalCapturedExecutor{core: behavior76CoreWithAssembler(f, provider, f.mandate("A"), tasks), errors: make(chan error, 1)}
			worker := input.NewService(input.NewStore(f.pool), session.NewPostgresRepository(f.pool), executor, run.NewService(run.NewPostgresRepository(f.pool), nil), nil)
			accepted, err := worker.Execute(ctx, ingress.InputEvent{EventID: "public-terminal-regression", IdempotencyKey: "public-terminal-regression", SessionKey: sessionKey, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: behavior76JSON(map[string]any{"text": "Public bounded terminal observer regression.", "user_id": behavior76Owner, "task_id": taskID}), CreatedAt: time.Now().UTC()})
			if err != nil {
				t.Fatal(err)
			}
			live, stop := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() { done <- worker.Run(live) }()
			var join sync.Once
			stopJoin := func() {
				join.Do(func() {
					unblock()
					stop()
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Error("ordinary worker not joined")
					}
				})
			}
			defer stopJoin()
			var runID string
			select {
			case runID = <-provider.entered:
			case <-ctx.Done():
				t.Fatal("provider not entered", ctx.Err())
			}
			if revocation {
				var leaseID string
				if err = f.pool.QueryRow(ctx, `SELECT lease_id FROM session_ownership WHERE run_id=$1 AND expires_at>clock_timestamp()`, runID).Scan(&leaseID); err != nil {
					t.Fatal(err)
				}
				if _, err = f.leases.ReleaseLease(ctx, leaseID); err != nil {
					t.Fatal("ordinary lease revocation failed", err)
				}
				select {
				case request := <-provider.cancel:
					if request.RunID != runID || request.Reason != "owner authority revoked or unavailable" {
						t.Fatal("wrong revocation witness", request)
					}
				case <-ctx.Done():
					t.Fatal("genuine preterminal revocation did not cancel provider", ctx.Err())
				}
				select {
				case err = <-executor.errors:
					if err == nil {
						t.Fatal("revocation execution succeeded")
					}
				case <-ctx.Done():
					t.Fatal("revoked owning execution did not finish", ctx.Err())
				}
				stopJoin()
				t.Log("REAL_REVOCATION actual_ReleaseLease=true watcher_CancelRun=true execution_error_retained=true native_calls=0")
				return
			}
			select {
			case w := <-tasks.committed:
				if w.run != runID || w.ctxErr != nil || !w.hasToken || w.token.RunID != runID || w.fenceErr != nil {
					t.Fatal("terminal cleanup lost original live context/fence", w.run, w.ctxErr, w.hasToken, w.fenceErr)
				}
			case <-ctx.Done():
				t.Fatal("own failed outcome did not commit", ctx.Err())
			}
			if err = tasks.Store.ValidateRunAuthority(ctx, runID); !errors.Is(err, task.ErrConflict) {
				t.Fatal("own post-commit task does not establish former observer conflict", err)
			}
			unblock()
			select {
			case err = <-executor.errors:
				if typed, ok := err.(*transport.Error); !ok || typed.Type != transport.ErrorTypeTransportConnectionError || typed.Message != "transport_connection_error" {
					t.Fatal("original normalized provider error replaced", err)
				}
			case <-ctx.Done():
				t.Fatal("owning execution did not finish", ctx.Err())
			}
			for {
				var terminal bool
				if err = f.pool.QueryRow(ctx, `SELECT a.status='blocked' AND a.error_code='transport_connection_error' AND r.current_state='failed' AND r.error_type='transport_error' AND r.error_message='transport_connection_error' FROM accepted_inputs a JOIN runs r ON r.run_id=a.run_id WHERE a.input_id=$1`, accepted.InputID).Scan(&terminal); err != nil {
					t.Fatal(err)
				}
				if terminal {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("ordinary blocked Input/failed run not persisted", ctx.Err())
				case <-time.After(5 * time.Millisecond):
				}
			}
			stopJoin()
			select {
			case request := <-provider.cancel:
				t.Fatal("own terminal failure caused provider cancellation", request)
			default:
			}
			t.Log("FIXED_TERMINAL own_task_conflict_demonstrated=true cleanup_context_live=true original_fence_valid=true original_transport_error_retained=true blocked_Input_failed_run=true own_CancelRun=0 native_calls=0")
		})
	}
}
