package experience

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/reader"
	"github.com/jackc/pgx/v5"
)

// Scripted owned-job data only; no native/model network request. An incompatible
// lesson and its descendant must not steal the page or poison the current run.
func TestExperienceSearchEnvironmentPrecedesRankingAndRunPinsIntegration(t *testing.T) {
	f := newLessonFixture(t)
	lesson := f.publish(t)
	f.accept(t, lesson)
	fragments := []catalog.Fragment{}
	for i := 0; i < 6; i++ {
		fragments = append(fragments, catalog.Fragment{Key: fmt.Sprint(i), Text: "Observed incompatible fragment", Locator: json.RawMessage(`{}`)})
	}
	if _, err := f.s.Sources.PublishFragments(f.ctx, lesson, fragments); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	descendant, err := f.s.Sources.CreateDerivedTx(f.ctx, tx, []catalog.SourceRef{lesson}, "memory_extract", "Observed descendant", "Observed incompatible derivative", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	ordinary, err := f.s.Sources.Put(f.ctx, catalog.PutSource{ExternalKey: "current-observed-data", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Observed current data", Body: "Observed current ordinary DATA"})
	if err != nil {
		t.Fatal(err)
	}
	live, _ := f.consumer(t)
	current, err := New(f.s.Sources, strings.Repeat("c", 64), f.s.Tools)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := current.Prepare(live, "consumer")
	if err != nil {
		t.Fatal(err)
	}
	subset := []catalog.SourceRef{lesson, ref(descendant), ref(ordinary)}
	makeReader := func(filter bool) reader.Reader {
		r := reader.Reader{Sources: f.s.Sources, SourceSubset: subset, SourceEligibility: func(ctx context.Context, tx pgx.Tx, s catalog.Source) (bool, error) {
			return current.EligibleTx(ctx, tx, s, prepared, true)
		}}
		if filter {
			r.ExperienceEnvironment = &current.EnvironmentHash
		}
		return r
	}
	// The old order sees only an incompatible top-ranked chunk, pins it, and
	// fails the unchanged final lineage check even though no body was released.
	tx, err = f.pool.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	before, err := makeReader(false).ReadTx(live, tx, readcontrol.Search, readcontrol.Args{Query: "Observed", Limit: 1})
	if err != nil || len(before.Items) != 0 {
		t.Fatal("counterexample not reproduced", err, before.Status)
	}
	if err = current.CheckPinnedTx(live, tx, "consumer"); err == nil {
		t.Fatal("unfiltered incompatible candidate did not poison the old read")
	}
	if err = tx.Rollback(live); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Sources.PublishFragments(f.ctx, ref(ordinary), []catalog.Fragment{{Key: "current", Text: "Observed current ordinary DATA", Locator: json.RawMessage(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	tx, err = f.pool.Begin(live)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(live)
	r := makeReader(true)
	got, err := r.ReadTx(live, tx, readcontrol.Search, readcontrol.Args{Query: "Observed", Limit: 1})
	if err != nil || got.Status != "ok" || len(got.Items) != 1 || got.Items[0].Source.ID != ordinary.ID {
		t.Fatal("compatible source hidden behind incompatible page", err, got.Status)
	}
	for _, source := range []catalog.SourceRef{lesson, ref(descendant)} {
		denied, err := r.ReadTx(live, tx, readcontrol.Open, readcontrol.Args{SourceID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash, MaxChars: 1000})
		if err != nil || denied.Status != "denied" || len(denied.Items) != 0 {
			t.Fatal("incompatible open was not an explicit DATA denial", err, denied.Status)
		}
	}
	var badPins int
	if err = tx.QueryRow(live, `SELECT count(*) FROM memory_run_sources WHERE run_id='consumer' AND source_id=ANY($1::text[])`, []string{lesson.ID, descendant.ID}).Scan(&badPins); err != nil || badPins != 0 {
		t.Fatal("filtered source was pinned", err, badPins)
	}
	if err = current.CheckPinnedTx(live, tx, "consumer"); err != nil {
		t.Fatal("filtered read failed final lineage", err)
	}
	// Semantic candidates come from an existing local reader. Apply the same
	// metadata gate before acquiring body/pins, retaining the ordinary candidate.
	for _, source := range []catalog.SourceRef{lesson, ref(ordinary)} {
		var candidate reader.SemanticCandidate
		if err = tx.QueryRow(live, `SELECT c.id,c.updated_at FROM memory_object_sources o JOIN memory_chunks c ON o.memory_type='chunk' AND o.object_id=c.id WHERE o.source_id=$1 ORDER BY c.id LIMIT 1`, source.ID).Scan(&candidate.ID, &candidate.Revision); err != nil {
			t.Fatal(err)
		}
		r.SemanticCandidates = append(r.SemanticCandidates, candidate)
	}
	semantic, err := r.ReadTx(live, tx, readcontrol.Search, readcontrol.Args{Query: "Observed", Strategy: "semantic_hybrid", Limit: 2})
	if err != nil || semantic.Status != "ok" || len(semantic.Items) != 1 || semantic.Items[0].Source.ID != ordinary.ID {
		t.Fatal("semantic candidate bypassed environment filter", err, semantic.Status)
	}
	// A compatible environment still exposes and pins the accepted lesson.
	originalPrepared, err := f.s.Prepare(live, "consumer")
	if err != nil {
		t.Fatal(err)
	}
	compatible := reader.Reader{Sources: f.s.Sources, SourceSubset: []catalog.SourceRef{lesson}, ExperienceEnvironment: &f.s.EnvironmentHash, SourceEligibility: func(ctx context.Context, tx pgx.Tx, s catalog.Source) (bool, error) {
		return f.s.EligibleTx(ctx, tx, s, originalPrepared, true)
	}}
	got, err = compatible.ReadTx(live, tx, readcontrol.Open, readcontrol.Args{SourceID: lesson.ID, Revision: lesson.Revision, AccessRevision: lesson.AccessRevision, Hash: lesson.Hash, MaxChars: 1000})
	if err != nil || got.Status != "ok" || len(got.Items) != 1 {
		t.Fatal("compatible lesson denied", err, got.Status)
	}
	if err = f.s.CheckPinnedTx(live, tx, "consumer"); err != nil {
		t.Fatal(err)
	}
	if err = current.CheckPinnedTx(live, tx, "consumer"); err == nil {
		t.Fatal("prefilter erased an already exposed incompatible lesson")
	}
}
