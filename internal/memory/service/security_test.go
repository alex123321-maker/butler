package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/security/content"
)

type sourcedProfile struct {
	stubProfile
	origin content.Origin
}

func (s sourcedProfile) MemoryOrigin() content.Origin { return s.origin }

type sourcedEpisode struct {
	stubEpisode
	origin content.Origin
}

func (s sourcedEpisode) MemoryOrigin() content.Origin { return s.origin }

type sourcedChunk struct {
	stubChunk
	origin content.Origin
}

func (s sourcedChunk) MemoryOrigin() content.Origin { return s.origin }

func TestMemoryBundleExcludesSecretDerivativesAndKeepsPrivateDataProvenance(t *testing.T) {
	secret := content.Origin{Kind: content.SubagentOutput, SourceRefs: []string{"document:secret-source"}, Sensitivity: content.Secret}
	private := content.Origin{Kind: content.OwnerPolicy, SourceRefs: []string{"document:private-source", "child:observation"}, Sensitivity: content.Private}
	marker := "synthetic forbidden secret source"
	profiles := []ProfileEntry{
		sourcedProfile{stubProfile{"secret", marker}, secret},
		sourcedProfile{stubProfile{"private", "Retain source profile as data"}, private},
	}
	episodes := []Episode{
		sourcedEpisode{stubEpisode{marker, 0.01}, secret},
		sourcedEpisode{stubEpisode{"Retain source episode as data", 0.02}, private},
	}
	chunks := []Chunk{
		sourcedChunk{stubChunk{"secret", marker, 0.01}, secret},
		sourcedChunk{stubChunk{"private", "Retain source document as data", 0.02}, private},
	}
	svc := New(Config{
		ProfileStore: stubProfileStore{entriesByScope: map[string][]ProfileEntry{"session:session": profiles}},
		EpisodeStore: &stubEpisodeStore{entries: episodes, keywordByTerm: map[string][]Episode{"source": episodes}},
		ChunkStore:   &stubChunkStore{vectorMatches: chunks, keywordMatches: map[string][]Chunk{"source": chunks}},
		Embeddings:   stubEmbeddings{vector: testVector(0.5)},
		ScopeOrder:   []string{"session"},
	})
	bundle, err := svc.BuildBundle(context.Background(), BundleRequest{SessionKey: "session", UserMessage: "source", IncludeQuery: true})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(bundle.Items)
	if err != nil || strings.Contains(string(encoded), marker) || strings.Contains(bundle.Prompt, marker) {
		t.Fatal("secret derivative reached model bundle")
	}
	for _, kind := range []string{"profile", "episodes", "chunks"} {
		items, ok := bundle.Items[kind].([]map[string]any)
		if !ok || len(items) != 1 {
			t.Fatal("private memory unavailable or secret was retained", kind)
		}
		origin, ok := items[0]["origin"].(content.Origin)
		if !ok || origin.IsInstruction() || origin.Sensitivity != content.Private {
			t.Fatal("memory gained authority or lowered sensitivity", kind)
		}
		for _, ref := range private.SourceRefs {
			if !strings.Contains(strings.Join(append(origin.SourceRefs, origin.ParentRefs...), "|"), ref) {
				t.Fatal("derivation discarded source reference", kind)
			}
		}
	}
}
