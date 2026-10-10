package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestOwnedDerivedCrossScopeCurrentParentsAndRestoreIntegration(t *testing.T) {
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture"})
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	create := func(store *catalog.Store) (catalog.Source, catalog.Source) {
		t.Helper()
		a, e := store.Put(ctx, catalog.PutSource{ExternalKey: "derived-user", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "private owner data"})
		if e != nil {
			t.Fatal(e)
		}
		b, e := store.Put(ctx, catalog.PutSource{ExternalKey: "derived-project", Kind: "document", ScopeType: "project", ScopeID: "project", Body: "private project data"})
		if e != nil {
			t.Fatal(e)
		}
		return a, b
	}
	ref := func(s catalog.Source) catalog.SourceRef {
		return catalog.SourceRef{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}
	}
	store := catalog.New(memorytest.Database(t), manifest)
	a, b := create(store)
	refs := []catalog.SourceRef{ref(b), ref(a)} // The first trusted parent anchors scope, independent of lock order.
	derive := func(store *catalog.Store, proof context.Context, parents []catalog.SourceRef) (catalog.Source, error) {
		tx, e := store.Pool().Begin(proof)
		if e != nil {
			return catalog.Source{}, e
		}
		defer tx.Rollback(proof)
		child, e := store.CreateDerivedTx(proof, tx, parents, "voice_asr", "derived", "private output", json.RawMessage(`{}`))
		if e != nil {
			return child, e
		}
		return child, tx.Commit(proof)
	}
	child, e := derive(store, ctx, refs)
	if e != nil {
		t.Fatal(e)
	}
	if child.ScopeType != b.ScopeType || child.ScopeID != b.ScopeID {
		t.Fatal("scope did not use first trusted parent")
	}
	var edges int
	if e = store.Pool().QueryRow(ctx, `SELECT count(*) FROM memory_source_dependencies WHERE child_source_id=$1`, child.ID).Scan(&edges); e != nil || edges != 2 {
		t.Fatal("parent omitted", e)
	}
	foreign := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "other", Channel: "web", DeliveryID: "fixture"})
	f, e := store.Put(foreign, catalog.PutSource{ExternalKey: "foreign", Kind: "document", ScopeType: "user", ScopeID: "other", Body: "foreign data"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = derive(store, ctx, []catalog.SourceRef{ref(b), ref(f)}); e == nil {
		t.Fatal("foreign parent accepted")
	}
	controller := budget.NewController(store.Pool(), budget.DefaultPolicy())
	controller.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	jobsStore := jobs.New(store.Pool(), store, controller, &jobs.LocalTarget{Endpoint: "local-models:9090", Identity: "pinned"})
	if _, e = jobsStore.Enqueue(ctx, jobs.Request{ID: "one-parent-only", Kind: "ingest", Purpose: "voice_asr", Sources: []catalog.SourceRef{ref(b)}, Allowance: budget.Units{ModelTurns: 1, Tokens: 4096, ProviderInputBytes: 4096, ActiveMillis: 60000}, Deadline: time.Now().Add(time.Hour)}); e != nil {
		t.Fatal(e)
	}
	job, ok, e := jobsStore.ClaimPurpose(context.Background(), "ingest", "voice_asr")
	if e != nil || !ok {
		t.Fatal(e)
	}
	if _, e = derive(store, jobsStore.Context(context.Background(), job), refs); e == nil {
		t.Fatal("ungranted cross-scope parent accepted")
	}
	// A separate pre-deletion PG snapshot has the same original identities and
	// dependency graph, but its own fresh derived ID. The private anchor is shared.
	restored := catalog.New(memorytest.Database(t), manifest)
	ra, rb := create(restored)
	backupChild, e := derive(restored, ctx, []catalog.SourceRef{ref(rb), ref(ra)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.Delete(ctx, a.ID, "delete", a.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = store.Get(ctx, child.ID); e == nil {
		t.Fatal("deleted cross-scope ancestor still readable")
	}
	if e = restored.CheckRestore(ctx); !errors.Is(e, catalog.ErrRestore) {
		t.Fatal("backup served before reconciliation", e)
	}
	if e = restored.Reconcile(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = restored.Get(ctx, backupChild.ID); e == nil {
		t.Fatal("restored derived text readable")
	}
	var text string
	if e = restored.Pool().QueryRow(ctx, `SELECT body FROM memory_source_revisions WHERE source_id=$1`, backupChild.ID).Scan(&text); e != nil || text != "" {
		t.Fatal("derived backup text not erased", e)
	}
	if _, e = derive(store, ctx, refs); e == nil {
		t.Fatal("late cross-scope publication accepted")
	}
}
