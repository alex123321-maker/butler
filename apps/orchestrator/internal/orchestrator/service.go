package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	activity "github.com/butler/butler/apps/orchestrator/internal/activity"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/observability"
	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/sanitize"
	memoryservice "github.com/butler/butler/internal/memory/service"
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
	ListRunsBySessionKey(context.Context, string) ([]*sessionv1.RunRecord, error)
}

type TranscriptStore interface {
	AppendMessage(context.Context, transcript.Message) (transcript.Message, error)
	AppendToolCall(context.Context, transcript.ToolCall) (transcript.ToolCall, error)
	GetTranscript(context.Context, string) (transcript.Transcript, error)
}

type ToolExecutor interface {
	ExecuteToolCall(context.Context, *toolbrokerv1.ToolCall) (*toolbrokerv1.ToolResult, error)
}

type ToolCatalog interface {
	ListTools(context.Context) ([]*toolbrokerv1.ToolContract, error)
}

type ProfileMemoryStore interface {
	GetByScope(context.Context, string, string) ([]memoryservice.ProfileEntry, error)
}

type EpisodicMemoryStore interface {
	Search(context.Context, string, string, []float32, int) ([]memoryservice.Episode, error)
	FindBySummary(context.Context, string, string, string) ([]memoryservice.Episode, error)
}

type WorkingMemoryStore interface {
	Get(context.Context, string) (WorkingMemorySnapshot, error)
	Save(context.Context, WorkingMemorySnapshot) (WorkingMemorySnapshot, error)
	Clear(context.Context, string) error
}

type TransientWorkingStore interface {
	Get(context.Context, string, string) (TransientWorkingState, error)
	Save(context.Context, TransientWorkingState, time.Duration) (TransientWorkingState, error)
	Clear(context.Context, string, string) error
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

type MemoryBundleService interface {
	BuildBundle(ctx context.Context, req memoryservice.BundleRequest) (memoryservice.Bundle, error)
}

type ChunkMemoryStore interface {
	Search(context.Context, string, string, []float32, int) ([]memoryservice.Chunk, error)
	FindByTitle(context.Context, string, string, string, int) ([]memoryservice.Chunk, error)
}

type WorkingMemorySnapshot struct {
	MemoryType       string
	SessionKey       string
	RunID            string
	Goal             string
	EntitiesJSON     string
	PendingStepsJSON string
	ScratchJSON      string
	Status           string
	SourceType       string
	SourceID         string
	ProvenanceJSON   string
}

type WorkingMemoryPolicy struct {
	OnCompleted string
	OnFailed    string
	OnCancelled string
	OnTimedOut  string
}

var ErrWorkingMemoryNotFound = errors.New("working memory snapshot not found")

var ErrTransientWorkingStateNotFound = errors.New("transient working state not found")

type TransientWorkingState struct {
	SessionKey  string
	RunID       string
	Status      string
	ScratchJSON string
	UpdatedAt   string
}

// TransitionLogger records run state transitions for observability.
type TransitionLogger interface {
	InsertTransition(ctx context.Context, t runservice.StateTransition) error
}

type Config struct {
	ProviderName     string
	ModelName        string
	OwnerID          string
	LeaseTTL         int64
	Delivery         DeliverySink
	Tools            ToolExecutor
	ToolCatalog      ToolCatalog
	ApprovalChecker  ApprovalChecker
	ApprovalGate     *ApprovalGate
	ApprovalService  *approvals.Service
	ProfileStore     ProfileMemoryStore
	EpisodeStore     EpisodicMemoryStore
	ChunkStore       ChunkMemoryStore
	Embeddings       EmbeddingProvider
	PipelineEnqueuer PipelineEnqueuer
	SummaryReader    SessionSummaryReader
	WorkingStore     WorkingMemoryStore
	WorkingPolicy    WorkingMemoryPolicy
	TransientStore   TransientWorkingStore
	TransientTTL     time.Duration
	MemoryBundles    MemoryBundleService
	PromptManager    PromptManager
	PromptAssembler  PromptAssembler
	ProfileLimit     int
	EpisodeLimit     int
	MemoryScopes     []string
	TransitionLogger TransitionLogger
	EventHub         *observability.Hub
	Artifacts        *artifacts.Service
	Activity         *activity.Service
}

type PromptManager interface {
	Get(context.Context) (promptmgmt.ConfigState, error)
	Update(context.Context, promptmgmt.UpdateRequest) (promptmgmt.ConfigState, error)
}

type PromptAssembler interface {
	Assemble(promptmgmt.ConfigState, promptmgmt.Context) promptmgmt.Assembly
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
	Prompt        promptmgmt.Assembly
	ToolDefs      []transport.ToolDefinition
	WorkingMemory *workingMemoryContext
	InputPayload  map[string]any
	SessionUserID string
	Channel       string

	// Deferred observability payloads — populated during prepareRun, emitted after CreateRun.
	observabilityMemory  map[string]any
	observabilityHistory map[string]any
	observabilityTools   map[string]any
	observabilityPrompt  map[string]any
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
	if cfg.TransientTTL <= 0 {
		cfg.TransientTTL = 30 * time.Minute
	}
	if strings.TrimSpace(cfg.WorkingPolicy.OnCompleted) == "" {
		cfg.WorkingPolicy.OnCompleted = "clear"
	}
	if strings.TrimSpace(cfg.WorkingPolicy.OnFailed) == "" {
		cfg.WorkingPolicy.OnFailed = "retain"
	}
	if strings.TrimSpace(cfg.WorkingPolicy.OnCancelled) == "" {
		cfg.WorkingPolicy.OnCancelled = "retain"
	}
	if strings.TrimSpace(cfg.WorkingPolicy.OnTimedOut) == "" {
		cfg.WorkingPolicy.OnTimedOut = "retain"
	}
	if cfg.MemoryBundles == nil {
		cfg.MemoryBundles = memoryservice.New(memoryservice.Config{
			ProfileStore:  cfg.ProfileStore,
			EpisodeStore:  cfg.EpisodeStore,
			ChunkStore:    cfg.ChunkStore,
			WorkingStore:  memoryBundleWorkingStore{store: cfg.WorkingStore},
			SummaryReader: cfg.SummaryReader,
			Embeddings:    cfg.Embeddings,
			ProfileLimit:  cfg.ProfileLimit,
			EpisodeLimit:  cfg.EpisodeLimit,
			ChunkLimit:    2,
			ScopeOrder:    cfg.MemoryScopes,
			Log:           log,
			Metrics:       nil,
		})
	}
	if cfg.PromptManager == nil {
		cfg.PromptManager = promptmgmt.NewStaticManager()
	}
	if cfg.PromptAssembler == nil {
		cfg.PromptAssembler = promptmgmt.NewAssembler()
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

	// Emit deferred observability events now that we have a run ID.
	if prepared.observabilityMemory != nil {
		s.emitEvent(runRecord.GetRunId(), event.SessionKey, observability.EventMemoryLoaded, prepared.observabilityMemory)
	}
	if prepared.observabilityTools != nil {
		s.emitEvent(runRecord.GetRunId(), event.SessionKey, observability.EventToolsLoaded, prepared.observabilityTools)
	}
	if prepared.observabilityPrompt != nil {
		s.emitEvent(runRecord.GetRunId(), event.SessionKey, observability.EventPromptAssembled, prepared.observabilityPrompt)
	}

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

	// Notify channel immediately so the user sees that the bot is working.
	if statusErr := s.config.Delivery.DeliverStatusEvent(ctx, StatusEvent{
		RunID:      current.GetRunId(),
		SessionKey: event.SessionKey,
		Status:     "thinking",
	}); statusErr != nil {
		runLog.Warn("failed to deliver thinking status", slog.String("error", statusErr.Error()))
	}

	if _, err := s.transcript.AppendMessage(ctx, transcript.Message{
		SessionKey:   event.SessionKey,
		RunID:        current.GetRunId(),
		Role:         "user",
		Content:      prepared.UserMessage,
		MetadataJSON: mustMarshalJSON(map[string]any{"event_id": event.EventID, "source": event.Source}),
	}); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("append user transcript: %w", err))
	}

	if err := s.savePreparedWorkingMemory(ctx, current.GetRunId(), event.SessionKey, prepared); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("save working memory: %w", err))
	}
	if err := s.saveTransientWorkingState(ctx, current.GetRunId(), event.SessionKey, "preparing", map[string]any{"goal": prepared.WorkingMemory.Goal}); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("save transient working memory: %w", err))
	}
	current = s.attachReusableProviderSessionRef(ctx, current, runLog)

	next, err = s.transition(ctx, current.GetRunId(), current.GetCurrentState(), commonv1.RunState_RUN_STATE_MODEL_RUNNING, lease.LeaseID, "", "")
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("mark run model_running: %w", err))
	}
	current = next

	existingProviderSessionRef, err := transport.ParseProviderSessionRef(current.GetProviderSessionRef())
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("parse persisted provider session ref: %w", err))
	}

	transportCtx := transport.WithSessionKey(runCtx, event.SessionKey)

	stream, err := s.provider.StartRun(transportCtx, transport.StartRunRequest{
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
		ToolDefinitions:  prepared.ToolDefs,
		StreamingEnabled: true,
	})
	if err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, err)
	}

	finalMessage, current, err := s.consumeModelStream(transportCtx, runLog, current, event.SessionKey, lease.LeaseID, stream)
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
	if s.config.Artifacts != nil {
		if _, saveErr := s.config.Artifacts.SaveAssistantFinal(ctx, current.GetRunId(), event.SessionKey, finalMessage, time.Now().UTC()); saveErr != nil {
			runLog.Warn("failed to persist assistant_final artifact", slog.String("run_id", current.GetRunId()), slog.String("error", saveErr.Error()))
		}
	}

	if err := s.finalizeWorkingMemory(ctx, event.SessionKey, current.GetRunId(), commonv1.RunState_RUN_STATE_COMPLETED, finalMessage); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("finalize working memory: %w", err))
	}
	if err := s.finalizeTransientWorkingState(ctx, event.SessionKey, current.GetRunId(), commonv1.RunState_RUN_STATE_COMPLETED, finalMessage); err != nil {
		return nil, s.failRun(ctx, current, lease.LeaseID, runLog, fmt.Errorf("finalize transient working memory: %w", err))
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
	s.emitEvent(next.GetRunId(), event.SessionKey, observability.EventRunCompleted, map[string]any{
		"response_length": len(finalMessage),
	})

	// Schedule EventHub cleanup to free subscriber maps after SSE clients have drained.
	s.scheduleHubCleanup(next.GetRunId())

	return &ExecutionResult{RunID: next.GetRunId(), SessionKey: event.SessionKey, CurrentState: next.GetCurrentState(), AssistantResponse: finalMessage}, nil
}

func (s *Service) consumeModelStream(ctx context.Context, runLog *slog.Logger, current *sessionv1.RunRecord, sessionKey, leaseID string, initialStream transport.EventStream) (string, *sessionv1.RunRecord, error) {
	var finalMessage string
	stream := initialStream

	for stream != nil {
		var nextStream transport.EventStream
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
					s.emitEvent(current.GetRunId(), sessionKey, observability.EventAssistantDelta, map[string]any{
						"sequence_no":    transportEvent.AssistantDelta.SequenceNo,
						"content_length": len(transportEvent.AssistantDelta.Content),
					})
				}
			case transport.EventTypeAssistantFinal:
				if transportEvent.AssistantFinal != nil {
					finalMessage = transportEvent.AssistantFinal.Content
					if err := s.config.Delivery.DeliverAssistantFinal(ctx, DeliveryEvent{RunID: current.GetRunId(), SessionKey: sessionKey, Content: finalMessage, Final: true}); err != nil {
						return "", current, err
					}
					s.emitEvent(current.GetRunId(), sessionKey, observability.EventAssistantFinal, map[string]any{
						"content_length": len(finalMessage),
						"finish_reason":  transportEvent.AssistantFinal.FinishReason,
					})
				}
			case transport.EventTypeToolCallRequested:
				resumed, updated, err := s.handleToolCall(ctx, runLog, current, leaseID, transportEvent.ToolCall)
				if err != nil {
					return "", updated, err
				}
				current = updated
				nextStream = resumed
				break
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
		stream = nextStream
	}
	return finalMessage, current, nil
}

func (s *Service) failRun(ctx context.Context, current *sessionv1.RunRecord, leaseID string, runLog *slog.Logger, err error) error {
	runLog.Error("run execution failed", slog.String("error", err.Error()))
	s.emitEvent(current.GetRunId(), current.GetSessionKey(), observability.EventRunError, map[string]any{
		"error_message": truncateForObservability(err.Error(), 500),
		"error_type":    classifyExecutionError(err),
	})
	state := current.GetCurrentState()
	if current != nil {
		if finalizeErr := s.finalizeWorkingMemory(ctx, current.GetSessionKey(), current.GetRunId(), commonv1.RunState_RUN_STATE_FAILED, err.Error()); finalizeErr != nil {
			runLog.Warn("working memory finalization failed on error", slog.String("error", finalizeErr.Error()))
		}
		if finalizeErr := s.finalizeTransientWorkingState(ctx, current.GetSessionKey(), current.GetRunId(), commonv1.RunState_RUN_STATE_FAILED, err.Error()); finalizeErr != nil {
			runLog.Warn("transient working memory finalization failed on error", slog.String("error", finalizeErr.Error()))
		}
	}
	if state == commonv1.RunState_RUN_STATE_UNSPECIFIED || state == commonv1.RunState_RUN_STATE_FAILED || state == commonv1.RunState_RUN_STATE_CANCELLED || state == commonv1.RunState_RUN_STATE_TIMED_OUT || state == commonv1.RunState_RUN_STATE_COMPLETED {
		return err
	}
	if _, transitionErr := s.transition(ctx, current.GetRunId(), state, commonv1.RunState_RUN_STATE_FAILED, leaseID, classifyExecutionError(err), err.Error()); transitionErr != nil {
		return fmt.Errorf("%w: additionally failed to transition run to failed: %v", err, transitionErr)
	}
	s.scheduleHubCleanup(current.GetRunId())
	return err
}

func (s *Service) attachReusableProviderSessionRef(ctx context.Context, current *sessionv1.RunRecord, runLog *slog.Logger) *sessionv1.RunRecord {
	if current == nil || s.runs == nil {
		return current
	}
	if strings.TrimSpace(current.GetProviderSessionRef()) != "" {
		return current
	}
	runs, err := s.runs.ListRunsBySessionKey(ctx, current.GetSessionKey())
	if err != nil {
		runLog.Warn("provider session reuse skipped; run lookup failed", slog.String("error", err.Error()))
		return current
	}
	ref, sourceRunID := latestReusableProviderSessionRef(runs, current.GetRunId(), s.config.ProviderName)
	if ref == nil {
		return current
	}
	encoded, err := transport.MarshalProviderSessionRef(ref)
	if err != nil {
		runLog.Warn("provider session reuse skipped; marshal failed", slog.String("source_run_id", sourceRunID), slog.String("error", err.Error()))
		return current
	}
	updated, err := s.runs.PersistProviderSessionRef(ctx, current.GetRunId(), encoded)
	if err != nil {
		runLog.Warn("provider session reuse skipped; persist failed", slog.String("source_run_id", sourceRunID), slog.String("error", err.Error()))
		return current
	}
	runLog.Info("provider session ref reused for new run", slog.String("source_run_id", sourceRunID), slog.String("run_id", current.GetRunId()))
	return updated
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

func latestReusableProviderSessionRef(runs []*sessionv1.RunRecord, currentRunID, providerName string) (*transport.ProviderSessionRef, string) {
	var selected *transport.ProviderSessionRef
	selectedRunID := ""
	selectedAt := time.Time{}
	for _, runRecord := range runs {
		if runRecord == nil || strings.TrimSpace(runRecord.GetRunId()) == strings.TrimSpace(currentRunID) {
			continue
		}
		if providerName != "" && !strings.EqualFold(strings.TrimSpace(runRecord.GetModelProvider()), strings.TrimSpace(providerName)) {
			continue
		}
		if strings.TrimSpace(runRecord.GetProviderSessionRef()) == "" {
			continue
		}
		ref, err := transport.ParseProviderSessionRef(runRecord.GetProviderSessionRef())
		if err != nil || ref == nil {
			continue
		}
		if providerName != "" && strings.TrimSpace(ref.ProviderName) != "" && !strings.EqualFold(strings.TrimSpace(ref.ProviderName), strings.TrimSpace(providerName)) {
			continue
		}
		candidateAt := runRecordTime(runRecord)
		if selected == nil || candidateAt.After(selectedAt) {
			copied := *ref
			selected = &copied
			selectedRunID = runRecord.GetRunId()
			selectedAt = candidateAt
		}
	}
	return selected, selectedRunID
}

func runRecordTime(runRecord *sessionv1.RunRecord) time.Time {
	if runRecord == nil {
		return time.Time{}
	}
	for _, value := range []string{runRecord.GetStartedAt(), runRecord.GetUpdatedAt(), runRecord.GetFinishedAt()} {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
		if err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
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

	now := time.Now().UTC()
	fromStr := runStateString(from)
	toStr := runStateString(to)

	// Log state transition to durable store (fire-and-forget).
	if s.config.TransitionLogger != nil {
		metaPayload := map[string]any{}
		if leaseID != "" {
			metaPayload["lease_id"] = leaseID
		}
		if errorType != "" {
			metaPayload["error_type"] = errorType
		}
		if errorMessage != "" {
			metaPayload["error_message"] = errorMessage
		}
		if logErr := s.config.TransitionLogger.InsertTransition(ctx, runservice.StateTransition{
			RunID:          runID,
			FromState:      fromStr,
			ToState:        toStr,
			TriggeredBy:    "orchestrator",
			MetadataJSON:   mustMarshalJSON(metaPayload),
			TransitionedAt: now,
		}); logErr != nil {
			s.log.Warn("failed to log state transition", slog.String("run_id", runID), slog.String("error", logErr.Error()))
		}
	}

	// Publish observability event (non-blocking).
	s.emitEvent(runID, resp.GetSessionKey(), observability.EventStateTransition, map[string]any{
		"from_state": fromStr,
		"to_state":   toStr,
	})

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
		ToolDefs:      nil,
		WorkingMemory: &workingMemoryContext{Status: "idle", Policy: s.config.WorkingPolicy, Scratch: map[string]any{}},
		InputPayload:  payload,
		SessionUserID: userID,
		Channel:       channel,
	}
	if err := s.attachMemoryContext(ctx, event.SessionKey, prepared.SessionUserID, message, &prepared); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachTranscriptContext(ctx, event.SessionKey, &prepared); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachToolContext(ctx, &prepared); err != nil {
		return preparedRun{}, err
	}
	if err := s.attachPromptContext(ctx, &prepared); err != nil {
		return preparedRun{}, err
	}
	return prepared, nil
}

func (s *Service) attachMemoryContext(ctx context.Context, sessionKey, userID, userMessage string, prepared *preparedRun) error {
	if prepared == nil {
		return nil
	}
	bundle, err := s.config.MemoryBundles.BuildBundle(ctx, memoryservice.BundleRequest{
		SessionKey:   sessionKey,
		UserID:       userID,
		UserMessage:  userMessage,
		IncludeQuery: true,
	})
	if err != nil {
		return err
	}
	workingMemory := workingMemoryFromBundle(bundle.Working, s.config.WorkingPolicy)
	prepared.WorkingMemory = workingMemory
	if len(bundle.Items) == 0 && workingMemoryIsEmpty(workingMemory) && strings.TrimSpace(bundle.Prompt) == "" {
		return nil
	}
	for key, value := range bundle.Items {
		prepared.MemoryBundle[key] = value
	}

	// Store memory_loaded observability payload for deferred emission after CreateRun.
	bundleKeys := make([]string, 0, len(bundle.Items))
	for key := range bundle.Items {
		bundleKeys = append(bundleKeys, key)
	}
	prepared.observabilityMemory = map[string]any{
		"bundle_keys":  bundleKeys,
		"has_working":  !workingMemoryIsEmpty(workingMemory),
		"has_prompt":   strings.TrimSpace(bundle.Prompt) != "",
		"bundle_count": len(bundle.Items),
	}

	return nil
}

func (s *Service) attachToolContext(ctx context.Context, prepared *preparedRun) error {
	if prepared == nil || s.config.ToolCatalog == nil {
		return nil
	}
	contracts, err := s.config.ToolCatalog.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("load tool contracts: %w", err)
	}
	prepared.ToolDefs = toolDefinitionsFromContracts(contracts)
	if summary := toolSummaryFromContracts(contracts); strings.TrimSpace(summary) != "" {
		prepared.MemoryBundle["tool_summary"] = summary
	}

	// Store tools_loaded observability payload for deferred emission after CreateRun.
	toolNames := make([]string, 0, len(prepared.ToolDefs))
	for _, td := range prepared.ToolDefs {
		toolNames = append(toolNames, td.Name)
	}
	prepared.observabilityTools = map[string]any{
		"tool_count": len(prepared.ToolDefs),
		"tool_names": toolNames,
	}

	return nil
}

func (s *Service) attachTranscriptContext(ctx context.Context, sessionKey string, prepared *preparedRun) error {
	if prepared == nil || s.transcript == nil {
		return nil
	}
	if strings.TrimSpace(stringFromAny(prepared.MemoryBundle["session_summary"])) != "" {
		return nil
	}
	full, err := s.transcript.GetTranscript(ctx, sessionKey)
	if err != nil {
		return nil
	}
	items := recentTranscriptInputItems(full, 10)
	if len(items) == 0 {
		return nil
	}
	prepared.InputItems = append(items, prepared.InputItems...)
	prepared.observabilityHistory = map[string]any{
		"replayed_message_count": len(items),
		"fallback_used":          true,
	}
	return nil
}

func (s *Service) attachPromptContext(ctx context.Context, prepared *preparedRun) error {
	if prepared == nil || s.config.PromptManager == nil || s.config.PromptAssembler == nil {
		return nil
	}
	state, err := s.config.PromptManager.Get(ctx)
	if err != nil {
		return fmt.Errorf("load prompt config: %w", err)
	}
	prepared.Prompt = s.config.PromptAssembler.Assemble(state, promptmgmt.Context{
		SessionSummary:  stringFromAny(prepared.MemoryBundle["session_summary"]),
		Working:         workingContextToPromptContext(prepared.MemoryBundle["working"]),
		Profile:         sliceOfMaps(prepared.MemoryBundle["profile"]),
		Episodes:        sliceOfMaps(prepared.MemoryBundle["episodes"]),
		Chunks:          sliceOfMaps(prepared.MemoryBundle["chunks"]),
		ToolSummary:     stringFromAny(prepared.MemoryBundle["tool_summary"]),
		BrowserStrategy: promptmgmt.BrowserStrategyContent,
	})
	if strings.TrimSpace(prepared.Prompt.FinalPrompt) != "" {
		prepared.InputItems = append([]transport.InputItem{{Role: "system", Content: prepared.Prompt.FinalPrompt, ContentType: "text/plain"}}, prepared.InputItems...)
	}

	// Store prompt_assembled observability payload for deferred emission after CreateRun.
	memorySections := make([]string, 0)
	for _, key := range []string{"session_summary", "working", "profile", "episodes", "chunks", "tool_summary"} {
		if _, ok := prepared.MemoryBundle[key]; ok {
			memorySections = append(memorySections, key)
		}
	}
	prepared.observabilityPrompt = map[string]any{
		"has_system_prompt": strings.TrimSpace(prepared.Prompt.FinalPrompt) != "",
		"memory_sections":   memorySections,
		"prompt_length":     len(prepared.Prompt.FinalPrompt),
	}

	return nil
}

func recentTranscriptInputItems(full transcript.Transcript, limit int) []transport.InputItem {
	if limit <= 0 || len(full.Messages) == 0 {
		return nil
	}
	messages := make([]transcript.Message, 0, len(full.Messages))
	for _, msg := range full.Messages {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(sanitize.TranscriptMessageContent(msg.Content))
		if content == "" {
			continue
		}
		msg.Content = truncateTranscriptReplay(content, 1200)
		messages = append(messages, msg)
	}
	if len(messages) == 0 {
		return nil
	}
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	items := make([]transport.InputItem, 0, len(messages))
	for _, msg := range messages {
		items = append(items, transport.InputItem{Role: strings.ToLower(strings.TrimSpace(msg.Role)), Content: msg.Content, ContentType: "text/plain"})
	}
	return items
}

func truncateTranscriptReplay(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= len("[truncated]") {
		return value[:limit]
	}
	return strings.TrimSpace(value[:limit-len("[truncated]")-1]) + "\n[truncated]"
}
