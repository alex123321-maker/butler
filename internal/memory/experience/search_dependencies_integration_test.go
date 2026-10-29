package experience

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/reader"
	"github.com/jackc/pgx/v5"
)

// C43: same environment, but the current task no longer permits a lesson's
// tool. A hidden hit must not poison the next provider release with its pins.
func TestExperienceSearchDependencyFilterPrecedesRunPinsIntegration(t *testing.T) {
	for _, mode := range []string{"legacy_order", "missing", "changed", "compatible"} {
		t.Run(mode, func(t *testing.T) {
			f := newLessonFixture(t)
			lesson := f.publish(t)
			f.accept(t, lesson)
			if _, err := f.s.Sources.PublishFragments(f.ctx, lesson, []catalog.Fragment{{Key: "observed", Text: "Observed lesson DATA", Locator: json.RawMessage(`{}`)}}); err != nil {
				t.Fatal(err)
			}
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			descendant, err := f.s.Sources.CreateDerivedTx(f.ctx, tx, []catalog.SourceRef{lesson}, "memory_extract", "Observed derivative", "Observed dependent DATA", json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			ordinary, err := f.s.Sources.Put(f.ctx, catalog.PutSource{ExternalKey: "ordinary", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Observed ordinary", Body: "Observed ordinary DATA"})
			if err != nil {
				t.Fatal(err)
			}
			live, _ := f.consumer(t)
			prepared, err := f.s.Prepare(live, "consumer")
			if err != nil {
				t.Fatal(err)
			}
			if mode != "compatible" {
				for name, d := range prepared.tools {
					if mode == "changed" {
						d.ContractRevision += "-changed"
						prepared.tools[name] = d
					} else {
						delete(prepared.tools, name)
					}
				}
			}
			r := reader.Reader{Sources: f.s.Sources, SourceSubset: []catalog.SourceRef{lesson, ref(descendant), ref(ordinary)}, ExperienceEnvironment: &f.s.EnvironmentHash, ExperienceDependencies: prepared.DependenciesJSON(), SourceEligibility: func(ctx context.Context, tx pgx.Tx, src catalog.Source) (bool, error) {
				return f.s.EligibleTx(ctx, tx, src, prepared, true)
			}}
			if mode == "legacy_order" {
				r.ExperienceDependencies = nil
			}
			tx, err = f.pool.Begin(live)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(live)
			out, err := r.ReadTx(live, tx, readcontrol.Search, readcontrol.Args{Query: "Observed", Limit: 1})
			if err != nil {
				t.Fatal(err)
			}
			var pins int
			count := func() int {
				if err := tx.QueryRow(live, `SELECT count(*) FROM memory_run_sources WHERE run_id='consumer' AND source_id=ANY($1::text[])`, []string{lesson.ID, descendant.ID}).Scan(&pins); err != nil {
					t.Fatal(err)
				}
				return pins
			}
			if mode == "legacy_order" {
				if len(out.Items) != 0 || count() == 0 {
					t.Fatal("C43 legacy empty-result/pinned-lesson counterexample missing")
				}
				if err = f.s.CheckPinnedTx(live, tx, "consumer"); err != nil {
					t.Fatal(err)
				}
				if err = tx.Commit(live); err != nil {
					t.Fatal(err)
				}
				if err = f.s.Sources.CheckRunSources(live, "consumer"); err != nil {
					t.Fatal("base sources unexpectedly stale", err)
				}
				if err = f.s.CheckPinned(live, "consumer"); err != nil {
					t.Fatal("original current dependencies unexpectedly stale", err)
				}
				current, err := New(f.s.Sources, f.s.EnvironmentHash, toolsFixture{readcontrol.Contract(readcontrol.Search)})
				if err != nil {
					t.Fatal(err)
				}
				if current.CheckPinned(live, "consumer") == nil {
					t.Fatal("legacy hidden dependency pin did not deny the next current release")
				}
				return
			}
			if out.Status != "ok" || len(out.Items) != 1 {
				t.Fatal("candidate ranking lost permitted DATA", out.Status)
			}
			if mode == "compatible" {
				if count() == 0 {
					t.Fatal("compatible lesson was not pinned")
				}
				return
			}
			if out.Items[0].Source.ID != ordinary.ID || count() != 0 {
				t.Fatal("unavailable dependency stole the page or left pins")
			}
			for _, source := range []catalog.SourceRef{lesson, ref(descendant)} {
				for _, tool := range []string{readcontrol.Open, readcontrol.Neighbors} {
					denied, err := r.ReadTx(live, tx, tool, readcontrol.Args{SourceID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash, MaxChars: 1000})
					if err != nil || denied.Status != "denied" || len(denied.Items) != 0 {
						t.Fatal("incompatible exact read released body/pins", err, denied.Status)
					}
				}
			}
			var id int64
			if err = tx.QueryRow(live, `SELECT c.id FROM memory_chunks c JOIN memory_object_sources o ON o.object_id=c.id AND o.memory_type='chunk' WHERE o.source_id=$1 LIMIT 1`, lesson.ID).Scan(&id); err != nil {
				t.Fatal(err)
			}
			var candidate reader.SemanticCandidate
			candidate.ID = id
			if err = tx.QueryRow(live, `SELECT updated_at FROM memory_chunks WHERE id=$1`, id).Scan(&candidate.Revision); err != nil {
				t.Fatal(err)
			}
			r.SemanticCandidates = []reader.SemanticCandidate{candidate}
			empty, err := r.ReadTx(live, tx, readcontrol.Search, readcontrol.Args{Query: "Observed", Strategy: "semantic_hybrid", Limit: 1})
			if err != nil || empty.Status != "empty" || count() != 0 {
				t.Fatal("semantic candidate poisoned run", err, empty.Status)
			}
			if err = f.s.CheckPinnedTx(live, tx, "consumer"); err != nil {
				t.Fatal("filtered DATA cannot continue", err)
			}
		})
	}
}
