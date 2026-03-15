package episodic

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/butler/butler/internal/memory/provenance"
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
	var first Episode
	defer func() {
		if first.ID != 0 {
			_, _ = store.Pool().Exec(context.Background(), `DELETE FROM memory_links WHERE source_memory_type = 'episodic' AND source_memory_id = $1`, first.ID)
		}
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM memory_episodes WHERE scope_type = 'session' AND scope_id = 'integration:episode'`)
	}()

	episodeStore := NewStore(store.Pool())
	linkStore := provenance.NewStore(store.Pool())
	first, err = episodeStore.Save(ctx, Episode{ScopeType: "session", ScopeID: "integration:episode", Summary: "Resolved Redis outage", Content: "Restarted Redis and verified leases", SourceType: "run", SourceID: "run-1", Status: StatusActive, TagsJSON: `["doctor","redis"]`, Embedding: testEmbedding(0.1)})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if _, err := linkStore.SaveLink(ctx, provenance.Link{SourceMemoryType: MemoryType, SourceMemoryID: first.ID, LinkType: "source", TargetType: "run", TargetID: "run-1", MetadataJSON: `{"safe_ref":"tool_call:1"}`}); err != nil {
		t.Fatalf("SaveLink returned error: %v", err)
	}
	if _, err := episodeStore.Save(ctx, Episode{ScopeType: "session", ScopeID: "integration:episode", Summary: "Telegram config fix", Content: "Updated allowed chat ids", SourceType: "run", SourceID: "run-2", Status: StatusInactive, TagsJSON: `["telegram"]`, Embedding: testEmbedding(0.9)}); err != nil {
		t.Fatalf("Save second returned error: %v", err)
	}
	entries, err := episodeStore.GetByScope(ctx, "session", "integration:episode")
	if err != nil {
		t.Fatalf("GetByScope returned error: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != StatusActive {
		t.Fatalf("unexpected scope entries: %+v", entries)
	}
	if entries[0].ProvenanceJSON == "" {
		t.Fatalf("expected provenance json on stored episode, got %+v", entries[0])
	}
	links, err := linkStore.ListBySource(ctx, MemoryType, first.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(links) != 1 || links[0].TargetID != "run-1" {
		t.Fatalf("unexpected links %+v", links)
	}
	results, err := episodeStore.Search(ctx, "session", "integration:episode", testEmbedding(0.1), 2)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) == 0 || results[0].Summary != "Resolved Redis outage" {
		t.Fatalf("unexpected search results: %+v", results)
	}
	var indexCount int
	if err := store.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'memory_episodes' AND indexname = 'idx_memory_episodes_embedding_cosine'`).Scan(&indexCount); err != nil {
		t.Fatalf("query pg_indexes returned error: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("expected similarity index to exist, got %d", indexCount)
	}
}

func testEmbedding(seed float32) []float32 {
	vector := make([]float32, embeddings.VectorDimensions)
	for i := range vector {
		vector[i] = seed
	}
	return vector
}
