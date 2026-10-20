package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

func retainedGuardFixture(t *testing.T, retain bool) (*Store, context.Context, catalog.Source, catalog.Source) {
	t.Helper()
	s, ctx, id, token := fixture(t)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = ConfigureAgentAccessTx(ctx, tx, "mobile-owner", true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	b := testBatch()
	b.Events[0].Payload = &Payload{Text: strptr("Exact selected source evidence")}
	if _, err = s.Accept(ctx, id, token, b); err != nil {
		t.Fatal(err)
	}
	var sid, message string
	if err = s.Pool.QueryRow(ctx, `SELECT source_id,message_id FROM mobile_messages LIMIT 1`).Scan(&sid, &message); err != nil {
		t.Fatal(err)
	}
	original, err := s.Catalog.Get(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	ref := catalog.SourceRef{ID: sid, Revision: original.Revision, AccessRevision: original.AccessRevision, Hash: original.Hash}
	loc, _ := json.Marshal(map[string]any{"adapter": "android_message_note_v1", "message_id": message, "original_source": ref})
	tx, err = s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	note, err := s.Catalog.CreateDerivedTx(ctx, tx, []catalog.SourceRef{ref}, "memory_read", "Note", "Candidate observation", loc)
	if err != nil {
		t.Fatal(err)
	}
	nref := catalog.SourceRef{ID: note.ID, Revision: note.Revision, AccessRevision: note.AccessRevision, Hash: note.Hash}
	if retain {
		if err = s.Catalog.RetainMobileNoteTx(ctx, tx, nref); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE mobile_messages SET state='remembered',result_source_id=$1 WHERE message_id=$2`, note.ID, message); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return s, ctx, original, note
}

func TestRetainedMobileCurrentOriginalGuardsIntegration(t *testing.T) {
	for _, change := range []string{"correction", "access", "revoke", "missing_function"} {
		t.Run(change, func(t *testing.T) {
			s, ctx, raw, note := retainedGuardFixture(t, true)
			tx, err := s.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			child, err := s.Catalog.CreateDerivedTx(ctx, tx, []catalog.SourceRef{{ID: note.ID, Revision: note.Revision, AccessRevision: note.AccessRevision, Hash: note.Hash}}, "memory_read", "Child", "Copied observation", json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "correction":
				_, err = s.Catalog.Put(ctx, catalog.PutSource{ExternalKey: raw.ExternalKey, ExpectedRevision: raw.Revision, Kind: raw.Kind, ScopeType: raw.ScopeType, ScopeID: raw.ScopeID, Title: raw.Title, Body: raw.Body + "\n", Locator: raw.Locator, Origin: raw.Origin})
			case "access":
				_, err = s.Pool.Exec(ctx, `UPDATE memory_sources SET access_revision=access_revision+1 WHERE source_id=$1`, raw.ID)
			case "revoke":
				_, err = s.Catalog.Delete(ctx, raw.ID, "revoke", raw.Revision)
			case "missing_function":
				_, err = s.Pool.Exec(ctx, `DROP FUNCTION butler_memory_retained_sources_current(TEXT[])`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Catalog.Get(ctx, note.ID); err == nil {
				t.Fatal("stale retained evidence released", change)
			}
			if _, err = s.Catalog.Get(ctx, child.ID); err == nil {
				t.Fatal("descendant bypassed retained guard", change)
			}
		})
	}
}

func TestRetainedMobileBackfillAndIndependentForgetIntegration(t *testing.T) {
	s, ctx, raw, note := retainedGuardFixture(t, false)
	if _, err := s.Pool.Exec(ctx, `UPDATE mobile_events SET received_at=clock_timestamp()-interval '361 days'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Cleanup(context.Background()); !errors.Is(err, catalog.ErrDenied) {
		t.Fatal("unconverted observation silently erased", err)
	}
	if err := s.Catalog.CheckRestore(ctx); err != nil {
		t.Fatal("rejected expiry poisoned manifest", err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = s.Catalog.RetainMobileNoteTx(ctx, tx, catalog.SourceRef{ID: note.ID, Revision: note.Revision, AccessRevision: note.AccessRevision, Hash: note.Hash}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "088_retained_mobile_evidence.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pool.Exec(ctx, string(down)); err == nil {
		t.Fatal("migration discarded evidence")
	}
	if _, err = s.Catalog.Delete(ctx, note.ID, "delete", note.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Catalog.Get(ctx, raw.ID); err != nil {
		t.Fatal("independent note forgetting erased raw archive", err)
	}
	if err = s.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Catalog.Get(ctx, note.ID); err == nil {
		t.Fatal("expiry resurrected independently forgotten fact")
	}
}
