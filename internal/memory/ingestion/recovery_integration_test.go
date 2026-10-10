package ingestion

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestParsedAssetRestartAndSafeDowngradeIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "recovery"})
	_, file, _, _ := runtime.Caller(0)
	migration := func(suffix string) string {
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations", "049_structured_source_assets."+suffix+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	// A pristine schema can be downgraded and upgraded without dropping history.
	if _, err := pool.Exec(ctx, migration("down")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, migration("up")); err != nil {
		t.Fatal(err)
	}
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, manifest)
	s := &Store{Catalog: c}
	u := Upload{ExternalKey: "restart-source", ScopeType: "user", ScopeID: "owner", Format: "markdown", Path: "память.md", Raw: []byte("# Проверка\nСохранённый исходник\n")}
	a, err := s.Ingest(ctx, u)
	if err != nil || a.Status != "ready" {
		t.Fatal(a, err)
	}
	// Durable state immediately after saving the original but before publishing
	// fragments. A new Store instance must repair this stage on exact replay.
	if _, err = pool.Exec(ctx, `DELETE FROM memory_object_sources WHERE memory_type='chunk'; DELETE FROM memory_chunks; UPDATE memory_source_assets SET status='parsed'`); err != nil {
		t.Fatal(err)
	}
	restarted := &Store{Catalog: catalog.New(pool, manifest)}
	repaired, err := restarted.Ingest(ctx, u)
	if err != nil || repaired.Status != "ready" || repaired.Source.ID != a.Source.ID || repaired.Source.Revision != a.Source.Revision || repaired.Fragments.Inserted != 1 {
		t.Fatal(repaired, err)
	}
	var state string
	if err = pool.QueryRow(ctx, `SELECT status FROM memory_source_assets`).Scan(&state); err != nil || state != "ready" {
		t.Fatal(state, err)
	}
	// A standalone attempted downgrade must reject before any durable table is
	// dropped. The original and repaired fragments remain readable afterwards.
	if _, err = pool.Exec(ctx, migration("down")); err == nil {
		t.Fatal("durable originals discarded by downgrade")
	}
	raw, path, err := restarted.Original(ctx, a.Source.ID, a.Source.Revision)
	if err != nil || path != u.Path || string(raw) != string(u.Raw) {
		t.Fatal(path, err)
	}
	var n int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM memory_chunks`).Scan(&n); err != nil || n != 1 {
		t.Fatal(n, err)
	}
}
