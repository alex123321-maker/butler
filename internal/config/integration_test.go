package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestPostgresSettingsStoreIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	t.Setenv(config.SettingsEncryptionKeyEnv, "integration-settings-key")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1, MaxConnLifetime: time.Minute}, nil)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()
	if err := store.RunMigrations(ctx, filepath.Clean(filepath.Join("..", "..", "migrations"))); err != nil {
		t.Fatalf("RunMigrations returned error: %v", err)
	}

	settingsStore := config.NewPostgresSettingsStore(store.Pool())
	keys := []string{"BUTLER_LOG_LEVEL", "BUTLER_OPENAI_API_KEY"}
	for _, key := range keys {
		_, _ = store.Pool().Exec(ctx, `DELETE FROM system_settings WHERE key = $1`, key)
	}
	defer func() {
		for _, key := range keys {
			_, _ = store.Pool().Exec(context.Background(), `DELETE FROM system_settings WHERE key = $1`, key)
		}
	}()

	plain, err := settingsStore.Set(ctx, config.Setting{Key: "BUTLER_LOG_LEVEL", Value: "debug", Component: "orchestrator", UpdatedBy: "integration-test"})
	if err != nil {
		t.Fatalf("Set plain setting returned error: %v", err)
	}
	if plain.Value != "debug" {
		t.Fatalf("expected plain value to round trip, got %q", plain.Value)
	}

	secret, err := settingsStore.Set(ctx, config.Setting{Key: "BUTLER_OPENAI_API_KEY", Value: "sk-integration-secret", Component: "orchestrator", IsSecret: true, UpdatedBy: "integration-test"})
	if err != nil {
		t.Fatalf("Set secret setting returned error: %v", err)
	}
	if secret.Value != "sk-integration-secret" {
		t.Fatalf("expected secret value to decrypt on readback, got %q", secret.Value)
	}

	var rawPlain string
	if err := store.Pool().QueryRow(ctx, `SELECT value FROM system_settings WHERE key = $1`, "BUTLER_LOG_LEVEL").Scan(&rawPlain); err != nil {
		t.Fatalf("query plain raw value: %v", err)
	}
	if rawPlain != "debug" {
		t.Fatalf("expected plaintext value in storage, got %q", rawPlain)
	}

	var rawSecret string
	if err := store.Pool().QueryRow(ctx, `SELECT value FROM system_settings WHERE key = $1`, "BUTLER_OPENAI_API_KEY").Scan(&rawSecret); err != nil {
		t.Fatalf("query secret raw value: %v", err)
	}
	if rawSecret == "sk-integration-secret" {
		t.Fatal("expected secret value to be encrypted in storage")
	}

	settings, err := settingsStore.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll returned error: %v", err)
	}
	if len(settings) < 2 {
		t.Fatalf("expected settings to include inserted records, got %+v", settings)
	}

	loaded, err := settingsStore.Get(ctx, "BUTLER_OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if loaded.Value != "sk-integration-secret" {
		t.Fatalf("expected decrypted secret from Get, got %q", loaded.Value)
	}

	if err := settingsStore.Delete(ctx, "BUTLER_LOG_LEVEL"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := settingsStore.Get(ctx, "BUTLER_LOG_LEVEL"); err != config.ErrSettingNotFound {
		t.Fatalf("expected ErrSettingNotFound after delete, got %v", err)
	}
}
