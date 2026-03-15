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
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"google.golang.org/grpc"
)

type App struct {
	config     config.ToolBrokerConfig
	log        *slog.Logger
	httpServer *http.Server
	grpcServer *grpc.Server
	grpcListen net.Listener
	router     *runtimeclient.Router
	postgres   *postgresstore.Store
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
			credentials.NewBroker(credentials.NewStore(postgres.Pool())),
			credentials.EnvSecretResolver{},
		)
	}
	router := runtimeclient.New(log)
	server := broker.NewServer(registryStore, router, credentialResolver, log)

	httpMux := http.NewServeMux()
	httpMux.Handle("/health", health.New(cfg.Shared.ServiceName).Handler())
	httpMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("tool-broker")) })
	httpServer := &http.Server{Addr: cfg.Shared.HTTPAddr, Handler: httpMux, ReadHeaderTimeout: 5 * time.Second}

	grpcListener, err := net.Listen("tcp", cfg.Shared.GRPCAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on grpc addr %s: %w", cfg.Shared.GRPCAddr, err)
	}
	grpcServer := grpc.NewServer()
	toolbrokerv1.RegisterToolBrokerServiceServer(grpcServer, server)

	return &App{config: cfg, log: log, httpServer: httpServer, grpcServer: grpcServer, grpcListen: grpcListener, router: router, postgres: postgres}, nil
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
