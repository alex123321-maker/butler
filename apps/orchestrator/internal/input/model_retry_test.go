package input

import (
	"context"
	"errors"
	"testing"

	"github.com/butler/butler/internal/ownership"
)

func TestModelRetryNoPrivateClaimDenied(t *testing.T) {
	store := &Store{}
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"no_claim", context.Background()}, {"claim_no_ownership", WithClaim(context.Background(), Record{InputID: "one", SessionKey: "one"})}, {"wrong_owned_run", ownership.WithToken(WithClaim(context.Background(), Record{InputID: "one", SessionKey: "one"}), nil, ownership.Token{SessionKey: "one", RunID: "other"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.ClaimModelRetry(tc.ctx, "run", "op"); !errors.Is(err, ErrModelRetryDenied) {
				t.Fatal("missing private claim reached database or mutated retry count")
			}
		})
	}
}
