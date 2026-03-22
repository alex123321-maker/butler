package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
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
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/chunks"
	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/pipeline"
	"github.com/butler/butler/internal/memory/profile"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/memory/working"
	"github.com/butler/butler/internal/metrics"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	redisstore "github.com/butler/butler/internal/storage/redis"
	"google.golang.org/grpc"
)

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
	pipelineQueue := pipeline.NewQueue(redis.Client())
	pipelineEnqueuer := pipeline.NewEnqueuer(pipelineQueue, m)
	profileStore := profile.NewStore(postgres.Pool())
	episodicStore := episodic.NewStore(postgres.Pool())
	chunkStore := chunks.NewStore(postgres.Pool())

	embeddingProvider, pipelineWorker := configureMemoryPipeline(log, cfg, authManager, m, pipelineQueue, transcriptStore, sessionRepo, profileStore, episodicStore, chunkStore)

	var memoryEmbeddings memoryservice.EmbeddingProvider
	if embeddingProvider != nil {
		memoryEmbeddings = embeddingProvider
	}

	transitionRepo := runservice.NewPostgresTransitionRepository(postgres.Pool())
	eventHub := observability.NewHub()

	executor := flow.NewService(
		sessionRepo,
		sessionLeaseManager,
		runManager,
		transcriptStore,
		providerResult.Provider,
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
	httpServer := newHTTPServer(httpServerDeps{
		cfg:                cfg,
		log:                log,
		settingsStore:      settingsStore,
		hotConfig:          hotConfig,
		postgres:           postgres,
		metrics:            m,
		health:             h,
		toolBrokerClient:   toolBrokerClient,
		authManager:        authManager,
		providerResult:     providerResult,
		executor:           executor,
		sessionRepo:        sessionRepo,
		runRepo:            runRepo,
		transcriptStore:    transcriptStore,
		transitionRepo:     transitionRepo,
		eventHub:           eventHub,
		approvalRepo:       approvalRepo,
		approvalService:    approvalService,
		artifactsRepo:      artifactsRepo,
		artifactsService:   artifactsService,
		activityRepo:       activityRepo,
		deliveryEventsRepo: deliveryEventsRepo,
		pipelineQueue:      pipelineQueue,
		pipelineWorker:     pipelineWorker,
		embeddingProvider:  embeddingProvider,
		profileStore:       profileStore,
		episodicStore:      episodicStore,
		chunkStore:         chunkStore,
	})

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
		httpServer:     httpServer,
		grpcServer:     grpcServer,
		grpcListen:     grpcListener,
		telegram:       telegram,
		toolBroker:     toolBrokerClient,
		pipelineWorker: pipelineWorker,
	}, nil
}

func configureMemoryPipeline(
	log *slog.Logger,
	cfg config.OrchestratorConfig,
	authManager *providerauth.Manager,
	m *metrics.Registry,
	pipelineQueue *pipeline.Queue,
	transcriptStore *transcript.Store,
	sessionRepo *session.PostgresRepository,
	profileStore *profile.Store,
	episodicStore *episodic.Store,
	chunkStore *chunks.Store,
) (pipeline.EmbeddingProvider, *pipeline.Worker) {
	var embeddingProvider pipeline.EmbeddingProvider
	var pipelineWorker *pipeline.Worker

	if cfg.MemoryEmbeddingDimensions > 0 {
		embeddings.SetVectorDimensions(cfg.MemoryEmbeddingDimensions)
	}

	openAIKey := strings.TrimSpace(cfg.OpenAIAPIKey)
	if !cfg.MemoryPipelineEnabled {
		log.Info("memory pipeline disabled by configuration")
		return nil, nil
	}

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
	default:
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
	if llmCaller == nil {
		return embeddingProvider, nil
	}

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

	return embeddingProvider, pipelineWorker
}
