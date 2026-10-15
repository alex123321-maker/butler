package app

import (
	"context"
	"errors"
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
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	deliveryevents "github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/initiative"
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
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/delegation"
	orchestratorv1 "github.com/butler/butler/internal/gen/orchestrator/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/candidatecontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/chunks"
	memorydoctor "github.com/butler/butler/internal/memory/doctor"
	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/pipeline"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/readcontrol"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/memory/working"
	"github.com/butler/butler/internal/metrics"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/butler/butler/internal/skills"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	redisstore "github.com/butler/butler/internal/storage/redis"
	"github.com/butler/butler/internal/workspaceexec"
	"github.com/jackc/pgx/v5"
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
	if err := memorydoctor.RegisterMetrics(m, postgres.Pool()); err != nil {
		redis.Close()
		postgres.Close()
		return nil, err
	}
	h := health.New(
		cfg.Shared.ServiceName,
		health.FuncChecker{CheckName: "postgres", Fn: postgres.HealthCheck},
		health.FuncChecker{CheckName: "redis", Fn: redis.HealthCheck},
	)

	providerBuilder := providerfactory.New(authManager, log)
	providerResult, err := providerBuilder.Build(ctx, providerfactory.BuildConfig{
		ActiveProvider:                 cfg.ModelProvider,
		OpenAIAPIKey:                   cfg.OpenAIAPIKey,
		OpenAIModel:                    cfg.OpenAIModel,
		OpenAIBaseURL:                  cfg.OpenAIBaseURL,
		OpenAIRealtimeURL:              cfg.OpenAIRealtimeURL,
		OpenAITransportMode:            cfg.OpenAITransportMode,
		OpenAICodexModel:               cfg.OpenAICodexModel,
		OpenAICodexBaseURL:             cfg.OpenAICodexBaseURL,
		GitHubCopilotModel:             cfg.GitHubCopilotModel,
		AntigravityModel:               cfg.AntigravityModel,
		AntigravityMaximumOutputTokens: cfg.AntigravityMaximumOutputTokens,
		Timeout:                        time.Duration(cfg.OpenAITimeoutSeconds) * time.Second,
	})
	if err != nil {
		redis.Close()
		postgres.Close()
		return nil, err
	}

	taskStore := task.NewStore(postgres.Pool())
	ownerGoals := goal.New(postgres.Pool(), taskStore, nil)
	budgetPolicy, policyErr := budget.ParsePolicy(cfg.ExecutionBudgetPolicyJSON)
	if policyErr != nil {
		redis.Close()
		postgres.Close()
		return nil, policyErr
	}
	budgetController := budget.NewController(postgres.Pool(), budgetPolicy)
	budgetController.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	taskStore.SetBudgets(budgetController, cfg.ActionMandateID)
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
	delivery = newVerifiedTaskDelivery(delivery, taskStore)
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
	sourceCatalog := catalog.New(postgres.Pool(), catalog.NewFileManifest(strings.TrimSpace(os.Getenv("BUTLER_MEMORY_DELETION_MANIFEST_FILE"))))
	taskStore.SetVerifiedResultSourceGuard(sourceCatalog.CheckRunSourcesTx)
	if restoreErr := sourceCatalog.Reconcile(ctx); restoreErr != nil {
		log.Warn("memory unavailable: deletion anchor requires operator reconciliation")
	}
	var childAdmission *DelegationAdmission
	if cfg.DelegationEnabled {
		if cfg.ActionAuthorityTokenFile == "" {
			redis.Close()
			postgres.Close()
			return nil, fmt.Errorf("delegation requires private action authority authentication")
		}
		limits := task.DefaultDelegationLimits()
		limits.MaxParallel = cfg.DelegationMaxParallel
		childAdmission, err = NewDelegationAdmission(postgres.Pool(), taskStore, sessionRepo, inputStore, sourceCatalog, limits)
		if err != nil {
			redis.Close()
			postgres.Close()
			return nil, err
		}
		if err = childAdmission.CheckReady(ctx); err != nil {
			redis.Close()
			postgres.Close()
			return nil, fmt.Errorf("delegation requires initialized immutable migrations057/061")
		}
	}
	var converterDeveloper *ConverterDelegation
	var converterContinuations *ConverterContinuations
	var converterContinuationReader flow.ConverterContinuationReader
	var childLifecycle *task.DelegationManager
	if childAdmission != nil {
		childLifecycle = childAdmission.Manager
	}
	if cfg.ConverterDelegationEnabled {
		if childAdmission == nil || cfg.ConverterControlURL == "" {
			redis.Close()
			postgres.Close()
			return nil, fmt.Errorf("converter delegation requires ordinary delegation and the guarded converter control endpoint")
		}
		profile, profileErr := generatedtools.DecodeProfile([]byte(cfg.ConverterProfileJSON))
		if profileErr != nil {
			redis.Close()
			postgres.Close()
			return nil, profileErr
		}
		if err = generatedtools.CheckDeveloperReady(ctx, postgres.Pool()); err != nil {
			redis.Close()
			postgres.Close()
			return nil, fmt.Errorf("converter delegation requires initialized immutable migrations053/054")
		}
		limits := task.DefaultDelegationLimits()
		limits.MaxParallel = cfg.DelegationMaxParallel
		converterDeveloper, err = NewConverterDelegation(postgres.Pool(), taskStore, sessionRepo, inputStore, sourceCatalog, limits, profile)
		if err == nil {
			childLifecycle, err = NewDelegationLifecycleManager(taskStore, sessionRepo, inputStore, sourceCatalog, limits, childAdmission, converterDeveloper)
		}
		if err == nil {
			var allowance budget.ModelAllowance
			allowance, err = converterContinuationAllowance(providerResult, budgetController.Policy)
			if err == nil {
				converterContinuations, err = NewConverterContinuations(postgres.Pool(), taskStore, inputStore, sourceCatalog, budgetController, allowance, profile)
				if err == nil {
					err = converterDeveloper.EnableContinuations(converterContinuations)
					if err == nil {
						converterContinuationReader = converterContinuations
					}
				}
			}
		}
		if err != nil {
			redis.Close()
			postgres.Close()
			return nil, err
		}
	}
	childContexts := delegatedContextReader{sources: sourceCatalog}
	var failedAttempts flow.FailedDelegationAttemptReader
	var childMaintenance *delegationMaintenance
	if childLifecycle != nil {
		childContexts.manager = childLifecycle
		failedAttempts = failedDelegationAttemptReader{pool: postgres.Pool(), manager: childLifecycle}
		childMaintenance = &delegationMaintenance{pool: postgres.Pool(), manager: childLifecycle}
	}
	ownedSemantic, closeOwnedSemantic := configureOwnedSemantic(log, sourceCatalog, budgetController)
	semanticReady := false
	defer func() {
		if !semanticReady && closeOwnedSemantic != nil {
			closeOwnedSemantic()
		}
	}()
	var semanticReader memoryservice.SemanticReader
	if ownedSemantic != nil {
		semanticReader = ownedSemantic.Reader
	}

	memoryRuntimeConfig := cfg
	memoryModelStore, memoryModelWorker := configureModelJobs(sourceCatalog, budgetController, providerResult, cfg.ContextBudgetTokens)
	if memoryRuntimeConfig.MemoryPipelineEnabled {
		// Completed-run jobs need their own durable admission; they cannot
		// borrow a historical live lease. The legacy cloud path has none yet.
		log.Warn("legacy memory pipeline denied: use independently authorized background model jobs and gated consumers")
		memoryRuntimeConfig.MemoryPipelineEnabled = false
	}
	embeddingProvider, pipelineWorker := configureMemoryPipeline(log, memoryRuntimeConfig, authManager, m, pipelineQueue, transcriptStore, summaryStore, profileStore, episodicStore, chunkStore)
	finalResults := completion.New(postgres.Pool())
	finalResults.DataGuard = sourceCatalog.CheckRunSourcesTx
	finalResults.Evidence = effectEvidence{journal: actionauth.NewPostgresStore(postgres.Pool())}
	var skillClient *skills.Client
	if cfg.SkillControlURL != "" {
		token, tokenErr := actionauth.ReadServiceToken(cfg.ActionAuthorityTokenFile)
		if tokenErr != nil {
			redis.Close()
			postgres.Close()
			return nil, tokenErr
		}
		skillClient = skills.NewClient(cfg.SkillControlURL, token)
		finalResults.DataGuard = func(ctx context.Context, tx pgx.Tx, run string) error {
			if err := sourceCatalog.CheckRunSourcesTx(ctx, tx, run); err != nil {
				return err
			}
			return skills.CheckPinnedLifecycleTx(ctx, tx, run)
		}
	}
	var converterClient *generatedtools.Client
	var memorySources interface {
		PinTranscript(context.Context, string, int64) error
		CheckRunSources(context.Context, string) error
	} = sourceCatalog
	if cfg.ConverterControlURL != "" {
		token, tokenErr := actionauth.ReadServiceToken(cfg.ActionAuthorityTokenFile)
		if tokenErr != nil {
			redis.Close()
			postgres.Close()
			return nil, tokenErr
		}
		converterClient, err = generatedtools.NewClient(cfg.ConverterControlURL, token)
		if err != nil {
			redis.Close()
			postgres.Close()
			return nil, err
		}
		previousGuard := finalResults.DataGuard
		finalResults.DataGuard = func(ctx context.Context, tx pgx.Tx, run string) error {
			if e := previousGuard(ctx, tx, run); e != nil {
				return e
			}
			if e := generatedtools.CheckContinuationRunTx(ctx, tx, sourceCatalog, run); e != nil {
				return e
			}
			if e := generatedtools.CheckDeveloperLifecycleTx(ctx, tx, sourceCatalog, run); e != nil {
				return e
			}
			if e := generatedtools.CheckDeveloperReturnLifecycleTx(ctx, tx, sourceCatalog, run); e != nil {
				return e
			}
			return generatedtools.CheckPinnedLifecycleTx(ctx, tx, run)
		}
		memorySources = converterSources{Store: sourceCatalog, pool: postgres.Pool()}
	}
	outboxWorker := &deliveryoutbox.Worker{Store: finalResults.Outbox}
	var verifiedTaskDeliveryWorker *deliveryoutbox.Worker
	verifiedDeliveryReady, err := taskStore.VerifiedDeliveryReady(ctx)
	if err != nil {
		redis.Close()
		postgres.Close()
		return nil, err
	}
	if verifiedDeliveryReady {
		verifiedTaskDeliveryWorker = &deliveryoutbox.Worker{Store: task.VerifiedNotificationStore{Tasks: taskStore}}
	}
	if telegram != nil {
		outboxWorker.Sender = telegram
		if verifiedTaskDeliveryWorker != nil {
			verifiedTaskDeliveryWorker.Sender = telegram
		}
	}
	auxiliaryWorker := &completion.AuxiliaryWorker{Coordinator: finalResults, Handler: completionAuxiliary{Working: working.NewStore(postgres.Pool()), Transient: working.NewTransientStore(redis.Client()), Pipeline: pipelineWorker}}

	var memoryEmbeddings memoryservice.EmbeddingProvider
	if embeddingProvider != nil {
		memoryEmbeddings = embeddingProvider
	}

	transitionRepo := runservice.NewPostgresTransitionRepository(postgres.Pool())
	eventHub := observability.NewHub()
	var guardedRecovery flow.GuardedTabRecovery
	var guardedCheckpoints *singletab.SelectionCheckpoints
	if cfg.GuardedTabOwnerEnabled {
		guardedCheckpoints, err = singletab.NewSelectionCheckpoints(postgres.Pool(), inputStore, finalResults.DataGuard, cfg.GuardedTabHost)
		if err != nil {
			redis.Close()
			postgres.Close()
			return nil, err
		}
		guardedRecovery = guardedCheckpoints
	}

	guardedRequests, err := configureGuardedTabRequests(ctx, postgres.Pool(), cfg, finalResults.DataGuard, guardedCheckpoints)
	if err != nil {
		redis.Close()
		postgres.Close()
		return nil, err
	}
	var memoryCandidates *MemoryCandidates
	var memoryReads *MemoryReads
	if cfg.MemoryReadEnabled {
		if cfg.ActionAuthorityTokenFile == "" {
			redis.Close()
			postgres.Close()
			return nil, readcontrol.ErrDenied
		}
		memoryReads, err = NewMemoryReads(postgres.Pool(), sourceCatalog)
		if err == nil {
			err = memoryReads.CheckReady(ctx)
		}
		if err != nil {
			redis.Close()
			postgres.Close()
			return nil, err
		}
	}
	if cfg.MemoryCandidateEnabled {
		if cfg.ActionAuthorityTokenFile == "" {
			redis.Close()
			postgres.Close()
			return nil, candidatecontrol.ErrDenied
		}
		memoryCandidates, err = NewMemoryCandidates(postgres.Pool(), sourceCatalog)
		if err == nil {
			err = memoryCandidates.CheckReady(ctx)
		}
		if err != nil {
			redis.Close()
			postgres.Close()
			return nil, err
		}
	}
	var guardedRequestAdapter flow.GuardedTabRequests
	if guardedRequests != nil {
		guardedRequestAdapter = guardedRequests
	}
	executor := flow.NewService(
		sessionRepo,
		sessionLeaseManager,
		runManager,
		transcriptStore,
		providerResult.Provider,
		flow.Config{
			Metrics: m,
			MemoryScopeResolver: func(ctx context.Context) ([]memoryservice.Scope, error) {
				scopes, e := sourceCatalog.ExtraScopes(ctx, "memory_read")
				if e != nil {
					return nil, e
				}
				out := make([]memoryservice.Scope, len(scopes))
				for i, scope := range scopes {
					out[i] = memoryservice.Scope{Type: scope.Type, ID: scope.ID}
				}
				return out, nil
			},
			SemanticMemory:           semanticReader,
			MemorySources:            memorySources,
			DelegationContexts:       childContexts,
			FailedDelegationAttempts: failedAttempts,
			ConverterContinuations:   converterContinuationReader,
			Skills:                   skillCatalogIfEnabled(skillClient),
			Budgets:                  budgetController,
			Completion:               finalResults,
			DurableMemoryPipeline:    pipelineWorker != nil,
			AutonomyMode:             autonomyMode(cfg.AutonomyMode),
			ActionAuthorizer:         toolBrokerClient,
			ActionMandateID:          cfg.ActionMandateID,
			Checkpoints:              inputStore,
			GuardedTabRecovery:       guardedRecovery,
			GuardedTabRequests:       guardedRequestAdapter,
			Tasks:                    taskStore,
			OwnerGoals:               ownerGoals,
			ProviderName:             providerResult.ProviderName,
			ModelName:                providerResult.ModelName,
			OwnerID:                  cfg.Shared.ServiceName,
			LeaseTTL:                 int64(cfg.SessionLeaseTTLSeconds),
			PromptManager:            promptmgmt.NewManager(settingsStore),
			PromptAssembler:          promptmgmt.NewAssembler(),
			ProfileLimit:             cfg.MemoryProfileLimit,
			EpisodeLimit:             cfg.MemoryEpisodicLimit,
			MemoryScopes:             cfg.MemoryScopeOrder,
			Delivery:                 delivery,
			Tools:                    toolBrokerClient,
			ToolCatalog:              toolBrokerClient,
			ApprovalChecker:          toolBrokerClient,
			ApprovalGate:             approvalGate,
			ApprovalService:          approvalService,
			WorkingStore:             workingStoreAdapter{store: working.NewStore(postgres.Pool())},
			TransientStore:           transientWorkingStoreAdapter{store: working.NewTransientStore(redis.Client()), sources: sourceCatalog},
			TransientTTL:             time.Duration(cfg.MemoryWorkingTransientTTLSeconds) * time.Second,
			ProfileStore:             profileStoreAdapter{store: profileStore, sources: sourceCatalog},
			EpisodeStore:             episodicStoreAdapter{store: episodicStore, sources: sourceCatalog},
			ChunkStore:               chunkStoreAdapter{store: chunkStore, sources: sourceCatalog},
			Embeddings:               memoryEmbeddings,
			PipelineEnqueuer:         pipelineEnqueuer,
			SummaryReader:            summaryStore,
			ContextBudgetTokens:      cfg.ContextBudgetTokens,
			TransitionLogger:         transitionRepo,
			EventHub:                 eventHub,
			Artifacts:                artifactsService,
			Activity:                 activityService,
		},
		logger.WithComponent(log, "executor"),
	)
	inputWorker := durableinput.NewService(inputStore, sessionRepo, executor, runManager, logger.WithComponent(log, "durable-input"))
	taskControls := task.NewController(taskStore, sessionLeaseManager.Authority(), sessionLeaseManager, runRepo, approvalRepo, inputStore, transcriptStore)
	ownerGoals.SetControls(taskControls)
	inputWorker.SetTaskControls(taskControls)
	initiativeStore := initiative.NewStore(postgres.Pool())
	initiativeStore.RunDataGuardTx = sourceCatalog.CheckRunSourcesTx
	initiativeDeliveryWorker := &deliveryoutbox.Worker{Store: initiative.DigestStore{Store: initiativeStore}}
	if telegram != nil {
		initiativeDeliveryWorker.Sender = telegram
	}
	allowance := budget.ModelAllowance{Provider: providerResult.ProviderName, Model: providerResult.ModelName}
	if limits, ok := providerResult.Provider.(interface{ InferenceLimits() (int64, int64) }); ok {
		allowance.InputTokens, allowance.OutputTokens = limits.InferenceLimits()
	}
	initiativeWorker := &initiative.Worker{Store: initiativeStore, Observer: initiative.ProjectObserver{Store: initiativeStore, RunDataGuard: func(ctx context.Context, runID string) error {
		return checkRecordedInitiativeSources(ctx, postgres.Pool(), sourceCatalog, runID)
	}}, Promoter: &initiative.Promoter{Store: initiativeStore, Goals: ownerGoals, Tasks: taskStore, Inputs: inputStore, Budgets: budgetController, Allowance: allowance}}
	inputWorker.SetMaintenance(func(ctx context.Context) error {
		if err := approvalService.ExpireInvalidPending(ctx); err != nil {
			return err
		}
		if err := ownerGoals.Sweep(ctx); err != nil {
			return err
		}
		maintenanceErr := initiativeWorker.Sweep(ctx)
		if childMaintenance != nil {
			// A denied branch must not prevent unrelated admitted maintenance.
			maintenanceErr = errors.Join(maintenanceErr, childMaintenance.Sweep(ctx))
		}
		if converterContinuations != nil {
			maintenanceErr = errors.Join(maintenanceErr, converterContinuations.Sweep(ctx))
		}
		return maintenanceErr
	})
	if telegram != nil {
		telegram.SetExecutor(inputWorker)
		telegram.SetOwnerTaskReader(taskStore)
		telegram.SetOwnerAuthority(ownerGoals)
		telegram.SetApprovalService(approvalService)
		telegram.SetSingleTabSelector(singleTabService)
		telegram.SetAuthManager(authManager)
	}

	var actionAuthorityHandler http.Handler
	var skillEvidenceHandler http.Handler
	var delegationHandler http.Handler
	var guardedRequestHandler http.Handler
	var memoryCandidateHandler http.Handler
	var memoryReadHandler http.Handler
	var converterDelegationHandler http.Handler
	workspaceReady := false
	if cfg.WorkspaceEnabled {
		provenance, readyErr := workspaceexec.NewProvenance(postgres.Pool(), sourceCatalog)
		if readyErr == nil {
			readyErr = provenance.CheckReady(ctx)
		}
		if readyErr == nil {
			readyErr = workspaceexec.CheckDispatchSchema(ctx, postgres.Pool())
		}
		if readyErr != nil {
			redis.Close()
			postgres.Close()
			return nil, fmt.Errorf("workspace catalog requires current private source/dispatch/input schema and deletion anchor")
		}
		workspaceReady = true
	}
	if cfg.ActionAuthorityTokenFile != "" {
		token, readErr := actionauth.ReadServiceToken(cfg.ActionAuthorityTokenFile)
		if readErr != nil {
			redis.Close()
			postgres.Close()
			return nil, readErr
		}
		if childAdmission != nil {
			delegationHandler, err = delegation.NewAdmissionHandler(token, childAdmission)
			if err != nil {
				redis.Close()
				postgres.Close()
				return nil, err
			}
		}
		if memoryCandidates != nil {
			// Publication has an independent gate from read-only access.
			memoryCandidateHandler, err = candidatecontrol.NewHandler(token, memoryCandidates)
			if err != nil {
				redis.Close()
				postgres.Close()
				return nil, err
			}
		}
		if memoryReads != nil {
			memoryReadHandler, err = readcontrol.NewHandler(token, memoryReads)
			if err != nil {
				redis.Close()
				postgres.Close()
				return nil, err
			}
		}
		if guardedRequests != nil {
			guardedRequestHandler, err = singletabcontrol.NewRequestHandler(token, guardedRequests)
			if err != nil {
				redis.Close()
				postgres.Close()
				return nil, err
			}
		}
		if converterDeveloper != nil {
			converterDelegationHandler, err = delegation.NewAdmissionHandler(token, converterDeveloper)
			if err != nil {
				redis.Close()
				postgres.Close()
				return nil, err
			}
		}
		actionAuthorityHandler = actionauth.NewContextHandler(token, &actionContextReader{runs: runManager, sessions: sessionRepo, leases: sessionLeaseManager, tasks: taskStore, approvals: approvalService, pool: postgres.Pool(), mandateID: cfg.ActionMandateID, converterLifecycleReady: converterClient != nil, delegationReady: childAdmission != nil, guardedTabRequestReady: guardedRequests != nil, guardedTabRuntimeReady: cfg.GuardedTabRuntimeEnabled && guardedRequests != nil, memoryCandidateReady: memoryCandidates != nil, memoryReadReady: memoryReads != nil, workspaceReady: workspaceReady, converterDelegationReady: converterDeveloper != nil && converterClient != nil})
		skillEvidenceHandler = skills.NewEvidenceHandler(token, &skillEvidence{tasks: taskStore, journal: actionauth.NewPostgresStore(postgres.Pool()), transcript: transcriptStore, budgets: budget.NewLedger(postgres.Pool()), results: finalResults, sources: sourceCatalog})
	} else {
		log.Warn("private action authority unavailable; Tool Broker will deny execution")
	}
	apiServer := apiservice.NewServer(inputWorker, logger.WithComponent(log, "api"))
	guardedTabOwner, err := configureGuardedTabOwner(ctx, postgres.Pool(), cfg, finalResults.DataGuard, guardedCheckpoints)
	if err != nil {
		redis.Close()
		postgres.Close()
		return nil, err
	}
	var delegationDecisions apiservice.DelegationDecisions
	if childLifecycle != nil {
		delegationDecisions = childLifecycle
	}
	httpServer := newHTTPServer(httpServerDeps{
		guardedTabOwner:            guardedTabOwner,
		guardedRequestHandler:      guardedRequestHandler,
		memoryCandidateHandler:     memoryCandidateHandler,
		memoryReadHandler:          memoryReadHandler,
		delegationDecisions:        delegationDecisions,
		delegationHandler:          delegationHandler,
		converterDelegationHandler: converterDelegationHandler,
		memoryIndexing:             ownedSemantic,
		memoryModelStore:           memoryModelStore,
		memoryModelConfigured:      memoryModelWorker != nil,
		memorySummaryConfigured:    summaryConsumerConfigured(memoryModelWorker),
		sourceCatalog:              sourceCatalog,
		cfg:                        cfg,
		finalResults:               finalResults,
		actionAuthorityHandler:     actionAuthorityHandler,
		skillEvidenceHandler:       skillEvidenceHandler,
		skillClient:                skillClient,
		converterClient:            converterClient,
		log:                        log,
		settingsStore:              settingsStore,
		hotConfig:                  hotConfig,
		postgres:                   postgres,
		metrics:                    m,
		health:                     h,
		toolBrokerClient:           toolBrokerClient,
		authManager:                authManager,
		providerResult:             providerResult,
		executor:                   executor,
		inputWorker:                inputWorker,
		inputStore:                 inputStore,
		ownerAuth:                  ownerAuth,
		sessionRepo:                sessionRepo,
		runRepo:                    runRepo,
		taskStore:                  taskStore,
		taskControls:               taskControls,
		ownerGoals:                 ownerGoals,
		initiativeStore:            initiativeStore,
		transcriptStore:            transcriptStore,
		transitionRepo:             transitionRepo,
		eventHub:                   eventHub,
		approvalRepo:               approvalRepo,
		approvalService:            approvalService,
		singleTabService:           singleTabService,
		artifactsRepo:              artifactsRepo,
		artifactsService:           artifactsService,
		activityRepo:               activityRepo,
		deliveryEventsRepo:         deliveryEventsRepo,
		pipelineQueue:              pipelineQueue,
		pipelineWorker:             pipelineWorker,
		embeddingProvider:          embeddingProvider,
		profileStore:               profileStore,
		episodicStore:              episodicStore,
		chunkStore:                 chunkStore,
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

	semanticReady = true
	return &App{
		config:                     cfg,
		log:                        log,
		metrics:                    m,
		health:                     h,
		postgres:                   postgres,
		redis:                      redis,
		httpServer:                 httpServer,
		grpcServer:                 grpcServer,
		grpcListen:                 grpcListener,
		telegram:                   telegram,
		toolBroker:                 toolBrokerClient,
		pipelineWorker:             pipelineWorker,
		inputWorker:                inputWorker,
		outboxWorker:               outboxWorker,
		initiativeDeliveryWorker:   initiativeDeliveryWorker,
		verifiedTaskDeliveryWorker: verifiedTaskDeliveryWorker,
		auxiliaryWorker:            auxiliaryWorker,
		memoryCleanup:              &catalog.CleanupWorker{Store: sourceCatalog, Cleaner: privacyCacheCleaner{store: working.NewTransientStore(redis.Client())}},
		memoryIndexing:             ownedSemantic,
		memoryIndexingClose:        closeOwnedSemantic,
		memoryModelJobs:            memoryModelWorker,
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
