package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	activity "github.com/butler/butler/apps/orchestrator/internal/activity"
	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
	telegramadapter "github.com/butler/butler/apps/orchestrator/internal/channel/telegram"
	deliveryevents "github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	"github.com/butler/butler/apps/orchestrator/internal/observability"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/config"
	orchestratorv1 "github.com/butler/butler/internal/gen/orchestrator/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/chunks"
	memorydoctor "github.com/butler/butler/internal/memory/doctor"
	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/pipeline"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/provenance"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/memory/working"
	"github.com/butler/butler/internal/metrics"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	redisstore "github.com/butler/butler/internal/storage/redis"
	"google.golang.org/grpc"
)

type App struct {
	config         config.OrchestratorConfig
	log            *slog.Logger
	metrics        *metrics.Registry
	health         *health.Service
	postgres       *postgresstore.Store
	redis          *redisstore.Store
	httpServer     *http.Server
	grpcServer     *grpc.Server
	grpcListen     net.Listener
	telegram       *telegramadapter.Adapter
	toolBroker     *tools.BrokerClient
	pipelineWorker *pipeline.Worker
}

type profileStoreAdapter struct{ store *profile.Store }

func (a profileStoreAdapter) GetByScope(ctx context.Context, scopeType, scopeID string) ([]memoryservice.ProfileEntry, error) {
	entries, err := a.store.GetByScope(ctx, scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	result := make([]memoryservice.ProfileEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result, nil
}

type episodicStoreAdapter struct{ store *episodic.Store }

type memoryEpisodeExactMatch struct{ episodic.Episode }

func (m memoryEpisodeExactMatch) EpisodeSummary() string   { return m.Summary }
func (m memoryEpisodeExactMatch) EpisodeDistance() float64 { return 0.25 }

func (a episodicStoreAdapter) Search(ctx context.Context, scopeType, scopeID string, embedding []float32, limit int) ([]memoryservice.Episode, error) {
	entries, err := a.store.Search(ctx, scopeType, scopeID, embedding, limit)
	if err != nil {
		return nil, err
	}
	result := make([]memoryservice.Episode, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result, nil
}

func (a episodicStoreAdapter) FindBySummary(ctx context.Context, scopeType, scopeID, summary string) ([]memoryservice.Episode, error) {
	entries, err := a.store.FindBySummary(ctx, scopeType, scopeID, summary)
	if err != nil {
		return nil, err
	}
	result := make([]memoryservice.Episode, 0, len(entries))
	for _, entry := range entries {
		result = append(result, memoryEpisodeExactMatch{Episode: entry})
	}
	return result, nil
}

type chunkStoreAdapter struct{ store *chunks.Store }

type memoryChunkExactMatch struct{ chunks.Chunk }

func (m memoryChunkExactMatch) ChunkTitle() string     { return m.Title }
func (m memoryChunkExactMatch) ChunkSummary() string   { return m.Summary }
func (m memoryChunkExactMatch) ChunkDistance() float64 { return 0.25 }

func (a chunkStoreAdapter) Search(ctx context.Context, scopeType, scopeID string, embedding []float32, limit int) ([]memoryservice.Chunk, error) {
	entries, err := a.store.Search(ctx, scopeType, scopeID, embedding, limit)
	if err != nil {
		return nil, err
	}
	result := make([]memoryservice.Chunk, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result, nil
}

func (a chunkStoreAdapter) FindByTitle(ctx context.Context, scopeType, scopeID, title string, limit int) ([]memoryservice.Chunk, error) {
	entries, err := a.store.FindByTitle(ctx, scopeType, scopeID, title, limit)
	if err != nil {
		return nil, err
	}
	result := make([]memoryservice.Chunk, 0, len(entries))
	for _, entry := range entries {
		result = append(result, memoryChunkExactMatch{Chunk: entry})
	}
	return result, nil
}

type workingStoreAdapter struct{ store *working.Store }

func (a workingStoreAdapter) Get(ctx context.Context, sessionKey string) (flow.WorkingMemorySnapshot, error) {
	snapshot, err := a.store.Get(ctx, sessionKey)
	if err != nil {
		if errors.Is(err, working.ErrSnapshotNotFound) {
			return flow.WorkingMemorySnapshot{}, flow.ErrWorkingMemoryNotFound
		}
		return flow.WorkingMemorySnapshot{}, err
	}
	return flow.WorkingMemorySnapshot{
		MemoryType:       snapshot.MemoryType,
		SessionKey:       snapshot.SessionKey,
		RunID:            snapshot.RunID,
		Goal:             snapshot.Goal,
		EntitiesJSON:     snapshot.EntitiesJSON,
		PendingStepsJSON: snapshot.PendingStepsJSON,
		ScratchJSON:      snapshot.ScratchJSON,
		Status:           snapshot.Status,
		SourceType:       snapshot.SourceType,
		SourceID:         snapshot.SourceID,
		ProvenanceJSON:   snapshot.ProvenanceJSON,
	}, nil
}

func (a workingStoreAdapter) Save(ctx context.Context, snapshot flow.WorkingMemorySnapshot) (flow.WorkingMemorySnapshot, error) {
	saved, err := a.store.Save(ctx, working.Snapshot{
		MemoryType:       snapshot.MemoryType,
		SessionKey:       snapshot.SessionKey,
		RunID:            snapshot.RunID,
		Goal:             snapshot.Goal,
		EntitiesJSON:     snapshot.EntitiesJSON,
		PendingStepsJSON: snapshot.PendingStepsJSON,
		ScratchJSON:      snapshot.ScratchJSON,
		Status:           snapshot.Status,
		SourceType:       snapshot.SourceType,
		SourceID:         snapshot.SourceID,
		ProvenanceJSON:   snapshot.ProvenanceJSON,
	})
	if err != nil {
		return flow.WorkingMemorySnapshot{}, err
	}
	return flow.WorkingMemorySnapshot{
		MemoryType:       saved.MemoryType,
		SessionKey:       saved.SessionKey,
		RunID:            saved.RunID,
		Goal:             saved.Goal,
		EntitiesJSON:     saved.EntitiesJSON,
		PendingStepsJSON: saved.PendingStepsJSON,
		ScratchJSON:      saved.ScratchJSON,
		Status:           saved.Status,
		SourceType:       saved.SourceType,
		SourceID:         saved.SourceID,
		ProvenanceJSON:   saved.ProvenanceJSON,
	}, nil
}

func (a workingStoreAdapter) Clear(ctx context.Context, sessionKey string) error {
	err := a.store.Clear(ctx, sessionKey)
	if err != nil && errors.Is(err, working.ErrSnapshotNotFound) {
		return flow.ErrWorkingMemoryNotFound
	}
	return err
}

type transientWorkingStoreAdapter struct{ store *working.TransientStore }

func (a transientWorkingStoreAdapter) Get(ctx context.Context, sessionKey, runID string) (flow.TransientWorkingState, error) {
	state, err := a.store.Get(ctx, sessionKey, runID)
	if err != nil {
		if errors.Is(err, working.ErrTransientStateNotFound) {
			return flow.TransientWorkingState{}, flow.ErrTransientWorkingStateNotFound
		}
		return flow.TransientWorkingState{}, err
	}
	return flow.TransientWorkingState{SessionKey: state.SessionKey, RunID: state.RunID, Status: state.Status, ScratchJSON: state.ScratchJSON, UpdatedAt: state.UpdatedAt}, nil
}

func (a transientWorkingStoreAdapter) Save(ctx context.Context, state flow.TransientWorkingState, ttl time.Duration) (flow.TransientWorkingState, error) {
	saved, err := a.store.Save(ctx, working.TransientState{SessionKey: state.SessionKey, RunID: state.RunID, Status: state.Status, ScratchJSON: state.ScratchJSON, UpdatedAt: state.UpdatedAt}, ttl)
	if err != nil {
		return flow.TransientWorkingState{}, err
	}
	return flow.TransientWorkingState{SessionKey: saved.SessionKey, RunID: saved.RunID, Status: saved.Status, ScratchJSON: saved.ScratchJSON, UpdatedAt: saved.UpdatedAt}, nil
}

func (a transientWorkingStoreAdapter) Clear(ctx context.Context, sessionKey, runID string) error {
	err := a.store.Clear(ctx, sessionKey, runID)
	if err != nil && errors.Is(err, working.ErrTransientStateNotFound) {
		return flow.ErrTransientWorkingStateNotFound
	}
	return err
}

func New(ctx context.Context) (*App, error) {
	baseCfg, _, err := config.LoadOrchestratorFromEnv()
	if err != nil {
		return nil, err
	}

	log := logger.New(logger.Options{
		Service:   baseCfg.Shared.ServiceName,
		Component: "bootstrap",
		Level:     parseLogLevel(baseCfg.Shared.LogLevel),
	})

	postgres, err := postgresstore.Open(ctx, postgresstore.ConfigFromShared(baseCfg.Postgres), logger.WithComponent(log, "postgres"))
	if err != nil {
		return nil, err
	}

	settingsOptions := []config.SettingsStoreOption{}
	if strings.TrimSpace(os.Getenv(config.SettingsEncryptionKeyEnv)) == "" {
		log.Warn("settings encryption key not set; using plaintext settings storage")
		settingsOptions = append(settingsOptions, config.WithPlaintextSecretStorage())
	}
	settingsStore := config.NewPostgresSettingsStore(postgres.Pool(), settingsOptions...)
	storedSettings, err := settingsStore.ListAll(ctx)
	if err != nil {
		postgres.Close()
		return nil, err
	}

	cfg, snapshot, err := config.LoadOrchestratorLayered(storedSettings)
	if err != nil {
		postgres.Close()
		return nil, err
	}
	log.Info("loaded layered orchestrator config", slog.String("config_summary", configSummary(snapshot)))
	hotConfig := config.NewHotConfig(snapshot)
	authManager := providerauth.NewManager(settingsStore)

	redis, err := redisstore.Open(ctx, redisstore.ConfigFromShared(cfg.Redis), logger.WithComponent(log, "redis"))
	if err != nil {
		postgres.Close()
		return nil, err
	}

	m := metrics.New()
	h := health.New(
		cfg.Shared.ServiceName,
		health.FuncChecker{CheckName: "postgres", Fn: postgres.HealthCheck},
		health.FuncChecker{CheckName: "redis", Fn: redis.HealthCheck},
	)

	providerBuilder := providerfactory.New(authManager, log)
	providerResult, err := providerBuilder.Build(ctx, providerfactory.BuildConfig{
		ActiveProvider:      cfg.ModelProvider,
		OpenAIAPIKey:        cfg.OpenAIAPIKey,
		OpenAIModel:         cfg.OpenAIModel,
		OpenAIBaseURL:       cfg.OpenAIBaseURL,
		OpenAIRealtimeURL:   cfg.OpenAIRealtimeURL,
		OpenAITransportMode: cfg.OpenAITransportMode,
		OpenAICodexModel:    cfg.OpenAICodexModel,
		OpenAICodexBaseURL:  cfg.OpenAICodexBaseURL,
		GitHubCopilotModel:  cfg.GitHubCopilotModel,
		Timeout:             time.Duration(cfg.OpenAITimeoutSeconds) * time.Second,
	})
	if err != nil {
		redis.Close()
		postgres.Close()
		return nil, err
	}
	provider := providerResult.Provider

	runRepo := runservice.NewPostgresRepository(postgres.Pool())
	runManager := runservice.NewService(runRepo, logger.WithComponent(log, "run-service"))
	toolBrokerClient, err := tools.Dial(cfg.ToolBrokerAddr)
	if err != nil {
		redis.Close()
		postgres.Close()
		return nil, err
	}
	var delivery flow.DeliverySink = flow.NewCompositeDeliverySink(flow.NewLoggingDeliverySink(log))
	var telegram *telegramadapter.Adapter
	approvalGate := flow.NewApprovalGate()
	approvalRepo := approvals.NewPostgresRepository(postgres.Pool())
	approvalService := approvals.NewService(approvalRepo, approvalGate)
	artifactsRepo := artifacts.NewPostgresRepository(postgres.Pool())
	artifactsService := artifacts.NewService(artifactsRepo)
	activityRepo := activity.NewPostgresRepository(postgres.Pool())
	activityService := activity.NewService(activityRepo)
	deliveryEventsRepo := deliveryevents.NewPostgresRepository(postgres.Pool())
	deliveryEventsService := deliveryevents.NewService(deliveryEventsRepo)
	if strings.TrimSpace(cfg.TelegramBotToken) != "" {
		telegramClient, err := telegramadapter.NewClient(cfg.TelegramBaseURL, cfg.TelegramBotToken, nil)
		if err != nil {
			redis.Close()
			postgres.Close()
			return nil, err
		}
		telegram, err = telegramadapter.NewAdapter(telegramadapter.Config{
			AllowedChatIDs: cfg.TelegramAllowedChatIDs,
			PollTimeout:    time.Duration(cfg.TelegramPollTimeout) * time.Second,
		}, telegramClient, approvalGate, logger.WithComponent(log, "telegram"))
		if err != nil {
			redis.Close()
			postgres.Close()
			return nil, err
		}
		delivery = flow.NewCompositeDeliverySink(delivery, telegram)
	}
	delivery = flow.NewObservedDeliverySink(delivery, deliveryEventsService)
	sessionRepo := session.NewPostgresRepository(postgres.Pool())
	sessionLeaseManager := session.NewRedisLeaseManager(redis.Client(), logger.WithComponent(log, "session-lease-store"))
	transcriptStore := transcript.NewStore(postgres.Pool())

	// Memory pipeline: enqueuer for async post-run extraction.
	pipelineQueue := pipeline.NewQueue(redis.Client())
	pipelineEnqueuer := pipeline.NewEnqueuer(pipelineQueue, m)

	// Memory pipeline: embedding provider and worker for async memory extraction.
	profileStore := profile.NewStore(postgres.Pool())
	episodicStore := episodic.NewStore(postgres.Pool())
	chunkStore := chunks.NewStore(postgres.Pool())

	var embeddingProvider pipeline.EmbeddingProvider
	var pipelineWorker *pipeline.Worker

	// Set embedding vector dimensions from config (defaults to 1536 for OpenAI).
	if cfg.MemoryEmbeddingDimensions > 0 {
		embeddings.SetVectorDimensions(cfg.MemoryEmbeddingDimensions)
	}

	// Memory extraction can run either with a direct OpenAI API key or with
	// OpenAI Codex provider auth. Embeddings can use OpenAI or Ollama.
	openAIKey := strings.TrimSpace(cfg.OpenAIAPIKey)
	if cfg.MemoryPipelineEnabled {
		// --- Embedding provider setup ---
		embProvider := strings.ToLower(strings.TrimSpace(cfg.MemoryEmbeddingProvider))
		switch embProvider {
		case "ollama":
			op, embErr := embeddings.NewOllamaProvider(embeddings.OllamaConfig{
				BaseURL: cfg.OllamaURL,
				Model:   cfg.MemoryEmbeddingModel,
				Timeout: 30 * time.Second,
			}, nil)
			if embErr != nil {
				log.Warn("ollama embedding provider init failed; episodic/chunk memory disabled", slog.String("error", embErr.Error()))
			} else {
				embeddingProvider = op
				log.Info("memory embedding provider configured (ollama)",
					slog.String("embedding_model", cfg.MemoryEmbeddingModel),
					slog.String("ollama_url", cfg.OllamaURL),
					slog.Int("dimensions", embeddings.VectorDimensions()),
				)
			}
		default: // "openai" or unset
			if openAIKey != "" {
				op, embErr := embeddings.NewProvider(embeddings.Config{
					APIKey:  openAIKey,
					Model:   cfg.MemoryEmbeddingModel,
					BaseURL: cfg.OpenAIBaseURL,
					Timeout: 30 * time.Second,
				}, nil)
				if embErr != nil {
					log.Warn("embedding provider init failed; episodic/chunk memory disabled", slog.String("error", embErr.Error()))
				} else {
					embeddingProvider = op
					log.Info("memory embedding provider configured (openai)", slog.String("embedding_model", cfg.MemoryEmbeddingModel))
				}
			} else {
				log.Warn("memory embeddings disabled: no OpenAI API key configured and embedding provider is not ollama")
			}
		}

		// --- LLM caller setup (for memory extraction) ---
		var llmCaller pipeline.LLMCaller
		if openAIKey != "" {
			caller, callerErr := pipeline.NewOpenAICaller(pipeline.OpenAICallerConfig{
				APIKey:  openAIKey,
				Model:   cfg.MemoryExtractionModel,
				BaseURL: cfg.OpenAIBaseURL,
				Timeout: 90 * time.Second,
			}, nil)
			if callerErr != nil {
				log.Warn("llm caller init failed; memory pipeline disabled", slog.String("error", callerErr.Error()))
			} else {
				llmCaller = caller
			}
		} else {
			caller, callerErr := pipeline.NewOpenAICodexCaller(pipeline.OpenAICodexCallerConfig{
				Model:      cfg.OpenAICodexModel,
				BaseURL:    cfg.OpenAICodexBaseURL,
				Timeout:    90 * time.Second,
				AuthSource: authManager,
			}, nil)
			if callerErr != nil {
				log.Warn("codex llm caller init failed; memory pipeline disabled", slog.String("error", callerErr.Error()))
			} else {
				llmCaller = caller
				log.Info("memory extraction configured via codex auth", slog.String("extraction_model", cfg.OpenAICodexModel))
			}
		}
		if llmCaller != nil {
			extractor := pipeline.NewLLMExtractor(llmCaller)
			pipelineWorker = pipeline.NewWorker(
				pipelineQueue,
				transcriptStore,
				extractor,
				profileStore,
				episodicStore,
				embeddingProvider,
				sessionRepo,
				pipeline.WorkerConfig{
					PollTimeout: time.Duration(cfg.MemoryPipelinePollTimeoutSeconds) * time.Second,
					MaxRetries:  cfg.MemoryPipelineMaxRetries,
				},
				m,
				logger.WithComponent(log, "memory-pipeline"),
			)
			pipelineWorker.SetChunkStore(chunkStore)
			log.Info("memory pipeline worker configured",
				slog.String("extraction_model", cfg.MemoryExtractionModel),
				slog.Bool("embeddings_enabled", embeddingProvider != nil),
			)
		}
	} else {
		log.Info("memory pipeline disabled by configuration")
	}

	var memoryEmbeddings memoryservice.EmbeddingProvider
	if embeddingProvider != nil {
		memoryEmbeddings = embeddingProvider
	}

	// Observability: transition repository and event hub for live SSE streaming.
	transitionRepo := runservice.NewPostgresTransitionRepository(postgres.Pool())
	eventHub := observability.NewHub()

	executor := flow.NewService(
		sessionRepo,
		sessionLeaseManager,
		runManager,
		transcriptStore,
		provider,
		flow.Config{
			ProviderName:     providerResult.ProviderName,
			ModelName:        providerResult.ModelName,
			OwnerID:          cfg.Shared.ServiceName,
			LeaseTTL:         int64(cfg.SessionLeaseTTLSeconds),
			PromptManager:    promptmgmt.NewManager(settingsStore),
			PromptAssembler:  promptmgmt.NewAssembler(),
			ProfileLimit:     cfg.MemoryProfileLimit,
			EpisodeLimit:     cfg.MemoryEpisodicLimit,
			MemoryScopes:     cfg.MemoryScopeOrder,
			Delivery:         delivery,
			Tools:            toolBrokerClient,
			ToolCatalog:      toolBrokerClient,
			ApprovalChecker:  toolBrokerClient,
			ApprovalGate:     approvalGate,
			ApprovalService:  approvalService,
			WorkingStore:     workingStoreAdapter{store: working.NewStore(postgres.Pool())},
			TransientStore:   transientWorkingStoreAdapter{store: working.NewTransientStore(redis.Client())},
			TransientTTL:     time.Duration(cfg.MemoryWorkingTransientTTLSeconds) * time.Second,
			ProfileStore:     profileStoreAdapter{store: profileStore},
			EpisodeStore:     episodicStoreAdapter{store: episodicStore},
			ChunkStore:       chunkStoreAdapter{store: chunkStore},
			Embeddings:       memoryEmbeddings,
			PipelineEnqueuer: pipelineEnqueuer,
			SummaryReader:    sessionRepo,
			TransitionLogger: transitionRepo,
			EventHub:         eventHub,
			Artifacts:        artifactsService,
			Activity:         activityService,
		},
		logger.WithComponent(log, "executor"),
	)
	if telegram != nil {
		telegram.SetExecutor(executor)
		telegram.SetApprovalService(approvalService)
		telegram.SetAuthManager(authManager)
	}
	apiServer := apiservice.NewServer(executor, logger.WithComponent(log, "api"))
	viewServer := apiservice.NewViewServer(sessionRepo, runRepo, transcriptStore, logger.WithComponent(log, "view-api"))
	doctorChecker := apiservice.NewToolBrokerDoctorChecker(func(ctx context.Context, toolName, argsJSON string) (string, error) {
		result, err := toolBrokerClient.ExecuteToolCall(ctx, &toolbrokerv1.ToolCall{
			ToolCallId: fmt.Sprintf("doctor-%d", time.Now().UnixNano()),
			ToolName:   toolName,
			ArgsJson:   argsJSON,
			Status:     "pending",
			StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return "", err
		}
		return result.GetResultJson(), nil
	})
	memoryDoctor := memorydoctor.NewReporter(pipelineQueue, postgres)
	memoryDoctor.SetPipelineEnabled(pipelineWorker != nil)
	memoryDoctor.SetEmbeddingConfigured(embeddingProvider != nil)
	doctorServer := apiservice.NewDoctorServer(postgres.Pool(), doctorChecker, memoryDoctor, artifactsService, logger.WithComponent(log, "doctor-api"))
	settingsService := config.NewSettingsService(settingsStore, hotConfig)
	settingsServer := apiservice.NewSettingsServer(settingsService)
	promptServer := apiservice.NewPromptServer(executor)
	memoryServer := apiservice.NewMemoryServer(working.NewStore(postgres.Pool()), profileStore, episodicStore, chunkStore, provenance.NewStore(postgres.Pool()))
	memoryServer.SetWriters(profileStore, episodicStore, chunkStore)
	providerServer := apiservice.NewProviderServer(authManager, providerResult.ProviderName, map[string]string{
		modelprovider.ProviderOpenAI:        cfg.OpenAIModel,
		modelprovider.ProviderOpenAICodex:   cfg.OpenAICodexModel,
		modelprovider.ProviderGitHubCopilot: cfg.GitHubCopilotModel,
	}, strings.TrimSpace(cfg.OpenAIAPIKey) != "")
	eventServer := apiservice.NewEventServer(transitionRepo, runRepo, eventHub, logger.WithComponent(log, "event-api"))
	taskViewServer := apiservice.NewTaskViewServer(runRepo, runRepo, sessionRepo, transcriptStore, transitionRepo, artifactsRepo, deliveryEventsRepo)
	overviewServer := apiservice.NewOverviewServer(runRepo, deliveryEventsRepo)
	approvalsServer := apiservice.NewApprovalsServer(approvalRepo, approvalService)
	artifactsServer := apiservice.NewArtifactsServer(artifactsRepo, activityRepo)
	activityServer := apiservice.NewActivityServer(activityRepo)
	systemServer := apiservice.NewSystemServer(postgres.Pool(), runRepo, approvalRepo, providerResult.ProviderName, strings.TrimSpace(cfg.OpenAIAPIKey) != "", pipelineWorker != nil)
	taskDebugServer := apiservice.NewTasksDebugServer(runRepo, transcriptStore)
	streamServer := apiservice.NewStreamServer(eventHub, taskViewServer, overviewServer, approvalsServer, systemServer, activityServer)

	mux := http.NewServeMux()
	mux.Handle("/health", h.Handler())
	mux.Handle("/metrics", m.Handler())
	mux.Handle("/api/v1/events", apiServer.HTTPHandler())
	mux.Handle("/api/v1/settings/restart", settingsServer.HandleRestart())
	mux.Handle("/api/v1/settings", settingsServer.HandleList())
	mux.Handle("/api/v1/settings/", settingsServer.HandleItem())
	mux.Handle("/api/v1/prompts/system", promptServer.HandleConfig())
	mux.Handle("/api/v1/prompts/system/preview", promptServer.HandlePreview())
	mux.Handle("/api/v1/providers", providerServer.HandleList())
	mux.Handle("/api/v1/providers/", providerServer.HandleItem())
	mux.Handle("/api/v1/memory", memoryServer.HandleList())
	mux.Handle("/api/v2/memory", memoryServer.HandleList())
	mux.Handle("/api/v2/memory/item", memoryServer.HandleGet())
	mux.Handle("/api/v2/memory/patch", memoryServer.HandlePatch())
	mux.Handle("/api/v2/memory/delete", memoryServer.HandleDelete())
	mux.Handle("/api/v2/memory/confirm", memoryServer.HandleConfirm())
	mux.Handle("/api/v2/memory/reject", memoryServer.HandleReject())
	mux.Handle("/api/v2/memory/suppress", memoryServer.HandleSuppress())
	mux.Handle("/api/v2/memory/unsuppress", memoryServer.HandleUnsuppress())
	mux.Handle("/api/tasks", taskViewServer.HandleListTasks())
	mux.Handle("/api/tasks/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/debug") {
			taskDebugServer.HandleGetTaskDebug("/api/tasks/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/artifacts") {
			artifactsServer.HandleListTaskArtifacts("/api/tasks/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/activity") {
			artifactsServer.HandleListTaskActivity("/api/tasks/").ServeHTTP(w, r)
			return
		}
		taskViewServer.HandleGetTaskDetail("/api/tasks/").ServeHTTP(w, r)
	}))
	mux.Handle("/api/overview", overviewServer.HandleGetOverview())
	mux.Handle("/api/approvals", approvalsServer.HandleListApprovals())
	mux.Handle("/api/approvals/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/approve") {
			approvalsServer.HandleApprove("/api/approvals/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/reject") {
			approvalsServer.HandleReject("/api/approvals/").ServeHTTP(w, r)
			return
		}
		approvalsServer.HandleGetApproval("/api/approvals/").ServeHTTP(w, r)
	}))
	mux.Handle("/api/v2/tasks", taskViewServer.HandleListTasks())
	mux.Handle("/api/v2/tasks/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/debug") {
			taskDebugServer.HandleGetTaskDebug("/api/v2/tasks/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/artifacts") {
			artifactsServer.HandleListTaskArtifacts("/api/v2/tasks/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/activity") {
			artifactsServer.HandleListTaskActivity("/api/v2/tasks/").ServeHTTP(w, r)
			return
		}
		taskViewServer.HandleGetTaskDetail("/api/v2/tasks/").ServeHTTP(w, r)
	}))
	mux.Handle("/api/v2/overview", overviewServer.HandleGetOverview())
	mux.Handle("/api/v2/approvals", approvalsServer.HandleListApprovals())
	mux.Handle("/api/v2/approvals/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/approve") {
			approvalsServer.HandleApprove("/api/v2/approvals/").ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/reject") {
			approvalsServer.HandleReject("/api/v2/approvals/").ServeHTTP(w, r)
			return
		}
		approvalsServer.HandleGetApproval("/api/v2/approvals/").ServeHTTP(w, r)
	}))
	mux.Handle("/api/v2/artifacts", artifactsServer.HandleListArtifacts())
	mux.Handle("/api/v2/artifacts/", artifactsServer.HandleGetArtifact("/api/v2/artifacts/"))
	mux.Handle("/api/v2/activity", activityServer.HandleListActivity())
	mux.Handle("/api/v2/system", systemServer.HandleGetSystem())
	mux.Handle("/api/system", systemServer.HandleGetSystem())
	mux.Handle("/api/v2/stream", streamServer.HandleStream())
	mux.Handle("/api/stream", streamServer.HandleStream())
	mux.Handle("/api/v1/sessions", viewServer.HandleListSessions())
	mux.Handle("/api/v1/sessions/", viewServer.HandleGetSession())
	mux.Handle("/api/v1/runs/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/transcript") {
			viewServer.HandleGetRunTranscript().ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/events") {
			eventServer.HandleSSE().ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/transitions") {
			eventServer.HandleListTransitions().ServeHTTP(w, r)
			return
		}
		viewServer.HandleGetRun().ServeHTTP(w, r)
	}))
	mux.Handle("/api/v1/doctor/check", doctorServer.HandleRunCheck())
	mux.Handle("/api/v1/doctor/reports", doctorServer.HandleListReports())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": cfg.Shared.ServiceName,
			"status":  "starting",
		})
	})

	server := &http.Server{
		Addr:              cfg.Shared.HTTPAddr,
		Handler:           instrumentHTTP(cfg.Shared.ServiceName, m, corsMiddleware(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	grpcListener, err := net.Listen("tcp", cfg.Shared.GRPCAddr)
	if err != nil {
		redis.Close()
		postgres.Close()
		return nil, fmt.Errorf("listen on grpc addr %s: %w", cfg.Shared.GRPCAddr, err)
	}

	grpcServer := grpc.NewServer()
	sessionv1.RegisterSessionServiceServer(
		grpcServer,
		session.NewServer(
			sessionRepo,
			sessionLeaseManager,
			runManager,
			time.Duration(cfg.SessionLeaseTTLSeconds)*time.Second,
			logger.WithComponent(log, "session-service"),
		),
	)
	orchestratorv1.RegisterOrchestratorServiceServer(grpcServer, apiServer)

	return &App{
		config:         cfg,
		log:            log,
		metrics:        m,
		health:         h,
		postgres:       postgres,
		redis:          redis,
		httpServer:     server,
		grpcServer:     grpcServer,
		grpcListen:     grpcListener,
		telegram:       telegram,
		toolBroker:     toolBrokerClient,
		pipelineWorker: pipelineWorker,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 3)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		a.log.Info("starting orchestrator http server", slog.String("http_addr", a.config.Shared.HTTPAddr))
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	go func() {
		a.log.Info("starting orchestrator grpc server", slog.String("grpc_addr", a.config.Shared.GRPCAddr))
		if err := a.grpcServer.Serve(a.grpcListen); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	if a.telegram != nil {
		go func() {
			a.log.Info("starting telegram adapter")
			if err := a.telegram.Run(runCtx); err != nil {
				errCh <- err
				return
			}
			errCh <- nil
		}()
	}

	if a.pipelineWorker != nil {
		go func() {
			a.log.Info("starting memory pipeline worker")
			if err := a.pipelineWorker.Run(runCtx); err != nil && runCtx.Err() == nil {
				a.log.Error("memory pipeline worker stopped with error", slog.String("error", err.Error()))
				errCh <- err
				return
			}
			a.log.Info("memory pipeline worker stopped")
		}()
	}

	sigCtx, stop := signal.NotifyContext(runCtx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		cancel()
		return a.shutdown(err)
	case <-sigCtx.Done():
		cancel()
		a.log.Info("received shutdown signal")
		return a.shutdown(nil)
	}
}

func (a *App) shutdown(runErr error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.httpServer.Shutdown(ctx); err != nil {
		a.log.Error("http shutdown failed", slog.String("error", err.Error()))
		if runErr == nil {
			runErr = err
		}
	}

	a.grpcServer.GracefulStop()
	if a.grpcListen != nil {
		if err := a.grpcListen.Close(); err != nil && !errors.Is(err, net.ErrClosed) && runErr == nil {
			runErr = err
		}
	}

	if err := a.redis.Close(); err != nil && runErr == nil {
		runErr = err
	}
	if err := a.toolBroker.Close(); err != nil && runErr == nil {
		runErr = err
	}
	a.postgres.Close()

	if runErr != nil {
		a.log.Error("orchestrator stopped with error", slog.String("error", runErr.Error()))
		return runErr
	}

	a.log.Info("orchestrator stopped cleanly")
	return nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func instrumentHTTP(service string, registry *metrics.Registry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		ww := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(ww, r)
		status := "ok"
		if ww.statusCode >= 400 {
			status = "error"
			_ = registry.IncrCounter(metrics.MetricErrorsTotal, map[string]string{
				"error_class": "http_error",
				"operation":   r.Method + " " + r.URL.Path,
				"service":     service,
			})
		}
		_ = registry.IncrCounter(metrics.MetricRequestsTotal, map[string]string{
			"operation": r.Method + " " + r.URL.Path,
			"service":   service,
			"status":    status,
		})
		_ = registry.ObserveHistogram(metrics.MetricRequestDurationSeconds, time.Since(started).Seconds(), map[string]string{
			"operation": r.Method + " " + r.URL.Path,
			"service":   service,
		})
	})
}

type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func parseLogLevel(value string) slog.Leveler {
	switch value {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func configSummary(snapshot config.Snapshot) string {
	entries := snapshot.ListKeys()
	summary := make(map[string]string, len(entries))
	for _, entry := range entries {
		summary[entry.Key] = entry.EffectiveValue
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		return fmt.Sprintf("%d keys", len(entries))
	}
	return string(encoded)
}
