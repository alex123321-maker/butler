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
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ingress"
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
}

func (s *Service) SetMaintenance(fn func(context.Context) error) { s.maintenance = fn }

func NewService(store *Store, sessions SessionWriter, executor Executor, runs RunReader, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, sessions: sessions, executor: executor, runs: runs, log: log, workers: 2, claimTTL: 60 * time.Second, executionTimeout: 10 * time.Minute}
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
	taskID := firstString(payload, "task_id")
	sessionRecord, _, err := s.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: event.SessionKey, UserID: owner, Channel: event.Source, MetadataJSON: `{}`})
	if err != nil {
		return nil, err
	}
	if sessionRecord.UserID != owner {
		return nil, fmt.Errorf("session owner mismatch")
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
	ctx, cancel := context.WithTimeout(WithClaim(parent, r), s.executionTimeout)
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
					cancel()
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
		code := "execution_stopped"
		if errors.Is(err, context.DeadlineExceeded) {
			code = "execution_deadline_exceeded"
		}
		s.finish(parent, r, "blocked", code, err.Error(), 0)
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
