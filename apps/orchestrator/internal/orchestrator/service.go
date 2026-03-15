package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/domain"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/embeddings"
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
	AppendToolCall(context.Context, transcript.ToolCall) (transcript.ToolCall, error)
}

type ToolExecutor interface {
	ExecuteToolCall(context.Context, *toolbrokerv1.ToolCall) (*toolbrokerv1.ToolResult, error)
}

type ProfileMemoryStore interface {
	GetByScope(context.Context, string, string) ([]MemoryProfileEntry, error)
}

type EpisodicMemoryStore interface {
	Search(context.Context, string, string, []float32, int) ([]MemoryEpisode, error)
}

type EmbeddingProvider interface {
	EmbedQuery(context.Context, string) ([]float32, error)
}

// PipelineEnqueuer enqueues async memory pipeline jobs after a run completes.
type PipelineEnqueuer interface {
	EnqueuePostRun(ctx context.Context, runID, sessionKey string) error
}

// SessionSummaryReader retrieves the current session summary for context assembly.
type SessionSummaryReader interface {
	GetSummary(ctx context.Context, sessionKey string) (string, error)
}

type MemoryProfileEntry interface {
	ProfileKey() string
	ProfileSummary() string
}

type MemoryEpisode interface {
	EpisodeSummary() string
	EpisodeDistance() float64
}

type Config struct {
	ProviderName     string
	ModelName        string
	OwnerID          string
	LeaseTTL         int64
	Delivery         DeliverySink
	Tools            ToolExecutor
	ApprovalChecker  ApprovalChecker
	ApprovalGate     *ApprovalGate
	ProfileStore     ProfileMemoryStore
	EpisodeStore     EpisodicMemoryStore
	Embeddings       EmbeddingProvider
	PipelineEnqueuer PipelineEnqueuer
	SummaryReader    SessionSummaryReader
	ProfileLimit     int
	EpisodeLimit     int
	MemoryScopes     []string
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

type memoryScope struct {
	Type string
	ID   string
}

type episodeMemoryItem struct {
	Summary  string
	Distance float64
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
	if cfg.ProfileLimit <= 0 {
		cfg.ProfileLimit = 20
	}
	if cfg.EpisodeLimit <= 0 {
		cfg.EpisodeLimit = 3
	}
	if len(cfg.MemoryScopes) == 0 {
		cfg.MemoryScopes = []string{"session", "user", "global"}
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
	prepared, err := s.prepareRun(ctx, event)
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
		return nil, s.failRun(ctx, current, "", runLog, fmt.Errorf("queue run: %w", err))
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
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run model_running: %w", err))
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

	finalMessage, current, err := s.consumeModelStream(runCtx, runLog, current, event.SessionKey, lease.LeaseID, stream)
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
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

	// Enqueue async memory pipeline job (fire-and-forget; failure does not block the run).
	if s.config.PipelineEnqueuer != nil {
		if enqueueErr := s.config.PipelineEnqueuer.EnqueuePostRun(ctx, current.GetRunId(), event.SessionKey); enqueueErr != nil {
			runLog.Warn("failed to enqueue memory pipeline job",
				slog.String("run_id", current.GetRunId()),
				slog.String("error", enqueueErr.Error()),
			)
		} else {
			runLog.Info("memory pipeline job enqueued", slog.String("run_id", current.GetRunId()))
		}
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_COMPLETED, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run completed: %w", err))
	}

	runLog.Info("run completed", slog.String("session_key", event.SessionKey))
	return &ExecutionResult{RunID: next.GetRunId(), SessionKey: event.SessionKey, CurrentState: next.GetCurrentState(), AssistantResponse: finalMessage}, nil
}

func (s *Service) consumeModelStream(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, sessionKey, leaseID string, stream transport.EventStream) (string, *sessionv1.RunRecord, error) {
	var finalMessage string
	for transportEvent := range stream {
		var err error
		if current, err = s.persistProviderSessionRef(ctx, current, transportEvent.ProviderSessionRef, runLog); err != nil {
			return "", current, err
		}
		switch transportEvent.EventType {
		case transport.EventTypeAssistantDelta:
			if transportEvent.AssistantDelta != nil {
				if err := s.config.Delivery.DeliverAssistantDelta(ctx, DeliveryEvent{RunID: current.GetRunId(), SessionKey: sessionKey, Content: transportEvent.AssistantDelta.Content, SequenceNo: transportEvent.AssistantDelta.SequenceNo}); err != nil {
					return "", current, err
				}
			}
		case transport.EventTypeAssistantFinal:
			if transportEvent.AssistantFinal != nil {
				finalMessage = transportEvent.AssistantFinal.Content
				if err := s.config.Delivery.DeliverAssistantFinal(ctx, DeliveryEvent{RunID: current.GetRunId(), SessionKey: sessionKey, Content: finalMessage, Final: true}); err != nil {
					return "", current, err
				}
			}
		case transport.EventTypeToolCallRequested:
			resumed, updated, err := s.handleToolCall(ctx, runLog, current, leaseID, transportEvent.ToolCall)
			if err != nil {
				return "", updated, err
			}
			current = updated
			resumedFinal, updated, err := s.consumeModelStream(ctx, runLog, current, sessionKey, leaseID, resumed)
			if err != nil {
				return "", updated, err
			}
			current = updated
			if strings.TrimSpace(resumedFinal) != "" {
				finalMessage = resumedFinal
			}
		case transport.EventTypeToolCallBatchRequested:
			resumedFinal, updated, err := s.handleToolBatch(ctx, runLog, current, sessionKey, leaseID, transportEvent.ToolCallBatch)
			if err != nil {
				return "", updated, err
			}
			current = updated
			if strings.TrimSpace(resumedFinal) != "" {
				finalMessage = resumedFinal
			}
		case transport.EventTypeTransportError:
			return "", current, transportError(transportEvent.TransportError)
		case transport.EventTypeTransportWarning:
			runLog.Warn("transport warning", slog.String("payload", transportEvent.PayloadJSON))
		}
	}
	return finalMessage, current, nil
}

func (s *Service) handleToolBatch(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, sessionKey, leaseID string, batch *transport.ToolCallBatch) (string, *sessionv1.RunRecord, error) {
	if batch == nil || len(batch.ToolCalls) == 0 {
		return "", current, nil
	}
	var finalMessage string
	for _, toolCall := range batch.ToolCalls {
		resumed, updated, err := s.handleToolCall(ctx, runLog, current, leaseID, &toolCall)
		if err != nil {
			return "", updated, err
		}
		current = updated
		resumedFinal, updated, err := s.consumeModelStream(ctx, runLog, current, sessionKey, leaseID, resumed)
		if err != nil {
			return "", updated, err
		}
		current = updated
		if strings.TrimSpace(resumedFinal) != "" {
			finalMessage = resumedFinal
		}
	}
	return finalMessage, current, nil
}

func (s *Service) handleToolCall(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, leaseID string, requested *transport.ToolCallRequest) (transport.EventStream, *sessionv1.RunRecord, error) {
	if requested == nil {
		return nil, current, fmt.Errorf("tool call request is required")
	}
	if s.config.Tools == nil {
		return nil, current, fmt.Errorf("tool executor is not configured")
	}
	next, err := s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_TOOL_PENDING, leaseID, "", "")
	if err != nil {
		return nil, current, fmt.Errorf("mark run tool_pending: %w", err)
	}
	current = next

	toolCallID := requested.ToolCallRef
	if strings.TrimSpace(toolCallID) == "" {
		toolCallID = fmt.Sprintf("tool-%s-%d", current.GetRunId(), time.Now().UTC().UnixNano())
	}
	brokerCall := &toolbrokerv1.ToolCall{ToolCallId: toolCallID, RunId: current.GetRunId(), ToolName: requested.ToolName, ArgsJson: requested.ArgsJSON, Status: "requested", AutonomyMode: current.GetAutonomyMode()}

	// Check if tool requires approval before execution.
	if s.config.ApprovalChecker != nil && s.config.ApprovalGate != nil {
		needsApproval, checkErr := s.config.ApprovalChecker.RequiresApproval(ctx, requested.ToolName)
		if checkErr != nil {
			runLog.Warn("approval check failed, proceeding without approval", slog.String("tool_name", requested.ToolName), slog.String("error", checkErr.Error()))
		} else if needsApproval {
			next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_AWAITING_APPROVAL, leaseID, "", "")
			if err != nil {
				return nil, current, fmt.Errorf("mark run awaiting_approval: %w", err)
			}
			current = next

			if deliveryErr := s.config.Delivery.DeliverApprovalRequest(ctx, ApprovalRequest{
				RunID:      current.GetRunId(),
				SessionKey: current.GetSessionKey(),
				ToolCallID: toolCallID,
				ToolName:   requested.ToolName,
				ArgsJSON:   requested.ArgsJSON,
			}); deliveryErr != nil {
				return nil, current, fmt.Errorf("deliver approval request: %w", deliveryErr)
			}

			resp, waitErr := s.config.ApprovalGate.Wait(ctx, toolCallID)
			if waitErr != nil {
				return nil, current, fmt.Errorf("wait for approval: %w", waitErr)
			}
			if !resp.Approved {
				rejectedResult := &toolbrokerv1.ToolResult{
					ToolCallId: toolCallID,
					RunId:      current.GetRunId(),
					ToolName:   requested.ToolName,
					Status:     "rejected",
					ResultJson: `{"rejected":true,"reason":"user rejected tool call"}`,
				}
				next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME, leaseID, "", "")
				if err != nil {
					return nil, current, fmt.Errorf("mark run awaiting_model_resume after rejection: %w", err)
				}
				current = next
				stream, submitErr := s.provider.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: current.GetRunId(), ProviderSessionRef: providerSessionRefFromRun(current), ToolCallRef: requested.ToolCallRef, ToolResultJSON: toolResultEnvelope(rejectedResult)})
				if submitErr != nil {
					return nil, current, fmt.Errorf("submit rejected tool result: %w", submitErr)
				}
				next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, leaseID, "", "")
				if err != nil {
					return nil, current, fmt.Errorf("mark run model_running after rejection: %w", err)
				}
				current = next
				return stream, current, nil
			}
			runLog.Info("tool call approved", slog.String("tool_call_id", toolCallID), slog.String("tool_name", requested.ToolName))
		}
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_TOOL_RUNNING, leaseID, "", "")
	if err != nil {
		return nil, current, fmt.Errorf("mark run tool_running: %w", err)
	}
	current = next

	startedAt := time.Now().UTC()
	result, err := s.config.Tools.ExecuteToolCall(ctx, brokerCall)
	if err != nil {
		return nil, current, fmt.Errorf("execute tool call %s: %w", requested.ToolName, err)
	}
	finishedAt := time.Now().UTC()
	if _, err := s.transcript.AppendToolCall(ctx, transcript.ToolCall{ToolCallID: toolCallID, RunID: current.GetRunId(), ToolName: requested.ToolName, ArgsJSON: normalizeJSON(requested.ArgsJSON, "{}"), Status: normalizeToolStatus(result.GetStatus()), RuntimeTarget: brokerCall.GetRuntimeTarget(), StartedAt: startedAt, FinishedAt: &finishedAt, ResultJSON: toolResultPayload(result), ErrorJSON: toolErrorPayload(result.GetError())}); err != nil {
		return nil, current, fmt.Errorf("append tool transcript: %w", err)
	}

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME, leaseID, "", "")
	if err != nil {
		return nil, current, fmt.Errorf("mark run awaiting_model_resume: %w", err)
	}
	current = next

	stream, err := s.provider.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: current.GetRunId(), ProviderSessionRef: providerSessionRefFromRun(current), ToolCallRef: requested.ToolCallRef, ToolResultJSON: toolResultEnvelope(result)})
	if err != nil {
		return nil, current, fmt.Errorf("submit tool result: %w", err)
	}
	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, leaseID, "", "")
	if err != nil {
		return nil, current, fmt.Errorf("mark run model_running after tool: %w", err)
	}
	current = next
	return stream, current, nil
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

func (s *Service) prepareRun(ctx context.Context, event ingress.InputEvent) (preparedRun, error) {
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
	prepared := preparedRun{
		InputItems:    []transport.InputItem{{Role: "user", Content: message, ContentType: "text/plain"}},
		UserMessage:   message,
		MemoryBundle:  map[string]any{},
		InputPayload:  payload,
		SessionUserID: userID,
		Channel:       channel,
	}
	if err := s.attachMemoryContext(ctx, event.SessionKey, prepared.SessionUserID, message, &prepared); err != nil {
		return preparedRun{}, err
	}
	return prepared, nil
}

func (s *Service) attachMemoryContext(ctx context.Context, sessionKey, userID, userMessage string, prepared *preparedRun) error {
	if prepared == nil {
		return nil
	}
	scopes := s.memoryScopes(sessionKey, userID)
	profileEntries, err := s.loadProfileMemory(ctx, scopes)
	if err != nil {
		return err
	}
	episodes, err := s.loadEpisodes(ctx, scopes, userMessage)
	if err != nil {
		return err
	}
	sessionSummary := s.loadSessionSummary(ctx, sessionKey)
	if len(profileEntries) == 0 && len(episodes) == 0 && sessionSummary == "" {
		return nil
	}
	prepared.MemoryBundle["profile"] = profileEntries
	prepared.MemoryBundle["episodes"] = episodes
	if sessionSummary != "" {
		prepared.MemoryBundle["session_summary"] = sessionSummary
	}
	memoryPrompt := formatMemoryPrompt(profileEntries, episodes, sessionSummary)
	if strings.TrimSpace(memoryPrompt) != "" {
		prepared.InputItems = append([]transport.InputItem{{Role: "system", Content: memoryPrompt, ContentType: "text/plain"}}, prepared.InputItems...)
	}
	return nil
}

func (s *Service) loadProfileMemory(ctx context.Context, scopes []memoryScope) ([]map[string]any, error) {
	if s.config.ProfileStore == nil || s.config.ProfileLimit <= 0 {
		return nil, nil
	}
	result := make([]map[string]any, 0, s.config.ProfileLimit)
	for _, scope := range scopes {
		entries, err := s.config.ProfileStore.GetByScope(ctx, scope.Type, scope.ID)
		if err != nil {
			return nil, fmt.Errorf("load profile memory: %w", err)
		}
		for _, entry := range entries {
			result = append(result, map[string]any{"key": entry.ProfileKey(), "summary": entry.ProfileSummary(), "scope_type": scope.Type})
			if len(result) >= s.config.ProfileLimit {
				return result, nil
			}
		}
	}
	return result, nil
}

func (s *Service) loadEpisodes(ctx context.Context, scopes []memoryScope, userMessage string) ([]map[string]any, error) {
	if s.config.EpisodeStore == nil || s.config.EpisodeLimit <= 0 || strings.TrimSpace(userMessage) == "" {
		return nil, nil
	}
	if s.config.Embeddings == nil {
		s.log.Info("episodic retrieval skipped; embedding provider is not configured")
		return nil, nil
	}
	queryEmbedding, err := s.config.Embeddings.EmbedQuery(ctx, userMessage)
	if err != nil {
		s.log.Warn("episodic retrieval skipped; embedding query failed", slog.String("error", err.Error()))
		return nil, nil
	}
	if len(queryEmbedding) != embeddings.VectorDimensions {
		s.log.Warn("episodic retrieval skipped; embedding dimensions are invalid", slog.Int("dimensions", len(queryEmbedding)))
		return nil, nil
	}
	items := make([]episodeMemoryItem, 0, s.config.EpisodeLimit)
	for _, scope := range scopes {
		results, err := s.config.EpisodeStore.Search(ctx, scope.Type, scope.ID, queryEmbedding, s.config.EpisodeLimit)
		if err != nil {
			return nil, fmt.Errorf("load episodic memory: %w", err)
		}
		for _, item := range results {
			items = append(items, episodeMemoryItem{Summary: item.EpisodeSummary(), Distance: item.EpisodeDistance()})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Distance < items[j].Distance })
	if len(items) > s.config.EpisodeLimit {
		items = items[:s.config.EpisodeLimit]
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{"summary": item.Summary, "distance": item.Distance})
	}
	return result, nil
}

func (s *Service) loadSessionSummary(ctx context.Context, sessionKey string) string {
	if s.config.SummaryReader == nil {
		return ""
	}
	summary, err := s.config.SummaryReader.GetSummary(ctx, sessionKey)
	if err != nil {
		s.log.Warn("session summary retrieval failed",
			slog.String("session_key", sessionKey),
			slog.String("error", err.Error()),
		)
		return ""
	}
	return strings.TrimSpace(summary)
}

func (s *Service) memoryScopes(sessionKey, userID string) []memoryScope {
	ids := map[string]string{
		"session": strings.TrimSpace(sessionKey),
		"user":    strings.TrimSpace(userID),
		"global":  "global",
	}
	seen := make(map[string]struct{}, len(s.config.MemoryScopes))
	scopes := make([]memoryScope, 0, len(s.config.MemoryScopes))
	for _, scopeType := range s.config.MemoryScopes {
		scopeType = strings.ToLower(strings.TrimSpace(scopeType))
		scopeID := ids[scopeType]
		if scopeType == "" || scopeID == "" {
			continue
		}
		key := scopeType + ":" + scopeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		scopes = append(scopes, memoryScope{Type: scopeType, ID: scopeID})
	}
	return scopes
}

func formatMemoryPrompt(profileEntries, episodes []map[string]any, sessionSummary string) string {
	sections := make([]string, 0, 3)
	if strings.TrimSpace(sessionSummary) != "" {
		sections = append(sections, "Session summary:\n"+sessionSummary)
	}
	if len(profileEntries) > 0 {
		lines := make([]string, 0, len(profileEntries))
		for _, entry := range profileEntries {
			lines = append(lines, fmt.Sprintf("- %s: %s", entry["key"], entry["summary"]))
		}
		sections = append(sections, "Profile memory:\n"+strings.Join(lines, "\n"))
	}
	if len(episodes) > 0 {
		lines := make([]string, 0, len(episodes))
		for _, entry := range episodes {
			lines = append(lines, fmt.Sprintf("- %s", entry["summary"]))
		}
		sections = append(sections, "Relevant episodes:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n\n")
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

func providerSessionRefFromRun(runRecord *sessionv1.RunRecord) *transport.ProviderSessionRef {
	if runRecord == nil {
		return nil
	}
	ref, err := transport.ParseProviderSessionRef(runRecord.GetProviderSessionRef())
	if err != nil {
		return nil
	}
	return ref
}

func normalizeJSON(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return fallback
	}
	return trimmed
}

func normalizeToolStatus(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "completed"
	}
	return trimmed
}

func toolResultEnvelope(result *toolbrokerv1.ToolResult) string {
	payload := map[string]any{"status": normalizeToolStatus(result.GetStatus())}
	if json.Valid([]byte(result.GetResultJson())) {
		payload["result"] = json.RawMessage(result.GetResultJson())
	}
	if result.GetError() != nil {
		payload["error"] = map[string]any{"error_class": result.GetError().GetErrorClass().String(), "message": result.GetError().GetMessage(), "retryable": result.GetError().GetRetryable(), "details_json": result.GetError().GetDetailsJson()}
	}
	return mustMarshalJSON(payload)
}

func toolResultPayload(result *toolbrokerv1.ToolResult) string {
	return normalizeJSON(result.GetResultJson(), "{}")
}

func toolErrorPayload(toolErr *toolbrokerv1.ToolError) string {
	if toolErr == nil {
		return "{}"
	}
	payload := map[string]any{"error_class": toolErr.GetErrorClass().String(), "message": toolErr.GetMessage(), "retryable": toolErr.GetRetryable()}
	if details := strings.TrimSpace(toolErr.GetDetailsJson()); details != "" {
		if json.Valid([]byte(details)) {
			payload["details"] = json.RawMessage(details)
		} else {
			payload["details"] = details
		}
	}
	return mustMarshalJSON(payload)
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
