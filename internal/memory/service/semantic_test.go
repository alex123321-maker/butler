package service

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/internal/security/content"
)

type semanticFixture struct{ calls int }

func (s *semanticFixture) Search(_ context.Context, scopes []Scope, _ string, _ int) ([]SemanticHit, error) {
	s.calls++
	return []SemanticHit{{Title: "old secret", Summary: "must stay hidden", Origin: content.Origin{Sensitivity: content.Secret}}, {Title: "Current decision", Summary: "Retrieved evidence", SourceRef: "memory_chunk:7", Revision: "revision-2", Origin: content.Origin{Kind: content.MemoryDerived, SourceRefs: []string{"chunk:7"}, ParentRefs: []string{"message:original"}, Sensitivity: content.Private}}}, nil
}
func TestSemanticBundleRetainsSourceAndRejectsSecretWithoutLegacyEmbeddings(t *testing.T) {
	reader := &semanticFixture{}
	s := New(Config{Semantic: reader})
	b, err := s.BuildBundle(context.Background(), BundleRequest{SessionKey: "test", UserMessage: "why", IncludeQuery: true})
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || !strings.Contains(b.Prompt, "memory_chunk:7") || !strings.Contains(b.Prompt, "revision-2") || strings.Contains(b.Prompt, "must stay hidden") {
		t.Fatal("source or privacy boundary lost", b.Prompt)
	}
	items := b.Items["chunks"].([]map[string]any)
	o := items[0]["origin"].(content.Origin)
	if o.IsInstruction() || len(o.ParentRefs) != 1 {
		t.Fatal("provenance lost or elevated")
	}
	if _, err := s.BuildBundle(context.Background(), BundleRequest{SessionKey: "test", IncludeQuery: false}); err != nil || reader.calls != 1 {
		t.Fatal("disabled query invoked local model")
	}
}
