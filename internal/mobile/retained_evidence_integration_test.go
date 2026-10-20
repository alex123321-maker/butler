package mobile

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/chunks"
	"github.com/butler/butler/internal/owneridentity"
)

func TestRetainedMobileEvidenceExpiryForgetAndRestartIntegration(t *testing.T) {
	for _, forget := range []string{"note", "original"} {
		t.Run(forget, func(t *testing.T) {
			s, ctx, id, token := fixture(t)
			path := filepath.Join(t.TempDir(), "retention-manifest")
			manifest := catalog.NewFileManifest(path)
			if err := manifest.Initialize(); err != nil {
				t.Fatal(err)
			}
			s.Catalog = catalog.New(s.Pool, manifest)
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
			text := "Мой постоянный адрес: улица Лесная, дом 8."
			b.Events[0].Payload = &Payload{Messages: []Message{{Text: text, Sender: strptr("Анна"), Timestamp: b.Events[0].Posted}}}
			if _, err = s.Accept(ctx, id, token, b); err != nil {
				t.Fatal(err)
			}
			var raw catalog.SourceRef
			var message string
			if err = s.Pool.QueryRow(ctx, `SELECT s.source_id,s.head_revision,s.access_revision,v.content_hash,m.message_id FROM mobile_messages m JOIN memory_sources s USING(source_id) JOIN memory_source_revisions v ON v.source_id=s.source_id AND v.revision=s.head_revision`).Scan(&raw.ID, &raw.Revision, &raw.AccessRevision, &raw.Hash, &message); err != nil {
				t.Fatal(err)
			}
			loc, _ := json.Marshal(map[string]any{"adapter": "android_message_note_v1", "message_id": message, "original_source": raw})
			tx, err = s.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			note, err := s.Catalog.CreateDerivedTx(ctx, tx, []catalog.SourceRef{raw}, "memory_read", "Адрес", "Сообщён адрес Анны: Лесная, 8.", loc)
			if err != nil {
				t.Fatal(err)
			}
			ref := catalog.SourceRef{ID: note.ID, Revision: note.Revision, AccessRevision: note.AccessRevision, Hash: note.Hash}
			if err = s.Catalog.RetainMobileNoteTx(ctx, tx, ref); err != nil {
				t.Fatal(err)
			}
			if err = s.Catalog.RetainMobileNoteTx(ctx, tx, ref); err != nil {
				t.Fatal("idempotence", err)
			}
			if _, err = s.Catalog.PublishFragmentsTx(ctx, tx, ref, []catalog.Fragment{{Key: "note", Text: note.Body, Locator: loc}}); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `UPDATE mobile_messages SET state='remembered',result_source_id=$1 WHERE message_id=$2`, note.ID, message); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			var days int
			if err = s.Pool.QueryRow(ctx, `SELECT retention_days FROM mobile_devices WHERE device_id=$1`, id).Scan(&days); err != nil || days != 360 {
				t.Fatal("default", days, err)
			}
			if _, err = s.Pool.Exec(ctx, `UPDATE mobile_events SET received_at=clock_timestamp()-interval '359 days'`); err != nil {
				t.Fatal(err)
			}
			if err = s.Cleanup(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Catalog.Get(ctx, raw.ID); err != nil {
				t.Fatal("early expiry", err)
			}
			if _, err = s.Pool.Exec(ctx, `UPDATE mobile_events SET received_at=clock_timestamp()-interval '361 days'`); err != nil {
				t.Fatal(err)
			}
			if err = s.Cleanup(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Catalog.Get(ctx, raw.ID); err == nil {
				t.Fatal("raw archive survived")
			}
			// Reconstruct service and Catalog after a restart with the same private anchor.
			s.Catalog = catalog.New(s.Pool, catalog.NewFileManifest(path))
			if err = s.Catalog.Reconcile(ctx); err != nil {
				t.Fatal("restart reconciliation", err)
			}
			if _, err = s.Catalog.Get(ctx, note.ID); err != nil {
				t.Fatal("note lost with archive", err)
			}
			tx, err = s.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			evidence, err := s.Catalog.RetainedEvidenceTx(ctx, tx, ref)
			if err != nil {
				t.Fatal(err)
			}
			var e map[string]any
			if json.Unmarshal(evidence, &e) != nil || e["quote"] != text || e["sender"] != "Анна" {
				t.Fatal("exact evidence missing")
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			// Pruning centuries later must preserve the derived searchable fragment.
			other, err := s.Catalog.Put(ctx, catalog.PutSource{ExternalKey: "unrelated", Kind: "document", ScopeType: "user", ScopeID: "mobile-owner", Body: "unrelated"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Catalog.PublishFragments(ctx, catalog.SourceRef{ID: other.ID}, []catalog.Fragment{{Key: "other", Text: "unrelated", Locator: json.RawMessage(`{}`)}}); err != nil {
				t.Fatal(err)
			}
			if _, err = chunks.NewStore(s.Pool).Prune(ctx, time.Now().AddDate(100, 0, 0), 1); err != nil {
				t.Fatal(err)
			}
			var n int
			if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM memory_object_sources WHERE source_id=$1 AND memory_type='chunk'`, note.ID).Scan(&n); err != nil || n != 1 {
				t.Fatal("fragment pruned", n, err)
			}
			foreign := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "foreign", Channel: "web", DeliveryID: "test"})
			if _, err = s.Catalog.Get(foreign, note.ID); err == nil {
				t.Fatal("foreign read")
			}
			target, rev := note.ID, note.Revision
			if forget == "original" {
				target, rev = raw.ID, raw.Revision
			}
			if _, err = s.Catalog.Delete(ctx, target, "delete", rev); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Catalog.Get(ctx, note.ID); err == nil {
				t.Fatal("forgotten note readable")
			}
			if _, err = s.Catalog.Get(ctx, other.ID); err != nil {
				t.Fatal("unrelated source erased", err)
			}
			if err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM memory_retained_mobile_evidence WHERE source_id=$1`, note.ID).Scan(&n); err != nil || n != 0 {
				t.Fatal("evidence survived forgetting", n, err)
			}
			var body string
			if err = s.Pool.QueryRow(ctx, `SELECT body FROM memory_source_revisions WHERE source_id=$1`, note.ID).Scan(&body); err != nil || strings.Contains(body, "Лесная") {
				t.Fatal("fact text survived forgetting", err)
			}
		})
	}
}

func strptr(s string) *string { return &s }
