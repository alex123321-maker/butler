package orchestrator

import (
	"context"
	"errors"
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
	"github.com/butler/butler/internal/logger"
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

type PipelineEnqueuer interface {
	EnqueuePostRun(ctx context.Context, runID, sessionKey string) error
}

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
