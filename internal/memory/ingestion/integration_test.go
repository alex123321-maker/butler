package ingestion

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestStructuredDuplicateRenameFunctionChangeOriginalAndRevocationIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	c := catalog.New(pool, m)
	s := &Store{Catalog: c}
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "upload"})
	u := Upload{ExternalKey: "repo:logical-file", ScopeType: "user", ScopeID: "owner", Format: "go", Path: "old.go", RepositoryRevision: "rev1", Raw: []byte("package p\n// A сохраняет значение.\nfunc A() int {return 1}\nfunc B() int {return 2}\n")}
	a, e := s.Ingest(ctx, u)
	if e != nil || a.Status != "ready" || a.Fragments.Inserted != 2 {
		t.Fatal(a, e)
	}
	duplicate, e := s.Ingest(ctx, u)
	if e != nil || duplicate.Source.ID != a.Source.ID || duplicate.Source.Revision != 1 || duplicate.Fragments.Unchanged != 2 {
		t.Fatal(duplicate, e)
	}
	var bID int64
	if e = pool.QueryRow(ctx, `SELECT object_id FROM memory_object_sources WHERE fragment_key='function:B:0'`).Scan(&bID); e != nil {
		t.Fatal(e)
	}
	u.Path = "renamed.go"
	u.ExpectedRevision = 1
	renamed, e := s.Ingest(ctx, u)
	if e != nil || renamed.Fragments.Unchanged != 2 {
		t.Fatal(renamed, e)
	}
	var retained int64
	if e = pool.QueryRow(ctx, `SELECT object_id FROM memory_object_sources WHERE fragment_key='function:B:0'`).Scan(&retained); e != nil || retained != bID {
		t.Fatal(retained, bID, e)
	}
	u.ExpectedRevision = renamed.Source.Revision
	u.RepositoryRevision = "rev2"
	u.Raw = []byte("package p\n// A сохраняет новое значение.\nfunc A() int {return 3}\nfunc B() int {return 2}\n")
	changed, e := s.Ingest(ctx, u)
	if e != nil || changed.Fragments.Updated != 1 || changed.Fragments.Unchanged != 1 {
		t.Fatal(changed, e)
	}
	citation, e := c.Citation(ctx, bID)
	if e != nil || citation.Source.Revision != changed.Source.Revision || citation.Source.Title != "renamed.go" {
		t.Fatal(citation, e)
	}
	raw, path, e := s.Original(ctx, changed.Source.ID, changed.Source.Revision)
	if e != nil || string(raw) != string(u.Raw) || path != "renamed.go" {
		t.Fatal(path, e)
	}
	if _, e = c.Delete(ctx, changed.Source.ID, "revoke", changed.Source.Revision); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Original(ctx, changed.Source.ID, changed.Source.Revision); !errors.Is(e, catalog.ErrDenied) {
		t.Fatal("revoked original served", e)
	}
	var n int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM memory_source_assets WHERE original_bytes IS NOT NULL`).Scan(&n); e != nil || n != 0 {
		t.Fatal("old asset retained", n, e)
	}
	u.ExpectedRevision = changed.Source.Revision
	if _, e = s.Ingest(ctx, u); !errors.Is(e, catalog.ErrDeleted) {
		t.Fatal("revoked source recreated", e)
	}
	bad, e := s.Ingest(ctx, Upload{ExternalKey: "corrupt-attachment", ScopeType: "user", ScopeID: "owner", Format: "docx", Path: "bad.docx", Raw: []byte("corrupt file")})
	if e != nil || bad.Status != "failed" || bad.ErrorCode != "corrupt_source" {
		t.Fatal(bad, e)
	}
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM memory_chunks`).Scan(&n); e != nil || n != 0 {
		t.Fatal("corrupt file partially indexed", n, e)
	}
}
