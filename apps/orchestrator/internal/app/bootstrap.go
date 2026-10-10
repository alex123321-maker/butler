package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	activity "github.com/butler/butler/apps/orchestrator/internal/activity"
	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
	telegramadapter "github.com/butler/butler/apps/orchestrator/internal/channel/telegram"
	deliveryevents "github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/observability"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	singletab "github.com/butler/butler/apps/orchestrator/internal/singletab"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
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
	"github.com/butler/butler/internal/memory/summary"
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
	ownerAuth, err := ownerControlAuth(cfg)
	if err != nil {
		postgres.Close()
		return nil, err
	}
	// Provider OAuth credentials always require encrypted persistence, including
	// installations that permit plaintext storage for unrelated legacy settings.
	authManager := providerauth.NewManager(config.NewPostgresSettingsStore(postgres.Pool()))

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
		AntigravityModel:    cfg.AntigravityModel,
		Timeout:             time.Duration(cfg.OpenAITimeoutSeconds) * time.Second,
	})
	if err != nil {
		redis.Close()
		postgres.Close()
		return nil, err
	}

	taskStore := task.NewStore(postgres.Pool())
	runRepo := runservice.NewPostgresRepository(postgres.Pool())
	runManager := runservice.NewService(runRepo, logger.WithComponent(log, "run-service"))
	toolBrokerClient, err := tools.DialWithTokenFile(cfg.ToolBrokerAddr, cfg.ActionAuthorityTokenFile)
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
	singleTabRepo := singletab.NewPostgresRepository(postgres.Pool())
	singleTabService := singletab.NewService(approvalRepo, singleTabRepo, approvalGate)
	singleTabService.SetApprovalCreator(approvalService)
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
	singleTabService.SetDeliverySink(delivery)

	sessionRepo := session.NewPostgresRepository(postgres.Pool())
	sessionLeaseManager := session.NewPostgresLeaseManager(postgres.Pool())
	inputStore := durableinput.NewStore(postgres.Pool())
	approvalRepo.SetCheckpointHooks(inputStore)
	transcriptStore := transcript.NewStore(postgres.Pool())
	summaryStore := summary.NewStore(postgres.Pool())
	pipelineQueue := pipeline.NewQueue(redis.Client())
	pipelineEnqueuer := pipeline.NewEnqueuer(pipelineQueue, m)
	profileStore := profile.NewStore(postgres.Pool())
	episodicStore := episodic.NewStore(postgres.Pool())
	chunkStore := chunks.NewStore(postgres.Pool())

	embeddingProvider, pipelineWorker := configureMemoryPipeline(log, cfg, authManager, m, pipelineQueue, transcriptStore, summaryStore, profileStore, episodicStore, chunkStore)

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
			AutonomyMode:        autonomyMode(cfg.AutonomyMode),
			ActionAuthorizer:    toolBrokerClient,
			ActionMandateID:     cfg.ActionMandateID,
			Checkpoints:         inputStore,
			Tasks:               taskStore,
			ProviderName:        providerResult.ProviderName,
			ModelName:           providerResult.ModelName,
			OwnerID:             cfg.Shared.ServiceName,
			LeaseTTL:            int64(cfg.SessionLeaseTTLSeconds),
			PromptManager:       promptmgmt.NewManager(settingsStore),
			PromptAssembler:     promptmgmt.NewAssembler(),
			ProfileLimit:        cfg.MemoryProfileLimit,
			EpisodeLimit:        cfg.MemoryEpisodicLimit,
			MemoryScopes:        cfg.MemoryScopeOrder,
			Delivery:            delivery,
			Tools:               toolBrokerClient,
			ToolCatalog:         toolBrokerClient,
			ApprovalChecker:     toolBrokerClient,
			ApprovalGate:        approvalGate,
			ApprovalService:     approvalService,
			WorkingStore:        workingStoreAdapter{store: working.NewStore(postgres.Pool())},
			TransientStore:      transientWorkingStoreAdapter{store: working.NewTransientStore(redis.Client())},
			TransientTTL:        time.Duration(cfg.MemoryWorkingTransientTTLSeconds) * time.Second,
			ProfileStore:        profileStoreAdapter{store: profileStore},
			EpisodeStore:        episodicStoreAdapter{store: episodicStore},
			ChunkStore:          chunkStoreAdapter{store: chunkStore},
			Embeddings:          memoryEmbeddings,
			PipelineEnqueuer:    pipelineEnqueuer,
			SummaryReader:       summaryStore,
			ContextBudgetTokens: cfg.ContextBudgetTokens,
			TransitionLogger:    transitionRepo,
			EventHub:            eventHub,
			Artifacts:           artifactsService,
			Activity:            activityService,
		},
		logger.WithComponent(log, "executor"),
	)
	inputWorker := durableinput.NewService(inputStore, sessionRepo, executor, runManager, logger.WithComponent(log, "durable-input"))
	inputWorker.SetMaintenance(approvalService.ExpireInvalidPending)
	if telegram != nil {
		telegram.SetExecutor(inputWorker)
		telegram.SetApprovalService(approvalService)
		telegram.SetSingleTabSelector(singleTabService)
		telegram.SetAuthManager(authManager)
	}

	var actionAuthorityHandler http.Handler
	if cfg.ActionAuthorityTokenFile != "" {
		token, readErr := actionauth.ReadServiceToken(cfg.ActionAuthorityTokenFile)
		if readErr != nil {
			redis.Close()
			postgres.Close()
			return nil, readErr
		}
		actionAuthorityHandler = actionauth.NewContextHandler(token, &actionContextReader{runs: runManager, sessions: sessionRepo, leases: sessionLeaseManager, tasks: taskStore, approvals: approvalService, pool: postgres.Pool(), mandateID: cfg.ActionMandateID})
	} else {
		log.Warn("private action authority unavailable; Tool Broker will deny execution")
	}
	apiServer := apiservice.NewServer(inputWorker, logger.WithComponent(log, "api"))
	httpServer := newHTTPServer(httpServerDeps{
		cfg:                    cfg,
		actionAuthorityHandler: actionAuthorityHandler,
		log:                    log,
		settingsStore:          settingsStore,
		hotConfig:              hotConfig,
		postgres:               postgres,
		metrics:                m,
		health:                 h,
		toolBrokerClient:       toolBrokerClient,
		authManager:            authManager,
		providerResult:         providerResult,
		executor:               executor,
		inputWorker:            inputWorker,
		inputStore:             inputStore,
		ownerAuth:              ownerAuth,
		sessionRepo:            sessionRepo,
		runRepo:                runRepo,
		taskStore:              taskStore,
		transcriptStore:        transcriptStore,
		transitionRepo:         transitionRepo,
		eventHub:               eventHub,
		approvalRepo:           approvalRepo,
		approvalService:        approvalService,
		singleTabService:       singleTabService,
		artifactsRepo:          artifactsRepo,
		artifactsService:       artifactsService,
		activityRepo:           activityRepo,
		deliveryEventsRepo:     deliveryEventsRepo,
		pipelineQueue:          pipelineQueue,
		pipelineWorker:         pipelineWorker,
		embeddingProvider:      embeddingProvider,
		profileStore:           profileStore,
		episodicStore:          episodicStore,
		chunkStore:             chunkStore,
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
		inputWorker:    inputWorker,
	}, nil
}

func configureMemoryPipeline(
	log *slog.Logger,
	cfg config.OrchestratorConfig,
	authManager *providerauth.Manager,
	m *metrics.Registry,
	pipelineQueue *pipeline.Queue,
	transcriptStore *transcript.Store,
	summaryStore *summary.Store,
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
		summaryStore,
		pipeline.WorkerConfig{
			PollTimeout: time.Duration(cfg.MemoryPipelinePollTimeoutSeconds) * time.Second,
			MaxRetries:  cfg.MemoryPipelineMaxRetries,
		},
		m,
		logger.WithComponent(log, "memory-pipeline"),
	)
	pipelineWorker.SetSessionSummarizer(extractor)
	pipelineWorker.SetChunkStore(chunkStore)
	log.Info("memory pipeline worker configured",
		slog.String("extraction_model", cfg.MemoryExtractionModel),
		slog.Bool("embeddings_enabled", embeddingProvider != nil),
	)

	return embeddingProvider, pipelineWorker
}
