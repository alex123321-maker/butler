package skills

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

func TestSkillDerivedTextDeleteRestoreAndContentFreeHistoryIntegration(t *testing.T) {
	s, m, a, _, owner := skillFixture(t)
	backup, bm, _, _, backupOwner := skillFixture(t)
	anchor := catalog.NewFileManifest(filepath.Join(t.TempDir(), "shared-deletions"))
	if err := anchor.Initialize(); err != nil {
		t.Fatal(err)
	}
	s.SetSources(catalog.New(s.pool, anchor))
	backup.SetSources(catalog.New(backup.pool, anchor))
	m.Description = "private-procedure-marker-do-not-retain"
	bm.Description = m.Description
	current := activeFixture(t, s, m, owner)
	historical := activeFixture(t, backup, bm, backupOwner)
	if _, err := s.Load(owner, "run", current.Digest, 8192); err != nil {
		t.Fatal(err)
	}
	// The complete manifest body belongs only to the derived source graph.
	var history string
	if err := s.pool.QueryRow(owner, `SELECT to_jsonb(v)::text FROM skill_versions v WHERE digest=$1`, current.Digest).Scan(&history); err != nil || strings.Contains(history, m.Description) || strings.Contains(history, "manifest_text") {
		t.Fatal("procedure copied into immutable skill history", err)
	}
	if _, err := s.sources.Delete(owner, a.parents[0].ID, "delete", a.parents[0].Revision); err != nil {
		t.Fatal(err)
	}
	assertRemoved := func(store *Store, hash string, ref catalog.SourceRef, ctx context.Context) {
		t.Helper()
		if _, err := store.Get(ctx, hash); err == nil {
			t.Fatal("deleted skill readable")
		}
		var body string
		if err := store.pool.QueryRow(ctx, `SELECT body FROM memory_source_revisions WHERE source_id=$1`, ref.ID).Scan(&body); err != nil || body != "" {
			t.Fatal("derived procedure bytes retained", err)
		}
	}
	assertRemoved(s, current.Digest, current.BodySource, owner)
	if _, err := s.Pinned(owner, "run"); err == nil {
		t.Fatal("deleted procedure resumed")
	}
	// A previous database snapshot shares the independent deletion anchor. It
	// must deny reads before reconciliation and physically redact on replay.
	if _, err := backup.Get(backupOwner, historical.Digest); err == nil {
		t.Fatal("restored database ignored deletion anchor")
	}
	if err := backup.sources.Reconcile(backupOwner); err != nil {
		t.Fatal(err)
	}
	assertRemoved(backup, historical.Digest, historical.BodySource, backupOwner)
	records, err := s.List(owner)
	if err != nil || len(records) != 1 || records[0].BodyAvailable || records[0].Digest != current.Digest {
		t.Fatal("content-free audit lost", err)
	}
	if _, err = s.Disable(owner, current.Digest, current.Generation, true); err != nil {
		t.Fatal("revocation of deleted version unavailable", err)
	}
	// A missing anchor is also a denied source, never a startup initialization.
	s.SetSources(catalog.New(s.pool, catalog.NewFileManifest(filepath.Join(t.TempDir(), "missing"))))
	if _, err = s.Get(owner, current.Digest); err == nil {
		t.Fatal("missing deletion anchor accepted")
	}
}

func TestSkillProposalRequiresActualSourceReadAuthorityIntegration(t *testing.T) {
	s, m, a, _, owner := skillFixture(t)
	// Same-owner data in another session is not authorized by citation alone.
	if _, err := s.pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:other','owner','web')`); err != nil {
		t.Fatal(err)
	}
	parent, err := s.sources.Put(owner, catalog.PutSource{ExternalKey: "foreign-session-source", Kind: "document", ScopeType: "session", ScopeID: "web:other", Body: "private other conversation"})
	if err != nil {
		t.Fatal(err)
	}
	a.parents = []catalog.SourceRef{{ID: parent.ID, Revision: parent.Revision, AccessRevision: parent.AccessRevision, Hash: parent.Hash}}
	if _, err = s.Propose(context.Background(), "run", packageOf(t, m)); err == nil {
		t.Fatal("source citation granted cross-session publication")
	}
	var count int
	if err = s.pool.QueryRow(owner, `SELECT count(*) FROM skill_versions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("denied proposal persisted", err)
	}
}
