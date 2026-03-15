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

	apiservice "github.com/butler/butler/apps/orchestrator/internal/api"
	telegramadapter "github.com/butler/butler/apps/orchestrator/internal/channel/telegram"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/config"
	orchestratorv1 "github.com/butler/butler/internal/gen/orchestrator/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/pipeline"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/metrics"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	redisstore "github.com/butler/butler/internal/storage/redis"
	"github.com/butler/butler/internal/transport"
	// Import openai provider to register it via init().
	"github.com/butler/butler/internal/transport/openai"
	"google.golang.org/grpc"
)

type App struct {
	config     config.OrchestratorConfig
	log        *slog.Logger
	metrics    *metrics.Registry
	health     *health.Service
	postgres   *postgresstore.Store
	redis      *redisstore.Store
	httpServer *http.Server
	grpcServer *grpc.Server
	grpcListen net.Listener
	telegram   *telegramadapter.Adapter
	toolBroker *tools.BrokerClient
}

type profileStoreAdapter struct{ store *profile.Store }

func (a profileStoreAdapter) GetByScope(ctx context.Context, scopeType, scopeID string) ([]flow.MemoryProfileEntry, error) {
	entries, err := a.store.GetByScope(ctx, scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	result := make([]flow.MemoryProfileEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result, nil
}

type episodicStoreAdapter struct{ store *episodic.Store }

func (a episodicStoreAdapter) Search(ctx context.Context, scopeType, scopeID string, embedding []float32, limit int) ([]flow.MemoryEpisode, error) {
	entries, err := a.store.Search(ctx, scopeType, scopeID, embedding, limit)
	if err != nil {
		return nil, err
	}
	result := make([]flow.MemoryEpisode, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}
	return result, nil
}

func New(ctx context.Context) (*App, error) {
	cfg, snapshot, err := config.LoadOrchestratorFromEnv()
	if err != nil {
		return nil, err
	}

	log := logger.New(logger.Options{
		Service:   cfg.Shared.ServiceName,
		Component: "bootstrap",
		Level:     parseLogLevel(cfg.Shared.LogLevel),
	})
	log.Info("loaded orchestrator config", slog.String("config_summary", configSummary(snapshot)))

	postgres, err := postgresstore.Open(ctx, postgresstore.ConfigFromShared(cfg.Postgres), logger.WithComponent(log, "postgres"))
	if err != nil {
		return nil, err
	}

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

	provider, err := transport.NewProvider("openai", openai.Config{
		APIKey:        cfg.OpenAIAPIKey,
		Model:         cfg.OpenAIModel,
		BaseURL:       cfg.OpenAIBaseURL,
		RealtimeURL:   cfg.OpenAIRealtimeURL,
		TransportMode: cfg.OpenAITransportMode,
		Timeout:       time.Duration(cfg.OpenAITimeoutSeconds) * time.Second,
		Logger:        logger.WithComponent(log, "transport-openai"),
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
	delivery := flow.NewCompositeDeliverySink(flow.NewLoggingDeliverySink(log))
	var telegram *telegramadapter.Adapter
	approvalGate := flow.NewApprovalGate()
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
	sessionRepo := session.NewPostgresRepository(postgres.Pool())
	sessionLeaseManager := session.NewRedisLeaseManager(redis.Client(), logger.WithComponent(log, "session-lease-store"))
	transcriptStore := transcript.NewStore(postgres.Pool())

	// Memory pipeline: enqueuer for async post-run extraction.
	pipelineQueue := pipeline.NewQueue(redis.Client())
	pipelineEnqueuer := pipeline.NewEnqueuer(pipelineQueue)

	executor := flow.NewService(
		sessionRepo,
		sessionLeaseManager,
		runManager,
		transcriptStore,
		provider,
		flow.Config{
			ProviderName:     "openai",
			ModelName:        cfg.OpenAIModel,
			OwnerID:          cfg.Shared.ServiceName,
			LeaseTTL:         int64(cfg.SessionLeaseTTLSeconds),
			ProfileLimit:     cfg.MemoryProfileLimit,
			EpisodeLimit:     cfg.MemoryEpisodicLimit,
			MemoryScopes:     cfg.MemoryScopeOrder,
			Delivery:         delivery,
			Tools:            toolBrokerClient,
			ApprovalChecker:  toolBrokerClient,
			ApprovalGate:     approvalGate,
			ProfileStore:     profileStoreAdapter{store: profile.NewStore(postgres.Pool())},
			EpisodeStore:     episodicStoreAdapter{store: episodic.NewStore(postgres.Pool())},
			PipelineEnqueuer: pipelineEnqueuer,
			SummaryReader:    sessionRepo,
		},
		logger.WithComponent(log, "executor"),
	)
	if telegram != nil {
		telegram.SetExecutor(executor)
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
	doctorServer := apiservice.NewDoctorServer(postgres.Pool(), doctorChecker, logger.WithComponent(log, "doctor-api"))

	mux := http.NewServeMux()
	mux.Handle("/health", h.Handler())
	mux.Handle("/metrics", m.Handler())
	mux.Handle("/api/v1/events", apiServer.HTTPHandler())
	mux.Handle("/api/v1/sessions", viewServer.HandleListSessions())
	mux.Handle("/api/v1/sessions/", viewServer.HandleGetSession())
	mux.Handle("/api/v1/runs/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/transcript") {
			viewServer.HandleGetRunTranscript().ServeHTTP(w, r)
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
		config:     cfg,
		log:        log,
		metrics:    m,
		health:     h,
		postgres:   postgres,
		redis:      redis,
		httpServer: server,
		grpcServer: grpcServer,
		grpcListen: grpcListener,
		telegram:   telegram,
		toolBroker: toolBrokerClient,
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
