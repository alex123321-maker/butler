package app

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/semantic"
	"google.golang.org/grpc/status"
)

// Metadata preflight cannot dispatch inference, including through Adapter.Verify.
type memoryMetadataOnlyBackend struct{ semantic.Backend }

func (memoryMetadataOnlyBackend) Infer(context.Context, *pb.InferRequest) (*pb.InferResponse, error) {
	return nil, errors.New("metadata preflight forbids inference")
}

func TestMemoryEvalLocalMetadataPreflight(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_EVAL_LOCAL_PREFLIGHT") != "1" {
		t.Skip("explicit metadata-only local preflight required")
	}
	target, err := budget.NewLocalTarget("local-models:9090", "operator-pinned-service")
	if err != nil {
		t.Fatal("local target invalid")
	}
	b, closeBackend, err := semantic.NewPinnedBackend(target, os.Getenv("BUTLER_MEMORY_EVAL_LOCAL_TOKEN_FILE"))
	if err != nil {
		t.Fatal("private local reference unavailable")
	}
	defer closeBackend()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = b.Describe(ctx, &pb.Empty{}); err != nil {
		// Only the fixed gRPC code is safe; never print provider error prose.
		t.Fatal("local metadata RPC failed", status.Code(err).String())
	}
	a := semantic.Adapter{Backend: memoryMetadataOnlyBackend{b}, Space: memoryEvalSpace()}
	if a.Verify(ctx) != nil {
		t.Fatal("local profile metadata differs from frozen index space")
	}
	t.Log("local metadata/profile preflight passed; inference disabled")
}
