package chunks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/embeddings"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestChunkStoreIntegration(t *testing.T) {
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
	_, _ = store.Pool().Exec(ctx, `DELETE FROM memory_chunks WHERE scope_type = 'session' AND scope_id = 'integration:chunk'`)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM memory_chunks WHERE scope_type = 'session' AND scope_id = 'integration:chunk'`)
	}()
	chunkStore := NewStore(store.Pool())
	saved, err := chunkStore.Save(ctx, Chunk{ScopeType: "session", ScopeID: "integration:chunk", Title: "Redis runbook", Content: "Restart Redis and verify leases", Summary: "Redis recovery runbook", SourceType: "doctor_report", SourceID: "1", TagsJSON: `["doctor","runbook"]`, Embedding: testEmbedding(0.2)})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if saved.ID == 0 {
		t.Fatal("expected saved chunk id")
	}
	matches, err := chunkStore.FindByTitle(ctx, "session", "integration:chunk", "Redis runbook", 2)
	if err != nil {
		t.Fatalf("FindByTitle returned error: %v", err)
	}
	if len(matches) != 1 || matches[0].Title != "Redis runbook" {
		t.Fatalf("unexpected title matches %+v", matches)
	}
	results, err := chunkStore.Search(ctx, "session", "integration:chunk", testEmbedding(0.2), 2)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "Redis runbook" {
		t.Fatalf("unexpected chunk search results %+v", results)
	}
}

func testEmbedding(seed float32) []float32 {
	vector := make([]float32, embeddings.VectorDimensions())
	for i := range vector {
		vector[i] = seed
	}
	return vector
}
