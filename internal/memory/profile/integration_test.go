package profile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestProfileStoreIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1, MaxConnLifetime: time.Minute}, nil)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()
	if err := store.RunMigrations(ctx, filepath.Clean(filepath.Join("..", "..", "..", "migrations"))); err != nil {
		t.Fatalf("RunMigrations returned error: %v", err)
	}
	_, _ = store.Pool().Exec(ctx, `DELETE FROM memory_profile WHERE scope_type = 'session' AND scope_id = 'integration:profile'`)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM memory_profile WHERE scope_type = 'session' AND scope_id = 'integration:profile'`)
	}()

	profileStore := NewStore(store.Pool())
	if _, err := profileStore.Save(ctx, Entry{ScopeType: "session", ScopeID: "integration:profile", Key: "language", ValueJSON: `{"value":"ru"}`, Summary: "User prefers Russian", SourceType: "run", SourceID: "run-1", Status: "active"}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	entries, err := profileStore.GetByScope(ctx, "session", "integration:profile")
	if err != nil {
		t.Fatalf("GetByScope returned error: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != "language" {
		t.Fatalf("unexpected profile entries: %+v", entries)
	}
}
