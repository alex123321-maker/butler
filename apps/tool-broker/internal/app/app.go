package app

import (
	"context"
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

	"github.com/butler/butler/apps/tool-broker/internal/broker"
	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/extensions"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/candidatecontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/butler/butler/internal/skills"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
)

type App struct {
	config        config.ToolBrokerConfig
	log           *slog.Logger
	httpServer    *http.Server
	grpcServer    *grpc.Server
	grpcListen    net.Listener
	router        *runtimeclient.Router
	postgres      *postgresstore.Store
	converterConn *grpc.ClientConn
}

func New(ctx context.Context) (*App, error) {
	cfg, snapshot, err := config.LoadToolBrokerFromEnv()
	if err != nil {
		return nil, err
	}
	log := logger.New(logger.Options{Service: cfg.Shared.ServiceName, Component: "bootstrap", Level: parseLogLevel(cfg.Shared.LogLevel), Writer: os.Stdout})
	log.Info("loaded tool broker config", slog.String("config_summary", configSummary(snapshot)))

	registryStore, err := registry.Load(cfg.RegistryPath, cfg.DefaultTarget)
	if err != nil {
		return nil, err
	}
	var postgres *postgresstore.Store
	var credentialResolver *credentials.ToolCallBroker
	if cfg.Postgres.URL != "" {
		postgres, err = postgresstore.Open(ctx, postgresstore.ConfigFromShared(cfg.Postgres), log)
		if err != nil {
			return nil, err
		}
		credentialResolver = credentials.NewToolCallBroker(
			credentials.NewBroker(credentials.NewStore(postgres.Pool()), credentials.NewAuditStore(postgres.Pool())),
			credentials.EnvSecretResolver{},
		)
	}
	if cfg.DelegationEnabled && (postgres == nil || cfg.ActionAuthorityTokenFile == "" || cfg.DelegationControlURL == "") {
		if postgres != nil {
			postgres.Close()
		}
		return nil, fmt.Errorf("delegation requires PostgreSQL and a fixed authenticated private control endpoint")
	}
	if cfg.ConverterDelegationEnabled && (!cfg.ConvertersEnabled || !cfg.DelegationEnabled || cfg.ConverterDelegationControlURL == "") {
		if postgres != nil {
			postgres.Close()
		}
		return nil, fmt.Errorf("converter delegation requires enabled converters, ordinary delegation and a fixed private developer endpoint")
	}
	router := runtimeclient.New(log)
	if cfg.MemoryCandidateEnabled && (postgres == nil || cfg.ActionAuthorityTokenFile == "" || cfg.MemoryCandidateControlURL == "") {
		if postgres != nil {
			postgres.Close()
		}
		return nil, fmt.Errorf("memory candidates require PostgreSQL and a fixed authenticated private endpoint")
	}
	if (cfg.GuardedTabRequestEnabled || cfg.GuardedTabRuntimeEnabled) && (postgres == nil || cfg.ActionAuthorityTokenFile == "" || cfg.GuardedTabRequestControlURL == "") {
		if postgres != nil {
			postgres.Close()
		}
		return nil, fmt.Errorf("guarded tab requests require PostgreSQL and a fixed authenticated private endpoint")
	}
	server := broker.NewServer(registryStore, router, credentialResolver, log)
	var mandateHandler http.Handler
	var effectHandler http.Handler
	var extensionHandler http.Handler
	var skillHandler http.Handler
	var converterHandler http.Handler
	var guardedResourceHandler http.Handler
	var converterConn *grpc.ClientConn
	ready := false
	defer func() {
		if !ready && converterConn != nil {
			_ = converterConn.Close()
		}
	}()
	serviceToken := ""
	if cfg.ActionAuthorityTokenFile != "" && postgres != nil {
		token, err := actionauth.ReadServiceToken(cfg.ActionAuthorityTokenFile)
		if err != nil {
			postgres.Close()
			return nil, err
		}
		serviceToken = token
		reader, err := actionauth.NewHTTPContextReader(cfg.ActionContextURL, token)
		if err != nil {
			postgres.Close()
			return nil, err
		}
		authorizationStore := actionauth.NewPostgresStore(postgres.Pool())
		sourceCatalog := catalog.New(postgres.Pool(), catalog.NewFileManifest(strings.TrimSpace(os.Getenv("BUTLER_MEMORY_DELETION_MANIFEST_FILE"))))
		if cfg.GuardedTabRuntimeEnabled {
			tx, restoreErr := postgres.Pool().Begin(ctx)
			if restoreErr == nil {
				restoreErr = sourceCatalog.CheckRestoreTx(ctx, tx)
				if restoreErr == nil {
					restoreErr = tx.Commit(ctx)
				}
				_ = tx.Rollback(context.Background())
			}
			if restoreErr != nil {
				postgres.Close()
				return nil, fmt.Errorf("guarded runtime requires a current private deletion manifest")
			}
		}
		sources := resourceexec.SourceGuard(func(ctx context.Context, tx pgx.Tx, run string) error {
			if err := sourceCatalog.CheckRunSourcesTx(ctx, tx, run); err != nil {
				return err
			}
			// Accepted technical inputs retain their lifecycle guard even when
			// new developer admissions are disabled by a later operator rollout.
			return generatedtools.CheckContinuationRunTx(ctx, tx, sourceCatalog, run)
		})
		authorizationStore.SetRunDataGuard(sources)
		budgetPolicy, policyErr := budget.ParsePolicy(cfg.ExecutionBudgetPolicyJSON)
		if policyErr != nil {
			postgres.Close()
			return nil, policyErr
		}
		budgetController := budget.NewController(postgres.Pool(), budgetPolicy)
		budgetController.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
		authorizationStore.SetBudgets(budgetController)
		profiles, profileErr := extensions.ParseProfiles(cfg.ExtensionProfilesJSON)
		if profileErr != nil {
			postgres.Close()
			return nil, profileErr
		}
		if len(profiles) > 0 {
			runtimeToken, tokenErr := actionauth.ReadServiceToken(cfg.ExtensionRuntimeTokenFile)
			if tokenErr != nil {
				postgres.Close()
				return nil, tokenErr
			}
			for _, profile := range profiles {
				if tokenErr = router.SetTargetAuthentication(profile.Target, runtimeToken); tokenErr != nil {
					postgres.Close()
					return nil, tokenErr
				}
			}
		}
		extensionStore := extensions.NewStore(postgres.Pool(), cfg.ExtensionOwnerID, profiles, budgetController)
		if len(profiles) > 0 {
			if cfg.ExtensionOwnerID == "" {
				postgres.Close()
				return nil, fmt.Errorf("configured extension owner is required")
			}
			if err = extensionStore.RecoverProbes(context.Background()); err != nil {
				postgres.Close()
				return nil, fmt.Errorf("extension readiness recovery unavailable")
			}
		}
		registryStore.SetExtensions(extensionStore)
		authorizationStore.SetContractGuard(extensionStore)
		server.SetBudgets(budgetController)
		server.SetBudgetTimeout(time.Duration(budgetPolicy.ToolMillis) * time.Millisecond)
		authorizer := actionauth.NewService(authorizationStore, reader, credentialResolver)
		authorizer.SetCapabilities(extensionStore)
		server.SetActionAuthorizer(authorizer)
		catalog := &broker.TaskCatalog{Registry: registryStore, Contexts: reader, Mandates: authorizationStore, Extensions: extensionStore}
		server.SetScopedCatalog(catalog)
		var skillStore *skills.Store
		var executor broker.RuntimeExecutor = router
		if cfg.SkillsEnabled {
			if cfg.ExtensionOwnerID == "" {
				postgres.Close()
				return nil, fmt.Errorf("configured skill owner is required")
			}
			if err = registryStore.RegisterOwned(skills.Contracts()); err != nil {
				postgres.Close()
				return nil, err
			}
			evidenceURL, endpointErr := skills.SiblingEndpoint(cfg.ActionContextURL, "/internal/skill-evidence")
			if endpointErr != nil {
				postgres.Close()
				return nil, endpointErr
			}
			skillStore = skills.NewStore(postgres.Pool(), cfg.ExtensionOwnerID, skills.NewClient(evidenceURL, token), broker.SkillContracts{Registry: registryStore, Extensions: extensionStore}, broker.SkillEnvironment{Contexts: reader, Catalog: catalog})
			skillStore.SetSources(sourceCatalog)
			executor = &skills.RoutedExecutor{Store: skillStore, Next: router}
			authorizationStore.SetContractGuard(broker.CatalogGuards{Extensions: extensionStore, Skills: skillStore})
			skillHandler = skills.NewHandler(token, skillStore)
		}
		var converterStore *generatedtools.Store
		if cfg.ConvertersEnabled {
			if cfg.ExtensionOwnerID == "" || skillStore == nil {
				postgres.Close()
				return nil, fmt.Errorf("converter development requires a configured owner and current reviewed skill search")
			}
			profile, profileErr := generatedtools.DecodeProfile([]byte(cfg.ConverterProfileJSON))
			if profileErr != nil {
				postgres.Close()
				return nil, profileErr
			}
			runtimeToken, tokenErr := actionauth.ReadServiceToken(cfg.ConverterRuntimeTokenFile)
			if tokenErr != nil {
				postgres.Close()
				return nil, tokenErr
			}
			runtimeClient, conn, clientErr := generatedtools.NewRuntimeClient(cfg.ConverterRuntimeTarget, runtimeToken)
			if clientErr != nil {
				postgres.Close()
				return nil, clientErr
			}
			converterConn = conn
			if err = registryStore.RegisterOwned(generatedtools.Contracts()); err != nil {
				postgres.Close()
				return nil, err
			}
			environment := &broker.ConverterEnvironment{Contexts: reader, Catalog: catalog, Skills: skillStore}
			converterStore = generatedtools.NewStore(postgres.Pool(), cfg.ExtensionOwnerID, profile, sourceCatalog, environment, runtimeClient)
			catalog.Converters = converterStore
			registryStore.SetConverters(converterStore)
			// Skill dependencies resolve current generated contracts on the same
			// owned transaction; no nested RPC/pool acquisition is permitted.
			skillStore.SetContracts(broker.SkillContracts{Registry: registryStore, Extensions: extensionStore, Converters: converterStore})
			executor = &generatedtools.RoutedExecutor{Store: converterStore, Next: executor}
			converterHandler = generatedtools.NewHandler(token, converterStore)
		}
		if cfg.DelegationEnabled {
			client, clientErr := delegation.NewAdmissionClient(cfg.DelegationControlURL, token)
			if clientErr != nil {
				postgres.Close()
				return nil, clientErr
			}
			if err = registryStore.RegisterOwned([]*toolbrokerv1.ToolContract{delegation.AdmissionContract()}); err != nil {
				postgres.Close()
				return nil, err
			}
			executor = &broker.DelegationExecutor{Admissions: client, Next: executor}
		}
		if cfg.ConverterDelegationEnabled {
			if err = generatedtools.CheckDeveloperReady(ctx, postgres.Pool()); err != nil {
				postgres.Close()
				return nil, fmt.Errorf("converter delegation requires initialized immutable migrations053/054")
			}
			client, clientErr := delegation.NewAdmissionClient(cfg.ConverterDelegationControlURL, token)
			if clientErr != nil {
				postgres.Close()
				return nil, clientErr
			}
			if err = registryStore.RegisterOwned([]*toolbrokerv1.ToolContract{generatedtools.DeveloperAdmissionContract()}); err != nil {
				postgres.Close()
				return nil, err
			}
			executor = &broker.ConverterDelegationExecutor{Admissions: client, Next: executor}
		}
		server.SetCatalogExecutor(executor)
		resourceStore, resourceHandler, resourceErr := configureGuardedTabRuntime(ctx, cfg, postgres.Pool(), registryStore, router, budgetController, sources)
		if resourceErr != nil {
			postgres.Close()
			return nil, resourceErr
		}
		if resourceStore != nil {
			catalog.GuardedTabOnly = true
			admission := broker.GuardedTabAdmission{Store: resourceStore}
			authorizationStore.SetDispatchClaimGuard(admission)
			server.SetResourceAdmission(admission)
			executor, err = broker.NewResourceExecutor(resourceStore, executor)
			if err != nil {
				postgres.Close()
				return nil, err
			}
			guardedResourceHandler = resourceHandler
			server.SetCatalogExecutor(executor)
		}
		if cfg.MemoryCandidateEnabled {
			client, clientErr := candidatecontrol.NewClient(cfg.MemoryCandidateControlURL, token)
			if clientErr != nil {
				postgres.Close()
				return nil, clientErr
			}
			if err = registryStore.RegisterOwned([]*toolbrokerv1.ToolContract{candidatecontrol.Contract()}); err != nil {
				postgres.Close()
				return nil, err
			}
			executor = &broker.MemoryCandidateExecutor{Candidates: client, Next: executor}
			server.SetCatalogExecutor(executor)
		}
		if cfg.GuardedTabRequestEnabled {
			client, clientErr := singletabcontrol.NewRequestClient(cfg.GuardedTabRequestControlURL, token)
			if clientErr != nil {
				postgres.Close()
				return nil, clientErr
			}
			if err = registryStore.RegisterOwned([]*toolbrokerv1.ToolContract{singletabcontrol.RequestContract()}); err != nil {
				postgres.Close()
				return nil, err
			}
			executor = &broker.GuardedTabRequestExecutor{Requests: client, Next: executor}
			server.SetCatalogExecutor(executor)
		}
		authorizer.SetCapabilities(broker.CapabilityGuards{Extensions: extensionStore, Converters: converterStore})
		authorizationStore.SetContractGuard(broker.CatalogGuards{Extensions: extensionStore, Skills: skillStore, Converters: converterStore})
		extensionHandler = extensions.NewControlHandler(token, extensionStore, router)
		effectHandler = actionauth.NewEffectHandler(token, authorizer, authorizationStore)
		mandateHandler = actionauth.NewMandateHandler(token, authorizationStore, reader)
	} else {
		log.Warn("action authorization unavailable; tool execution will fail closed")
	}

	httpMux := http.NewServeMux()
	if guardedResourceHandler != nil {
		httpMux.Handle("/internal/guarded-tab-resources", guardedResourceHandler)
	}
	if converterHandler != nil {
		httpMux.Handle("/internal/converters", converterHandler)
	}
	if skillHandler != nil {
		httpMux.Handle("/internal/skills", skillHandler)
	}
	if extensionHandler != nil {
		httpMux.Handle("/internal/extensions", extensionHandler)
	}
	if mandateHandler != nil {
		httpMux.Handle("/internal/action-mandates", mandateHandler)
	}
	if effectHandler != nil {
		httpMux.Handle("/internal/action-effects", effectHandler)
	}
	httpMux.Handle("/health", health.New(cfg.Shared.ServiceName).Handler())
	httpMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("tool-broker")) })
	httpServer := &http.Server{Addr: cfg.Shared.HTTPAddr, Handler: httpMux, ReadHeaderTimeout: 5 * time.Second}

	grpcListener, err := net.Listen("tcp", cfg.Shared.GRPCAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on grpc addr %s: %w", cfg.Shared.GRPCAddr, err)
	}
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(actionauth.ExecutionUnaryInterceptor(serviceToken)))
	toolbrokerv1.RegisterToolBrokerServiceServer(grpcServer, server)

	ready = true
	return &App{config: cfg, log: log, httpServer: httpServer, grpcServer: grpcServer, grpcListen: grpcListener, router: router, postgres: postgres, converterConn: converterConn}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 2)
	go func() {
		a.log.Info("starting tool broker http server", slog.String("http_addr", a.config.Shared.HTTPAddr))
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	go func() {
		a.log.Info("starting tool broker grpc server", slog.String("grpc_addr", a.config.Shared.GRPCAddr))
		if err := a.grpcServer.Serve(a.grpcListen); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errCh:
		return a.shutdown(err)
	case <-sigCtx.Done():
		a.log.Info("received shutdown signal")
		return a.shutdown(nil)
	}
}

func (a *App) shutdown(runErr error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.httpServer.Shutdown(ctx); err != nil && runErr == nil {
		runErr = err
	}
	a.grpcServer.GracefulStop()
	if a.grpcListen != nil {
		if err := a.grpcListen.Close(); err != nil && !errors.Is(err, net.ErrClosed) && runErr == nil {
			runErr = err
		}
	}
	if a.router != nil {
		if err := a.router.Close(); err != nil && runErr == nil {
			runErr = err
		}
	}
	if a.postgres != nil {
		a.postgres.Close()
	}
	if a.converterConn != nil {
		if err := a.converterConn.Close(); err != nil && runErr == nil {
			runErr = err
		}
	}
	if runErr != nil {
		a.log.Error("tool broker stopped with error", slog.String("error", runErr.Error()))
		return runErr
	}
	return nil
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

func configSummary(snapshot config.Introspector) string {
	keys := snapshot.ListKeys()
	if len(keys) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(keys))
	for _, item := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s(%s)", item.Key, item.EffectiveValue, item.Source))
	}
	return strings.Join(parts, ", ")
}
