package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestSettingsFlowIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	t.Setenv("BUTLER_POSTGRES_URL", dsn)
	t.Setenv("BUTLER_REDIS_URL", "redis://localhost:6379/0")
	t.Setenv(config.SettingsEncryptionKeyEnv, "integration-settings-key")
	t.Setenv("BUTLER_OPENAI_MODEL", "env-model")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1, MaxConnLifetime: time.Minute}, nil)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()
	if err := store.RunMigrations(ctx, filepath.Clean(filepath.Join("..", "..", "..", "..", "migrations"))); err != nil {
		t.Fatalf("RunMigrations returned error: %v", err)
	}

	keys := []string{"BUTLER_LOG_LEVEL", "BUTLER_OPENAI_API_KEY", "BUTLER_OPENAI_MODEL", "BUTLER_HTTP_ADDR"}
	for _, key := range keys {
		_, _ = store.Pool().Exec(ctx, `DELETE FROM system_settings WHERE key = $1`, key)
	}
	defer func() {
		for _, key := range keys {
			_, _ = store.Pool().Exec(context.Background(), `DELETE FROM system_settings WHERE key = $1`, key)
		}
	}()

	hot := config.NewHotConfig(config.Snapshot{})
	settingsStore := config.NewPostgresSettingsStore(store.Pool())
	service := config.NewSettingsService(settingsStore, hot)
	server := NewSettingsServer(service)

	put(t, server.HandleItem(), "/api/v1/settings/BUTLER_LOG_LEVEL", `{"value":"debug"}`, http.StatusOK)
	if value, ok := hot.Get("BUTLER_LOG_LEVEL"); !ok || value != "debug" {
		t.Fatalf("expected hot config update, got %q %v", value, ok)
	}

	put(t, server.HandleItem(), "/api/v1/settings/BUTLER_OPENAI_API_KEY", `{"value":"sk-test-secret-1234"}`, http.StatusOK)
	var rawSecret string
	if err := store.Pool().QueryRow(ctx, `SELECT value FROM system_settings WHERE key = $1`, "BUTLER_OPENAI_API_KEY").Scan(&rawSecret); err != nil {
		t.Fatalf("query raw secret: %v", err)
	}
	if rawSecret == "sk-test-secret-1234" {
		t.Fatal("expected encrypted secret in storage")
	}

	put(t, server.HandleItem(), "/api/v1/settings/BUTLER_OPENAI_MODEL", `{"value":"db-model"}`, http.StatusOK)
	settings := listSettings(t, server.HandleList())
	if item := findSetting(t, settings, "BUTLER_OPENAI_MODEL"); item.Source != "env" || item.Value != "env-model" {
		t.Fatalf("expected env override to win, got %+v", item)
	}
	if item := findSetting(t, settings, "BUTLER_OPENAI_API_KEY"); item.Value != "...1234" {
		t.Fatalf("expected masked secret value, got %+v", item)
	}

	cold := put(t, server.HandleItem(), "/api/v1/settings/BUTLER_HTTP_ADDR", `{"value":":9099"}`, http.StatusOK)
	if !cold.RequiresRestart {
		t.Fatalf("expected cold setting to require restart, got %+v", cold)
	}

	deleteSetting := del(t, server.HandleItem(), "/api/v1/settings/BUTLER_LOG_LEVEL", http.StatusOK)
	if deleteSetting.Source != "default" || deleteSetting.Value != "info" {
		t.Fatalf("expected delete to revert to default, got %+v", deleteSetting)
	}
}

func put(t *testing.T, handler http.Handler, path, body string, wantStatus int) settingDTO {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != wantStatus {
		t.Fatalf("expected status %d, got %d with body %s", wantStatus, rr.Code, rr.Body.String())
	}
	var payload struct {
		Setting settingDTO `json:"setting"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode put response: %v", err)
	}
	return payload.Setting
}

func del(t *testing.T, handler http.Handler, path string, wantStatus int) settingDTO {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != wantStatus {
		t.Fatalf("expected status %d, got %d with body %s", wantStatus, rr.Code, rr.Body.String())
	}
	var payload struct {
		Setting settingDTO `json:"setting"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	return payload.Setting
}

func listSettings(t *testing.T, handler http.Handler) []settingDTO {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Components []struct {
			Settings []settingDTO `json:"settings"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	items := make([]settingDTO, 0)
	for _, component := range payload.Components {
		items = append(items, component.Settings...)
	}
	return items
}

func findSetting(t *testing.T, settings []settingDTO, key string) settingDTO {
	t.Helper()
	for _, setting := range settings {
		if setting.Key == key {
			return setting
		}
	}
	t.Fatalf("setting %s not found", key)
	return settingDTO{}
}
