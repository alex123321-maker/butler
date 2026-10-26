package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Research-only, explicitly constructed for a prospective study. Nothing in a
// model response, corpus event or environment can install or reset this scope.
// The old collectors have nil scope and keep their strict global policy.
var errBehavior76FixtureStopped = errors.New("behavior76 fixture admission stopped")

const behavior76ScopeDDL = `CREATE TABLE behavior76_fixture_stops(
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 cause text NOT NULL CHECK(cause IN ('effect_unknown','model_empty_answer')),
 binding jsonb NOT NULL,effects jsonb NOT NULL,native_evidence jsonb,observed_at timestamptz NOT NULL DEFAULT clock_timestamp());
 CREATE TABLE behavior76_recipient_diagnostics(
 action_id text PRIMARY KEY,run_id text NOT NULL,tool_call_id text NOT NULL,
 category text NOT NULL,at timestamptz NOT NULL DEFAULT clock_timestamp());`

type behavior76FixtureScope struct {
	mu             sync.Mutex
	pool           *pgxpool.Pool
	global         *atomic.Bool
	stopped        bool
	stopCause      string
	localEmpty     bool // Trusted prospective construction only; legacy remains strict.
	closed         bool
	active         int
	evaluations    int
	changed        chan struct{}
	authorityTrace *behavior76AuthorityTrace
	binding        behavior76ScopeBinding
}

type behavior76ScopeBinding struct {
	Window         string `json:"window"`
	Arm            string `json:"arm"`
	ProtocolSHA256 string `json:"protocol_sha256"`
	Producer       string `json:"producer"`
	Schema         string `json:"schema"`
	Mode           string `json:"mode"`
}

func newBehavior76FixtureScope(pool *pgxpool.Pool, global *atomic.Bool) *behavior76FixtureScope {
	return &behavior76FixtureScope{pool: pool, global: global, changed: make(chan struct{})}
}

func (s *behavior76FixtureScope) signalLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// All model reservations and recipient admissions use the same mutex. A local
// stop is durable before the mutex is released. Work already admitted before
// that point must drain; no later admission is allowed, including automatic runs.
func (s *behavior76FixtureScope) checkLocked(ctx context.Context) error {
	if s.authorityTrace != nil && s.authorityTrace.failed() {
		s.global.Store(true)
	}
	if s.global.Load() {
		return errBehavior76Native
	}
	if err := s.reconcileLocked(ctx); err != nil {
		return err
	}
	if s.stopped || s.closed {
		return errBehavior76FixtureStopped
	}
	return nil
}

func (s *behavior76FixtureScope) reconcileLocked(ctx context.Context) error {
	var unknown, classified bool
	var effects []byte
	err := s.pool.QueryRow(ctx, `SELECT count(*)>0,
 coalesce(bool_and(coalesce(d.category IN ('foreign_task','cas_rejected','lost_ack'),false)),true),
 coalesce(jsonb_agg(jsonb_build_object('effect',to_jsonb(e),'intent_binding',i.binding,
 'owner_inputs',coalesce((SELECT jsonb_agg(jsonb_build_object('input_id',a.input_id,'run_id',a.run_id,'session_key',a.session_key,'session_sequence',a.session_sequence,'status',a.status) ORDER BY a.input_id) FROM accepted_inputs a WHERE a.run_id=i.binding->>'run_id'),'[]'::jsonb),
 'dispatches',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.authorization_id) FROM action_effect_dispatches x WHERE x.action_id=e.action_id),'[]'::jsonb)) ORDER BY e.action_id),'[]'::jsonb)
 FROM action_effects e LEFT JOIN action_intents i ON i.effect_id=e.action_id
 LEFT JOIN behavior76_recipient_diagnostics d ON d.action_id=e.action_id
 AND d.run_id=i.binding->>'run_id' AND d.tool_call_id=i.binding->>'tool_call_id'
 WHERE e.state='effect_unknown'`).Scan(&unknown, &classified, &effects)
	if err != nil || (unknown && !classified) {
		s.global.Store(true)
		return errBehavior76Native
	}
	if !unknown || s.stopped {
		return nil
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO behavior76_fixture_stops(singleton,cause,binding,effects) VALUES(true,'effect_unknown',$1,$2)`, behavior76JSON(s.binding), effects); err != nil {
		s.global.Store(true)
		return errBehavior76Native
	}
	s.stopped = true
	s.stopCause = "effect_unknown"
	s.signalLocked()
	return errBehavior76FixtureStopped
}

func (s *behavior76FixtureScope) check(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.checkLocked(ctx)
}

// reserve runs under the admission lock, including the durable model journal
// reservation. Active covers that reservation, provider entry/stream/finish or
// recipient transaction/diagnostic write, rather than just callback invocation.
func (s *behavior76FixtureScope) begin(ctx context.Context, reserve func() error) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkLocked(ctx); err != nil {
		return nil, err
	}
	if reserve != nil {
		if err := reserve(); err != nil {
			return nil, err
		}
	}
	if s.global.Load() {
		return nil, errBehavior76Native
	}
	s.active++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.active--
			s.signalLocked()
		})
	}, nil
}

func (s *behavior76FixtureScope) localStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

func (s *behavior76FixtureScope) disposition(tail bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := "fixture_local_unknown"
	if s.stopCause == "model_empty_answer" {
		state = "fixture_local_model_failure"
	}
	if tail {
		return "not_executed_after_" + state
	}
	return state
}

func (s *behavior76FixtureScope) enterEvaluation(ctx context.Context) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkLocked(ctx); err != nil {
		return nil, err
	}
	s.evaluations++
	return func() { s.mu.Lock(); s.evaluations--; s.signalLocked(); s.mu.Unlock() }, nil
}

func (s *behavior76FixtureScope) claimAdmission(ctx context.Context) (func(), error) {
	s.mu.Lock()
	if err := s.checkLocked(ctx); err != nil {
		s.mu.Unlock()
		return nil, input.ErrClaimAdmissionClosed
	}
	return s.mu.Unlock, nil
}

func (f *behavior76Fixture) waitOwningWorkers(ctx context.Context) error {
	for {
		var live bool
		err := f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs WHERE claim_expires_at>clock_timestamp()) OR EXISTS(SELECT 1 FROM session_ownership WHERE expires_at>clock_timestamp())`).Scan(&live)
		if err != nil {
			f.scope.global.Store(true)
			return err
		}
		if !live {
			return nil
		}
		select {
		case <-ctx.Done():
			f.scope.global.Store(true)
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// Closing admission is irreversible even on timeout. No cancellation exemption
// is used: already admitted provider streams finish under their original limit.
func (s *behavior76FixtureScope) drain(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	for s.active != 0 {
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			s.global.Store(true)
			return ctx.Err()
		}
		s.mu.Lock()
	}
	err := s.reconcileLocked(ctx)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if s.global.Load() {
		return errBehavior76Native
	}
	return nil
}

func (s *behavior76FixtureScope) observe() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.check(ctx)
}

type behavior76Quiescence struct {
	Schema                       string    `json:"schema"`
	ObservedAt                   time.Time `json:"observed_at"`
	FixtureLocallyStopped        bool      `json:"fixture_locally_stopped"`
	BrokerReaped                 bool      `json:"broker_reaped"`
	RuntimeStopped               bool      `json:"runtime_stopped"`
	ProviderAndRecipientInFlight int       `json:"provider_and_recipient_in_flight"`
	LiveLeases                   int       `json:"live_leases"`
	LiveClaims                   int       `json:"live_claims"`
	BrokerBackends               int       `json:"broker_backends"`
	BrokerObserved               bool      `json:"broker_application_observed_before_stop"`
}

// Only call after evaluate has joined its worker. Client closure, process Wait,
// GracefulStop and owning durable lease/claim reads precede evidence export.
func (f *behavior76Fixture) quiesce(ctx context.Context) (receipt behavior76Quiescence, err error) {
	if f.scope == nil {
		return receipt, errors.New("explicit prospective fixture scope required")
	}
	fail := func(e error) (behavior76Quiescence, error) { f.scope.global.Store(true); return receipt, e }
	if !f.brokerObserved || len(f.brokerApplication) > 63 {
		return fail(errors.New("exact broker PostgreSQL identity not observed"))
	}
	receipt.BrokerObserved = f.brokerObserved
	if e := f.scope.drain(ctx); e != nil {
		return fail(e)
	}
	f.scope.mu.Lock()
	activeEvaluations := f.scope.evaluations
	f.scope.mu.Unlock()
	if activeEvaluations != 0 {
		return fail(errors.New("fixture evaluation or owning worker not joined"))
	}
	if e := f.broker.Close(); e != nil {
		return fail(e)
	}
	if e := f.stopBroker(ctx); e != nil {
		return fail(e)
	}
	receipt.BrokerReaped = true
	if e := f.stopRuntime(ctx); e != nil {
		return fail(e)
	}
	receipt.RuntimeStopped = true
	for {
		if e := f.pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1`, f.brokerApplication).Scan(&receipt.BrokerBackends); e != nil {
			return fail(e)
		}
		if receipt.BrokerBackends == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
	// Broker completion can follow recipient return. Reconcile once more only
	// after its process and PostgreSQL backends can no longer write late rows.
	f.scope.mu.Lock()
	reconcileErr := f.scope.reconcileLocked(ctx)
	f.scope.mu.Unlock()
	if reconcileErr != nil && reconcileErr != errBehavior76FixtureStopped {
		return fail(reconcileErr)
	}
	if e := f.pool.QueryRow(ctx, `SELECT current_schema(),
 (SELECT count(*) FROM session_ownership WHERE expires_at>clock_timestamp()),
 (SELECT count(*) FROM accepted_inputs WHERE claim_expires_at>clock_timestamp())`).Scan(&receipt.Schema, &receipt.LiveLeases, &receipt.LiveClaims); e != nil {
		return fail(e)
	}
	if receipt.LiveLeases != 0 || receipt.LiveClaims != 0 {
		return fail(errors.New("owning lease or claim remains live"))
	}
	receipt.FixtureLocallyStopped = f.scope.localStopped()
	if f.authorityTrace != nil {
		trace := f.authorityTrace.snapshot()
		diagnosticDirectory := ""
		if os.Getenv("BUTLER_BEHAVIOR76_NATIVE_ENABLE") == "disabled" {
			switch os.Getenv("BUTLER_BEHAVIOR76_PUBLIC_AUTHORITY_TRACE") {
			case "composition-once-v1":
				diagnosticDirectory = "composition-diag-v1"
			case "composition-matrix-once-v1":
				diagnosticDirectory = "composition-matrix-diag-v1"
			}
		}
		if diagnosticDirectory != "" {
			f.writeAuthorityQuiescenceDiagnostic(trace, "decision", diagnosticDirectory)
			f.t.Cleanup(func() {
				f.writeAuthorityQuiescenceDiagnostic(f.authorityTrace.snapshot(), "cleanup", diagnosticDirectory)
			})
		}
		if trace.Failed || trace.OpenSpans != 0 {
			return fail(fmt.Errorf("authority trace unavailable or not quiescent: observer_failed=%t open_spans=%d dropped_events=%d operation_failures=%d events=%d", trace.Failed, trace.OpenSpans, trace.Dropped, trace.OperationFailures, len(trace.Events)))
		}
	}
	receipt.ObservedAt = time.Now().UTC()
	if f.scope.global.Load() {
		return fail(errBehavior76Native)
	}
	return receipt, nil
}

func (f *behavior76Fixture) writeAuthorityQuiescenceDiagnostic(trace behavior76AuthorityTraceSnapshot, point, directory string) {
	f.scope.mu.Lock()
	scope := map[string]any{"active": f.scope.active, "evaluations": f.scope.evaluations, "closed": f.scope.closed, "stopped": f.scope.stopped, "global": f.scope.global.Load()}
	f.scope.mu.Unlock()
	behavior76Write(f.t, filepath.Join("/src20/.cache/issue76-independent", directory, point+"-"+behavior76Hash([]byte(f.t.Name()))+".json"), map[string]any{"version": "behavior76-quiescence-diagnostic-v1", "trace": trace, "scope": scope, "read_pool": behavior76AuthorityTraceStats(f.reads)})
}
