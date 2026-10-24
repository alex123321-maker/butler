package catalog

import (
	"testing"
)

func TestAccessAllRootAncestorRevisionsIntegration(t *testing.T) {
	for _, kind := range []string{"current", "second-root-stale", "shared-parent-revoked", "conflicting-parent-revision"} {
		t.Run(kind, func(t *testing.T) {
			s, ctx := fixture(t)
			ancestor := put(t, s, ctx, "ancestor", "original ancestor")
			left := put(t, s, ctx, "left", "left derived data")
			right := put(t, s, ctx, "right", "right derived data")
			other := put(t, s, ctx, "other", "other ancestor")
			// A shared ancestor plus an independent branch on the second root.
			// These retained edges are the authority graph, not citation JSON.
			if _, err := s.pool.Exec(ctx, `INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision) VALUES($1,$3,1),($2,$3,1),($2,$4,1)`, left.ID, right.ID, ancestor.ID, other.ID); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "second-root-stale":
				_, err = s.Put(ctx, PutSource{ExternalKey: "other", ExpectedRevision: other.Revision, Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "revised other ancestor"})
			case "shared-parent-revoked":
				_, err = s.Delete(ctx, ancestor.ID, "revoke", ancestor.Revision)
			case "conflicting-parent-revision":
				_, err = s.Put(ctx, PutSource{ExternalKey: "ancestor", ExpectedRevision: ancestor.Revision, Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "revised ancestor"})
				if err == nil {
					_, err = s.pool.Exec(ctx, `UPDATE memory_source_dependencies SET parent_revision=2 WHERE child_source_id=$1 AND parent_source_id=$2`, right.ID, ancestor.ID)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			tx, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			rows, err := s.AccessTx(ctx, tx, []SourceRef{ref(left), ref(right)}, "communication_export")
			if kind == "current" {
				if err != nil || len(rows) != 2 {
					t.Fatal("current graph denied or ancestors exposed as selected DATA", len(rows), err)
				}
			} else if err == nil || len(rows) != 0 {
				t.Fatal("stale ancestor graph released DATA", kind, len(rows), err)
			}
		})
	}
}
