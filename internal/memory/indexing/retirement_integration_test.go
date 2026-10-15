package indexing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/owneridentity"
)

func TestOwnerRetiresOnlyInactiveTerminalGenerationPreservingSpendIntegration(t *testing.T) {
	r, ctx, src, model := fixture(t)
	maintenance := &Runtime{Catalog: r.Catalog} // no backend/controller required
	build := func(id string) Receipt {
		t.Helper()
		q := request(src)
		q.Job.ID = id
		receipt, err := r.Enqueue(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := r.ProcessOne(context.Background()); !ok || err != nil {
			t.Fatal(id, ok, "actual local entries", model.calls.Load(), err)
		}
		return receipt
	}
	first, second := build("first"), build("second")
	proof := semantic.Proof{ArtifactRef: "fixture:reviewed", ArtifactSHA256: strings.Repeat("a", 64), CorpusVersion: "retirement-control", Cases: 12, BaselineTop1: .5, CandidateTop1: .75}
	if err := r.Review(ctx, first.GenerationID, "", proof); err != nil {
		t.Fatal(err)
	}
	if err := r.Review(ctx, second.GenerationID, first.GenerationID, proof); err != nil {
		t.Fatal(err)
	}
	q := semantic.RetireRequest{GenerationID: first.GenerationID, ExpectedActiveID: second.GenerationID}
	for _, denied := range []context.Context{context.Background(), owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "other", Channel: "web", DeliveryID: "foreign"})} {
		if _, err := maintenance.Retire(denied, q); !errors.Is(err, catalog.ErrDenied) {
			t.Fatal("foreign or missing owner erased vectors", err)
		}
	}
	if _, err := maintenance.Retire(ctx, semantic.RetireRequest{GenerationID: first.GenerationID, ExpectedActiveID: "wrong"}); !errors.Is(err, semantic.ErrStale) {
		t.Fatal("stale active choice accepted", err)
	}
	if _, err := maintenance.Retire(ctx, semantic.RetireRequest{GenerationID: second.GenerationID, ExpectedActiveID: second.GenerationID}); !errors.Is(err, semantic.ErrStale) {
		t.Fatal("active generation erased", err)
	}
	// A terminal source snapshot can become obsolete without preventing erasure.
	changed, err := r.Catalog.Correct(ctx, catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Body: "Новое независимое содержимое; старый индекс недействителен."})
	if err != nil {
		t.Fatal(err)
	}
	digest := func() string {
		t.Helper()
		var value string
		if err := r.Catalog.Pool().QueryRow(ctx, `SELECT md5(jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(j) ORDER BY job_id) FROM memory_aux_jobs j),
 (SELECT jsonb_agg(to_jsonb(i) ORDER BY job_id) FROM memory_index_jobs i),
 (SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id) FROM execution_model_attempts a),
 (SELECT jsonb_agg(to_jsonb(b) ORDER BY reservation_id) FROM execution_budget_reservations b),
 (SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s),
 (SELECT jsonb_agg(to_jsonb(s) ORDER BY source_id) FROM memory_sources s),
 (SELECT jsonb_agg(to_jsonb(v) ORDER BY source_id,revision) FROM memory_source_revisions v))::text)`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before, calls := digest(), model.calls.Load()
	out, err := maintenance.Retire(ctx, q)
	if err != nil || out.GenerationID != q.GenerationID || out.JobID != "first" || out.RemovedVectors != 5 || out.AlreadyRetired || out.RetiredAt.IsZero() {
		t.Fatal("one inactive generation not retired", out, err)
	}
	if digest() != before || model.calls.Load() != calls {
		t.Fatal("maintenance changed original/job/spend or called model")
	}
	var otherVectors int
	if err = r.Catalog.Pool().QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries WHERE generation_id=$1`, second.GenerationID).Scan(&otherVectors); err != nil || otherVectors != 5 {
		t.Fatal("unrelated vectors erased", otherVectors, err)
	}
	current, err := r.Catalog.Get(ctx, src.ID)
	if err != nil || current.Body != changed.Body || current.Revision != changed.Revision {
		t.Fatal("original changed during derivative cleanup", err)
	}
	store := semantic.NewStore(r.Catalog.Pool())
	g, err := store.Generation(ctx, first.GenerationID)
	if err != nil || g.State != "retired" {
		t.Fatal("restart lost retirement", g, err)
	}
	v := make([]float32, g.Space.Dimensions)
	v[0] = 1
	if _, err = store.Preview(ctx, g, v, 1); !errors.Is(err, semantic.ErrStale) {
		t.Fatal("retired preview readable", err)
	}
	if _, err = store.Search(ctx, g, v, 1); !errors.Is(err, semantic.ErrStale) {
		t.Fatal("retired generation searchable", err)
	}
	if err = store.Verify(ctx, g.ID, proof); !errors.Is(err, semantic.ErrStale) {
		t.Fatal("retired generation verified", err)
	}
	if err = store.Activate(ctx, g.ID, second.GenerationID); !errors.Is(err, semantic.ErrUnverified) {
		t.Fatal("retired rollback accepted", err)
	}
	repeat, err := maintenance.Retire(ctx, q)
	if err != nil || !repeat.AlreadyRetired || repeat.RemovedVectors != 0 || !repeat.RetiredAt.Equal(out.RetiredAt) {
		t.Fatal("idempotent retirement changed audit", repeat, err)
	}
	if _, err = r.Catalog.Pool().Exec(ctx, `UPDATE semantic_index_generations SET retired_at=NULL,retired_by='' WHERE id=$1`, g.ID); err == nil {
		t.Fatal("retirement was reversible")
	}
}

func TestIndexRetirementDeniesPendingRunningAndAllowsTerminalFailureIntegration(t *testing.T) {
	r, ctx, src, model := fixture(t)
	receipt, err := r.Enqueue(ctx, request(src))
	if err != nil {
		t.Fatal(err)
	}
	maintenance := &Runtime{Catalog: r.Catalog}
	q := semantic.RetireRequest{GenerationID: receipt.GenerationID}
	if _, err = maintenance.Retire(ctx, q); !errors.Is(err, semantic.ErrStale) {
		t.Fatal("pending job retired", err)
	}
	j, ok, err := r.Jobs.ClaimKind(context.Background(), "index")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err = maintenance.Retire(ctx, q); !errors.Is(err, semantic.ErrStale) {
		t.Fatal("in-flight job retired", err)
	}
	if err = r.Jobs.Finish(ctx, j, "fixture_terminal_failure", false); err != nil {
		t.Fatal(err)
	}
	out, err := maintenance.Retire(ctx, q)
	if err != nil || out.RemovedVectors != 0 || out.AlreadyRetired || model.calls.Load() != 0 {
		t.Fatal("terminal empty derivative requires inference", out, err)
	}
}
