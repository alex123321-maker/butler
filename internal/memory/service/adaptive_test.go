package service

import (
	"context"
	"strings"
	"testing"
)

func TestBuildBundleAdaptivePreservesScopedPreferencesAndMandatoryState(t *testing.T) {
	episodes := &stubEpisodeStore{entries: []Episode{stubEpisode{summary: "optional", distance: 0.1}}}
	svc := New(Config{
		ProfileStore:  stubProfileStore{entriesByScope: map[string][]ProfileEntry{"session:s": {stubProfile{key: "language", summary: "Answer in Russian"}}}},
		WorkingStore:  stubWorkingStore{snapshot: WorkingSnapshot{Goal: "Current constraint", EntitiesJSON: `{}`, PendingStepsJSON: `[]`, Status: "active"}},
		SummaryReader: stubSummaryReader{summary: "Current summary"}, EpisodeStore: episodes, Embeddings: &nilEmbeddingProvider{},
	})
	bundle, err := svc.BuildBundle(context.Background(), BundleRequest{SessionKey: "s", UserID: "owner", UserMessage: "2+2", IncludeQuery: true, SkipOptionalSearch: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Answer in Russian", "Current constraint", "Current summary"} {
		if !strings.Contains(bundle.Prompt, required) {
			t.Fatalf("missing %q in %s", required, bundle.Prompt)
		}
	}
	if episodes.calls != 0 {
		t.Fatal("optional episode search", episodes.calls)
	}
	if _, ok := bundle.Items["episodes"]; ok {
		t.Fatal("optional episodes included")
	}
}
