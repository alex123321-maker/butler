package redis

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	configpkg "github.com/butler/butler/internal/config"
)

func TestConfigFromShared(t *testing.T) {
	shared := configpkg.RedisConfig{URL: "redis://localhost:6379/0"}
	got := ConfigFromShared(shared)
	if got.URL != shared.URL {
		t.Fatalf("expected URL %q, got %q", shared.URL, got.URL)
	}
}

func TestOpenIntegration(t *testing.T) {
	redisURL := os.Getenv("BUTLER_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("BUTLER_TEST_REDIS_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	store, err := Open(ctx, Config{URL: redisURL}, log)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	}()

	if err := store.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}

	key := "t005-probe"
	if err := store.Client().Set(ctx, key, "ok", time.Minute).Err(); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	value, err := store.Client().Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if value != "ok" {
		t.Fatalf("expected redis value %q, got %q", "ok", value)
	}
	if err := store.Client().Del(ctx, key).Err(); err != nil {
		t.Fatalf("Del returned error: %v", err)
	}

	logs := buf.String()
	if !strings.Contains(logs, "redis connected") {
		t.Fatalf("expected connection log entry, got %q", logs)
	}
	if strings.Contains(logs, redisURL) {
		t.Fatal("expected Redis URL to be masked in logs")
	}
}
