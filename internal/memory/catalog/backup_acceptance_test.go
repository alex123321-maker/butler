package catalog_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/ingestion"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Invoked three times around actual pg_dump/pg_restore by the lab acceptance
// procedure. Ordinary suites skip it; it never touches a non-test database.
func TestActualPostgresDumpRestoreDeletionIntegration(t *testing.T) {
	phase := os.Getenv("BUTLER_TEST_MEMORY_BACKUP_PHASE")
	if phase == "" {
		t.Skip("actual external pg_dump/restore procedure required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MEMORY_BACKUP_POSTGRES_URL"))
	if err != nil || cfg == nil || (cfg.ConnConfig.Database != "butler_memory_backup_test" && cfg.ConnConfig.Database != "butler_memory_restore_test" && cfg.ConnConfig.Database != "butler_memory_restore_retry_test") {
		t.Fatal("dedicated backup/restore test DB required")
	}
	cfg.MaxConns = 2
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	manifest := catalog.NewFileManifest(os.Getenv("BUTLER_TEST_MEMORY_BACKUP_MANIFEST"))
	c := catalog.New(pool, manifest)
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "backup-proof-owner", Channel: "web", DeliveryID: "backup-proof"})
	if phase == "seed" {
		if _, err = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
			t.Fatal(err)
		}
		_, file, _, _ := runtime.Caller(0)
		paths, err := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations", "*.up.sql"))
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(paths)
		for _, path := range paths {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, string(raw)); err != nil {
				t.Fatalf("%s: %v", filepath.Base(path), err)
			}
		}
		if err = manifest.Initialize(); err != nil {
			t.Fatal(err)
		}
		a, err := (&ingestion.Store{Catalog: c}).Ingest(ctx, ingestion.Upload{ExternalKey: "backup-original", ScopeType: "user", ScopeID: "backup-proof-owner", Format: "markdown", Path: "исходник.md", Raw: []byte("# Частные данные\nПроверяемое удаление исходника Service.Restart\n")})
		if err != nil || a.Status != "ready" {
			t.Fatal(a, err)
		}
		child, err := c.Put(ctx, catalog.PutSource{ExternalKey: "backup-child", Kind: "model_candidate", ScopeType: "user", ScopeID: "backup-proof-owner", Body: "Неавторитетный пересказ", Parents: []catalog.SourceRef{{ID: a.Source.ID, Revision: a.Source.Revision, AccessRevision: a.Source.AccessRevision, Hash: a.Source.Hash}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.Propose(ctx, catalog.ClaimRequest{SourceID: child.ID, SourceRevision: child.Revision, Key: "claim", Value: []byte(`"candidate"`), Confidence: 1}); err != nil {
			t.Fatal(err)
		}
		return
	}
	var id string
	var revision int64
	if err = pool.QueryRow(ctx, `SELECT source_id,head_revision FROM memory_sources WHERE external_key='backup-original'`).Scan(&id, &revision); err != nil {
		t.Fatal(err)
	}
	switch phase {
	case "delete":
		if _, err = c.Delete(ctx, id, "delete", revision); err != nil {
			t.Fatal(err)
		}
	case "restore":
		if err = c.CheckRestore(ctx); !errors.Is(err, catalog.ErrRestore) {
			t.Fatal("restored old PG accepted before replay", err)
		}
		if _, _, err = (&ingestion.Store{Catalog: c}).Original(ctx, id, revision); !errors.Is(err, catalog.ErrRestore) {
			t.Fatal("restored private original served", err)
		}
		if err = c.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		if err = c.CheckRestore(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err = c.Get(ctx, id); !errors.Is(err, catalog.ErrDenied) {
			t.Fatal("restored deleted source served", err)
		}
		var body string
		if err = pool.QueryRow(ctx, `SELECT string_agg(body,'') FROM memory_source_revisions`).Scan(&body); err != nil || strings.TrimSpace(body) != "" {
			t.Fatal("restored source/child text remained", err)
		}
		var remaining int
		if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_source_assets WHERE original_bytes IS NOT NULL)+(SELECT count(*) FROM memory_chunks)+(SELECT count(*) FROM memory_claims WHERE summary<>'' OR value_json<> '{}'::jsonb)`).Scan(&remaining); err != nil || remaining != 0 {
			t.Fatal("restored original/derived data remained", remaining, err)
		}
		if _, err = c.Put(ctx, catalog.PutSource{ExternalKey: "backup-original", Kind: "document", ScopeType: "user", ScopeID: "backup-proof-owner", Body: "reindex"}); !errors.Is(err, catalog.ErrDeleted) {
			t.Fatal("reindex resurrected restored source", err)
		}
	default:
		t.Fatal("unknown backup acceptance phase")
	}
}
