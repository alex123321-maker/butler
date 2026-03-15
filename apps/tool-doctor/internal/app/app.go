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
	"syscall"
	"time"

	"github.com/butler/butler/apps/tool-doctor/internal/runtime"
	"github.com/butler/butler/internal/config"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
	"google.golang.org/grpc"
)

type App struct {
	config     config.ToolDoctorConfig
	log        *slog.Logger
	httpServer *http.Server
	grpcServer *grpc.Server
	grpcListen net.Listener
}

func New(context.Context) (*App, error) {
	cfg, snapshot, err := config.LoadToolDoctorFromEnv()
	if err != nil {
		return nil, err
	}
	log := logger.New(logger.Options{Service: cfg.Shared.ServiceName, Component: "bootstrap", Level: parseLogLevel(cfg.Shared.LogLevel), Writer: os.Stdout})

	grpcListener, err := net.Listen("tcp", cfg.Shared.GRPCAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on grpc addr %s: %w", cfg.Shared.GRPCAddr, err)
	}
	grpcServer := grpc.NewServer()
	runtimev1.RegisterToolRuntimeServiceServer(grpcServer, runtime.NewServer(runtime.NewChecker(cfg, snapshot), log))

	httpMux := http.NewServeMux()
	httpMux.Handle("/health", health.New(cfg.Shared.ServiceName).Handler())
	httpMux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("tool-doctor")) })
	httpServer := &http.Server{Addr: cfg.Shared.HTTPAddr, Handler: httpMux, ReadHeaderTimeout: 5 * time.Second}

	return &App{config: cfg, log: log, httpServer: httpServer, grpcServer: grpcServer, grpcListen: grpcListener}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 2)
	go func() {
		a.log.Info("starting tool doctor server", slog.String("http_addr", a.config.Shared.HTTPAddr))
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	go func() {
		a.log.Info("starting tool doctor grpc server", slog.String("grpc_addr", a.config.Shared.GRPCAddr))
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
	if runErr != nil {
		a.log.Error("tool doctor stopped with error", slog.String("error", runErr.Error()))
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
