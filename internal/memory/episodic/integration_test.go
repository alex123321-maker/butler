package episodic

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestEpisodicStoreIntegration(t *testing.T) {
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
	_, _ = store.Pool().Exec(ctx, `DELETE FROM memory_episodes WHERE scope_type = 'session' AND scope_id = 'integration:episode'`)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM memory_episodes WHERE scope_type = 'session' AND scope_id = 'integration:episode'`)
	}()

	episodeStore := NewStore(store.Pool())
	if _, err := episodeStore.Save(ctx, Episode{ScopeType: "session", ScopeID: "integration:episode", Summary: "Resolved Redis outage", Content: "Restarted Redis and verified leases", SourceType: "run", SourceID: "run-1", Status: "active", TagsJSON: `["doctor","redis"]`, Embedding: []float32{0.1, 0.2, 0.3}}); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if _, err := episodeStore.Save(ctx, Episode{ScopeType: "session", ScopeID: "integration:episode", Summary: "Telegram config fix", Content: "Updated allowed chat ids", SourceType: "run", SourceID: "run-2", Status: "active", TagsJSON: `["telegram"]`, Embedding: []float32{0.9, 0.8, 0.7}}); err != nil {
		t.Fatalf("Save second returned error: %v", err)
	}
	results, err := episodeStore.Search(ctx, "session", "integration:episode", []float32{0.1, 0.2, 0.29}, 2)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) == 0 || results[0].Summary != "Resolved Redis outage" {
		t.Fatalf("unexpected search results: %+v", results)
	}
}
