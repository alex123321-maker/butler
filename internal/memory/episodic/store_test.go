package episodic

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/memory/embeddings"
)

func TestVectorLiteral(t *testing.T) {
	t.Parallel()
	if got := vectorLiteral([]float32{0.1, 0.2, 0.3}); got != "[0.1,0.2,0.3]" {
		t.Fatalf("vectorLiteral = %q", got)
	}
}

func TestSaveRequiresFixedDimensions(t *testing.T) {
	t.Parallel()
	store := NewStore(nil)
	_, err := store.Save(context.Background(), Episode{ScopeType: "session", ScopeID: "s-1", Summary: "summary", Embedding: []float32{1, 2, 3}})
	if err == nil {
		t.Fatal("expected embedding dimension validation error")
	}
}

func TestSearchRequiresFixedDimensions(t *testing.T) {
	t.Parallel()
	store := NewStore(nil)
	_, err := store.Search(context.Background(), "session", "s-1", make([]float32, embeddings.VectorDimensions-1), 3)
	if err == nil {
		t.Fatal("expected embedding dimension validation error")
	}
}

func TestSearchRequiresStorePool(t *testing.T) {
	t.Parallel()
	store := NewStore(nil)
	_, err := store.Search(context.Background(), "session", "s-1", testVector(), 3)
	if err != ErrStoreNotConfigured {
		t.Fatalf("expected ErrStoreNotConfigured, got %v", err)
	}
}

func testVector() []float32 {
	vector := make([]float32, embeddings.VectorDimensions)
	for i := range vector {
		vector[i] = 0.1
	}
	return vector
}
