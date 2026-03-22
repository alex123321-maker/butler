package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	restartsvc "github.com/butler/butler/apps/restart-helper/internal/restart"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
)

type App struct {
	config     config.RestartHelperConfig
	log        *slog.Logger
	httpServer *http.Server
}

func New(context.Context) (*App, error) {
	cfg, _, err := config.LoadRestartHelperFromEnv()
	if err != nil {
		return nil, err
	}
	log := logger.New(logger.Options{Service: cfg.Shared.ServiceName, Component: "bootstrap", Level: parseLogLevel(cfg.Shared.LogLevel), Writer: os.Stdout})

	dockerClient, err := restartsvc.NewDockerClient(cfg.DockerHost, time.Duration(cfg.RestartTimeoutSeconds+5)*time.Second)
	if err != nil {
		return nil, err
	}

	service, err := restartsvc.NewService(restartsvc.Config{
		Project:         cfg.DockerProject,
		AllowedServices: cfg.AllowedServices,
		SelfService:     cfg.SelfService,
		Delay:           time.Duration(cfg.RestartDelaySeconds) * time.Second,
		RestartTimeout:  time.Duration(cfg.RestartTimeoutSeconds) * time.Second,
	}, dockerClient, logger.WithComponent(log, "restart-service"))
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/health", health.New(cfg.Shared.ServiceName, health.FuncChecker{
		CheckName: "docker",
		Fn:        dockerClient.Ping,
	}).Handler())
	mux.Handle("/v1/restarts", restartsvc.NewHandler(service))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"service":"restart-helper","status":"ready"}`))
	})

	httpServer := &http.Server{
		Addr:              cfg.Shared.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &App{config: cfg, log: log, httpServer: httpServer}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		a.log.Info("starting restart helper server", slog.String("http_addr", a.config.Shared.HTTPAddr))
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
	if runErr != nil {
		a.log.Error("restart helper stopped with error", slog.String("error", runErr.Error()))
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
