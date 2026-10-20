package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Driven by mobile-retention-restore.sh around actual pg_dump/pg_restore.
func TestRetainedMobileActualBackupPhase(t *testing.T) {
	phase := os.Getenv("BUTLER_TEST_MOBILE_RETENTION_PHASE")
	if phase == "" {
		t.Skip("external isolated pg_dump/restore phase required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MOBILE_RETENTION_POSTGRES_URL"))
	if err != nil {
		t.Fatal(err)
	}
	switch cfg.ConnConfig.Database {
	case "mobile_retention_backup_test", "mobile_retention_expiry_test", "mobile_retention_forget_test":
	default:
		t.Fatal("dedicated retention test DB required")
	}
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "mobile-owner", Channel: "web", DeliveryID: "restore-proof"})
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	manifest := catalog.NewFileManifest(os.Getenv("BUTLER_TEST_MOBILE_RETENTION_MANIFEST"))
	s := &Store{Pool: pool, Catalog: catalog.New(pool, manifest)}
	if phase == "seed" {
		if _, err = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public`); err != nil {
			t.Fatal(err)
		}
		_, file, _, _ := runtime.Caller(0)
		paths, _ := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "*.up.sql"))
		sort.Strings(paths)
		for _, p := range paths {
			b, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = pool.Exec(ctx, string(b)); e != nil {
				t.Fatal(filepath.Base(p), e)
			}
		}
		if err = manifest.Initialize(); err != nil {
			t.Fatal(err)
		}
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if e = ConfigureAgentAccessTx(ctx, tx, "mobile-owner", true); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		code, e := s.CreateCode(ctx)
		if e != nil {
			t.Fatal(e)
		}
		p, e := s.Exchange(ctx, PairRequest{Code: code["pairing_code"].(string), Device: "restore_mobile_device", Name: "synthetic", SDK: 36, Version: "test"})
		if e != nil {
			t.Fatal(e)
		}
		b := testBatch()
		b.Events[0].Payload = &Payload{Text: strptr("Exact persistent notification evidence")}
		if _, e = s.Accept(ctx, p.Device, p.Token, b); e != nil {
			t.Fatal(e)
		}
		var raw catalog.SourceRef
		var message string
		if e = pool.QueryRow(ctx, `SELECT s.source_id,s.head_revision,s.access_revision,v.content_hash,m.message_id FROM mobile_messages m JOIN memory_sources s USING(source_id) JOIN memory_source_revisions v ON v.source_id=s.source_id AND v.revision=s.head_revision LIMIT 1`).Scan(&raw.ID, &raw.Revision, &raw.AccessRevision, &raw.Hash, &message); e != nil {
			t.Fatal(e)
		}
		loc, _ := json.Marshal(map[string]any{"adapter": "android_message_note_v1", "message_id": message, "original_source": raw})
		tx, e = pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		note, e := s.Catalog.CreateDerivedTx(ctx, tx, []catalog.SourceRef{raw}, "memory_read", "Retained restore fixture", "Persistent selected observation", loc)
		if e != nil {
			t.Fatal(e)
		}
		ref := catalog.SourceRef{ID: note.ID, Revision: note.Revision, AccessRevision: note.AccessRevision, Hash: note.Hash}
		// The actual dump deliberately predates conversion of this older note.
		// Expiry replay must reconstruct evidence before erasing its raw parent.
		if _, e = s.Catalog.PublishFragmentsTx(ctx, tx, ref, []catalog.Fragment{{Key: "note", Text: note.Body, Locator: loc}}); e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec(ctx, `UPDATE mobile_messages SET state='remembered',result_source_id=$1 WHERE message_id=$2`, note.ID, message); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, `UPDATE mobile_events SET received_at=clock_timestamp()-interval '361 days'`); e != nil {
			t.Fatal(e)
		}
		return
	}
	var original, note string
	var rev int64
	if err = pool.QueryRow(ctx, `SELECT m.source_id,m.result_source_id,s.head_revision FROM mobile_messages m JOIN memory_sources s USING(source_id) WHERE m.state='remembered' LIMIT 1`).Scan(&original, &note, &rev); err != nil {
		t.Fatal(err)
	}
	switch phase {
	case "expire":
		if err = s.Cleanup(ctx); err != nil {
			t.Fatal(err)
		}
	case "forget":
		if _, err = s.Catalog.Delete(ctx, original, "delete", rev); err != nil {
			t.Fatal(err)
		}
	case "restore-expiry", "restore-forget":
		if err = s.Catalog.CheckRestore(ctx); !errors.Is(err, catalog.ErrRestore) {
			t.Fatal("old backup accepted before replay", err)
		}
		if err = s.Catalog.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("invalid phase")
	}
	if _, err = s.Catalog.Get(ctx, original); err == nil {
		t.Fatal("raw archive resurrected")
	}
	_, err = s.Catalog.Get(ctx, note)
	shouldRemain := phase == "expire" || phase == "restore-expiry"
	if (err == nil) != shouldRemain {
		t.Fatal("retained note lifecycle", phase, err)
	}
	var n int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM memory_retained_mobile_evidence WHERE source_id=$1`, note).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if (n == 1) != shouldRemain {
		t.Fatal("evidence lifecycle", phase, n)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM memory_chunks`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if (n == 1) != shouldRemain {
		t.Fatal("fragment lifecycle", phase, n)
	}
	t.Log("actual backup phase passed:", phase)
}
