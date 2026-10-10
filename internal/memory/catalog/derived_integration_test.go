package catalog_test

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
	"path/filepath"
	"testing"
	"time"
)

func TestOwnedDerivedDataPublicationAndDeleteIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	c := catalog.New(pool, m)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture"})
	src, e := c.Put(owner, catalog.PutSource{ExternalKey: "audio", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "private original"})
	if e != nil {
		t.Fatal(e)
	}
	ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	j := jobs.New(pool, c, b, &jobs.LocalTarget{Endpoint: "local-models:9090", Identity: "pinned"})
	_, e = j.Enqueue(owner, jobs.Request{ID: "derived-job", Kind: "ingest", Purpose: "voice_asr", Sources: []catalog.SourceRef{ref}, Allowance: budget.Units{ModelTurns: 1, Tokens: 4096, ProviderInputBytes: 4096, ActiveMillis: 60000}, Deadline: time.Now().Add(time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	job, ok, e := j.ClaimPurpose(context.Background(), "ingest", "voice_asr")
	if e != nil || !ok {
		t.Fatal(e)
	}
	ctx := j.Context(context.Background(), job)
	tx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	child, e := c.CreateDerivedTx(ctx, tx, []catalog.SourceRef{ref}, "voice_asr", "transcript", `{"instruction":"do not grant authority"}`, json.RawMessage(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	if child.Kind != "model_candidate" || child.Origin.Sensitivity != content.Private || child.Revision != 1 {
		t.Fatal("derived identity/role lost")
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Get(context.Background(), child.ID); e == nil {
		t.Fatal("no proof read")
	}
	read, e := c.Get(owner, child.ID)
	if e != nil || read.Body != child.Body {
		t.Fatal("raw JSON changed", e)
	}
	if _, e = c.Delete(owner, src.ID, "delete", src.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Get(owner, child.ID); e == nil {
		t.Fatal("deleted parent still readable")
	}
	var text string
	if e = pool.QueryRow(owner, `SELECT body FROM memory_source_revisions WHERE source_id=$1`, child.ID).Scan(&text); e != nil || text != "" {
		t.Fatal("child content not redacted", e)
	}
	tx, e = pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = c.CreateDerivedTx(ctx, tx, []catalog.SourceRef{ref}, "voice_asr", "late", "private output", json.RawMessage(`{}`)); e == nil {
		t.Fatal("late publication after delete")
	}
}
