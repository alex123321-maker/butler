package input

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
)

type Executor interface {
	Execute(context.Context, ingress.InputEvent) (*flow.ExecutionResult, error)
}
type RunReader interface {
	GetRun(context.Context, string) (*sessionv1.RunRecord, error)
}
type SessionWriter interface {
	CreateSession(context.Context, session.CreateSessionParams) (session.SessionRecord, bool, error)
}

type Service struct {
	store            *Store
	sessions         SessionWriter
	executor         Executor
	runs             RunReader
	log              *slog.Logger
	workers          int
	claimTTL         time.Duration
	executionTimeout time.Duration
	maintenance      func(context.Context) error
	controls         *task.Controller
}

func (s *Service) SetTaskControls(c *task.Controller) { s.controls = c }

func (s *Service) SetMaintenance(fn func(context.Context) error) { s.maintenance = fn }

func NewService(store *Store, sessions SessionWriter, executor Executor, runs RunReader, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, sessions: sessions, executor: executor, runs: runs, log: log, workers: 2, claimTTL: 60 * time.Second, executionTimeout: 10 * time.Minute}
}

// NewServiceWithExecutionTimeout sets a finite startup profile. Earlier caller
// and durable authority deadlines still apply; existing services are unchanged.
func NewServiceWithExecutionTimeout(store *Store, sessions SessionWriter, executor Executor, runs RunReader, log *slog.Logger, timeout time.Duration) (*Service, error) {
	if timeout <= 0 || timeout > 40*time.Minute {
		return nil, fmt.Errorf("input execution timeout must be positive and at most forty minutes")
	}
	s := NewService(store, sessions, executor, runs, log)
	s.executionTimeout = timeout
	return s, nil
}

// Execute is ingress acceptance, never execution. Its context is used only to
// commit the receipt. The application's worker context owns subsequent work.
func (s *Service) Execute(ctx context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	if event.SessionKey == "" || event.Source == "" || (event.EventID == "" && event.IdempotencyKey == "") || event.EventType == 0 {
		return nil, fmt.Errorf("session, source, event type and delivery identity required")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("invalid event payload: %w", err)
	}
	owner := firstString(payload, "user_id", "external_user_id", "author_id")
	if owner == "" {
		owner = event.SessionKey
	}
	if actor, ok := owneridentity.FromContext(ctx); ok {
		if actor.Channel != event.Source || actor.SessionKey != event.SessionKey {
			return nil, task.ErrUnauthorized
		}
		owner = actor.OwnerID
		payload["user_id"] = owner
		encoded, _ := json.Marshal(payload)
		event.PayloadJSON = string(encoded)
	}
	taskID := firstString(payload, "task_id")
	sessionRecord, _, err := s.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: event.SessionKey, UserID: owner, Channel: event.Source, MetadataJSON: `{}`})
	if err != nil {
		return nil, err
	}
	if sessionRecord.UserID != owner {
		return nil, fmt.Errorf("session owner mismatch")
	}
	if s.controls != nil {
		kind := firstString(payload, "intent")
		explicitConversation := kind == "conversation"
		policy := ""
		if kind == "conversation" {
			kind = ""
		} else if kind == "" {
			if _, ok := owneridentity.FromContext(ctx); ok {
				kind, policy = task.ClassifyOwnerText(firstString(payload, "text", "message", "content"))
			}
		}
		switch kind {
		case task.ControlAmend, task.ControlStop, task.ControlPause, task.ControlResume:
			actor, ok := owneridentity.FromContext(ctx)
			if !ok || actor.OwnerID != owner || actor.Channel != event.Source || (actor.Channel == "telegram" && actor.SessionKey != event.SessionKey) {
				return nil, task.ErrUnauthorized
			}
			if taskID == "" {
				previous, err := s.controls.DeliveryTarget(ctx)
				if err != nil {
					return nil, err
				}
				taskID = previous
			}
			target, resolveErr := s.controls.ResolveTarget(ctx, taskID)
			if errors.Is(resolveErr, task.ErrConflict) {
				receipt, _, err := s.store.AcceptWaiting(ctx, event, owner)
				if err != nil {
					return nil, err
				}
				return &flow.ExecutionResult{Accepted: true, InputID: receipt.InputID, SessionKey: receipt.SessionKey, ReceiptStatus: receipt.Status, ErrorCode: receipt.ErrorCode, AssistantResponse: "Specify the task to change; no task authority was changed."}, nil
			}
			if resolveErr != nil {
				return nil, resolveErr
			}
			if policy == "" {
				policy = firstString(payload, "execution_policy")
			}
			expected := int64(0)
			if raw, exists := payload["expected_revision"]; exists {
				v, ok := raw.(float64)
				if !ok || v < 0 || v > float64(1<<53) || float64(int64(v)) != v {
					return nil, task.ErrInvalid
				}
				expected = int64(v)
			}
			control, err := s.controls.Apply(ctx, task.ControlRequest{TaskID: target.TaskID, ExpectedRevision: expected, Kind: kind, Instruction: firstString(payload, "text", "message", "content"), ExecutionPolicy: policy})
			if err != nil {
				return nil, err
			}
			return &flow.ExecutionResult{Accepted: true, InputID: control.InputID, TaskID: target.TaskID, SessionKey: target.SessionKey, ReceiptStatus: "completed", AssistantResponse: "Owner control applied: " + kind + ". " + control.EffectBoundary}, nil
		case "", "new_task":
			if kind == "new_task" || explicitConversation {
				actor, ok := owneridentity.FromContext(ctx)
				if !ok {
					return nil, task.ErrUnauthorized
				}
				if kind == "new_task" && taskID == "" {
					return nil, fmt.Errorf("create a task with explicit goal and criteria via /api/v3/tasks before submitting its task_id")
				}
				if err := s.store.ResolveConversationAmbiguity(ctx, event.SessionKey, actor.OwnerID); err != nil {
					return nil, err
				}
			}
		default:
			return nil, fmt.Errorf("unsupported input intent")
		}
	}
	r, _, err := s.store.Accept(ctx, event, owner, taskID)
	if err != nil {
		return nil, err
	}
	return &flow.ExecutionResult{Accepted: true, InputID: r.InputID, TaskID: r.TaskID, RunID: r.RunID, SessionKey: r.SessionKey, CurrentState: commonv1.RunState_RUN_STATE_QUEUED, ReceiptStatus: r.Status}, nil
}
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func (s *Service) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	if s.maintenance != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			timer := time.NewTicker(time.Second)
			defer timer.Stop()
			for {
				if err := s.maintenance(ctx); err != nil && ctx.Err() == nil {
					s.log.Warn("review pending inputs", slog.String("error", err.Error()))
				}
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
				}
			}
		}()
	}
	for n := 0; n < s.workers; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.work(ctx) }()
	}
	<-ctx.Done()
	wg.Wait()
	return nil
}
func (s *Service) work(ctx context.Context) {
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.store.ExpireQueued(ctx); err != nil && ctx.Err() == nil {
			s.log.Warn("expire queued inputs", slog.String("error", err.Error()))
		}
		r, err := s.store.Claim(ctx, s.claimTTL)
		if err == nil {
			s.process(ctx, r)
			continue
		}
		if !errors.Is(err, ErrNotFound) && ctx.Err() == nil {
			s.log.Error("claim accepted input", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
func (s *Service) process(parent context.Context, r Record) {
	if time.Now().After(r.ExpiresAt) {
		s.finish(parent, r, "expired", "input_expired", "accepted input deadline expired", 0)
		return
	}
	if r.Attempts > r.MaxAttempts {
		s.finish(parent, r, "failed", "retry_exhausted", "bounded input retries exhausted", 0)
		return
	}
	resume := false
	if r.RunID != "" {
		previous, err := s.runs.GetRun(parent, r.RunID)
		if err != nil {
			s.finish(parent, r, "blocked", "recovery_unavailable", err.Error(), 0)
			return
		}
		switch previous.GetCurrentState() {
		case commonv1.RunState_RUN_STATE_COMPLETED:
			s.finish(parent, r, "completed", "", "", 0)
			return
		case commonv1.RunState_RUN_STATE_FAILED, commonv1.RunState_RUN_STATE_TIMED_OUT, commonv1.RunState_RUN_STATE_CANCELLED:
			s.finish(parent, r, "failed", "prior_attempt_terminal", "previous attempt ended; explicit retry required", 0)
			return
		case commonv1.RunState_RUN_STATE_CREATED, commonv1.RunState_RUN_STATE_QUEUED, commonv1.RunState_RUN_STATE_ACQUIRED, commonv1.RunState_RUN_STATE_PREPARING:
			// These states precede context preparation, user transcript and dispatch.
		default:
			resume = true
		}
	}
	bounded, timeoutCancel := context.WithTimeout(WithClaim(parent, r), s.executionTimeout)
	defer timeoutCancel()
	ctx, cancelCause := context.WithCancelCause(bounded)
	cancel := func() { cancelCause(context.Canceled) }
	if doctorAdmission(r.CheckpointJSON) {
		ctx = flow.WithOperatorDoctor(ctx)
	}
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(s.claimTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.store.Renew(ctx, r, s.claimTTL); err != nil {
					if ctx.Err() != nil {
						return
					}
					cause := &claimCancellation{lost: errors.Is(err, ErrClaimLost)}
					s.log.Warn("input claim renewal stopped", slog.String("input_id", r.InputID), slog.String("run_id", r.RunID), slog.String("reason", cause.Error()))
					cancelCause(cause)
					return
				}
			}
		}
	}()
	// Preserve the accepted identity on every safe retry; run service deduplicates
	// run creation if the process stopped before the run checkpoint was saved.
	event := r.Event
	event.IdempotencyKey = r.InputID
	var result *flow.ExecutionResult
	var err error
	if resume {
		if recovering, ok := s.executor.(interface {
			Resume(context.Context, string, ingress.InputEvent) (*flow.ExecutionResult, error)
		}); ok {
			result, err = recovering.Resume(ctx, r.RunID, event)
		} else {
			err = fmt.Errorf("checkpoint recovery unavailable")
		}
	} else {
		result, err = s.executor.Execute(ctx, event)
	}
	err = preserveClaimCancellation(ctx, err)
	cancel()
	<-heartbeatDone
	// Shutdown deliberately leaves the claim for bounded takeover. A client
	// disconnect never reaches this worker context.
	if parent.Err() != nil {
		return
	}
	if errors.Is(err, session.ErrLeaseConflict) {
		s.finish(parent, r, "queued", "session_busy", err.Error(), time.Second)
		return
	}
	if err != nil {
		if errors.Is(err, flow.ErrCommunicationStatePending) && r.Attempts < r.MaxAttempts && time.Now().Before(r.ExpiresAt) {
			s.finish(parent, r, "queued", "communication_state_pending", "stored communication state awaits verification", time.Duration(1<<min(r.Attempts, 6))*time.Second)
			return
		}
		if errors.Is(err, flow.ErrModelRetryPending) && r.Attempts < r.MaxAttempts && time.Now().Before(r.ExpiresAt) {
			s.finish(parent, r, "queued", "model_retry_pending", "one bounded model retry awaits durable backoff", 0)
			return
		}
		if errors.Is(err, flow.ErrResultPersistencePending) && r.Attempts < r.MaxAttempts && time.Now().Before(r.ExpiresAt) {
			s.finish(parent, r, "queued", "result_persistence_pending", "saved final answer awaits durable persistence", time.Duration(1<<min(r.Attempts, 6))*time.Second)
			return
		}
		code, message := executionFailure(err)
		var claimCause *claimCancellation
		if errors.As(err, &claimCause) {
			code = claimCause.Error()
		}
		if doctorAdmission(r.CheckpointJSON) {
			var diagnostic interface{ DiagnosticCode() string }
			if errors.As(err, &diagnostic) {
				switch diagnostic.DiagnosticCode() {
				case "broker_transport", "preparation_denied", "authorization_denied":
					code = diagnostic.DiagnosticCode()
				}
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			code = "execution_deadline_exceeded"
		}
		if doctorAdmission(r.CheckpointJSON) {
			message = "diagnostic execution stopped; inspect bounded error code"
		}
		s.finish(parent, r, "blocked", code, message, 0)
		return
	}
	if result != nil && result.ReceiptStatus == "blocked" {
		s.finish(parent, r, "blocked", result.ErrorCode, "owner decision or reconciliation required", 0)
		return
	}
	if result != nil && result.CurrentState == commonv1.RunState_RUN_STATE_AWAITING_APPROVAL {
		s.finish(parent, r, "waiting_for_user", "", "", 0)
		return
	}
	s.finish(parent, r, "completed", "", "", 0)
}
func (s *Service) finish(ctx context.Context, r Record, status, code, message string, delay time.Duration) {
	if err := s.store.Finish(ctx, r, status, code, message, delay); err != nil {
		s.log.Warn("finish accepted input", slog.String("input_id", r.InputID), slog.String("error", err.Error()))
	}
}
