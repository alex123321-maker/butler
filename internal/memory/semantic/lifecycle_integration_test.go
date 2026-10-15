package semantic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	memoryservice "github.com/butler/butler/internal/memory/service"
)

func TestLifecycleOldGenerationsForgetAndIndependentBundleIntegration(t *testing.T) {
	for _, change := range []string{"correct", "delete"} {
		t.Run(change, func(t *testing.T) {
			f := ownedMemory(t, 12)
			attack := memorytest.LifecycleAttacks(t)[0]
			poison, e := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "poison", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: attack.Body})
			if e != nil {
				t.Fatal(e)
			}
			child, e := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "derived-lesson", Kind: "model_candidate", ScopeType: "user", ScopeID: "owner", Body: attack.Body, Parents: []catalog.SourceRef{{ID: poison.ID, Revision: poison.Revision, AccessRevision: poison.AccessRevision, Hash: poison.Hash}}})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.catalog.PublishFragments(f.owner, catalog.SourceRef{ID: child.ID, Revision: child.Revision}, []catalog.Fragment{{Key: "poison", Title: attack.Body, Text: "Service.Restart " + attack.Body, Locator: json.RawMessage(`{}`)}}); e != nil {
				t.Fatal(e)
			}
			store := NewStore(f.pool)
			store.Guard = f.guard
			var generations []Generation
			for range 2 {
				g, e := store.Create(f.owner, testSpace(768), Scope{"user", "owner"})
				if e != nil {
					t.Fatal(e)
				}
				sources, e := store.Sources(f.owner, g.Scope, 10)
				if e != nil || len(sources) != 2 {
					t.Fatal("fixture has no independent and poisoned chunks", len(sources), e)
				}
				for _, src := range sources {
					if e = store.Put(f.owner, g.ID, src, unitVector(768)); e != nil {
						t.Fatal(e)
					}
				}
				if e = store.Verify(f.owner, g.ID, qualityProof()); e != nil {
					t.Fatal(e)
				}
				g, e = store.Generation(f.owner, g.ID)
				if e != nil {
					t.Fatal(e)
				}
				generations = append(generations, g)
			}
			if e = store.Activate(f.owner, generations[0].ID, ""); e != nil {
				t.Fatal(e)
			}
			if e = store.Activate(f.owner, generations[1].ID, generations[0].ID); e != nil {
				t.Fatal(e)
			}
			svc := memoryservice.New(memoryservice.Config{Semantic: &HybridReader{Store: store}, ScopeOrder: []string{"user"}})
			req := memoryservice.BundleRequest{SessionKey: "session", UserID: "owner", UserMessage: "Service.Restart", IncludeQuery: true}
			before, e := svc.BuildBundle(f.owner, req)
			if e != nil || !strings.Contains(before.Prompt, attack.Body) {
				t.Fatal("poison never entered DATA bundle", e)
			}
			if _, e = f.catalog.Get(f.live, child.ID); e != nil {
				t.Fatal(e)
			}
			if change == "correct" {
				_, e = f.catalog.Correct(f.owner, catalog.Correction{SourceID: poison.ID, ExpectedRevision: 1, Body: "Corrected: read only"})
			} else {
				_, e = f.catalog.Delete(f.owner, poison.ID, "delete", 1)
			}
			if e != nil {
				t.Fatal(e)
			}
			restarted := NewStore(f.pool)
			restarted.Guard = f.guard
			for _, g := range generations {
				hits, e := restarted.Search(f.owner, g, unitVector(768), 4)
				if e == nil {
					for _, h := range hits {
						if h.Source.SourceID == child.ID || strings.Contains(h.Source.Content, attack.Body) {
							t.Fatal("old generation released withdrawn derivative")
						}
					}
				}
			}
			if e = f.catalog.CheckRunSources(f.live, "run"); e == nil {
				t.Fatal("prepared old context survived withdrawal")
			}
			after, e := svc.BuildBundle(f.owner, req)
			if e == nil && strings.Contains(after.Prompt, attack.Body) {
				t.Fatal("withdrawn content entered fresh bundle")
			}
			// Correction fences old descendants. Explicit selective deletion then
			// removes their vectors and allows unrelated current memory to be read.
			if change == "correct" {
				if _, e = f.catalog.Delete(f.owner, child.ID, "delete", 1); e != nil {
					t.Fatal(e)
				}
			}
			hits, e := restarted.Search(f.owner, generations[0], unitVector(768), 4)
			if e != nil || len(hits) != 1 || hits[0].Source.SourceID != f.source.ID {
				t.Fatal("unrelated vector lost", len(hits), e)
			}
			req.UserMessage = "Проверяемые данные"
			svc = memoryservice.New(memoryservice.Config{Semantic: &HybridReader{Store: restarted}, ScopeOrder: []string{"user"}})
			bundle, e := svc.BuildBundle(f.owner, req)
			if e != nil || !strings.Contains(bundle.Prompt, "Исходник") || strings.Contains(bundle.Prompt, attack.Body) {
				t.Fatal("unrelated memory unavailable after forget/restart", e)
			}
		})
	}
}
