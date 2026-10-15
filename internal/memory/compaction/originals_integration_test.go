package compaction

import (
	"errors"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/security/content"
	"testing"
)

func TestCorrectedSnapshotAndDerivativeCannotBeRetrievedIntegration(t *testing.T) {
	f := newConsumerFixture(t)
	f.poll(t)
	if _, err := f.worker.ProcessOne(f.ctx); err != nil {
		t.Fatal(err)
	}
	var result string
	if err := f.pool.QueryRow(f.ctx, `SELECT result_source_id FROM memory_model_attempts WHERE state='published'`).Scan(&result); err != nil {
		t.Fatal(err)
	}
	root := f.policy.Job.Sources[0].ID
	for _, id := range []string{root, result} {
		if _, err := f.cat.Get(f.ctx, id); err != nil {
			t.Fatal("current source absent", err)
		}
	}
	if _, err := f.cat.Correct(f.ctx, catalog.Correction{SourceID: "message:" + f.firstMessage(t), ExpectedRevision: 1, Body: "Поправка: только черновик, отправку отменить"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{root, result} {
		if _, err := f.cat.Get(f.ctx, id); !errors.Is(err, catalog.ErrStale) {
			t.Fatal("corrected-away data stayed current", id, err)
		}
	}
	s, err := summary.NewStore(f.pool).GetSummarySnapshot(f.ctx, "session")
	if err != nil || s.Summary != "" {
		t.Fatal("corrected summary survived", err)
	}
}

func TestOriginalAncestryPrivacyAndCorrectionFenceIntegration(t *testing.T) {
	for _, mode := range []string{"correct", "delete", "secret", "project"} {
		t.Run(mode, func(t *testing.T) {
			f := newConsumerFixture(t)
			if err := f.js.ControlModel(f.ctx, f.policy.ID, "revoked"); err != nil {
				t.Fatal(err)
			}
			request := catalog.PutSource{ExternalKey: "original-document", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Original report", Body: "Original report excludes an obsolete detail"}
			if mode == "secret" {
				request.Origin = content.Origin{Kind: content.Document, Sensitivity: content.Secret}
			}
			if mode == "project" {
				request.ScopeType = "project"
				request.ScopeID = "private-project"
			}
			doc, err := f.cat.Put(f.ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.pool.Exec(f.ctx, `INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision)VALUES($1,$2,$3)`, "message:"+f.firstMessage(t), doc.ID, doc.Revision); err != nil {
				t.Fatal(err)
			}
			q := f.policy.ModelPolicyRequest
			q.ID = "summary-ancestry-test"
			q.Job.Sources = nil
			p, err := (&Policies{Catalog: f.cat, Jobs: f.js}).Create(f.ctx, PolicyRequest{SessionKey: "session", Policy: q})
			if mode == "secret" || mode == "project" {
				if err == nil {
					t.Fatal("snapshot laundered restricted ancestor")
				}
				if f.provider.calls != 0 {
					t.Fatal("restricted source reached model")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			f.policy = p
			f.poll(t)
			if _, err = f.worker.ProcessOne(f.ctx); err != nil {
				t.Fatal(err)
			}
			var result string
			if err = f.pool.QueryRow(f.ctx, `SELECT result_source_id FROM memory_model_attempts WHERE state='published'`).Scan(&result); err != nil {
				t.Fatal(err)
			}
			if mode == "correct" {
				_, err = f.cat.Correct(f.ctx, catalog.Correction{SourceID: doc.ID, ExpectedRevision: doc.Revision, Body: "Corrected original report"})
			} else {
				_, err = f.cat.Delete(f.ctx, doc.ID, "delete", doc.Revision)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{p.Job.Sources[0].ID, result} {
				if _, err = f.cat.Get(f.ctx, id); err == nil {
					t.Fatal("stale original stayed retrievable")
				}
			}
			s, err := summary.NewStore(f.pool).GetSummarySnapshot(f.ctx, "session")
			if err != nil || s.Summary != "" {
				t.Fatal("stale original retained coverage", err)
			}
			if mode == "delete" {
				var bytes int
				if err = f.pool.QueryRow(f.ctx, `SELECT COALESCE(sum(length(body)),0)::int FROM memory_source_revisions WHERE source_id=ANY($1)`, []string{p.Job.Sources[0].ID, result}).Scan(&bytes); err != nil || bytes != 0 {
					t.Fatal("copied original survived privacy deletion", bytes, err)
				}
			}
		})
	}
}

func TestSnapshotEndpointDoesNotRebaseExistingIdentityIntegration(t *testing.T) {
	f := newConsumerFixture(t)
	q := f.policy.ModelPolicyRequest
	q.Job.Sources = nil
	if _, err := (&Policies{Catalog: f.cat, Jobs: f.js}).Create(f.ctx, PolicyRequest{SessionKey: "session", Policy: q}); !errors.Is(err, jobs.ErrClaim) {
		t.Fatal("existing snapshot identity was repinned", err)
	}
}
