package budget

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

func TestOwnerQueryRequiresWebActorAndBoundedLocalOperation(t *testing.T) {
	c := &Controller{}
	target, _ := NewLocalTarget("local-models:9090", "test")
	access := func(context.Context, pgx.Tx) error { t.Fatal("invalid request reached database"); return nil }
	op := LocalOperation{Kind: "query", Items: 1, InputBytes: 10, Timeout: time.Second}
	if _, _, err := c.AdmitOwnerQuery(context.Background(), target, op, access); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "query"})
	for _, bad := range []LocalOperation{{Kind: "index", Items: 1, InputBytes: 10, Timeout: time.Second}, {Kind: "query", Items: 2, InputBytes: 10, Timeout: time.Second}, {Kind: "query", Items: 1, InputBytes: 2049, Timeout: time.Second}, {Kind: "query", Items: 1, InputBytes: 10, Timeout: 31 * time.Second}} {
		if _, _, err := c.AdmitOwnerQuery(ctx, target, bad, access); !errors.Is(err, ErrInvalid) {
			t.Fatal(bad, err)
		}
	}
}
