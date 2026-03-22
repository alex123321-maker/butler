package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/metrics"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
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
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func configSummary(snapshot config.Snapshot) string {
	sourceCounts := map[string]int{}
	for _, key := range snapshot.ListKeys() {
		sourceCounts[key.Source]++
	}
	parts := []string{
		"keys=" + strconv.Itoa(len(snapshot.ListKeys())),
		"env=" + strconv.Itoa(sourceCounts[config.ConfigSourceEnv]),
		"db=" + strconv.Itoa(sourceCounts[config.ConfigSourceDB]),
		"default=" + strconv.Itoa(sourceCounts[config.ConfigSourceDefault]),
	}
	return strings.Join(parts, " ")
}
