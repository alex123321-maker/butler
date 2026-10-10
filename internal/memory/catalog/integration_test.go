package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
)

func fixture(t *testing.T) (*Store, context.Context) {
	t.Helper()
	m := NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	return New(memorytest.Database(t), m), owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture"})
}
func put(t *testing.T, s *Store, ctx context.Context, key, body string) Source {
	t.Helper()
	v, e := s.Put(ctx, PutSource{ExternalKey: key, Kind: "document", ScopeType: "user", ScopeID: "owner", Body: body, Title: key})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func ref(s Source) SourceRef { return SourceRef{s.ID, s.Revision, s.AccessRevision, s.Hash} }
func propose(t *testing.T, s *Store, ctx context.Context, src Source, value string) Claim {
	t.Helper()
	v, e := s.Propose(ctx, ClaimRequest{SourceID: src.ID, SourceRevision: src.Revision, Key: "language", Value: json.RawMessage(value), Summary: value, Confidence: .999})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestSourceDuplicateAndConflictingPreferencesIntegration(t *testing.T) {
	s, ctx := fixture(t)
	a := put(t, s, ctx, "message:a", "Пиши по-русски")
	same := put(t, s, ctx, "message:a", "Пиши по-русски")
	if same.ID != a.ID || same.Revision != 1 {
		t.Fatal("duplicate source changed identity")
	}
	old := propose(t, s, ctx, a, `"ru"`)
	if old.Status != "candidate" {
		t.Fatal("confidence became acceptance")
	}
	old, e := s.Decide(ctx, Decision{ClaimID: old.ID})
	if e != nil {
		t.Fatal(e)
	}
	b := put(t, s, ctx, "old-document-loaded-today", "English only")
	next := propose(t, s, ctx, b, `"en"`)
	if _, e = s.Decide(ctx, Decision{ClaimID: next.ID}); !errors.Is(e, ErrStale) {
		t.Fatal("contradiction resolved without current owner decision", e)
	}
	next, e = s.Decide(ctx, Decision{ClaimID: next.ID, ExpectedActiveID: old.ID})
	if e != nil || next.SupersedesID != old.ID {
		t.Fatal(e)
	}
	claims, e := s.Claims(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if len(claims) != 2 || claims[0].Status != "superseded" || claims[1].Status != "confirmed" {
		t.Fatal(claims)
	}
	foreign := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "other", Channel: "web", DeliveryID: "x"})
	if _, e = s.Get(foreign, a.ID); !errors.Is(e, ErrDenied) {
		t.Fatal("foreign owner read source", e)
	}
	if _, e = s.Get(context.Background(), a.ID); !errors.Is(e, ErrDenied) {
		t.Fatal("unproved read source", e)
	}
}
func TestConcurrentExplicitSupersessionIntegration(t *testing.T) {
	s, ctx := fixture(t)
	src := put(t, s, ctx, "source", "data")
	a := propose(t, s, ctx, src, `"ru"`)
	a, e := s.Decide(ctx, Decision{ClaimID: a.ID})
	if e != nil {
		t.Fatal(e)
	}
	b := propose(t, s, ctx, src, `"en"`)
	c := propose(t, s, ctx, src, `"de"`)
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for _, claim := range []Claim{b, c} {
		wg.Add(1)
		go func(c Claim) {
			defer wg.Done()
			bounded, stop := context.WithTimeout(ctx, 3*time.Second)
			defer stop()
			_, e := s.Decide(bounded, Decision{ClaimID: c.ID, ExpectedActiveID: a.ID})
			out <- e
		}(claim)
	}
	wg.Wait()
	close(out)
	success, stale := 0, 0
	for e := range out {
		if e == nil {
			success++
		} else if errors.Is(e, ErrStale) {
			stale++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatal(success, stale)
	}
}
func TestTransitiveDeletionAndBackupRestoreIntegration(t *testing.T) {
	s, ctx := fixture(t)
	src := put(t, s, ctx, "original", "Личные исходные данные")
	child, e := s.Put(ctx, PutSource{ExternalKey: "report", Kind: "model_candidate", ScopeType: "user", ScopeID: "owner", Title: "report", Body: "false report", Parents: []SourceRef{ref(src)}, Origin: content.Origin{Kind: content.SubagentOutput, SourceRefs: []string{"child-report"}, Sensitivity: content.Private}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.PublishFragments(ctx, ref(child), []Fragment{{Key: "paragraph", Title: "report", Text: "false report", Locator: json.RawMessage(`{"line":1}`)}}); e != nil {
		t.Fatal(e)
	}
	c := propose(t, s, ctx, child, `"false"`)
	if _, e = s.Decide(ctx, Decision{ClaimID: c.ID}); e != nil {
		t.Fatal(e)
	}
	// A separate pre-deletion snapshot has the same stable identities and shares
	// the external anchor. It must be blocked until deletion replay redacts it.
	restored := New(memorytest.Database(t), s.manifest)
	backup := put(t, restored, ctx, "original", src.Body)
	if backup.ID != src.ID {
		t.Fatal("backup identity mismatch")
	}
	if _, e = s.Delete(ctx, src.ID, "delete", 1); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Get(ctx, child.ID); !errors.Is(e, ErrDenied) {
		t.Fatal("deleted parent retained descendant", e)
	}
	var count int
	if e = s.pool.QueryRow(ctx, `SELECT count(*) FROM memory_chunks`).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
	if e = restored.CheckRestore(ctx); !errors.Is(e, ErrRestore) {
		t.Fatal("old backup accepted", e)
	}
	if e = restored.Reconcile(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = restored.Get(ctx, src.ID); !errors.Is(e, ErrDenied) {
		t.Fatal("restored deleted text served", e)
	}
	if _, e = restored.Put(ctx, PutSource{ExternalKey: "original", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: src.Body}); !errors.Is(e, ErrDeleted) {
		t.Fatal("reindex resurrected source", e)
	}
	var body string
	if e = restored.pool.QueryRow(ctx, `SELECT body FROM memory_source_revisions WHERE source_id=$1`, src.ID).Scan(&body); e != nil || body != "" {
		t.Fatal("backup body not purged", e)
	}
}
func TestSourceChangeAndRevocationFenceFragmentsIntegration(t *testing.T) {
	s, ctx := fixture(t)
	src := put(t, s, ctx, "repository:file", "func A() {}")
	frags := []Fragment{{Key: "func:A", Title: "A", Text: "func A() {}", Locator: json.RawMessage(`{"path":"old.go","lines":[1,1]}`)}}
	r, e := s.PublishFragments(ctx, ref(src), frags)
	if e != nil || r.Inserted != 1 {
		t.Fatal(r, e)
	}
	var id int64
	_ = s.pool.QueryRow(ctx, `SELECT id FROM memory_chunks`).Scan(&id)
	changed, e := s.Put(ctx, PutSource{ExternalKey: "repository:file", Kind: "document", ScopeType: "user", ScopeID: "owner", ExpectedRevision: 1, Body: src.Body, Title: "renamed.go"})
	if e != nil {
		t.Fatal(e)
	}
	frags[0].Locator = json.RawMessage(`{"path":"renamed.go","lines":[1,1]}`)
	r, e = s.PublishFragments(ctx, ref(changed), frags)
	if e != nil || r.Unchanged != 1 {
		t.Fatal(r, e)
	}
	citation, e := s.Citation(ctx, id)
	if e != nil || citation.ChunkID != id || citation.Source.Title != "renamed.go" {
		t.Fatal(citation, e)
	}
	if _, e = s.PublishFragments(ctx, ref(src), frags); !errors.Is(e, ErrStale) {
		t.Fatal("old writer published", e)
	}
	if _, e = s.Delete(ctx, changed.ID, "revoke", changed.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = s.PublishFragments(ctx, ref(changed), frags); !errors.Is(e, ErrDenied) {
		t.Fatal("revoked source published", e)
	}
}
func TestMissingManifestFailsClosedIntegration(t *testing.T) {
	s, ctx := fixture(t)
	s.manifest = NewFileManifest(filepath.Join(t.TempDir(), "missing"))
	if _, e := s.View(ctx); !errors.Is(e, ErrRestore) {
		t.Fatal(e)
	}
}

func TestMatchingWatermarkCannotHideWrongDeletionHistoryIntegration(t *testing.T) {
	s, ctx := fixture(t)
	src := put(t, s, ctx, "original", "private original")
	other := put(t, s, ctx, "other", "other text")
	if _, e := s.Delete(ctx, src.ID, "delete", src.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e := s.pool.Exec(ctx, `UPDATE memory_deletions SET source_id=$1`, other.ID); e != nil {
		t.Fatal(e)
	}
	if e := s.CheckRestore(ctx); !errors.Is(e, ErrRestore) {
		t.Fatal("equal watermark accepted wrong tombstone history", e)
	}
}
