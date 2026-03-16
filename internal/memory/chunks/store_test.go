package chunks

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/embeddings"
)

func TestVectorLiteral(t *testing.T) {
	t.Parallel()
	if got := vectorLiteral([]float32{0.1, 0.2}); got != "[0.1,0.2]" {
		t.Fatalf("unexpected vector literal %q", got)
	}
}

func TestSaveRequiresConfiguredStore(t *testing.T) {
	t.Parallel()
	_, err := NewStore(nil).Save(context.Background(), Chunk{ScopeType: "session", ScopeID: "s-1", Title: "doc", Content: "body"})
	if err != ErrStoreNotConfigured {
		t.Fatalf("expected ErrStoreNotConfigured, got %v", err)
	}
}

func TestSearchRequiresFixedDimensions(t *testing.T) {
	t.Parallel()
	_, err := NewStore(nil).Search(context.Background(), "session", "s-1", make([]float32, embeddings.VectorDimensions-1), 3)
	if err == nil {
		t.Fatal("expected dimension error")
	}
}

func TestPruneRequiresConfiguredStore(t *testing.T) {
	t.Parallel()
	_, err := NewStore(nil).Prune(context.Background(), time.Now().UTC(), 5)
	if err != ErrStoreNotConfigured {
		t.Fatalf("expected ErrStoreNotConfigured, got %v", err)
	}
}
