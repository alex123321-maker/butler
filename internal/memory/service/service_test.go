package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/embeddings"
)

func TestBuildBundleOwnsScopeOrderAndLimits(t *testing.T) {
	t.Parallel()

	svc := New(Config{
		ProfileStore: stubProfileStore{entriesByScope: map[string][]ProfileEntry{
			"session:session-1": {stubProfile{key: "language", summary: "ru"}},
			"user:user-1":       {stubProfile{key: "style", summary: "concise"}},
		}},
		WorkingStore:  stubWorkingStore{snapshot: WorkingSnapshot{Goal: "Finish task", EntitiesJSON: `{"service":"redis"}`, PendingStepsJSON: `["check logs"]`, Status: "active"}},
		SummaryReader: stubSummaryReader{summary: "Session summary text"},
		ProfileLimit:  1,
		ScopeOrder:    []string{"user", "session", "user", "global"},
	})

	bundle, err := svc.BuildBundle(context.Background(), BundleRequest{SessionKey: "session-1", UserID: "user-1"})
	if err != nil {
		t.Fatalf("BuildBundle returned error: %v", err)
	}
	profile := bundle.Items["profile"].([]map[string]any)
	if len(profile) != 1 || profile[0]["scope_type"] != "user" {
		t.Fatalf("expected scope order and limit to be owned by service, got %+v", profile)
	}
	working := bundle.Items["working"].(map[string]any)
	if working["goal"] != "Finish task" {
		t.Fatalf("unexpected working bundle %+v", working)
	}
	if !strings.Contains(bundle.Prompt, "Working memory:") || !strings.Contains(bundle.Prompt, "Session summary:") {
		t.Fatalf("expected prompt to contain memory sections, got %q", bundle.Prompt)
	}
}

func TestBuildBundleSkipsEpisodesWithoutEmbeddings(t *testing.T) {
	t.Parallel()

	episodeStore := &stubEpisodeStore{entries: []Episode{stubEpisode{summary: "Recovered service", distance: 0.1}}}
	svc := New(Config{EpisodeStore: episodeStore})
	bundle, err := svc.BuildBundle(context.Background(), BundleRequest{SessionKey: "session-1", UserID: "user-1", UserMessage: "help", IncludeQuery: true})
	if err != nil {
		t.Fatalf("BuildBundle returned error: %v", err)
	}
	if _, ok := bundle.Items["episodes"]; ok {
		t.Fatalf("expected episodes to be skipped without embeddings, got %+v", bundle.Items)
	}
	if episodeStore.calls != 0 {
		t.Fatalf("expected episode store not to be called, got %d", episodeStore.calls)
	}
}

func TestBuildBundleUsesEmbeddingProviderForEpisodes(t *testing.T) {
	t.Parallel()

	episodeStore := &stubEpisodeStore{entries: []Episode{stubEpisode{summary: "Recovered postgres", distance: 0.05}, stubEpisode{summary: "Older event", distance: 0.2}}}
	svc := New(Config{EpisodeStore: episodeStore, Embeddings: stubEmbeddings{vector: testVector(0.4)}, EpisodeLimit: 1})
	bundle, err := svc.BuildBundle(context.Background(), BundleRequest{SessionKey: "session-1", UserID: "user-1", UserMessage: "check postgres", IncludeQuery: true})
	if err != nil {
		t.Fatalf("BuildBundle returned error: %v", err)
	}
	episodes := bundle.Items["episodes"].([]map[string]any)
	if len(episodes) != 1 || episodes[0]["summary"] != "Recovered postgres" {
		t.Fatalf("unexpected episode bundle %+v", episodes)
	}
	if episodeStore.calls == 0 {
		t.Fatal("expected episode store to be called")
	}
	if !strings.Contains(bundle.Prompt, "Relevant episodes:") {
		t.Fatalf("expected episodic section in prompt, got %q", bundle.Prompt)
	}
}

func TestBuildBundlePropagatesProfileStoreError(t *testing.T) {
	t.Parallel()

	svc := New(Config{ProfileStore: stubProfileStore{err: errors.New("profile store failed")}})
	if _, err := svc.BuildBundle(context.Background(), BundleRequest{SessionKey: "session-1", UserID: "user-1"}); err == nil {
		t.Fatal("expected BuildBundle error")
	}
}

type stubProfileStore struct {
	entriesByScope map[string][]ProfileEntry
	err            error
}

func (s stubProfileStore) GetByScope(_ context.Context, scopeType, scopeID string) ([]ProfileEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.entriesByScope[scopeType+":"+scopeID], nil
}

type stubWorkingStore struct{ snapshot WorkingSnapshot }

func (s stubWorkingStore) Get(context.Context, string) (WorkingSnapshot, error) {
	return s.snapshot, nil
}

type stubSummaryReader struct{ summary string }

func (s stubSummaryReader) GetSummary(context.Context, string) (string, error) { return s.summary, nil }

type stubProfile struct {
	key     string
	summary string
}

func (s stubProfile) ProfileKey() string     { return s.key }
func (s stubProfile) ProfileSummary() string { return s.summary }

type stubEpisodeStore struct {
	entries []Episode
	calls   int
}

func (s *stubEpisodeStore) Search(context.Context, string, string, []float32, int) ([]Episode, error) {
	s.calls++
	return s.entries, nil
}

type stubEpisode struct {
	summary  string
	distance float64
}

func (s stubEpisode) EpisodeSummary() string   { return s.summary }
func (s stubEpisode) EpisodeDistance() float64 { return s.distance }

type stubEmbeddings struct{ vector []float32 }

func (s stubEmbeddings) EmbedQuery(context.Context, string) ([]float32, error) { return s.vector, nil }

func testVector(value float32) []float32 {
	vector := make([]float32, embeddings.VectorDimensions)
	for i := range vector {
		vector[i] = value
	}
	return vector
}
