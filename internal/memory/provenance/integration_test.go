package provenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestMemoryLinkStoreIntegration(t *testing.T) {
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
	_, _ = store.Pool().Exec(ctx, `DELETE FROM memory_links WHERE source_memory_type = 'profile' AND source_memory_id = 42`)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM memory_links WHERE source_memory_type = 'profile' AND source_memory_id = 42`)
	}()

	linkStore := NewStore(store.Pool())
	stored, err := linkStore.SaveLink(ctx, Link{SourceMemoryType: "profile", SourceMemoryID: 42, LinkType: "source", TargetType: "run", TargetID: "run-1", MetadataJSON: `{"safe_ref":"message:1"}`})
	if err != nil {
		t.Fatalf("SaveLink returned error: %v", err)
	}
	if stored.ID == 0 {
		t.Fatal("expected persisted link id")
	}
	links, err := linkStore.ListBySource(ctx, "profile", 42)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(links) != 1 || links[0].TargetID != "run-1" {
		t.Fatalf("unexpected links %+v", links)
	}
}
