package profile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/provenance"
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
	var first, second Entry
	defer func() {
		if first.ID != 0 || second.ID != 0 {
			_, _ = store.Pool().Exec(context.Background(), `DELETE FROM memory_links WHERE source_memory_type = 'profile' AND source_memory_id = ANY($1)`, []int64{first.ID, second.ID})
		}
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM memory_profile WHERE scope_type = 'session' AND scope_id = 'integration:profile'`)
	}()

	profileStore := NewStore(store.Pool())
	linkStore := provenance.NewStore(store.Pool())
	first, err = profileStore.Save(ctx, Entry{ScopeType: "session", ScopeID: "integration:profile", Key: "language", ValueJSON: `{"value":"ru"}`, Summary: "User prefers Russian", SourceType: "run", SourceID: "run-1", Status: StatusActive})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if _, err := linkStore.SaveLink(ctx, provenance.Link{SourceMemoryType: MemoryType, SourceMemoryID: first.ID, LinkType: "source", TargetType: "run", TargetID: "run-1", MetadataJSON: `{"safe_ref":"transcript:user"}`}); err != nil {
		t.Fatalf("SaveLink returned error: %v", err)
	}
	second, err = profileStore.Supersede(ctx, first.ID, Entry{Summary: "User prefers English", ValueJSON: `{"value":"en"}`, SourceType: "run", SourceID: "run-2"})
	if err != nil {
		t.Fatalf("Supersede returned error: %v", err)
	}
	if _, err := linkStore.SaveLink(ctx, provenance.Link{SourceMemoryType: MemoryType, SourceMemoryID: second.ID, LinkType: "source", TargetType: "run", TargetID: "run-2", MetadataJSON: `{"safe_ref":"transcript:user"}`}); err != nil {
		t.Fatalf("SaveLink second returned error: %v", err)
	}
	if second.SupersedesID == nil || *second.SupersedesID != first.ID {
		t.Fatalf("unexpected supersedes pointer: %+v", second)
	}
	current, err := profileStore.Get(ctx, "session", "integration:profile", "language")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if current.ID != second.ID {
		t.Fatalf("expected active entry %d, got %d", second.ID, current.ID)
	}
	entries, err := profileStore.GetByScope(ctx, "session", "integration:profile")
	if err != nil {
		t.Fatalf("GetByScope returned error: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != "language" {
		t.Fatalf("unexpected profile entries: %+v", entries)
	}
	if entries[0].ID != second.ID || entries[0].Status != StatusActive {
		t.Fatalf("expected active superseding entry, got %+v", entries[0])
	}
	if first.ProvenanceJSON == "" || second.ProvenanceJSON == "" {
		t.Fatalf("expected provenance json to be stored, got first=%q second=%q", first.ProvenanceJSON, second.ProvenanceJSON)
	}
	links, err := linkStore.ListBySource(ctx, MemoryType, second.ID)
	if err != nil {
		t.Fatalf("ListBySource returned error: %v", err)
	}
	if len(links) != 1 || links[0].TargetID != "run-2" {
		t.Fatalf("unexpected provenance links %+v", links)
	}
	history, err := profileStore.GetHistory(ctx, "session", "integration:profile", "language")
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(history) != 2 || history[0].Status != StatusSuperseded || history[1].Status != StatusActive {
		t.Fatalf("expected profile version history, got %+v", history)
	}
}
