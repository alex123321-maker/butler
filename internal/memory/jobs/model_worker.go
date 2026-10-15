package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync/atomic"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ModelHandler is trusted versioned code, never a prompt or executable from a
// source. Validation must establish the consumer's own quality/security gate.
// The generic worker publishes only immutable DATA, not accepted memory facts.
type ModelHandler interface {
	Instruction() string
	Validate([]catalog.Source, string) (string, error)
}

// ModelPublicationHandler optionally materializes a validated DATA result inside
// the existing publication transaction. Implementations are trusted versioned
// code and cannot accept memory claims or grant task/action authority. Returning
// an error rolls back both materialization and the job/result checkpoint.
type ModelPublicationHandler interface {
	PublishTx(context.Context, pgx.Tx, *catalog.Store, Job, ModelAttempt, []catalog.Source, catalog.Source, string) error
}

// ModelInputSelector narrows model-visible bodies without weakening the complete
// durable source manifest used by admission, supervision and publication. It runs
// in the existing admission transaction after every source is locked/checked.
type ModelInputSelector interface {
	InputSourcesTx(context.Context, pgx.Tx, *catalog.Store, Job, []catalog.Source) ([]catalog.Source, error)
}
type ModelWorker struct {
	Jobs     *Store
	Provider transport.ModelProvider
	Model    string
	Handlers map[string]ModelHandler
	// Optional trusted, content-free diagnostic sink. It does not alter admission,
	// retry or cancellation. Source text, SQL/args and raw errors are excluded.
	ObserveProbe func(ModelProbe)
}
type ModelProbe struct {
	Stage                 string `json:"stage"`
	Millis                int64  `json:"millis"`
	Diagnostic            string `json:"diagnostic"`
	Acquired, Idle, Total int32
}
type ModelAttempt struct {
	OperationID   string
	ReservationID string
	Deadline      time.Time
	Quote         budget.Units
}

var errStorageProbeWait = errors.New("background storage authority probe timed out")

// ErrConsumerOutputInvalid is a fixed diagnostic, never raw consumer/model text.
var ErrConsumerOutputInvalid = errors.New("background consumer output invalid")

func (s *Store) SetModelControl(ctx context.Context, state string) error {
	// The composition root/operator owns this method; it is not a model tool.
	if state != "enabled" && state != "disabled" && state != "paused" {
		return ErrClaim
	}
	_, e := s.pool.Exec(ctx, `UPDATE memory_model_control SET state=$1 WHERE singleton`, state)
	return e
}
func (s *Store) modelReady(ctx context.Context) (bool, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	var state string
	if e = tx.QueryRow(ctx, `SELECT state FROM memory_model_control WHERE singleton`).Scan(&state); e != nil {
		return false, e
	}
	busy, e := interactiveTx(ctx, tx)
	if e != nil {
		return false, e
	}
	reason := ""
	if state != "enabled" {
		reason = "worker_" + state
	} else if busy {
		reason = "interactive_priority"
	}
	if reason != "" {
		_, e = tx.Exec(ctx, `UPDATE memory_aux_jobs SET error_code=$1 WHERE kind='model' AND status='pending'`, reason)
		if e != nil {
			return false, e
		}
	}
	return reason == "", tx.Commit(ctx)
}

func (w *ModelWorker) quote() (budget.Units, error) {
	// Only the current native adapter has verified limits and an actual HTTP
	// authority hook. Price (including subscription) never proves local execution.
	if w.Provider == nil || w.Provider.Name() != "antigravity" {
		return budget.Units{}, ErrClaim
	}
	limits, ok := w.Provider.(interface{ InferenceLimits() (int64, int64) })
	if !ok {
		return budget.Units{}, ErrClaim
	}
	input, output := limits.InferenceLimits()
	if input < 1 || input > 100000000 || output < 1 || output > 65536 {
		return budget.Units{}, ErrClaim
	}
	p := w.Jobs.budgets.Policy
	if output > p.OutputTokens {
		return budget.Units{}, ErrClaim
	}
	p.OutputTokens = output
	if p.ModelMillis > 90000 {
		p.ModelMillis = 90000
	} // Leave publication time inside 2m claim.
	return p.ModelQuote(w.Provider.Name(), w.Model, input)
}
func (w *ModelWorker) Admit(ctx context.Context, j Job, handlers ...ModelHandler) (ModelAttempt, []catalog.Source, error) {
	if len(handlers) > 1 {
		return ModelAttempt{}, nil, ErrClaim
	}
	var a ModelAttempt
	quote, e := w.quote()
	if e != nil {
		return a, nil, e
	}
	s := w.Jobs
	ctx = s.Context(ctx, j)
	if e = s.catalog.CheckRestore(ctx); e != nil {
		return a, nil, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return a, nil, e
	}
	defer tx.Rollback(ctx)
	sources, e := s.catalog.AccessTx(ctx, tx, j.Sources, j.Purpose)
	if e != nil {
		return a, nil, e
	}
	p, e := s.checkModelPolicyTx(ctx, tx, j)
	if e != nil {
		return a, nil, e
	}
	if p.Privacy != "cloud_allowed" {
		return a, nil, &waitReason{"local_backend_unavailable"}
	}
	if e = sourcePolicyPrivacyTx(ctx, tx, p, sources); e != nil {
		return a, nil, e
	}
	busy, e := interactiveTx(ctx, tx)
	if e != nil {
		return a, nil, e
	}
	if busy {
		return a, nil, ErrWaiting
	}
	if _, e = tx.Exec(ctx, `SELECT job_id FROM memory_aux_jobs WHERE job_id=$1 FOR UPDATE`, j.ID); e != nil {
		return a, nil, e
	}
	op, e := budget.NewOperationID()
	if e != nil {
		return a, nil, e
	}
	admission, e := s.budgets.ReserveAuxiliaryTx(ctx, tx, budget.AuxiliaryIdentity{OwnerID: j.OwnerID, TaskID: j.TaskID, MandateID: j.Authority.MandateID, JobID: j.ID, OperationID: op, Limits: j.Allowance, Deadline: j.Deadline, InputBytes: quote.ProviderInputBytes, Tokens: quote.Tokens, ActiveMillis: quote.ActiveMillis, Quote: &quote, PolicyID: p.ID, PolicyLimits: p.Job.Allowance, PolicyDeadline: p.Job.Deadline})
	if e != nil {
		return a, nil, e
	}
	checker := &claimChecker{s, j}
	if _, e = checker.CheckMemoryAccessTx(ctx, tx, j.Sources, j.Purpose); e != nil {
		return a, nil, e
	}
	if e = checker.FinalizeMemoryAccessTx(ctx, tx, j.Sources, j.Purpose); e != nil {
		return a, nil, e
	}
	deadline := admission.Deadline
	// All scope deadlines, including owner UTC-day, remain hard bounds in-flight.
	var scopeDeadline time.Time
	e = tx.QueryRow(ctx, `SELECT min(s.deadline) FROM execution_budget_reservations r CROSS JOIN LATERAL jsonb_array_elements_text(r.request_json->'scope_ids') ids(id) JOIN execution_budget_scopes s ON s.scope_id=ids.id WHERE r.reservation_id=$1`, admission.ReservationID).Scan(&scopeDeadline)
	if e != nil {
		return a, nil, e
	}
	if scopeDeadline.Before(deadline) {
		deadline = scopeDeadline
	}
	if j.ClaimUntil.Before(deadline) {
		deadline = *j.ClaimUntil
	}
	if j.Authority.TaskID != "" {
		var now time.Time
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
			return a, nil, e
		}
		m, g, e := actionauth.CheckAssignmentTx(ctx, tx, j.Authority, now)
		if e != nil {
			return a, nil, e
		}
		d, e := actionauth.AuthorityDeadline(m, g, now)
		if e != nil {
			return a, nil, e
		}
		if d.Before(deadline) {
			deadline = d
		}
	}
	_, e = tx.Exec(ctx, `INSERT INTO memory_model_attempts(operation_id,job_id,claim_id,claim_epoch,reservation_id,provider,model,source_fingerprint,locality,quote) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'cloud',$9)`, op, j.ID, j.ClaimID, j.ClaimEpoch, admission.ReservationID, w.Provider.Name(), w.Model, j.Fingerprint, jsonBytes(quote))
	if e != nil {
		return a, nil, e
	}
	a = ModelAttempt{op, admission.ReservationID, deadline, quote}
	if len(handlers) == 1 {
		if h, ok := handlers[0].(ModelInputSelector); ok {
			sources, e = h.InputSourcesTx(ctx, tx, s.catalog, j, sources)
			if e != nil {
				return a, nil, e
			}
		}
	}
	return a, sources, tx.Commit(ctx)
}

// Dispatch is invoked by the native transport immediately before HTTP. Its
// durable marker means possibly sent: a crash after commit keeps the full quote.
func (s *Store) checkModelReleaseTx(ctx context.Context, tx pgx.Tx, j Job, a ModelAttempt) error {
	sources, e := s.catalog.AccessTx(s.Context(ctx, j), tx, j.Sources, j.Purpose)
	if e != nil {
		return e
	}
	p, e := s.checkModelPolicyTx(ctx, tx, j)
	if e != nil {
		return e
	}
	if e = sourcePolicyPrivacyTx(ctx, tx, p, sources); e != nil {
		return e
	}
	busy, e := interactiveTx(ctx, tx)
	if e != nil {
		return e
	}
	if busy {
		return ErrWaiting
	}
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return e
	}
	if !now.Before(a.Deadline) {
		return ErrClaim
	}
	return nil
}
func (s *Store) checkModelRelease(ctx context.Context, j Job, a ModelAttempt) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = s.checkModelReleaseTx(ctx, tx, j, a); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) dispatchModel(ctx context.Context, j Job, a ModelAttempt) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = s.checkModelReleaseTx(ctx, tx, j, a); e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `UPDATE memory_model_attempts SET state='dispatched' WHERE operation_id=$1 AND job_id=$2 AND claim_id=$3 AND claim_epoch=$4 AND state='reserved'`, a.OperationID, j.ID, j.ClaimID, j.ClaimEpoch)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrClaim
	}
	return tx.Commit(ctx)
}
func (s *Store) observeModel(ctx context.Context, a ModelAttempt, usage *transport.Usage, output string) error {
	if usage != nil && usage.Validate() != nil {
		return ErrClaim
	}
	sum := sha256.Sum256([]byte(output))
	hash := hex.EncodeToString(sum[:])
	var raw any
	if usage != nil {
		raw = jsonBytes(usage)
	}
	tag, e := s.pool.Exec(ctx, `UPDATE memory_model_attempts SET state='observed',usage=$2,output_hash=$3,observed_at=clock_timestamp() WHERE operation_id=$1 AND state='dispatched'`, a.OperationID, raw, hash)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrClaim
	}
	return nil
}
func (s *Store) publishModel(ctx context.Context, j Job, a ModelAttempt, body string, handlers ...ModelHandler) (string, error) {
	if len(handlers) > 1 {
		return "", ErrClaim
	}
	if len(body) == 0 || len(body) > 32768 || sanitize.Text(body) != body {
		return "", ErrClaim
	}
	// A lost commit ACK can return its content-free receipt without rerunning a
	// model or requiring an expired claim. Only a committed published row qualifies.
	var prior string
	if s.pool.QueryRow(ctx, `SELECT result_source_id FROM memory_model_attempts WHERE operation_id=$1 AND job_id=$2 AND state='published'`, a.OperationID, j.ID).Scan(&prior) == nil {
		return prior, nil
	}
	ctx = s.Context(ctx, j)
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	sources, e := s.catalog.AccessTx(ctx, tx, j.Sources, j.Purpose)
	if e != nil {
		return "", e
	}
	p, e := s.checkModelPolicyTx(ctx, tx, j)
	if e != nil {
		return "", e
	}
	if e = sourcePolicyPrivacyTx(ctx, tx, p, sources); e != nil {
		return "", e
	}
	if _, e = tx.Exec(ctx, `SELECT job_id FROM memory_aux_jobs WHERE job_id=$1 FOR UPDATE`, j.ID); e != nil {
		return "", e
	}
	var state string
	if e = tx.QueryRow(ctx, `SELECT state FROM memory_model_attempts WHERE operation_id=$1 AND job_id=$2 AND claim_id=$3 AND claim_epoch=$4 FOR UPDATE`, a.OperationID, j.ID, j.ClaimID, j.ClaimEpoch).Scan(&state); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return "", ErrClaim
		}
		return "", e
	}
	if state != "observed" {
		return "", ErrClaim
	}
	src, e := s.catalog.CreateDerivedTx(ctx, tx, j.Sources, j.Purpose, "Background memory candidate", body, jsonBytes(map[string]string{"job_id": j.ID, "operation_id": a.OperationID, "handler": j.Purpose}))
	if e != nil {
		return "", e
	}
	if len(handlers) == 1 {
		if h, ok := handlers[0].(ModelPublicationHandler); ok {
			if e = h.PublishTx(ctx, tx, s.catalog, j, a, sources, src, body); e != nil {
				return "", e
			}
		}
	}
	tag, e := tx.Exec(ctx, `UPDATE memory_aux_jobs SET status='done',claim_id='',claim_until=NULL,error_code='',updated_at=clock_timestamp() WHERE job_id=$1 AND claim_id=$2 AND claim_epoch=$3 AND status='running' AND claim_until>clock_timestamp() AND deadline>clock_timestamp()`, j.ID, j.ClaimID, j.ClaimEpoch)
	if e != nil {
		return "", e
	}
	if tag.RowsAffected() != 1 {
		return "", ErrClaim
	}
	_, e = tx.Exec(ctx, `UPDATE memory_model_attempts SET state='published',result_source_id=$2 WHERE operation_id=$1`, a.OperationID, src.ID)
	if e != nil {
		return "", e
	}
	return src.ID, tx.Commit(ctx)
}
func (w *ModelWorker) ProcessOne(ctx context.Context) (bool, error) {
	ctx, stop := context.WithTimeout(ctx, 110*time.Second)
	defer stop()
	ready, e := w.Jobs.modelReady(ctx)
	if e != nil || !ready {
		return false, e
	}
	names := make([]string, 0, len(w.Handlers))
	for name := range w.Handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		j, ok, e := w.Jobs.ClaimPurpose(ctx, "model", name)
		if e != nil {
			return false, e
		}
		if !ok {
			continue
		}
		return true, w.execute(ctx, j, w.Handlers[name])
	}
	return false, nil
}
func (w *ModelWorker) execute(ctx context.Context, j Job, h ModelHandler) (err error) {
	s := w.Jobs
	var a ModelAttempt
	var waiting atomic.Bool
	var dispatched atomic.Bool
	var persistenceFailure atomic.Pointer[error]
	defer func() {
		if err == nil {
			return
		}
		if waiting.Load() {
			err = ErrWaiting
		}
		if cause := persistenceFailure.Load(); cause != nil {
			err = *cause
		}
		finish, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer stop()
		if a.OperationID != "" {
			var published bool
			if s.pool.QueryRow(finish, `SELECT EXISTS(SELECT 1 FROM memory_model_attempts a JOIN memory_aux_jobs j USING(job_id) WHERE a.operation_id=$1 AND a.state='published' AND j.status='done')`, a.OperationID).Scan(&published) == nil && published {
				err = nil
				return
			}
		}
		code, retry := "authority_or_source_denied", false
		var exhausted *budget.ExhaustedError
		var te *transport.Error
		switch {
		case ctx.Err() != nil:
			// A native adapter may normalize cancellation into a buffered
			// transport error. Outer worker shutdown still leaves resumable work,
			// regardless of which ready stream/context branch won the race.
			code = "worker_stopped"
			retry = true
		case errors.As(err, &exhausted):
			code = "budget_exhausted"
		case errors.Is(err, budget.ErrUnknownCost):
			code = "cost_unknown"
		case errors.Is(err, ErrConsumerOutputInvalid):
			code = "consumer_output_invalid"
		case errors.Is(err, ErrWaiting):
			code = "admission_wait"
			var why *waitReason
			if errors.As(err, &why) {
				code = why.reason
			}
			retry = true
		case errors.As(err, &te):
			code = "provider_failed"
			retry = te.Retryable
		case errors.Is(err, context.DeadlineExceeded):
			code = "provider_timeout"
			retry = true
		case errors.Is(err, context.Canceled):
			code = "cancelled"
			if ctx.Err() != nil {
				code = "worker_stopped"
				retry = true
			}
		case transientPersistence(err):
			code = "storage_unavailable"
			retry = true
		}
		if a.OperationID != "" {
			attemptCode := code
			if code == "provider_failed" {
				attemptCode = transport.DiagnosticCode(err)
				if status := transport.DiagnosticHTTPStatus(err); status != 0 {
					attemptCode += fmt.Sprintf("_http_%d", status)
				}
				if reason := transport.DiagnosticReason(err); reason != "" {
					attemptCode += ":" + reason
				}
			}
			_, _ = s.pool.Exec(finish, `UPDATE memory_model_attempts SET state='failed',reason=$2 WHERE operation_id=$1 AND state<>'published'`, a.OperationID, attemptCode)
		}
		if errors.Is(err, ErrWaiting) && !dispatched.Load() {
			// No new network dispatch was authorized. Keep the already reserved
			// amount conservative, but a pause is not a failed model retry.
			_, _ = s.pool.Exec(finish, `UPDATE memory_aux_jobs SET status='pending',claim_id='',claim_until=NULL,error_code=$4,attempts=greatest(attempts-1,0),next_attempt_at=clock_timestamp()+INTERVAL '10 seconds',updated_at=clock_timestamp() WHERE job_id=$1 AND claim_id=$2 AND claim_epoch=$3 AND status='running'`, j.ID, j.ClaimID, j.ClaimEpoch, code)
			return
		}
		_ = s.Finish(finish, j, code, retry)
	}()
	var sources []catalog.Source
	a, sources, err = w.Admit(ctx, j, h)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithDeadline(s.Context(ctx, j), a.Deadline)
	supervisorCtx, stopSupervisor := context.WithCancel(callCtx)
	done := make(chan struct{})
	go func(supervisorCtx context.Context) {
		defer close(done)
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-supervisorCtx.Done():
				return
			case <-t.C:
				check, stop := context.WithTimeout(supervisorCtx, 200*time.Millisecond)
				started := time.Now()
				stage := "model_supervisor"
				e := s.checkModelSupervisor(check, j, func(phase string) { stage = phase })
				if e == nil {
					stage = "model_supervisor"
				}
				if w.ObserveProbe != nil {
					diagnostic := ""
					if e != nil {
						var pg *pgconn.PgError
						switch {
						case errors.As(e, &pg):
							diagnostic = "pg_" + pg.Code
						case errors.Is(e, context.DeadlineExceeded):
							diagnostic = "deadline_exceeded"
						case errors.Is(e, context.Canceled):
							diagnostic = "cancelled"
						default:
							diagnostic = "authority_or_storage_error"
						}
					}
					stat := s.pool.Stat()
					w.ObserveProbe(ModelProbe{Stage: stage, Millis: time.Since(started).Milliseconds(), Diagnostic: diagnostic, Acquired: stat.AcquiredConns(), Idle: stat.IdleConns(), Total: stat.TotalConns()})
				}
				stop()
				if supervisorCtx.Err() != nil {
					return
				}
				if e != nil {
					if errors.Is(e, context.DeadlineExceeded) && supervisorCtx.Err() == nil {
						e = errStorageProbeWait
					}
					if transientPersistence(e) {
						persistenceFailure.Store(&e)
					}
					if errors.Is(e, ErrWaiting) {
						waiting.Store(true)
					}
					cancel()
					return
				}
			}
		}
	}(supervisorCtx)
	defer func() { stopSupervisor(); cancel(); <-done }()
	callCtx = budget.WithModelOperation(callCtx, a.OperationID)
	callCtx = transport.WithInputByteLimit(callCtx, a.Quote.ProviderInputBytes)
	callCtx = transport.WithInferenceAuthority(callCtx, func(c context.Context) error {
		e := s.checkModelRelease(c, j, a)
		if errors.Is(e, ErrWaiting) {
			waiting.Store(true)
		}
		if transientPersistence(e) {
			persistenceFailure.Store(&e)
		}
		return e
	})
	callCtx = transport.WithInferenceDispatch(callCtx, func(c context.Context) error {
		e := s.dispatchModel(c, j, a)
		if transientPersistence(e) {
			persistenceFailure.Store(&e)
		}
		if e == nil {
			dispatched.Store(true)
		}
		if errors.Is(e, ErrWaiting) {
			waiting.Store(true)
		}
		return e
	})
	items := []transport.InputItem{content.Instruction(h.Instruction(), content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{"memory-handler:" + j.Purpose}, Sensitivity: content.Public})}
	for _, src := range sources {
		if sanitize.Text(src.Body) != src.Body || sanitize.Text(src.Title) != src.Title {
			return ErrClaim
		}
		items = append(items, content.Data(map[string]string{"source_id": src.ID, "title": src.Title, "body": src.Body}, content.Derive(content.MemoryDerived, src.Origin)))
	}
	if _, err = content.Normalize(items); err != nil {
		return err
	}
	// This fresh synthetic transport ID is correlation only, never a session lease.
	req := transport.StartRunRequest{Context: transport.TransportRunContext{RunID: a.OperationID, SessionKey: "memory:" + j.ID, ProviderName: w.Provider.Name(), ModelName: w.Model}, InputItems: items}
	stream, e := w.Provider.StartRun(callCtx, req)
	if e != nil {
		return transport.NormalizeError(e, w.Provider.Name())
	}
	defer func() {
		stop, finish := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer finish()
		_, _ = w.Provider.CancelRun(stop, transport.CancelRunRequest{RunID: a.OperationID, Reason: "background_attempt_finished"})
	}()
	var output string
	var usage *transport.Usage
	final := false
	size := 0
	for {
		select {
		case <-callCtx.Done():
			return callCtx.Err()
		case event, ok := <-stream:
			if !ok {
				if !final {
					return &transport.Error{Type: transport.ErrorTypeProviderUnavailable, Retryable: true}
				}
				goto observed
			}
			if event.RunID != a.OperationID || event.ProviderName != w.Provider.Name() {
				return ErrClaim
			}
			if event.Usage != nil {
				if event.Usage.Validate() != nil {
					return ErrClaim
				}
				inputLimit, outputLimit := w.Provider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
				u := event.Usage
				exceeded := int64(u.InputTokens) > inputLimit || int64(u.OutputTokens) > outputLimit || int64(u.TotalTokens) > a.Quote.Tokens || u.TotalTokens > 0 && int64(u.TotalTokens-u.InputTokens) > outputLimit
				if usage != nil && *usage != *event.Usage {
					return ErrClaim
				}
				observed := *event.Usage
				usage = &observed
				// Save complete observed usage before processing a terminal error or
				// publication decision. Revocation cannot erase a paid observation.
				observe, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
				_, saveErr := s.pool.Exec(observe, `UPDATE memory_model_attempts SET usage=$2,observed_at=clock_timestamp() WHERE operation_id=$1 AND (usage IS NULL OR usage=$2::jsonb)`, a.OperationID, jsonBytes(usage))
				stop()
				if saveErr != nil {
					return saveErr
				}
				if exceeded {
					return ErrClaim
				}
			}
			if event.AssistantDelta != nil {
				size += len(event.AssistantDelta.Content)
				if size > 32768 {
					return ErrClaim
				}
			}
			switch event.EventType {
			case transport.EventTypeAssistantFinal:
				if event.AssistantFinal == nil || final || len(event.AssistantFinal.Content) > 32768 {
					return ErrClaim
				}
				output = event.AssistantFinal.Content
				final = true
			case transport.EventTypeToolCallRequested, transport.EventTypeToolCallBatchRequested:
				return ErrClaim
			case transport.EventTypeTransportError:
				if event.TransportError == nil {
					return ErrClaim
				}
				return event.TransportError
			case transport.EventTypeRunCancelled:
				return context.Canceled
			case transport.EventTypeRunTimedOut:
				return context.DeadlineExceeded
			}
		}
	}
observed:
	// No provider work remains. Join its periodic observer before durable local
	// publication takes the same policy/session locks. callCtx remains current;
	// full AccessTx publication proof still serializes correction/revocation.
	stopSupervisor()
	<-done
	// Usage survives revocation and failed publication; missing observations are
	// NULL. The conservative reservation is never refunded, including before send.
	observeCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	err = s.observeModel(observeCtx, a, usage, output)
	stop()
	if err != nil {
		return err
	}
	body, e := h.Validate(sources, output)
	if e != nil {
		return ErrConsumerOutputInvalid
	}
	if callCtx.Err() != nil {
		return callCtx.Err()
	}
	_, err = s.publishModel(callCtx, j, a, body, h)
	return err
}

func transientPersistence(err error) bool {
	if errors.Is(err, errStorageProbeWait) {
		return true
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return len(pg.Code) >= 2 && (pg.Code[:2] == "08" || pg.Code[:2] == "40" || pg.Code[:2] == "53" || pg.Code == "57P01" || pg.Code == "57P02" || pg.Code == "57P03")
	}
	var connect *pgconn.ConnectError
	return pgconn.SafeToRetry(err) || errors.As(err, &connect) || errors.Is(err, io.EOF)
}
func (w *ModelWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		poll, stop := context.WithTimeout(ctx, 5*time.Second)
		preparers := map[string]ModelScheduleHandler{}
		for name, handler := range w.Handlers {
			if p, ok := handler.(ModelScheduleHandler); ok {
				preparers[name] = p
			}
		}
		e := w.Jobs.PollModelPolicies(poll, func(name string) bool { return w.Handlers[name] != nil }, preparers)
		stop()
		if e == nil {
			_, _ = w.ProcessOne(ctx)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
