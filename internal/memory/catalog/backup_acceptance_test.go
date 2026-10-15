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
		valid, err := (&ingestion.Store{Catalog: c}).Ingest(ctx, ingestion.Upload{ExternalKey: "backup-unrelated", ScopeType: "user", ScopeID: "backup-proof-owner", Format: "markdown", Path: "valid.md", Raw: []byte("# Independent\nUnrelated valid preference: Russian.\n")})
		if err != nil || valid.Status != "ready" {
			t.Fatal(valid, err)
		}
		claim, err := c.Propose(ctx, catalog.ClaimRequest{SourceID: valid.Source.ID, SourceRevision: valid.Source.Revision, Key: "language", Value: []byte(`"ru"`), Summary: "Unrelated valid preference", Confidence: 1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.Decide(ctx, catalog.Decision{ClaimID: claim.ID}); err != nil {
			t.Fatal(err)
		}
		// Explicit synthetic primary tool-cache fixtures. Actual Broker capture is
		// independently covered by the owning ordinary-read composition test.
		if _, err = pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel)VALUES('backup-cache','backup-proof-owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('backup-private-run','backup-cache','private','completed','mode_0','completed','fixture',clock_timestamp(),clock_timestamp()),('backup-valid-run','backup-cache','valid','completed','mode_0','completed','fixture',clock_timestamp(),clock_timestamp());INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,result)VALUES('backup-private-tool','backup-private-run','memory.open','{}','unknown','memory',clock_timestamp(),'"private cached tool payload79"'),('backup-valid-tool','backup-valid-run','memory.open','{}','completed','memory',clock_timestamp(),'"unrelated cached tool payload79"')`); err != nil {
			t.Fatal(err)
		}
		for _, pair := range []struct{ run, source string }{{"backup-private-run", a.Source.ID}, {"backup-valid-run", valid.Source.ID}} {
			if _, err = pool.Exec(ctx, `INSERT INTO memory_run_sources(run_id,source_id,source_revision,access_revision,content_hash) SELECT $1,s.source_id,s.head_revision,s.access_revision,r.content_hash FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.source_id=$2`, pair.run, pair.source); err != nil {
				t.Fatal(err)
			}
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
		if err = pool.QueryRow(ctx, `SELECT coalesce(string_agg(body,''),'') FROM memory_source_revisions JOIN memory_sources USING(source_id) WHERE status IN ('deleted','revoked')`).Scan(&body); err != nil || strings.TrimSpace(body) != "" {
			t.Fatal("restored source/child text remained", err)
		}
		var remaining int
		if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_source_assets a JOIN memory_sources s USING(source_id) WHERE s.status IN ('deleted','revoked') AND original_bytes IS NOT NULL)+(SELECT count(*) FROM memory_chunks c JOIN memory_sources s ON s.source_id=c.source_id WHERE s.status IN ('deleted','revoked'))+(SELECT count(*) FROM memory_claims c JOIN memory_sources s USING(source_id) WHERE s.status IN ('deleted','revoked') AND (summary<>'' OR value_json<> '{}'::jsonb))`).Scan(&remaining); err != nil || remaining != 0 {
			t.Fatal("restored original/derived data remained", remaining, err)
		}
		if _, err = c.Put(ctx, catalog.PutSource{ExternalKey: "backup-original", Kind: "document", ScopeType: "user", ScopeID: "backup-proof-owner", Body: "reindex"}); !errors.Is(err, catalog.ErrDeleted) {
			t.Fatal("reindex resurrected restored source", err)
		}
		var validID string
		if err = pool.QueryRow(ctx, `SELECT source_id FROM memory_sources WHERE external_key='backup-unrelated'`).Scan(&validID); err != nil {
			t.Fatal(err)
		}
		if valid, err := c.Get(ctx, validID); err != nil || !strings.Contains(valid.Body, "Unrelated valid preference") {
			t.Fatal("restore replay erased unrelated memory", err)
		}
		if raw, _, err := (&ingestion.Store{Catalog: c}).Original(ctx, validID, 1); err != nil || !strings.Contains(string(raw), "Unrelated valid preference") {
			t.Fatal("restore replay erased unrelated original", err)
		}
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM memory_claims WHERE source_id=$1 AND status='confirmed' AND value_json='"ru"'::jsonb`, validID).Scan(&remaining); err != nil || remaining != 1 {
			t.Fatal("restore replay changed unrelated accepted claim", remaining, err)
		}
	default:
		t.Fatal("unknown backup acceptance phase")
	}
	var private, unrelated, actualState string
	if err = pool.QueryRow(ctx, `SELECT (SELECT result::text FROM tool_calls WHERE tool_call_id='backup-private-tool'),(SELECT result::text FROM tool_calls WHERE tool_call_id='backup-valid-tool'),(SELECT status FROM tool_calls WHERE tool_call_id='backup-private-tool')`).Scan(&private, &unrelated, &actualState); err != nil || strings.Contains(private, "private cached tool payload79") || !strings.Contains(private, "privacy_redacted") || !strings.Contains(unrelated, "unrelated cached tool payload79") || actualState != "unknown" {
		t.Fatal("tool cache privacy/retained unknown/unrelated cache failed", err)
	}
	if err = c.Reconcile(ctx); err != nil {
		t.Fatal("idempotent cache privacy reconciliation", err)
	}
}
