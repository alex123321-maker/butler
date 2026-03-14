package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/health"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/metrics"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	redisstore "github.com/butler/butler/internal/storage/redis"
)

type App struct {
	config     config.OrchestratorConfig
	log        *slog.Logger
	metrics    *metrics.Registry
	health     *health.Service
	postgres   *postgresstore.Store
	redis      *redisstore.Store
	httpServer *http.Server
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

	mux := http.NewServeMux()
	mux.Handle("/health", h.Handler())
	mux.Handle("/metrics", m.Handler())
	mux.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = m.IncrCounter(metrics.MetricRequestsTotal, map[string]string{
			"operation": "submit_event",
			"service":   cfg.Shared.ServiceName,
			"status":    "not_implemented",
		})
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "submit event is not implemented yet"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": cfg.Shared.ServiceName,
			"status":  "starting",
		})
	})

	server := &http.Server{
		Addr:              cfg.Shared.HTTPAddr,
		Handler:           instrumentHTTP(cfg.Shared.ServiceName, m, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &App{
		config:     cfg,
		log:        log,
		metrics:    m,
		health:     h,
		postgres:   postgres,
		redis:      redis,
		httpServer: server,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		a.log.Info("starting orchestrator http server", slog.String("http_addr", a.config.Shared.HTTPAddr))
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

	if err := a.httpServer.Shutdown(ctx); err != nil {
		a.log.Error("http shutdown failed", slog.String("error", err.Error()))
		if runErr == nil {
			runErr = err
		}
	}

	if err := a.redis.Close(); err != nil && runErr == nil {
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
