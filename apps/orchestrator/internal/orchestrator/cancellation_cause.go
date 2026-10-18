package orchestrator

import (
	"context"
	"errors"

	"github.com/butler/butler/internal/transport"
)

// ownedCancellation contains only a fixed Core reason, never storage/provider
// details. It retains the cancelled classification and the existing ownership
// sentinel independently. Source text and transport events cannot construct it.
type ownedCancellation struct {
	reason  string
	failure error
}

func (e *ownedCancellation) Error() string { return e.reason }
func (e *ownedCancellation) Unwrap() []error {
	return []error{context.Canceled, e.failure}
}

func preserveOwnedCancellation(ctx context.Context, err error) error {
	if !errors.Is(err, context.Canceled) {
		return err
	}
	// Joined or wrapped independent failures retain their own diagnosis even
	// when cancellation is also present. Only a plain cancelled stream acquires
	// the owning supervisor's cause.
	var providerFailure *transport.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &providerFailure) {
		return err
	}
	var cause *ownedCancellation
	if errors.As(context.Cause(ctx), &cause) {
		return cause
	}
	return err
}
