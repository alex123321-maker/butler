package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/logger"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	log := logger.New(logger.Options{Service: "local-models", Component: "main", Writer: os.Stdout, Level: slog.LevelInfo})
	if err := run(log); err != nil {
		log.Error("local model service stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
func run(log *slog.Logger) error {
	token, err := readToken(os.Getenv("BUTLER_LOCAL_MODELS_TOKEN_FILE"))
	if err != nil {
		return err
	}
	data, err := os.ReadFile(env("BUTLER_LOCAL_MODELS_CATALOG", "/etc/butler/models.json"))
	if err != nil {
		return fmt.Errorf("read model catalog")
	}
	var catalog pb.Catalog
	if (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(data, &catalog) != nil {
		return fmt.Errorf("invalid model catalog")
	}
	budget, err := number("BUTLER_LOCAL_MODELS_BUDGET_MIB", 6144, 1, 65536)
	if err != nil {
		return err
	}
	reserve, err := number("BUTLER_LOCAL_MODELS_RESERVE_MIB", 512, 1, 65536)
	if err != nil {
		return err
	}
	idle, err := number("BUTLER_LOCAL_MODELS_IDLE_SECONDS", 30, 1, 3600)
	if err != nil {
		return err
	}
	queue, err := number("BUTLER_LOCAL_MODELS_QUEUE_SIZE", 16, 1, 256)
	if err != nil {
		return err
	}
	device, err := number("BUTLER_LOCAL_MODELS_GPU_INDEX", 0, 0, 32)
	if err != nil {
		return err
	}
	backend, err := localmodels.NewProcess(localmodels.ProcessConfig{Python: env("BUTLER_LOCAL_MODELS_PYTHON", "/data/venv/bin/python"), Script: env("BUTLER_LOCAL_MODELS_WORKER", "/app/model-worker.py"), ModelRoot: env("BUTLER_LOCAL_MODELS_MODEL_ROOT", "/data/models"), GPUIndex: uint32(device)})
	if err != nil {
		return err
	}
	cpuBackend, err := localmodels.NewProcess(localmodels.ProcessConfig{Python: env("BUTLER_LOCAL_MODELS_PYTHON", "/data/venv/bin/python"), Script: env("BUTLER_LOCAL_MODELS_WORKER", "/app/model-worker.py"), ModelRoot: env("BUTLER_LOCAL_MODELS_MODEL_ROOT", "/data/models"), GPUIndex: uint32(device)})
	if err != nil {
		return err
	}
	ramBudget, err := number("BUTLER_LOCAL_MODELS_RAM_BUDGET_MIB", 6144, 512, 65536)
	if err != nil {
		return err
	}
	ramReserve, err := number("BUTLER_LOCAL_MODELS_RAM_RESERVE_MIB", 512, 1, 65536)
	if err != nil {
		return err
	}
	manager, err := localmodels.NewRouter(localmodels.Config{BudgetMiB: uint32(budget), ReserveMiB: uint32(reserve), RAMBudgetMiB: uint32(ramBudget), RAMReserveMiB: uint32(ramReserve), RAMTelemetry: localmodels.ProcMemory{}, IdleTimeout: time.Duration(idle) * time.Second, QueueSize: queue}, catalog.Models, backend, cpuBackend, localmodels.NvidiaSMI{Device: uint32(device)})
	if err != nil {
		return err
	}
	defer manager.Close()
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(localmodels.AuthInterceptor(token)), grpc.MaxRecvMsgSize(5<<20), grpc.MaxSendMsgSize(8<<20))
	pb.RegisterLocalModelsServiceServer(grpcServer, &localmodels.Server{Manager: manager})
	listener, err := net.Listen("tcp", env("BUTLER_LOCAL_MODELS_GRPC_ADDR", ":9090"))
	if err != nil {
		return fmt.Errorf("listen on gRPC address")
	}
	defer listener.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		s := manager.Status(ctx)
		if !s.BackendHealthy {
			http.Error(w, "backend unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		s := manager.Status(ctx)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprintf(w, "butler_local_models_completed_total %d\nbutler_local_models_failed_total %d\nbutler_local_models_queue %d\nbutler_local_models_cpu_fallbacks_total %d\nbutler_local_models_oom_retries_total %d\nbutler_local_models_device_peak_mib %d\nbutler_local_models_ram_peak_mib %d\n", s.Completed+s.CpuLane.Completed, s.Failed+s.CpuLane.Failed, s.Queued+s.CpuLane.Queued, s.CpuFallbacks, s.OomRetries, s.ObservedPeakMib, max(s.ObservedPeakRamMib, s.CpuLane.ObservedPeakRamMib))
	})
	httpServer := &http.Server{Addr: env("BUTLER_LOCAL_MODELS_HTTP_ADDR", ":8080"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 2)
	go func() { errCh <- grpcServer.Serve(listener) }()
	go func() { errCh <- httpServer.ListenAndServe() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Info("local model service started", slog.Int("profiles", len(catalog.Models)), slog.Int("budget_mib", budget), slog.Int("reserve_mib", reserve))
	select {
	case <-ctx.Done():
	case err = <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	// Cancel running inference before stopping RPCs; callers receive an explicit retryable error.
	grpcServer.Stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	return err
}
func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func number(key string, fallback, low, high int) (int, error) {
	v, err := strconv.Atoi(env(key, strconv.Itoa(fallback)))
	if err != nil || v < low || v > high {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return v, nil
}
func readToken(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("private token file required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file")
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || len(token) > 256 {
		return "", fmt.Errorf("token must contain 32 to 256 characters")
	}
	return token, nil
}
