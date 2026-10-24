package input

import (
	"context"
	"errors"

	"github.com/butler/butler/internal/transport"
)

// A heartbeat failure cancels dispatch immediately, without exposing a storage
// error or losing the distinction between a replaced claim and unavailable DB.
type claimCancellation struct{ lost bool }

func (e *claimCancellation) Error() string {
	if e.lost {
		return "input_claim_lost"
	}
	return "input_claim_renewal_failed"
}
func (e *claimCancellation) Unwrap() []error {
	if e.lost {
		return []error{context.Canceled, ErrClaimLost}
	}
	return []error{context.Canceled}
}

func preserveClaimCancellation(ctx context.Context, err error) error {
	var provider *transport.Error
	var diagnosed interface{ DiagnosticCode() string }
	if !errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &provider) || errors.As(err, &diagnosed) {
		return err
	}
	var cause *claimCancellation
	if errors.As(context.Cause(ctx), &cause) {
		return cause
	}
	return err
}
