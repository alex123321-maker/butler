package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/domain"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
)

type SessionRepository interface {
	CreateSession(context.Context, session.CreateSessionParams) (session.SessionRecord, bool, error)
}

type RunManager interface {
	CreateRun(context.Context, *sessionv1.CreateRunRequest) (*sessionv1.RunRecord, error)
	TransitionRun(context.Context, *sessionv1.UpdateRunStateRequest) (*sessionv1.RunRecord, error)
	GetRun(context.Context, string) (*sessionv1.RunRecord, error)
	PersistProviderSessionRef(context.Context, string, string) (*sessionv1.RunRecord, error)
}

type TranscriptStore interface {
	AppendMessage(context.Context, transcript.Message) (transcript.Message, error)
}

type Config struct {
	ProviderName string
	ModelName    string
	OwnerID      string
	LeaseTTL     int64
	Delivery     DeliverySink
}

type Service struct {
	sessions   SessionRepository
	leases     session.LeaseManager
	runs       RunManager
	transcript TranscriptStore
	provider   transport.ModelProvider
	config     Config
	log        *slog.Logger
}

type ExecutionResult struct {
	RunID             string
	SessionKey        string
	CurrentState      commonv1.RunState
	AssistantResponse string
}

type DeliveryEvent struct {
	RunID      string
	SessionKey string
	Content    string
	Final      bool
	SequenceNo int
}

var errLeaseRenewalFailed = errors.New("lease renewal failed")

type preparedRun struct {
	InputItems    []transport.InputItem
	UserMessage   string
	MemoryBundle  map[string]any
	InputPayload  map[string]any
	SessionUserID string
	Channel       string
}

func NewService(sessions SessionRepository, leases session.LeaseManager, runs RunManager, transcriptStore TranscriptStore, provider transport.ModelProvider, cfg Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	if cfg.ProviderName == "" {
		cfg.ProviderName = "openai"
	}
	if cfg.OwnerID == "" {
		cfg.OwnerID = "orchestrator"
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 60
	}
	if cfg.Delivery == nil {
		cfg.Delivery = NopDeliverySink{}
	}
	return &Service{
		sessions:   sessions,
		leases:     leases,
		runs:       runs,
		transcript: transcriptStore,
		provider:   provider,
		config:     cfg,
		log:        logger.WithComponent(log, "orchestrator-run"),
	}
}

func (s *Service) Execute(ctx context.Context, event ingress.InputEvent) (*ExecutionResult, error) {
	if err := validateInputEvent(event); err != nil {
		return nil, err
	}
	prepared, err := prepareRun(event)
	if err != nil {
		return nil, err
	}

	if _, _, err := s.sessions.CreateSession(ctx, session.CreateSessionParams{
		SessionKey:   event.SessionKey,
		UserID:       prepared.SessionUserID,
		Channel:      prepared.Channel,
		MetadataJSON: mustMarshalJSON(map[string]any{"source": event.Source}),
	}); err != nil {
		return nil, fmt.Errorf("ensure session: %w", err)
	}

	runRecord, err := s.runs.CreateRun(ctx, &sessionv1.CreateRunRequest{
		SessionKey:    event.SessionKey,
		InputEvent:    event.ToProto(),
		AutonomyMode:  commonv1.AutonomyMode_AUTONOMY_MODE_1,
		ModelProvider: s.config.ProviderName,
		MetadataJson: mustMarshalJSON(map[string]any{
			"input_payload": prepared.InputPayload,
			"memory_bundle": prepared.MemoryBundle,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}

	runLog := logger.WithRunID(s.log, runRecord.GetRunId())
	result, execErr := s.executeRun(ctx, runLog, runRecord, event, prepared)
	if execErr != nil {
		return nil, execErr
	}
	return result, nil
}

func (s *Service) executeRun(ctx context.Context, runLog *slog.Logger, runRecord *sessionv1.RunRecord, event ingress.InputEvent, prepared preparedRun) (*ExecutionResult, error) {
	current := runRecord

	next, err := s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_QUEUED, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("queue run: %w", err)
	}
	current = next

	lease, err := s.leases.AcquireLease(ctx, session.AcquireLeaseParams{
		SessionKey: event.SessionKey,
		RunID:      current.GetRunId(),
		OwnerID:    s.config.OwnerID,
		TTL:        time.Duration(s.config.LeaseTTL) * time.Second,
	})
	if err != nil {
		return nil, s.failRun(ctx, current, "", runLog, fmt.Errorf("acquire lease: %w", err))
	}
	runCtx, stopRenewing, renewErrs := s.startLeaseRenewer(ctx, lease, runLog)
	defer stopRenewing()
	defer func() {
		if _, releaseErr := s.leases.ReleaseLease(context.Background(), lease.LeaseID); releaseErr != nil {
			runLog.Error("release lease failed", slog.String("lease_id", lease.LeaseID), slog.String("error", releaseErr.Error()))
		}
	}()

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_ACQUIRED, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run acquired: %w", err))
	}
	current = next

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_PREPARING, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run preparing: %w", err))
	}
	current = next

	if _, err := s.transcript.AppendMessage(ctx, transcript.Message{
		SessionKey:   event.SessionKey,
		RunID:        current.GetRunId(),
		Role:         "user",
		Content:      prepared.UserMessage,
		MetadataJSON: mustMarshalJSON(map[string]any{"event_id": event.EventID, "source": event.Source}),
	}); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("append user transcript: %w", err))
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, lease.LeaseID, "", "")
	if err != nil {
		return nil, fmt.Errorf("mark run model_running: %w", err)
	}
	current = next

	existingProviderSessionRef, err := transport.ParseProviderSessionRef(current.GetProviderSessionRef())
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("parse persisted provider session ref: %w", err))
	}

	stream, err := s.provider.StartRun(runCtx, transport.StartRunRequest{
		Context: transport.TransportRunContext{
			RunID:                  current.GetRunId(),
			SessionKey:             event.SessionKey,
			ProviderName:           s.config.ProviderName,
			ModelName:              s.config.ModelName,
			ProviderSessionRef:     existingProviderSessionRef,
			SupportsStreaming:      true,
			SupportsToolCalls:      true,
			SupportsStatefulResume: true,
		},
		InputItems:       prepared.InputItems,
		StreamingEnabled: true,
	})
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
	}

	var finalMessage string
	for transportEvent := range stream {
		if current, err = s.persistProviderSessionRef(ctx, current, transportEvent.ProviderSessionRef, runLog); err != nil {
			return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
		}
		switch transportEvent.EventType {
		case transport.EventTypeAssistantDelta:
			if transportEvent.AssistantDelta != nil {
				if err := s.config.Delivery.DeliverAssistantDelta(runCtx, DeliveryEvent{
					RunID:      current.GetRunId(),
					SessionKey: event.SessionKey,
					Content:    transportEvent.AssistantDelta.Content,
					SequenceNo: transportEvent.AssistantDelta.SequenceNo,
				}); err != nil {
					return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
				}
			}
		case transport.EventTypeAssistantFinal:
			if transportEvent.AssistantFinal != nil {
				finalMessage = transportEvent.AssistantFinal.Content
				if err := s.config.Delivery.DeliverAssistantFinal(runCtx, DeliveryEvent{RunID: current.GetRunId(), SessionKey: event.SessionKey, Content: finalMessage, Final: true}); err != nil {
					return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
				}
			}
		case transport.EventTypeTransportError:
			return nil, s.failRun(ctx, current, lease.LeaseID, runLog, transportError(transportEvent.TransportError))
		case transport.EventTypeTransportWarning:
			runLog.Warn("transport warning", slog.String("payload", transportEvent.PayloadJSON))
		}
	}
	if renewErr := drainRenewError(renewErrs); renewErr != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, renewErr)
	}

	if strings.TrimSpace(finalMessage) == "" {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("model run completed without assistant_final event"))
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_FINALIZING, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run finalizing: %w", err))
	}
	current = next

	if _, err := s.transcript.AppendMessage(ctx, transcript.Message{
		SessionKey:   event.SessionKey,
		RunID:        current.GetRunId(),
		Role:         "assistant",
		Content:      finalMessage,
		MetadataJSON: `{"source":"model"}`,
	}); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("append assistant transcript: %w", err))
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_COMPLETED, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run completed: %w", err))
	}

	runLog.Info("run completed", slog.String("session_key", event.SessionKey))
	return &ExecutionResult{RunID: next.GetRunId(), SessionKey: event.SessionKey, CurrentState: next.GetCurrentState(), AssistantResponse: finalMessage}, nil
}

func (s *Service) failRun(ctx context.Context, current *sessionv1.RunRecord, leaseID string, runLog *slog.Logger, err error) error {
	runLog.Error("run execution failed", slog.String("error", err.Error()))
	state := current.GetCurrentState()
	if state == commonv1.RunState_RUN_STATE_UNSPECIFIED || state == commonv1.RunState_RUN_STATE_FAILED || state == commonv1.RunState_RUN_STATE_CANCELLED || state == commonv1.RunState_RUN_STATE_TIMED_OUT || state == commonv1.RunState_RUN_STATE_COMPLETED {
		return err
	}
	if _, transitionErr := s.transition(ctx, current.GetRunId(), state, commonv1.RunState_RUN_STATE_FAILED, leaseID, classifyExecutionError(err), err.Error()); transitionErr != nil {
		return fmt.Errorf("%w: additionally failed to transition run to failed: %v", err, transitionErr)
	}
	return err
}

func (s *Service) persistProviderSessionRef(ctx context.Context, current *sessionv1.RunRecord, ref *transport.ProviderSessionRef, runLog *slog.Logger) (*sessionv1.RunRecord, error) {
	if ref == nil {
		return current, nil
	}
	encoded, err := transport.MarshalProviderSessionRef(ref)
	if err != nil {
		return current, fmt.Errorf("marshal provider session ref: %w", err)
	}
	if strings.TrimSpace(encoded) == strings.TrimSpace(current.GetProviderSessionRef()) {
		return current, nil
	}
	updated, err := s.runs.PersistProviderSessionRef(ctx, current.GetRunId(), encoded)
	if err != nil {
		return current, fmt.Errorf("persist provider session ref: %w", err)
	}
	runLog.Info("provider session ref persisted", slog.String("run_id", current.GetRunId()))
	return updated, nil
}

func (s *Service) startLeaseRenewer(parent context.Context, lease session.LeaseRecord, runLog *slog.Logger) (context.Context, context.CancelFunc, <-chan error) {
	runCtx, cancel := context.WithCancel(parent)
	errCh := make(chan error, 1)
	ttl := time.Duration(s.config.LeaseTTL) * time.Second
	if ttl <= 0 {
		ttl = time.Minute
	}
	interval := ttl / 2
	if interval < time.Second {
		interval = time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				callCtx, callCancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, err := s.leases.RenewLease(callCtx, lease.LeaseID, ttl)
				callCancel()
				if err != nil {
					runLog.Error("lease renew failed", slog.String("lease_id", lease.LeaseID), slog.String("error", err.Error()))
					select {
					case errCh <- fmt.Errorf("%w: %v", errLeaseRenewalFailed, err):
					default:
					}
					cancel()
					return
				}
			}
		}
	}()
	return runCtx, cancel, errCh
}

func drainRenewError(errCh <-chan error) error {
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (s *Service) transition(ctx context.Context, runID string, from, to commonv1.RunState, leaseID, errorType, errorMessage string) (*sessionv1.RunRecord, error) {
	resp, err := s.runs.TransitionRun(ctx, &sessionv1.UpdateRunStateRequest{
		RunId:        runID,
		FromState:    from,
		ToState:      to,
		LeaseId:      leaseID,
		ErrorType:    toErrorClass(errorType),
		ErrorMessage: errorMessage,
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func prepareRun(event ingress.InputEvent) (preparedRun, error) {
	payload := map[string]any{}
	if strings.TrimSpace(event.PayloadJSON) != "" {
		if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
			return preparedRun{}, fmt.Errorf("decode input payload: %w", err)
		}
	}
	message := extractUserMessage(payload)
	if strings.TrimSpace(message) == "" {
		message = event.PayloadJSON
	}
	userID := firstString(payload, "user_id", "external_user_id", "author_id")
	if userID == "" {
		userID = event.SessionKey
	}
	channel := strings.TrimSpace(event.Source)
	if channel == "" {
		channel = "unknown"
	}
	return preparedRun{
		InputItems:    []transport.InputItem{{Role: "user", Content: message, ContentType: "text/plain"}},
		UserMessage:   message,
		MemoryBundle:  map[string]any{},
		InputPayload:  payload,
		SessionUserID: userID,
		Channel:       channel,
	}, nil
}

func extractUserMessage(payload map[string]any) string {
	for _, key := range []string{"text", "message", "content", "prompt"} {
		if value := strings.TrimSpace(stringFromAny(payload[key])); value != "" {
			return value
		}
	}
	if message, ok := payload["message"].(map[string]any); ok {
		for _, key := range []string{"text", "content"} {
			if value := strings.TrimSpace(stringFromAny(message[key])); value != "" {
				return value
			}
		}
	}
	return ""
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringFromAny(payload[key])); value != "" {
			return value
		}
	}
	return ""
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func validateInputEvent(event ingress.InputEvent) error {
	if strings.TrimSpace(event.EventID) == "" {
		return fmt.Errorf("event_id is required")
	}
	if strings.TrimSpace(event.SessionKey) == "" {
		return fmt.Errorf("session_key is required")
	}
	if strings.TrimSpace(event.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if event.EventType == runv1.InputEventType_INPUT_EVENT_TYPE_UNSPECIFIED {
		return fmt.Errorf("event_type is required")
	}
	return nil
}

func mustMarshalJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func transportError(err *transport.Error) error {
	if err == nil {
		return errors.New("transport error")
	}
	return err
}

func toErrorClass(value string) commonv1.ErrorClass {
	switch strings.TrimSpace(value) {
	case "":
		return commonv1.ErrorClass_ERROR_CLASS_UNSPECIFIED
	case string(domain.ErrorClassValidation):
		return commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR
	case string(domain.ErrorClassTransport):
		return commonv1.ErrorClass_ERROR_CLASS_TRANSPORT_ERROR
	case string(domain.ErrorClassTool):
		return commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR
	case string(domain.ErrorClassPolicy):
		return commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED
	case string(domain.ErrorClassCredential):
		return commonv1.ErrorClass_ERROR_CLASS_CREDENTIAL_ERROR
	case string(domain.ErrorClassApproval):
		return commonv1.ErrorClass_ERROR_CLASS_APPROVAL_ERROR
	case string(domain.ErrorClassTimeout):
		return commonv1.ErrorClass_ERROR_CLASS_TIMEOUT
	case string(domain.ErrorClassCancelled):
		return commonv1.ErrorClass_ERROR_CLASS_CANCELLED
	case string(domain.ErrorClassInternal):
		return commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
	default:
		return commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
	}
}

func classifyExecutionError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return string(domain.ErrorClassTimeout)
	case errors.Is(err, context.Canceled):
		return string(domain.ErrorClassCancelled)
	case errors.Is(err, errLeaseRenewalFailed), errors.Is(err, session.ErrLeaseConflict), errors.Is(err, session.ErrLeaseNotFound):
		return string(domain.ErrorClassInternal)
	}
	var transportErr *transport.Error
	if errors.As(err, &transportErr) {
		return string(domain.ErrorClassTransport)
	}
	return string(domain.ErrorClassInternal)
}
